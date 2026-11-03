package middleware

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 站点图标的几种文件都要带正确的 Content-Type，否则浏览器不认 manifest 和 SVG 图标。
func TestFrontendIconContentTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	files := map[string]string{
		"site.webmanifest": `{"name":"ADTY AI"}`,
		"favicon.svg":      `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"favicon.ico":      "\x00\x00\x01\x00",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	router := gin.New()
	router.NoRoute(ServeFrontendFiles(static.LocalFile(dir, false)), func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	for target, want := range map[string]string{
		"/site.webmanifest": "application/manifest+json",
		"/favicon.svg":      "image/svg+xml",
		"/favicon.ico":      "image/vnd.microsoft.icon",
	} {
		response := performFrontendRequest(router, http.MethodGet, target, "gzip")
		require.Equal(t, http.StatusOK, response.Code, target)
		assert.Equal(t, want, response.Header().Get("Content-Type"), target)
	}
}
