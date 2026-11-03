package controller

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func uploadSiteLogo(t *testing.T, ownerID int, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/adty/site/logo", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Set("id", ownerID)
	UploadAdtySiteLogo(ctx)
	return recorder
}

func TestUploadSiteLogoAcceptsWebPAndRejectsSVG(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}))
	restore := service.SetAdtySiteAssetDirForTest(t.TempDir())
	t.Cleanup(restore)
	owner := &model.User{Username: "logo-owner", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(&model.AdtySite{
		OwnerUserId: owner.Id, Subdomain: "logo", RootDomain: "example.com",
		Status: model.AdtySiteStatusActive, AgreementVer: service.AdtyCurrentSiteAgreementVersion(),
	}).Error)

	webp := append([]byte("RIFF\x00\x00\x00\x00WEBP"), bytes.Repeat([]byte{0}, 16)...)
	okResp := uploadSiteLogo(t, owner.Id, "logo.webp", webp)
	require.Equal(t, http.StatusOK, okResp.Code)
	require.Contains(t, okResp.Body.String(), `"success":true`)
	require.Contains(t, okResp.Body.String(), ".webp")

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	bad := uploadSiteLogo(t, owner.Id, "logo.svg", svg)
	require.Contains(t, bad.Body.String(), `"success":false`)
}
