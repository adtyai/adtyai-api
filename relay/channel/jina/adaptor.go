package jina

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/common_handler"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

const contextKeyEmbeddingRequest = "jina_embedding_request"

func embeddingRequestError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == constant.RelayModeRerank {
		return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
	} else if info.RelayMode == constant.RelayModeEmbeddings {
		return fmt.Sprintf("%s/v1/embeddings", info.ChannelBaseUrl), nil
	}
	return "", errors.New("invalid relay mode")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == constant.RelayModeEmbeddings {
		if c == nil || requestBody == nil {
			return nil, embeddingRequestError("missing token-counted Jina embedding request")
		}
		value, exists := c.Get(contextKeyEmbeddingRequest)
		expected, ok := value.(string)
		if !exists || !ok {
			return nil, embeddingRequestError("missing token-counted Jina embedding request")
		}
		body, err := io.ReadAll(requestBody)
		if err != nil {
			return nil, err
		}
		var final map[string]any
		if err := common.Unmarshal(body, &final); err != nil {
			return nil, embeddingRequestError("invalid final Jina embedding request")
		}
		model, ok := final["model"].(string)
		if !ok || strings.TrimSpace(model) == "" || final["input"] == nil {
			return nil, embeddingRequestError("Jina embedding model and input are required")
		}
		counted, err := common.Marshal(map[string]any{"model": model, "input": final["input"]})
		if err != nil {
			return nil, err
		}
		if string(counted) != expected {
			return nil, embeddingRequestError("Jina embedding overrides cannot change the token-counted model or input")
		}
		requestBody = bytes.NewReader(body)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	input := request.Input
	unsupportedInput := errors.New("Jina embeddings support plain text or OpenAI text, image_url and video_url parts; native objects, audio, PDFs and merged content groups are not supported")
	switch input.(type) {
	case string, []any:
	default:
		return nil, unsupportedInput
	}
	if items, ok := input.([]any); ok {
		converted := make([]any, 0, len(items))
		mediaInputs := 0
		for _, item := range items {
			if _, ok := item.(string); ok {
				converted = append(converted, item)
				continue
			}
			part, ok := item.(map[string]any)
			if !ok || part["type"] == nil {
				return nil, unsupportedInput
			}
			switch part["type"] {
			case dto.ContentTypeText:
				text, ok := part["text"].(string)
				if !ok {
					return nil, unsupportedInput
				}
				converted = append(converted, map[string]any{"text": text})
			case dto.ContentTypeImageURL, dto.ContentTypeVideoUrl:
				key, nativeKey := "image_url", "image"
				if part["type"] == dto.ContentTypeVideoUrl {
					key, nativeKey = "video_url", "video"
				}
				media, ok := part[key].(map[string]any)
				if !ok {
					return nil, unsupportedInput
				}
				mediaURL, ok := media["url"].(string)
				if !ok || strings.TrimSpace(mediaURL) == "" {
					return nil, unsupportedInput
				}
				mediaInputs++
				converted = append(converted, map[string]any{nativeKey: mediaURL})
			default:
				return nil, unsupportedInput
			}
		}
		// The shared estimator decodes the whole original part before counting
		// its media. Reject any shape that it cannot reserve for.
		if len(request.GetTokenCountMeta().Files) != mediaInputs {
			return nil, unsupportedInput
		}
		input = converted
	}
	result := map[string]any{"model": request.Model, "input": input}
	if c != nil {
		// Store an immutable snapshot before channel overrides can replace the
		// converted native input after the host has reserved its token estimate.
		counted, err := common.Marshal(result)
		if err != nil {
			return nil, err
		}
		c.Set(contextKeyEmbeddingRequest, string(counted))
	}
	if request.Dimensions != nil {
		result["dimensions"] = request.Dimensions
	}
	if request.EncodingFormat != "" {
		result["embedding_type"] = request.EncodingFormat
	}
	return result, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeRerank {
		usage, err = common_handler.RerankHandler(c, info, resp)
	} else if info.RelayMode == constant.RelayModeEmbeddings {
		defer service.CloseResponseBodyGracefully(resp)
		var response map[string]any
		if decodeErr := common.DecodeJson(resp.Body, &response); decodeErr != nil {
			return nil, types.NewError(decodeErr, types.ErrorCodeBadResponseBody)
		}
		if response["error"] != nil {
			return nil, types.NewError(errors.New("Jina embeddings upstream returned an error"), types.ErrorCodeBadResponseBody)
		}
		data, ok := response["data"].([]any)
		if !ok || len(data) == 0 {
			return nil, types.NewError(errors.New("Jina response contains no embeddings"), types.ErrorCodeBadResponseBody)
		}
		for _, item := range data {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, types.NewError(errors.New("Jina response contains an invalid embedding"), types.ErrorCodeBadResponseBody)
			}
			switch embedding := entry["embedding"].(type) {
			case []any:
				if len(embedding) == 0 {
					return nil, types.NewError(errors.New("Jina response contains an empty embedding"), types.ErrorCodeBadResponseBody)
				}
				for _, value := range embedding {
					number, ok := value.(float64)
					if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
						return nil, types.NewError(errors.New("Jina response contains an invalid embedding value"), types.ErrorCodeBadResponseBody)
					}
				}
			case string:
				decoded, decodeErr := base64.StdEncoding.DecodeString(embedding)
				if decodeErr != nil || len(decoded) == 0 {
					return nil, types.NewError(errors.New("Jina response contains an invalid base64 embedding"), types.ErrorCodeBadResponseBody)
				}
			default:
				return nil, types.NewError(errors.New("Jina response contains an invalid embedding"), types.ErrorCodeBadResponseBody)
			}
		}
		// Only documented embedding counters are upstream accounting facts.
		// Internal billing snapshots and modality/cache details are not trusted.
		var reportedUsage struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
			TotalTokens      *int `json:"total_tokens"`
		}
		if rawUsage, exists := response["usage"]; exists {
			encodedUsage, marshalErr := common.Marshal(rawUsage)
			if marshalErr != nil {
				return nil, types.NewError(marshalErr, types.ErrorCodeBadResponseBody)
			}
			if decodeErr := common.Unmarshal(encodedUsage, &reportedUsage); decodeErr != nil {
				return nil, types.NewError(decodeErr, types.ErrorCodeBadResponseBody)
			}
		}
		for _, tokens := range []*int{reportedUsage.PromptTokens, reportedUsage.TotalTokens, reportedUsage.CompletionTokens} {
			if tokens != nil && (*tokens < 0 || *tokens > common.MaxQuota) {
				return nil, types.NewError(errors.New("Jina response contains an invalid token count"), types.ErrorCodeBadResponseBody)
			}
		}
		if reportedUsage.CompletionTokens != nil && *reportedUsage.CompletionTokens != 0 {
			return nil, types.NewError(errors.New("Jina embeddings cannot report completion tokens"), types.ErrorCodeBadResponseBody)
		}
		if reportedUsage.PromptTokens != nil && reportedUsage.TotalTokens != nil && *reportedUsage.PromptTokens != *reportedUsage.TotalTokens {
			return nil, types.NewError(errors.New("Jina response contains inconsistent embedding token usage"), types.ErrorCodeBadResponseBody)
		}
		inputTokens := info.GetEstimatePromptTokens()
		if reportedUsage.PromptTokens != nil {
			inputTokens = *reportedUsage.PromptTokens
		} else if reportedUsage.TotalTokens != nil {
			inputTokens = *reportedUsage.TotalTokens
		}
		if inputTokens < 0 || inputTokens > common.MaxQuota {
			return nil, types.NewError(errors.New("Jina embedding token estimate is out of range"), types.ErrorCodeBadResponseBody)
		}
		embeddingUsage := dto.Usage{PromptTokens: inputTokens, TotalTokens: inputTokens}
		response["usage"] = embeddingUsage
		body, marshalErr := common.Marshal(response)
		if marshalErr != nil {
			return nil, types.NewError(marshalErr, types.ErrorCodeBadResponseBody)
		}
		c.Data(resp.StatusCode, "application/json", body)
		return &embeddingUsage, nil
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
