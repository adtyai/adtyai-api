package smartrouting

import (
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/tokenkit"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// BindRouteCaps stores this request's caps on the gin context.
// Missing fields stay "not required". A missing body does not reuse another request.
func BindRouteCaps(c *gin.Context, isStream bool, realtimeTools, promptTokens int) model.RouteCaps {
	caps := model.RouteCaps{IsStream: isStream, NeedContext: promptTokens}
	if realtimeTools > 0 {
		caps.NeedTools = true
	}
	if c != nil && c.Request != nil && c.Request.URL != nil {
		if strings.Contains(c.Request.URL.Path, "streamGenerateContent") || c.Request.URL.Query().Get("alt") == "sse" {
			caps.IsStream = true
		}
	}
	if c != nil {
		var raw []byte
		if body, err := common.GetRequestBody(c); err == nil && body != nil {
			if reader, ok := body.(io.Reader); ok {
				var readErr error
				raw, readErr = io.ReadAll(reader)
				_, _ = body.Seek(0, io.SeekStart)
				if readErr == nil {
					mergeCapsFromBody(&caps, raw)
				} else {
					raw = nil
				}
			}
		}
		if caps.NeedContext == 0 && len(raw) > 0 {
			modelName := ""
			if gjson.ValidBytes(raw) {
				modelName = gjson.GetBytes(raw, "model").String()
			}
			if n := tokenkit.Count(modelName, string(raw)); n > 0 {
				caps.NeedContext = n
			}
		}
		common.SetContextKey(c, constant.ContextKeyRouteCaps, caps)
	}
	return caps
}

// RouteCapsFromContext returns the caps bound for this request. Missing means not required.
func RouteCapsFromContext(c *gin.Context) model.RouteCaps {
	if c == nil {
		return model.RouteCaps{}
	}
	caps, ok := common.GetContextKeyType[model.RouteCaps](c, constant.ContextKeyRouteCaps)
	if !ok {
		return model.RouteCaps{}
	}
	return caps
}

func mergeCapsFromBody(caps *model.RouteCaps, raw []byte) {
	if caps == nil || len(raw) == 0 || !gjson.ValidBytes(raw) {
		return
	}
	if gjson.GetBytes(raw, "stream").Bool() {
		caps.IsStream = true
	}
	if jsonArrayLen(raw, "tools") > 0 || jsonArrayLen(raw, "functions") > 0 || jsonPresent(raw, "tools") {
		caps.NeedTools = true
	}
	choice := gjson.GetBytes(raw, "tool_choice")
	if choice.Exists() {
		if choice.Type == gjson.String {
			if choice.String() != "" && choice.String() != "none" {
				caps.NeedTools = true
			}
		} else if choice.Raw != "" && choice.Raw != "null" {
			caps.NeedTools = true
		}
	}
	switch gjson.GetBytes(raw, "response_format.type").String() {
	case "json_object", "json_schema":
		caps.NeedJSON = true
	}
	switch gjson.GetBytes(raw, "text.format.type").String() {
	case "json_object", "json_schema":
		caps.NeedJSON = true
	}
	if gjson.GetBytes(raw, "generationConfig.responseSchema").Exists() ||
		gjson.GetBytes(raw, "generationConfig.responseMimeType").String() == "application/json" ||
		gjson.GetBytes(raw, "generation_config.response_schema").Exists() ||
		gjson.GetBytes(raw, "generation_config.response_mime_type").String() == "application/json" {
		caps.NeedJSON = true
	}
	if bodyHasVision(raw) {
		caps.NeedVision = true
	}
}

func jsonArrayLen(raw []byte, path string) int {
	v := gjson.GetBytes(raw, path)
	if !v.IsArray() {
		return 0
	}
	return len(v.Array())
}

func jsonPresent(raw []byte, path string) bool {
	v := gjson.GetBytes(raw, path)
	if !v.Exists() {
		return false
	}
	switch v.Raw {
	case "", "null", "[]", "{}":
		return false
	}
	return true
}

func bodyHasVision(raw []byte) bool {
	for _, path := range []string{"messages", "input", "contents"} {
		arr := gjson.GetBytes(raw, path)
		if !arr.IsArray() {
			continue
		}
		found := false
		arr.ForEach(func(_, item gjson.Result) bool {
			if contentHasVision(item) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func contentHasVision(item gjson.Result) bool {
	if partIsVision(item) {
		return true
	}
	for _, path := range []string{"content", "parts"} {
		parts := item.Get(path)
		if !parts.IsArray() {
			continue
		}
		found := false
		parts.ForEach(func(_, part gjson.Result) bool {
			if partIsVision(part) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func partIsVision(part gjson.Result) bool {
	switch part.Get("type").String() {
	case "image_url", "image", "input_image":
		return true
	}
	return part.Get("inline_data").Exists() || part.Get("inlineData").Exists() ||
		part.Get("file_data").Exists() || part.Get("fileData").Exists()
}
