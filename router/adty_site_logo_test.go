package router

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 站长品牌图片上传：PNG、JPEG、WebP，最大 200 KB，存 uploads/site-assets；不接受 SVG。

func TestSiteLogoSVGRules(t *testing.T) {
	for _, tc := range []struct {
		name string
		svg  string
		want error
	}{
		{"plain shapes", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#f00"/></svg>`, nil},
		{"gradient by local id", `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"><stop offset="0" stop-color="#000"/></linearGradient></defs><rect fill="url(#g)" width="1" height="1"/></svg>`, nil},
		{"script element", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"script with namespace prefix", `<svg xmlns="http://www.w3.org/2000/svg" xmlns:x="http://www.w3.org/2000/svg"><x:script>alert(1)</x:script></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"event handler", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><rect width="1" height="1"/></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"foreignObject", `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><div>x</div></foreignObject></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"link with javascript href", `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="javascript:alert(1)"><rect width="1" height="1"/></a></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"external image", `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://evil.example/x.png"/></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"external url in fill", `<svg xmlns="http://www.w3.org/2000/svg"><rect fill="url(https://evil.example/x#g)" width="1" height="1"/></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"style element", `<svg xmlns="http://www.w3.org/2000/svg"><style>rect{fill:red}</style></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"doctype with entity", `<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg xmlns="http://www.w3.org/2000/svg"><text>&x;</text></svg>`, service.ErrAdtySiteLogoUnsafe},
		{"not svg at all", `<html><body>hi</body></html>`, service.ErrAdtySiteLogoType},
		{"broken xml", `<svg xmlns="http://www.w3.org/2000/svg"><rect`, service.ErrAdtySiteLogoType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := service.ValidateAdtySiteSVG([]byte(tc.svg))
			if tc.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func uploadLogo(t *testing.T, f *ownerFixture, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/adty/site/logo", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+f.token)
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)
	return recorder
}

// 1x1 透明 PNG。
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")

func TestClosingSiteRejectsLogoUpload(t *testing.T) {
	f := newOwnerFixture(t)
	dir := t.TempDir()
	restore := service.SetAdtySiteAssetDirForTest(dir)
	t.Cleanup(restore)
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("status", model.AdtySiteStatusClosing).Error)

	blocked := uploadLogo(t, f, "logo.png", tinyPNG)
	assert.Contains(t, blocked.Body.String(), "This site is closing and can only be viewed")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a closing site must not store a logo file")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	allowed := uploadLogo(t, f, "logo.png", tinyPNG)
	assert.Contains(t, allowed.Body.String(), `"success":true`)
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestSiteLogoUpload(t *testing.T) {
	f := newOwnerFixture(t)
	dir := t.TempDir()
	restore := service.SetAdtySiteAssetDirForTest(dir)
	t.Cleanup(restore)
	engine := gin.New()
	SetApiRouter(engine)
	SetAdtyWebRoutes(engine)
	f.engine = engine

	tooBig := append(append([]byte{}, tinyPNG...), bytes.Repeat([]byte{0}, service.AdtySiteLogoMaxBytes)...)
	assert.Contains(t, uploadLogo(t, f, "big.png", tooBig).Body.String(), "The logo must be at most 200 KB")
	scripted := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(document.cookie)</script></svg>`
	assert.Contains(t, uploadLogo(t, f, "x.svg", []byte(scripted)).Body.String(), "The logo must be a PNG, JPEG, or WebP image")
	assert.Contains(t, uploadLogo(t, f, "x.gif", []byte("GIF89a\x01\x00\x01\x00")).Body.String(), `"success":false`)

	resp := uploadLogo(t, f, "logo.png", tinyPNG)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, resp.Body.String())
	assert.True(t, strings.HasPrefix(envelope.Data.URL, service.AdtySiteAssetPrefix))

	served := httptest.NewRecorder()
	engine.ServeHTTP(served, httptest.NewRequest(http.MethodGet, envelope.Data.URL, nil))
	assert.Equal(t, http.StatusOK, served.Code)
	assert.Equal(t, "image/png", served.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", served.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, tinyPNG, served.Body.Bytes())

	saved := f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"name":"Logo AI","logo_url":"`+envelope.Data.URL+`"}}`, "")
	assert.Contains(t, saved.Body.String(), envelope.Data.URL, "the uploaded address can be saved as the brand logo")

	webp, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	resp = uploadLogo(t, f, "favicon.webp", webp)
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, resp.Body.String())
	require.True(t, strings.HasSuffix(envelope.Data.URL, ".webp"))
	served = httptest.NewRecorder()
	engine.ServeHTTP(served, httptest.NewRequest(http.MethodGet, envelope.Data.URL, nil))
	assert.Equal(t, http.StatusOK, served.Code, "an uploaded WebP must be readable")
	assert.Equal(t, "image/webp", served.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", served.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, webp, served.Body.Bytes())

	for _, path := range []string{"/uploads/site-assets/../../etc/passwd", "/uploads/site-assets/site-1-nothere.png", "/uploads/site-assets/x.html"} {
		miss := httptest.NewRecorder()
		engine.ServeHTTP(miss, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, miss.Code, path)
	}
}
