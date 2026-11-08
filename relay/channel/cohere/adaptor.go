package cohere

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
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
	if info.RelayMode != constant.RelayModeAudioTranscription {
		return nil, errors.New("Cohere audio mode is not supported")
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "json" {
		return nil, errors.New("Cohere transcription supports JSON responses")
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, err
	}
	defer form.RemoveAll()
	if len(form.Value["language"]) != 1 || strings.TrimSpace(form.Value["language"][0]) == "" {
		return nil, errors.New("language is required for Cohere transcription")
	}
	for name := range form.Value {
		switch name {
		case "model", "language", "response_format", "temperature":
		default:
			return nil, fmt.Errorf("unsupported Cohere transcription parameter: %s", name)
		}
	}
	files := form.File["file"]
	if len(files) != 1 {
		return nil, errors.New("Cohere transcription requires one audio file")
	}
	if files[0].Size > 25*1024*1024 {
		return nil, errors.New("Cohere transcription audio exceeds 25 MB")
	}
	switch strings.ToLower(filepath.Ext(files[0].Filename)) {
	case ".flac", ".mp3", ".mpeg", ".mpga", ".ogg", ".wav":
	default:
		return nil, errors.New("unsupported Cohere transcription audio file type")
	}
	return (&openai.Adaptor{}).ConvertAudioRequest(c, info, request)
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
		return fmt.Sprintf("%s/v2/embed", info.ChannelBaseUrl), nil
	} else if info.RelayMode == constant.RelayModeAudioTranscription {
		return fmt.Sprintf("%s/compatibility/v1/audio/transcriptions", info.ChannelBaseUrl), nil
	} else if usesCompatibilityChat(info) {
		return fmt.Sprintf("%s/compatibility/v1/chat/completions", info.ChannelBaseUrl), nil
	} else {
		return fmt.Sprintf("%s/v1/chat", info.ChannelBaseUrl), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if usesCompatibilityChat(info) {
		return request, nil
	}
	return requestOpenAI2Cohere(*request), nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == constant.RelayModeAudioTranscription {
		return channel.DoFormRequest(a, c, info, requestBody)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return requestConvertRerank2Cohere(request), nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	var texts []string
	switch input := request.Input.(type) {
	case string:
		texts = []string{input}
	case []string:
		texts = input
	case []any:
		for _, item := range input {
			text, ok := item.(string)
			if !ok {
				return nil, errors.New("Cohere embeddings require text input")
			}
			texts = append(texts, text)
		}
	default:
		return nil, errors.New("Cohere embeddings require text input")
	}
	if len(texts) == 0 || len(texts) > 96 {
		return nil, errors.New("Cohere embeddings require between 1 and 96 texts")
	}
	encoding := request.EncodingFormat
	if encoding == "" {
		encoding = "float"
	}
	if encoding != "float" && encoding != "base64" {
		return nil, errors.New("unsupported Cohere embedding encoding format")
	}
	return &CohereEmbeddingRequest{
		Model: request.Model, Texts: texts, InputType: "search_document",
		EmbeddingTypes: []string{encoding}, OutputDimension: request.Dimensions,
	}, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeRerank {
		usage, err = cohereRerankHandler(c, resp, info)
	} else if info.RelayMode == constant.RelayModeEmbeddings {
		usage, err = cohereEmbeddingHandler(c, resp, info)
	} else if info.RelayMode == constant.RelayModeAudioTranscription {
		err, usage = openai.OpenaiSTTHandler(c, resp, info, "json")
	} else if usesCompatibilityChat(info) {
		if info.IsStream {
			usage, err = openai.OaiStreamHandler(c, info, resp)
		} else {
			usage, err = openai.OpenaiHandler(c, info, resp)
		}
	} else {
		if info.IsStream {
			usage, err = cohereStreamHandler(c, info, resp) // TODO: fix this
		} else {
			usage, err = cohereHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func usesCompatibilityChat(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || info.RelayMode != constant.RelayModeChatCompletions {
		return false
	}
	switch info.UpstreamModelName {
	case "c4ai-aya-expanse-32b", "north-mini-code-1.0", "north-small-translate-1.0", "tiny-aya-earth", "tiny-aya-fire", "tiny-aya-global", "tiny-aya-water":
		return true
	default:
		return false
	}
}
