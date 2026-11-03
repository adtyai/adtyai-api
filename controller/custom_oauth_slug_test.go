package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCustomOAuthSlugConflictUsesNormalizedName(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.CustomOAuthProvider{}, &model.UserOAuthBinding{}))
	gin.SetMode(gin.TestMode)

	rejected := callCustomOAuth(t, CreateCustomOAuthProvider, "", customOAuthBody("GitHub"))
	require.Contains(t, rejected.Body.String(), "conflicts with a built-in OAuth provider")
	require.Contains(t, rejected.Body.String(), `"success":false`)
	var rows int64
	require.NoError(t, db.Model(&model.CustomOAuthProvider{}).Where("LOWER(slug) = ?", "github").Count(&rows).Error)
	require.Zero(t, rows)
	require.False(t, oauth.IsCustomProvider("github"))
	require.NotNil(t, oauth.GetProvider("github"))

	created := callCustomOAuth(t, CreateCustomOAuthProvider, "", customOAuthBody("Acme"))
	require.Contains(t, created.Body.String(), `"success":true`)
	require.True(t, oauth.IsCustomProvider("acme"))
	t.Cleanup(func() {
		oauth.UnregisterCustomProvider("acme")
	})
	var stored model.CustomOAuthProvider
	require.NoError(t, db.Where("slug = ?", "acme").First(&stored).Error)

	updated := callCustomOAuth(t, UpdateCustomOAuthProvider, strconv.Itoa(stored.Id), customOAuthBody("Discord"))
	require.Contains(t, updated.Body.String(), "conflicts with a built-in OAuth provider")
	require.False(t, oauth.IsCustomProvider("discord"))
	require.NotNil(t, oauth.GetProvider("discord"))
	require.NoError(t, db.First(&stored, stored.Id).Error)
	require.Equal(t, "acme", stored.Slug)
	require.True(t, oauth.IsCustomProvider("acme"))
}

func customOAuthBody(slug string) string {
	return `{"name":"Custom","slug":"` + slug + `","enabled":true,"client_id":"id","client_secret":"secret","authorization_endpoint":"https://example.com/auth","token_endpoint":"https://example.com/token","user_info_endpoint":"https://example.com/user"}`
}

func callCustomOAuth(t *testing.T, handler gin.HandlerFunc, id string, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-oauth", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if id != "" {
		ctx.Params = gin.Params{{Key: "id", Value: id}}
	}
	handler(ctx)
	return recorder
}
