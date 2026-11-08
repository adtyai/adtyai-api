package perplexity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// Provider-qualified models use the Agent API. Unqualified Sonar models retain
// the existing Chat Completions protocol and its parameter normalization.
func isAgentModel(model string) bool {
	provider, _, ok := strings.Cut(model, "/")
	if !ok {
		return false
	}
	switch provider {
	case "openai", "anthropic", "google", "xai", "perplexity":
		return true
	}
	return false
}

func agentRequestError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// agentRequest preserves raw function schemas, replay signatures and explicit
// scalar zero values. The native API uses response_format rather than text.format.
func agentRequest(data []byte) (map[string]json.RawMessage, error) {
	var request map[string]json.RawMessage
	if err := common.Unmarshal(data, &request); err != nil {
		return nil, agentRequestError("invalid Perplexity Agent request: " + err.Error())
	}
	for key := range request {
		switch key {
		case "model", "input", "instructions", "max_output_tokens", "previous_response_id", "reasoning", "store", "stream", "temperature", "top_p", "tools", "tool_choice", "text", "response_format", "stream_options", "service_tier", "max_steps", "background":
		default:
			return nil, agentRequestError(fmt.Sprintf("Perplexity Agent does not support the %s request field", key))
		}
	}
	var params struct {
		Model           string             `json:"model"`
		MaxOutputTokens *uint              `json:"max_output_tokens"`
		MaxSteps        *int               `json:"max_steps"`
		Temperature     *float64           `json:"temperature"`
		TopP            *float64           `json:"top_p"`
		Background      *bool              `json:"background"`
		Reasoning       *dto.Reasoning     `json:"reasoning"`
		StreamOptions   *dto.StreamOptions `json:"stream_options"`
	}
	if err := common.Unmarshal(data, &params); err != nil {
		return nil, agentRequestError("invalid Perplexity Agent parameters: " + err.Error())
	}
	if !isAgentModel(params.Model) {
		return nil, agentRequestError("Perplexity Agent requires a provider-qualified model")
	}
	if params.MaxOutputTokens != nil && (*params.MaxOutputTokens == 0 || helper.ExceedsMaxTokensLimit(params.MaxOutputTokens)) {
		return nil, agentRequestError("max_output_tokens is invalid")
	}
	if _, present := request["max_output_tokens"]; present && params.MaxOutputTokens == nil {
		return nil, agentRequestError("max_output_tokens is invalid")
	}
	if strings.HasPrefix(params.Model, "anthropic/") && params.MaxOutputTokens == nil {
		return nil, agentRequestError("max_output_tokens is required for Perplexity Agent Anthropic models")
	}
	if params.MaxSteps != nil && (*params.MaxSteps < 1 || *params.MaxSteps > 500) {
		return nil, agentRequestError("Perplexity Agent max_steps must be between 1 and 500")
	}
	if params.Temperature != nil && (*params.Temperature < 0 || *params.Temperature > 2) || params.TopP != nil && (*params.TopP < 0 || *params.TopP > 1) {
		return nil, agentRequestError("Perplexity Agent sampling parameters are out of range")
	}
	if params.Background != nil && *params.Background {
		return nil, agentRequestError("Perplexity Agent background responses are not supported")
	}
	if params.Reasoning != nil {
		if params.Reasoning.Summary != "" || len(params.Reasoning.Mode) != 0 || len(params.Reasoning.Context) != 0 {
			return nil, agentRequestError("Perplexity Agent supports reasoning.effort only")
		}
		switch params.Reasoning.Effort {
		case "", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return nil, agentRequestError("unsupported Perplexity Agent reasoning effort")
		}
	}
	if params.StreamOptions != nil && params.StreamOptions.IncludeObfuscation {
		return nil, agentRequestError("Perplexity Agent does not support stream obfuscation")
	}
	// Agent streams always carry terminal usage. OpenAI stream_options is a
	// downstream option and is not accepted by the upstream Agent API.
	delete(request, "stream_options")
	if len(request["input"]) == 0 || string(request["input"]) == "null" {
		return nil, agentRequestError("Perplexity Agent input is required")
	}
	if common.GetJsonType(request["input"]) == "array" {
		var items []map[string]json.RawMessage
		if err := common.Unmarshal(request["input"], &items); err != nil {
			return nil, agentRequestError("invalid Perplexity Agent input items")
		}
		for _, item := range items {
			if len(item["type"]) == 0 && len(item["role"]) != 0 {
				item["type"] = json.RawMessage(`"message"`)
			}
		}
		var err error
		request["input"], err = common.Marshal(items)
		if err != nil {
			return nil, err
		}
	} else if common.GetJsonType(request["input"]) != "string" {
		return nil, agentRequestError("Perplexity Agent input must be a string or an array")
	}
	if raw := request["text"]; len(raw) != 0 {
		var text map[string]json.RawMessage
		if err := common.Unmarshal(raw, &text); err != nil || len(text) != 1 || len(text["format"]) == 0 {
			return nil, agentRequestError("Perplexity Agent supports text.format only")
		}
		var format map[string]json.RawMessage
		if err := common.Unmarshal(text["format"], &format); err != nil {
			return nil, agentRequestError("invalid Perplexity Agent text.format")
		}
		var formatType string
		_ = common.Unmarshal(format["type"], &formatType)
		switch formatType {
		case "text":
		case "json_schema":
			if len(request["response_format"]) != 0 {
				return nil, agentRequestError("text.format and response_format cannot be combined")
			}
			delete(format, "type")
			responseFormat, err := common.Marshal(map[string]any{"type": "json_schema", "json_schema": format})
			if err != nil {
				return nil, err
			}
			request["response_format"] = responseFormat
		default:
			return nil, agentRequestError("Perplexity Agent supports text or json_schema output only")
		}
		delete(request, "text")
	}
	if raw := request["tools"]; len(raw) != 0 {
		var tools []map[string]json.RawMessage
		if err := common.Unmarshal(raw, &tools); err != nil {
			return nil, agentRequestError("invalid Perplexity Agent tools")
		}
		for _, tool := range tools {
			var kind, name string
			_ = common.Unmarshal(tool["type"], &kind)
			_ = common.Unmarshal(tool["name"], &name)
			if kind != "function" || name == "" {
				return nil, agentRequestError("Perplexity Agent currently supports named function tools only; hosted tools require provider-specific billing support")
			}
		}
	}
	if raw := request["tool_choice"]; len(raw) != 0 && string(raw) != "null" {
		var choice string
		if common.Unmarshal(raw, &choice) == nil {
			if choice != "auto" && choice != "none" && choice != "required" {
				return nil, agentRequestError("unsupported Perplexity Agent tool_choice")
			}
		} else {
			var choice struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			if err := common.Unmarshal(raw, &choice); err != nil || choice.Type != "function" || choice.Name == "" {
				return nil, agentRequestError("Perplexity Agent tool_choice must select a named function")
			}
		}
	}
	return request, nil
}

// normalizeAgentResponse adds standard Responses cache keys while retaining all
// provider output fields, including opaque function-call thought signatures.
func normalizeAgentResponse(data []byte) ([]byte, *dto.Usage, *types.NewAPIError) {
	var response struct {
		Status string             `json:"status"`
		Error  *types.OpenAIError `json:"error"`
		Usage  *struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
			TotalTokens  *int `json:"total_tokens"`
			InputDetails struct {
				Cached   *int `json:"cached_tokens"`
				Read     *int `json:"cache_read_input_tokens"`
				Creation *int `json:"cache_creation_input_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails dto.OutputTokenDetails `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := common.Unmarshal(data, &response); err != nil {
		return nil, nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if response.Error != nil || response.Status == "failed" {
		if response.Error == nil {
			response.Error = &types.OpenAIError{Message: "Perplexity Agent response failed"}
		}
		return nil, nil, types.WithOpenAIError(*response.Error, http.StatusBadGateway)
	}
	if response.Usage == nil {
		return data, nil, nil
	}
	u := response.Usage
	if u.InputTokens == nil || u.OutputTokens == nil || u.TotalTokens == nil {
		return nil, nil, types.NewOpenAIError(errors.New("incomplete Perplexity Agent usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	for _, count := range []*int{u.InputTokens, u.OutputTokens, u.TotalTokens, u.InputDetails.Cached, u.InputDetails.Read, u.InputDetails.Creation, &u.OutputDetails.ReasoningTokens} {
		if count != nil && (*count < 0 || *count > math.MaxInt32) {
			return nil, nil, types.NewOpenAIError(errors.New("invalid Perplexity Agent token usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
	}
	read, write := 0, 0
	if u.InputDetails.Read != nil {
		read = *u.InputDetails.Read
	}
	if u.InputDetails.Cached != nil {
		if u.InputDetails.Read != nil && *u.InputDetails.Cached != read {
			return nil, nil, types.NewOpenAIError(errors.New("inconsistent Perplexity Agent cache usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		read = *u.InputDetails.Cached
	}
	if u.InputDetails.Creation != nil {
		write = *u.InputDetails.Creation
	}
	if read > *u.InputTokens || write > *u.InputTokens-read || u.OutputDetails.ReasoningTokens > *u.OutputTokens || *u.TotalTokens != *u.InputTokens+*u.OutputTokens {
		return nil, nil, types.NewOpenAIError(errors.New("inconsistent Perplexity Agent token usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	var err error
	data, err = sjson.SetBytes(data, "usage.input_tokens_details.cached_tokens", read)
	if err == nil {
		data, err = sjson.SetBytes(data, "usage.input_tokens_details.cache_write_tokens", write)
	}
	if err != nil {
		return nil, nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usage := &dto.Usage{InputTokens: *u.InputTokens, OutputTokens: *u.OutputTokens, TotalTokens: *u.TotalTokens, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: read, CacheWriteTokens: write}, OutputTokensDetails: &u.OutputDetails}
	result := &dto.Usage{}
	service.ApplyResponsesUsage(result, usage)
	return data, result, nil
}

func agentResponseHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid Perplexity Agent response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	data, usage, apiErr := normalizeAgentResponse(data)
	if apiErr != nil {
		return nil, apiErr
	}
	if usage == nil {
		return nil, types.NewOpenAIError(errors.New("Perplexity Agent response omitted usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	var response dto.OpenAIResponsesResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	var status string
	_ = common.Unmarshal(response.Status, &status)
	switch status {
	case "completed", "incomplete", "cancelled":
	default:
		return nil, types.NewOpenAIError(errors.New("Perplexity Agent synchronous response did not reach a terminal status"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	info.ObserveResponseModel(response.Model)
	for _, item := range response.Output {
		if item.Type == dto.BuildInCallFunctionCall {
			info.CountBillableToolCall(item.Type, item.Name)
		}
	}
	service.IOCopyBytesGracefully(c, resp, data)
	return usage, nil
}

func agentStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid Perplexity Agent stream"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)
	accumulator := service.NewResponsesUsageAccumulator(info)
	var reportedUsage *dto.Usage
	var streamErr *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var envelope map[string]json.RawMessage
		if err := common.UnmarshalJsonStr(data, &envelope); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		var event dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if event.Type == "response.failed" || event.Type == "response.error" || event.Type == "error" {
			upstreamError := types.OpenAIError{}
			if event.Response != nil && event.Response.GetOpenAIError() != nil {
				upstreamError = *event.Response.GetOpenAIError()
			} else {
				_ = common.Unmarshal(envelope["error"], &upstreamError)
			}
			if upstreamError.Message == "" {
				upstreamError.Message = "Perplexity Agent stream failed"
			}
			streamErr = types.WithOpenAIError(upstreamError, http.StatusBadGateway)
			info.StreamStatus.MarkFailed(fmt.Sprint(upstreamError.Code), upstreamError.Type, http.StatusBadGateway)
			// Keep the native error event visible to a client already receiving SSE.
			_ = helper.ResponseChunkData(c, event, data)
			sr.Stop(streamErr)
			return
		}
		var eventUsage *dto.Usage
		if raw := envelope["response"]; len(raw) != 0 && string(raw) != "null" {
			normalized, usage, apiErr := normalizeAgentResponse(raw)
			if apiErr != nil {
				streamErr = apiErr
				sr.Stop(apiErr)
				return
			}
			eventUsage = usage
			envelope["response"] = normalized
			encoded, err := common.Marshal(envelope)
			if err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			data = string(encoded)
			if err := common.Unmarshal(encoded, &event); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
		}
		switch event.Type {
		case "response.completed", "response.incomplete", "response.cancelled", "response.canceled":
			if event.Response == nil || eventUsage == nil {
				streamErr = types.NewOpenAIError(errors.New("Perplexity Agent terminal event omitted usage"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			reportedUsage = eventUsage
		}
		accumulator.Observe(&event, []byte(data))
		if err := helper.ResponseChunkData(c, event, data); err != nil {
			sr.Stop(err)
			return
		}
		switch event.Type {
		case "response.completed", "response.incomplete", "response.cancelled", "response.canceled":
			sr.Done()
		}
	})
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	usage := accumulator.Finish()
	// Reported zero is authoritative; the generic accumulator estimates zeros
	// for providers that omit counters. Agent always reports explicit counters.
	if reportedUsage != nil {
		usage = reportedUsage
	}
	return usage, streamErr
}
