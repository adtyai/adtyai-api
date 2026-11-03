package controller

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayEditsImageJSONUsesImageHelper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/edits", strings.NewReader(`{"model":"gpt-image-1","prompt":"a cat","image":"aGVsbG8="}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := editsRelayInfo(t, ctx)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Contains(t, info.RequestURLPath, "/v1/images/edits")
	_, isImage := info.Request.(*dto.ImageRequest)
	require.True(t, isImage)

	apiErr := relayHandler(ctx, info)
	require.Error(t, apiErr)
	require.NotContains(t, apiErr.Error(), "GeneralOpenAIRequest")
	require.NotContains(t, apiErr.Error(), "invalid request type")
	require.Contains(t, apiErr.Error(), "do request failed")
}

func TestRelayEditsLegacyInstructionUsesTextHelper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/edits", strings.NewReader(`{"model":"text-davinci-edit-001","input":"wek","instruction":"Fix spelling"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := editsRelayInfo(t, ctx)
	require.Equal(t, relayconstant.RelayModeEdits, info.RelayMode)
	require.Contains(t, info.RequestURLPath, "/v1/edits")
	require.NotContains(t, info.RequestURLPath, "/v1/images/edits")
	_, isText := info.Request.(*dto.GeneralOpenAIRequest)
	require.True(t, isText)

	apiErr := relayHandler(ctx, info)
	require.Error(t, apiErr)
	require.NotContains(t, apiErr.Error(), "ImageRequest")
	require.NotContains(t, apiErr.Error(), "invalid request type")
	require.Contains(t, apiErr.Error(), "do request failed")
}

func TestRelayEditsMultipartUsesImageHelper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "a cat"))
	part, err := writer.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("png"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	info := editsRelayInfo(t, ctx)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Contains(t, info.RequestURLPath, "/v1/images/edits")
	imageRequest, isImage := info.Request.(*dto.ImageRequest)
	require.True(t, isImage)
	require.Equal(t, "gpt-image-1", imageRequest.Model)

	apiErr := relayHandler(ctx, info)
	require.Error(t, apiErr)
	require.NotContains(t, apiErr.Error(), "GeneralOpenAIRequest")
	require.NotContains(t, apiErr.Error(), "invalid request type")
	require.Contains(t, apiErr.Error(), "do request failed")
}

func editsRelayInfo(t *testing.T, ctx *gin.Context) *relaycommon.RelayInfo {
	t.Helper()
	format := editsRelayFormat(ctx)
	request, err := helper.GetAndValidateRequest(ctx, format)
	require.NoError(t, err)
	info, err := relaycommon.GenRelayInfo(ctx, format, request, nil)
	require.NoError(t, err)
	if format == types.RelayFormatOpenAIImage {
		require.IsType(t, &dto.ImageRequest{}, request)
	}
	return info
}
