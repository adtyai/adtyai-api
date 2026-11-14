package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inheritedEmailLoginDB(t *testing.T) (main, inherited *model.User) {
	t.Helper()
	require.NoError(t, i18n.Init())
	oldRedis := common.RedisEnabled
	oldPasswordLogin := common.PasswordLoginEnabled
	common.RedisEnabled = false
	common.PasswordLoginEnabled = true
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		common.PasswordLoginEnabled = oldPasswordLogin
	})
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.UserAccessToken{}, &model.TwoFA{}, &model.PasskeyCredential{}, &model.AuditLog{}))
	mainHash, err := common.HashAccountPassword("main-password-1")
	require.NoError(t, err)
	inheritedHash, err := common.HashAccountPassword("inherited-password-1")
	require.NoError(t, err)
	mainUser := &model.User{
		Username: "api-inherit-main", Password: mainHash, Email: "api-shared@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	inheritedUser := &model.User{
		Username: "api-inherit-closed", Password: inheritedHash, Email: "API-SHARED@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		InheritedFromSiteId: 9,
	}
	require.NoError(t, db.Create(mainUser).Error)
	require.NoError(t, db.Create(inheritedUser).Error)
	return mainUser, inheritedUser
}

func callJSONHandler(handler gin.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	handler(c)
	return recorder
}

func TestInheritedEmailLoginRequiresUsernameThenPassword(t *testing.T) {
	main, inherited := inheritedEmailLoginDB(t)

	ambiguous := callJSONHandler(Login, http.MethodPost, "/api/user/login",
		`{"username":"api-shared@example.com","password":"main-password-1"}`)
	assert.Contains(t, ambiguous.Body.String(), `"success":false`)
	assert.Contains(t, ambiguous.Body.String(), model.EmailAmbiguousCode)
	assert.Contains(t, ambiguous.Body.String(), model.EmailAmbiguousPrompt)

	wrongPassword := callJSONHandler(Login, http.MethodPost, "/api/user/login",
		`{"username":"api-inherit-main","password":"inherited-password-1"}`)
	assert.Contains(t, wrongPassword.Body.String(), `"success":false`)
	assert.NotContains(t, wrongPassword.Body.String(), model.EmailAmbiguousCode)

	ok := callJSONHandler(Login, http.MethodPost, "/api/user/login",
		`{"username":"api-inherit-main","password":"main-password-1"}`)
	assert.Contains(t, ok.Body.String(), `"success":true`)
	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			User struct {
				Id       int    `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(ok.Body.Bytes(), &parsed))
	assert.Equal(t, main.Id, parsed.Data.User.Id)
	assert.Equal(t, "api-inherit-main", parsed.Data.User.Username)
	assert.NotEqual(t, inherited.Id, parsed.Data.User.Id)
}

func TestInheritedEmailResetRequiresUsernameAndBindsTheChosenAccount(t *testing.T) {
	main, inherited := inheritedEmailLoginDB(t)
	sent := captureAdtyMail(t)

	ambiguous := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("api-shared@example.com"))
	assert.Contains(t, ambiguous.Body.String(), `"success":false`)
	assert.Contains(t, ambiguous.Body.String(), model.EmailAmbiguousCode)
	assert.Empty(t, sent())

	unknownName := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("api-shared@example.com")+"&username=nobody-here")
	assert.JSONEq(t, `{"success":true,"message":""}`, unknownName.Body.String())
	assert.Empty(t, sent())

	wrongEmail := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("other@example.com")+"&username=api-inherit-closed")
	assert.JSONEq(t, `{"success":true,"message":""}`, wrongEmail.Body.String())
	assert.Empty(t, sent())

	issued := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("api-shared@example.com")+"&username=api-inherit-closed")
	assert.JSONEq(t, `{"success":true,"message":""}`, issued.Body.String())
	mails := sent()
	require.Len(t, mails, 1)
	assert.Equal(t, "api-shared@example.com", mails[0].to)
	assert.Contains(t, mails[0].content, "username=api-inherit-closed")
	_, after, found := strings.Cut(mails[0].content, "token=")
	require.True(t, found)
	token := after[:strings.IndexAny(after, `"&<' `)]

	wrongComplete := callJSONHandler(ResetPassword, http.MethodPost, "/api/user/reset",
		`{"email":"api-shared@example.com","username":"api-inherit-main","token":"`+token+`","password":"ResetPass123"}`)
	assert.Contains(t, wrongComplete.Body.String(), "Password reset link is invalid or has expired")

	ok := callJSONHandler(ResetPassword, http.MethodPost, "/api/user/reset",
		`{"email":"api-shared@example.com","username":"api-inherit-closed","token":"`+token+`","password":"ResetPass123"}`)
	assert.Contains(t, ok.Body.String(), `"success":true`)

	var storedMain, storedInherited model.User
	require.NoError(t, model.DB.First(&storedMain, main.Id).Error)
	require.NoError(t, model.DB.First(&storedInherited, inherited.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("main-password-1", storedMain.Password))
	assert.True(t, common.ValidatePasswordAndHash("ResetPass123", storedInherited.Password))
	assert.Equal(t, main.Id, storedMain.Id)
	assert.Equal(t, inherited.Id, storedInherited.Id)
}
