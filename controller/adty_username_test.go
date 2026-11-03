package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 用户名不能含 @（执行层决定 16、任务 8.7）：登录框按 username = ? OR email = ? 取第一条，
// 用户名像邮箱就可能挡住别人用邮箱登录。校验在服务端做，前端只是提前提示。

func TestUsernameRejectsAt(t *testing.T) {
	for _, test := range []struct {
		username string
		valid    bool
	}{
		{"bob_01", true},
		{"a@b.co", false},
		{"@", false},
		{strings.Repeat("a", 20), true},
		{strings.Repeat("a", 21), false},
	} {
		err := common.Validate.Struct(&model.User{Username: test.username, Password: "password-123"})
		if test.valid {
			assert.NoError(t, err, test.username)
		} else {
			assert.Error(t, err, test.username)
		}
	}
}

func TestUsernameWithAtRejectedOnAccountCreation(t *testing.T) {
	oldRegister, oldPassword, oldVerify := common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = true, true, false
	t.Cleanup(func() {
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = oldRegister, oldPassword, oldVerify
	})
	// 校验在查库之前，库里放不进任何用户：请求一旦越过校验就会因为没有表而报数据库错误。
	adtyTestDB(t)

	for _, test := range []struct {
		name    string
		path    string
		handler gin.HandlerFunc
	}{
		{"register", "/api/user/register", Register},
		{"admin create", "/api/user/", CreateUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, test.path,
				strings.NewReader(`{"username":"victim@example.com","password":"password-123","role":1}`))
			c.Set("role", common.RoleRootUser)
			test.handler(c)

			body := recorder.Body.String()
			assert.Contains(t, body, `"success":false`)
			assert.Contains(t, body, "excludes", "rejected by the username rule, not by a later step")
		})
	}
}

type adtyAtUsernameOAuthProvider struct{ authFlowTestOAuthProvider }

func (*adtyAtUsernameOAuthProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return &oauth.OAuthUser{ProviderUserID: "external-at", Username: "a@b.co"}, nil
}

// 第三方登录建号不走 Validate，提供方给的用户名（OIDC 的 preferred_username 常是邮箱）要单独挡住。
func TestOAuthSignUpIgnoresUsernameWithAt(t *testing.T) {
	setupAuthFlowControllerTest(t)
	oldRegister := common.RegisterEnabled
	common.RegisterEnabled = true
	t.Cleanup(func() { common.RegisterEnabled = oldRegister })

	provider := &adtyAtUsernameOAuthProvider{}
	oauthUser, err := provider.GetUserInfo(context.Background(), nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/test", nil)

	user, _, err := findOrCreateOAuthUser(c, provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Equal(t, "flow_1", user.Username)
	assert.NotContains(t, user.Username, "@")
}
