package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 防邮箱枚举（任务 7.3）。

type sentMail struct{ subject, to, content string }

// captureAdtyMail 把异步发信换成同步记录，结束时还原。
func captureAdtyMail(t *testing.T) func() []sentMail {
	t.Helper()
	var mu sync.Mutex
	var sent []sentMail
	old := service.AdtySendEmailAsync
	service.AdtySendEmailAsync = func(subject, to, content string) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, sentMail{subject, to, content})
	}
	service.ResetAdtyReminderThrottle()
	t.Cleanup(func() {
		service.AdtySendEmailAsync = old
		service.ResetAdtyReminderThrottle()
	})
	return func() []sentMail {
		mu.Lock()
		defer mu.Unlock()
		return append([]sentMail(nil), sent...)
	}
}

func emailEnumDB(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.UserAccessToken{}))
	require.NoError(t, db.Create(&model.User{Username: "taken", Password: "hashed-password", Email: "taken@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
}

func callEmailHandler(handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	handler(c)
	return recorder
}

func TestVerificationDoesNotSendACodeWhenTheStoreIsDown(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousEnabled, previousClient := common.RedisEnabled, common.RDB
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousEnabled, previousClient
		_ = client.Close()
	})
	common.RedisEnabled = true
	common.RDB = client
	server.Close()

	fresh := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("new@example.com"))
	reset := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("taken@example.com"))
	assert.Equal(t, http.StatusOK, fresh.Code)
	assert.Equal(t, http.StatusOK, reset.Code)
	for _, mail := range sent() {
		assert.NotEqual(t, "new@example.com", mail.to)
		assert.NotContains(t, mail.content, "/user/reset")
	}
	assert.False(t, common.VerifyCodeWithKey("new@example.com", "000000", common.EmailVerificationPurpose))

	common.RDB = nil
	before := len(sent())
	missing := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("new@example.com"))
	registered := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("taken@example.com"))
	assert.Equal(t, http.StatusServiceUnavailable, missing.Code)
	assert.Equal(t, missing.Code, registered.Code)
	assert.Len(t, sent(), before)
}

func TestStoreFailureDoesNotRevealWhetherTheEmailExists(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousEnabled, previousClient := common.RedisEnabled, common.RDB
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousEnabled, previousClient
		_ = client.Close()
	})
	common.RedisEnabled = true
	common.RDB = client
	server.Close()

	knownReset := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("taken@example.com"))
	unknownReset := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape("missing@example.com"))
	assert.Equal(t, knownReset.Code, unknownReset.Code)
	assert.JSONEq(t, knownReset.Body.String(), unknownReset.Body.String())

	fresh := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("new@example.com"))
	taken := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("taken@example.com"))
	assert.Equal(t, fresh.Code, taken.Code)
	assert.JSONEq(t, fresh.Body.String(), taken.Body.String())
	for _, mail := range sent() {
		assert.NotContains(t, mail.content, "/user/reset")
		assert.NotContains(t, mail.content, "verification code is")
	}
}

func TestVerificationSameResponseForRegisteredEmail(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)

	fresh := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("new@example.com"))
	taken := callEmailHandler(SendEmailVerification, "/api/verification?email="+url.QueryEscape("taken@example.com"))

	assert.Equal(t, fresh.Code, taken.Code)
	assert.JSONEq(t, fresh.Body.String(), taken.Body.String())
	mails := sent()
	require.Len(t, mails, 2)
	assert.Equal(t, "new@example.com", mails[0].to)
	assert.Equal(t, "taken@example.com", mails[1].to)
	assert.Contains(t, mails[1].content, "sign in or reset your password")
	assert.False(t, common.VerifyCodeWithKey("taken@example.com", "", common.EmailVerificationPurpose))
	for _, code := range strings.Fields(mails[1].content) {
		assert.False(t, common.VerifyCodeWithKey("taken@example.com", code, common.EmailVerificationPurpose),
			"the reminder carries no usable code")
	}
}

func TestReminderRateLimitedPerEmail(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	for range 3 {
		resp := callEmailHandler(SendEmailVerification, "/api/verification?email=taken%40example.com")
		assert.Contains(t, resp.Body.String(), `"success":true`)
	}
	assert.Len(t, sent(), 1, "one reminder per 10 minutes")

	old := service.AdtyReminderInterval
	service.AdtyReminderInterval = time.Millisecond
	t.Cleanup(func() { service.AdtyReminderInterval = old })
	time.Sleep(5 * time.Millisecond)
	callEmailHandler(SendEmailVerification, "/api/verification?email=taken%40example.com")
	assert.Len(t, sent(), 2, "another one after the interval")
}

func TestResetRequestSameResponse(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	known := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email=taken%40example.com")
	unknown := callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email=nobody%40example.com")
	assert.JSONEq(t, known.Body.String(), unknown.Body.String())
	mails := sent()
	require.Len(t, mails, 1)
	assert.Equal(t, "taken@example.com", mails[0].to)
}

func TestResetLinkSingleUse(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email=taken%40example.com")
	mails := sent()
	require.Len(t, mails, 1)
	_, after, found := strings.Cut(mails[0].content, "token=")
	require.True(t, found)
	token := after[:strings.IndexAny(after, `"&<' `)]

	reset := func() string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset",
			strings.NewReader(`{"email":"taken@example.com","token":"`+token+`","password":"reset-pass-1"}`))
		ResetPassword(c)
		return recorder.Body.String()
	}
	assert.Contains(t, reset(), `"success":true`)
	assert.Contains(t, reset(), "Password reset link is invalid or has expired", "the link works once")
}

func TestResetLinkConsumedOnceUnderConcurrency(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email=taken%40example.com")
	token := resetTokenFromMail(t, sent())

	var wg sync.WaitGroup
	bodies := make([]string, 2)
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bodies[i] = postPasswordReset("taken@example.com", token)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, body := range bodies {
		var parsed struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
		}
		require.NoError(t, common.Unmarshal([]byte(body), &parsed))
		if parsed.Success {
			successes++
			assert.Empty(t, parsed.Data, "the new password must not be returned")
			assert.NotContains(t, body, "reset-pass-1")
			continue
		}
		assert.Contains(t, body, "Password reset link is invalid or has expired")
	}
	assert.Equal(t, 1, successes)
	var user model.User
	require.NoError(t, model.DB.Where("email = ?", "taken@example.com").First(&user).Error)
	assert.True(t, common.ValidatePasswordAndHash("reset-pass-1", user.Password), "the chosen password is the one stored")
	assert.Contains(t, postPasswordReset("taken@example.com", token), "Password reset link is invalid or has expired")
}

func TestResetLinkRestoredWhenPasswordWriteFails(t *testing.T) {
	emailEnumDB(t)
	sent := captureAdtyMail(t)
	callEmailHandler(SendPasswordResetEmail, "/api/reset_password?email=taken%40example.com")
	token := resetTokenFromMail(t, sent())

	failWrite := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("reset_password_write_fail", func(tx *gorm.DB) {
		if failWrite && tx.Statement.Table == "users" {
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() {
		_ = model.DB.Callback().Update().Remove("reset_password_write_fail")
	})

	failed := postPasswordReset("taken@example.com", token)
	assert.NotContains(t, failed, `"success":true`)
	failWrite = false
	kept := postPasswordReset("taken@example.com", token)
	assert.Contains(t, kept, `"success":true`)
	assert.NotContains(t, kept, "reset-pass-1")
	var user model.User
	require.NoError(t, model.DB.Where("email = ?", "taken@example.com").First(&user).Error)
	assert.True(t, common.ValidatePasswordAndHash("reset-pass-1", user.Password))
}

func resetTokenFromMail(t *testing.T, mails []sentMail) string {
	t.Helper()
	require.Len(t, mails, 1)
	_, after, found := strings.Cut(mails[0].content, "token=")
	require.True(t, found)
	return after[:strings.IndexAny(after, `"&<' `)]
}

func postPasswordReset(email, token string) string {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, _ := common.Marshal(map[string]string{"email": email, "token": token, "password": "reset-pass-1"})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", strings.NewReader(string(body)))
	ResetPassword(c)
	return recorder.Body.String()
}

func TestRegisterExistingEmailGenericError(t *testing.T) {
	emailEnumDB(t)
	oldVerify, oldRegister, oldPassword := common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled
	common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled = true, true, true
	t.Cleanup(func() {
		common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled = oldVerify, oldRegister, oldPassword
	})
	// A code for the taken address (only possible through a bug or a race) still meets the generic error.
	common.RegisterVerificationCodeWithKey("taken@example.com", "123456", common.EmailVerificationPurpose)

	register := func(email, code string) string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(
			`{"username":"newcomer","password":"password-123","email":"`+email+`","verification_code":"`+code+`"}`))
		Register(c)
		return recorder.Body.String()
	}
	taken := register("taken@example.com", "123456")
	wrong := register("other@example.com", "000000")
	assert.Contains(t, taken, "Verification code is incorrect or has expired")
	assert.Contains(t, wrong, "Verification code is incorrect or has expired")
	assert.NotContains(t, taken, "already in use")
}
