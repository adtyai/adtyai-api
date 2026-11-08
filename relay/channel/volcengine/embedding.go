package volcengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const contextKeyVisionEmbeddingRequest = "volcengine_vision_embedding_request"

type visionEmbeddingRequest struct {
	Model          string           `json:"model"`
	Input          []map[string]any `json:"input"`
	Dimensions     *int             `json:"dimensions,omitempty"`
	EncodingFormat string           `json:"encoding_format"`
}

func usesVisionEmbedding(info *relaycommon.RelayInfo) bool {
	return strings.HasPrefix(info.UpstreamModelName, "doubao-embedding-vision") || strings.HasPrefix(info.OriginModelName, "doubao-embedding-vision")
}

func visionEmbeddingRequestError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// A native request produces one fused vector. Accept a single string or chat
// content parts, which EmbeddingRequest.GetTokenCountMeta can count. A string
// batch must not silently become one fused vector, and native content objects
// must not bypass the host's text/file pre-consume estimate.
func convertVisionEmbeddingRequest(request dto.EmbeddingRequest) (*visionEmbeddingRequest, error) {
	if request.EncodingFormat != "" && request.EncodingFormat != "float" {
		return nil, visionEmbeddingRequestError("Volcengine vision embeddings support float encoding only")
	}
	if request.Dimensions != nil && *request.Dimensions != 1024 && *request.Dimensions != 2048 {
		return nil, visionEmbeddingRequestError("Volcengine vision embedding dimensions must be 1024 or 2048")
	}
	if request.Seed != nil || request.Temperature != nil || request.TopP != nil || request.FrequencyPenalty != nil || request.PresencePenalty != nil {
		return nil, visionEmbeddingRequestError("sampling parameters are not supported for Volcengine vision embeddings")
	}
	var items []any
	switch input := request.Input.(type) {
	case string:
		items = []any{map[string]any{"type": dto.ContentTypeText, "text": input}}
	case []any:
		items = input
		if len(input) == 1 {
			if text, ok := input[0].(string); ok {
				items = []any{map[string]any{"type": dto.ContentTypeText, "text": text}}
			}
		}
	default:
		return nil, visionEmbeddingRequestError("Volcengine vision embedding input must be a string or chat content parts")
	}
	if len(items) == 0 {
		return nil, visionEmbeddingRequestError("Volcengine vision embedding input is empty")
	}
	converted := &visionEmbeddingRequest{Model: request.Model, Dimensions: request.Dimensions, EncodingFormat: "float", Input: make([]map[string]any, 0, len(items))}
	for _, item := range items {
		part, ok := item.(map[string]any)
		if !ok {
			return nil, visionEmbeddingRequestError("Volcengine vision embeddings require chat content parts; string batches and token arrays are not supported")
		}
		kind, _ := part["type"].(string)
		switch kind {
		case dto.ContentTypeText:
			text, ok := part["text"].(string)
			if !ok || text == "" || len(part) != 2 {
				return nil, visionEmbeddingRequestError("Volcengine vision embedding text parts require non-empty text only")
			}
			converted.Input = append(converted.Input, map[string]any{"type": kind, "text": text})
		case dto.ContentTypeImageURL, dto.ContentTypeVideoUrl:
			media, ok := part[kind].(map[string]any)
			url, _ := media["url"].(string)
			if !ok || url == "" || len(part) != 2 || len(media) != 1 {
				return nil, visionEmbeddingRequestError("Volcengine vision embedding media parts require a URL only; native sampling extensions are not supported")
			}
			converted.Input = append(converted.Input, map[string]any{"type": kind, kind: map[string]any{"url": url}})
		default:
			return nil, visionEmbeddingRequestError(fmt.Sprintf("unsupported Volcengine vision embedding content type: %s", kind))
		}
	}
	return converted, nil
}

func validateVisionEmbeddingBody(c *gin.Context, body io.Reader) ([]byte, error) {
	expectedValue, exists := c.Get(contextKeyVisionEmbeddingRequest)
	expected, ok := expectedValue.(*visionEmbeddingRequest)
	if !exists || !ok {
		return nil, visionEmbeddingRequestError("Volcengine vision embeddings require a converted, token-counted request")
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var fields map[string]common.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return nil, visionEmbeddingRequestError("invalid Volcengine vision embedding request")
	}
	for key := range fields {
		switch key {
		case "model", "input", "dimensions", "encoding_format":
		default:
			return nil, visionEmbeddingRequestError("unsupported Volcengine vision embedding field: " + key)
		}
	}
	var request dto.EmbeddingRequest
	if err := common.Unmarshal(data, &request); err != nil {
		return nil, visionEmbeddingRequestError("invalid Volcengine vision embedding parameters")
	}
	converted, err := convertVisionEmbeddingRequest(request)
	if err != nil {
		return nil, err
	}
	input, err := common.Marshal(converted.Input)
	if err != nil {
		return nil, err
	}
	expectedInput, err := common.Marshal(expected.Input)
	if err != nil {
		return nil, err
	}
	if converted.Model != expected.Model || !bytes.Equal(input, expectedInput) {
		return nil, visionEmbeddingRequestError("Volcengine vision embedding overrides cannot change the token-counted model or input")
	}
	return common.Marshal(converted)
}

// The native endpoint reports prompt_tokens including visual tokens, and
// prompt_tokens_details.image_tokens/text_tokens. It returns a single fused
// data.embedding object, unlike the standard text endpoint's data array.
// Reference: docs.volcengine.com/docs/zh/ark/multimodal-vectorization-api.md
func visionEmbeddingResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid Volcengine vision embedding response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)
	var response struct {
		Model string `json:"model"`
		Data  struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *types.OpenAIError `json:"error"`
		Usage *struct {
			PromptTokens *int `json:"prompt_tokens"`
			TotalTokens  *int `json:"total_tokens"`
			Details      struct {
				TextTokens  *int `json:"text_tokens"`
				ImageTokens *int `json:"image_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := common.DecodeJson(resp.Body, &response); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if response.Error != nil {
		return nil, types.WithOpenAIError(*response.Error, http.StatusBadGateway)
	}
	u := response.Usage
	if len(response.Data.Embedding) == 0 || u == nil || u.PromptTokens == nil || u.TotalTokens == nil {
		return nil, types.NewOpenAIError(errors.New("incomplete Volcengine vision embedding response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	for _, count := range []*int{u.PromptTokens, u.TotalTokens, u.Details.TextTokens, u.Details.ImageTokens} {
		if count != nil && (*count < 0 || *count > math.MaxInt32) {
			return nil, types.NewOpenAIError(errors.New("invalid Volcengine vision embedding token usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
	}
	text, image := 0, 0
	if u.Details.TextTokens != nil {
		text = *u.Details.TextTokens
	}
	if u.Details.ImageTokens != nil {
		image = *u.Details.ImageTokens
	}
	if *u.PromptTokens != *u.TotalTokens || image > *u.PromptTokens || text > *u.PromptTokens-image {
		return nil, types.NewOpenAIError(errors.New("inconsistent Volcengine vision embedding token usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usage := &dto.Usage{PromptTokens: *u.PromptTokens, TotalTokens: *u.TotalTokens, PromptTokensDetails: dto.InputTokenDetails{TextTokens: text, ImageTokens: image}}
	info.ObserveResponseModel(response.Model)
	if response.Model == "" {
		response.Model = info.UpstreamModelName
	}
	output := dto.EmbeddingResponse{Object: "list", Model: response.Model, Data: []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: response.Data.Embedding}}, Usage: *usage}
	data, err := common.Marshal(output)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusBadGateway)
	}
	service.IOCopyBytesGracefully(c, resp, data)
	return usage, nil
}
