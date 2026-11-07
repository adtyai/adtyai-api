package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// RelayEdits serves POST /v1/edits. Image edits use the image helper. A JSON
// body that only carries the legacy text-edit instruction stays on the text helper.
func RelayEdits(c *gin.Context) {
	Relay(c, editsRelayFormat(c))
}

func editsRelayFormat(c *gin.Context) types.RelayFormat {
	if legacyTextEdit(c) {
		c.Set("relay_mode", relayconstant.RelayModeEdits)
		return types.RelayFormatOpenAI
	}
	c.Set("relay_mode", relayconstant.RelayModeImagesEdits)
	return types.RelayFormatOpenAIImage
}

func legacyTextEdit(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if strings.Contains(strings.ToLower(c.ContentType()), "multipart/form-data") {
		return false
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	body, err := storage.Bytes()
	if err != nil {
		return false
	}
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return false
	}
	c.Request.Body = io.NopCloser(storage)

	var payload struct {
		Instruction string          `json:"instruction"`
		Image       json.RawMessage `json:"image"`
		Images      json.RawMessage `json:"images"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return false
	}
	if strings.TrimSpace(payload.Instruction) == "" {
		return false
	}
	return !jsonFieldPresent(payload.Image) && !jsonFieldPresent(payload.Images)
}

func jsonFieldPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && string(trimmed) != "null"
}
