package perplexity

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if isAgentModel(info.UpstreamModelName) {
		return nil, agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
	}
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
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
	if isAgentModel(info.UpstreamModelName) {
		if info.RelayMode != relayconstant.RelayModeResponses || (info.RelayFormat != "" && info.RelayFormat != types.RelayFormatOpenAIResponses) {
			return "", agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
		}
		return fmt.Sprintf("%s/v1/agent", strings.TrimRight(info.ChannelBaseUrl, "/")), nil
	}
	if info.RelayMode == relayconstant.RelayModeResponses {
		return fmt.Sprintf("%s/v1/responses", info.ChannelBaseUrl), nil
	}
	return fmt.Sprintf("%s/chat/completions", info.ChannelBaseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if isAgentModel(request.Model) || isAgentModel(info.UpstreamModelName) {
		return nil, agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
	}
	if lo.FromPtrOr(request.TopP, 0) >= 1 {
		request.TopP = lo.ToPtr(0.99)
	}
	return requestOpenAI2Perplexity(*request), nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if isAgentModel(request.Model) || isAgentModel(info.UpstreamModelName) {
		if info.RelayFormat != "" && info.RelayFormat != types.RelayFormatOpenAIResponses {
			return nil, agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
		}
		data, err := common.Marshal(request)
		if err != nil {
			return nil, err
		}
		return agentRequest(data)
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if isAgentModel(info.UpstreamModelName) {
		if info.RelayMode != relayconstant.RelayModeResponses || (info.RelayFormat != "" && info.RelayFormat != types.RelayFormatOpenAIResponses) {
			return nil, agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
		}
		// Validate the final body too: passthrough and parameter overrides run
		// after conversion and must not bypass token bounds or tool restrictions.
		data, err := io.ReadAll(requestBody)
		if err != nil {
			return nil, err
		}
		request, err := agentRequest(data)
		if err != nil {
			return nil, err
		}
		data, err = common.Marshal(request)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(data)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if isAgentModel(info.UpstreamModelName) {
		if info.RelayMode != relayconstant.RelayModeResponses || (info.RelayFormat != "" && info.RelayFormat != types.RelayFormatOpenAIResponses) {
			return nil, agentRequestError("Perplexity Agent models require the /v1/responses endpoint")
		}
		if info.IsStream {
			return agentStreamHandler(c, resp, info)
		}
		return agentResponseHandler(c, resp, info)
	}
	adaptor := openai.Adaptor{}
	usage, err = adaptor.DoResponse(c, resp, info)
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
