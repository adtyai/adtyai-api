package controller

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func emailEnumDB(t *testing.T) *securityMailbox {
	t.Helper()
	require.NoError(t, i18n.Init())
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.UserAccessToken{}, &model.AuditLog{}))
	require.NoError(t, db.Create(&model.User{Username: "taken", Password: "hashed-password", Email: "taken@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	common.OptionMapRWMutex.Lock()
	previousProvider, hadProvider := common.OptionMap["EmailProvider"]
	providerMapWasNil := common.OptionMap == nil
	if providerMapWasNil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap["EmailProvider"] = "smtp"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if providerMapWasNil {
			common.OptionMap = nil
			return
		}
		if hadProvider {
			common.OptionMap["EmailProvider"] = previousProvider
		} else {
			delete(common.OptionMap, "EmailProvider")
		}
	})
	return newSecurityMailbox(t)
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

func TestPasswordResetCommitOutcome(t *testing.T) {
	for _, failure := range []string{"none", "publish_query", "session_revoke_query", "commit_acknowledgement", "notification"} {
		t.Run(failure, func(t *testing.T) {
			mailbox := emailEnumDB(t)
			db := model.DB
			if failure == "notification" {
				// The real local SMTP server does not advertise AUTH. This makes
				// notification delivery fail at SMTP negotiation without external mail.
				common.SMTPAccount, common.SMTPToken = "sender@example.com", "test-only-smtp-password"
			}
			var owner model.User
			require.NoError(t, db.Where("email = ?", "taken@example.com").First(&owner).Error)
			require.NoError(t, db.Create(&model.UserSession{
				SID: "reset-existing-session", UserID: owner.Id, Version: 1,
				UserAuthVersion: owner.AuthVersion, Status: model.UserSessionStatusActive,
				RefreshHash: strings.Repeat("a", 64), LoginMethod: "password", ExpiresAt: time.Now().Unix() + 3600,
			}).Error)
			require.NoError(t, db.Create(&model.UserAccessToken{UserId: owner.Id, TokenHash: strings.Repeat("b", 64)}).Error)
			const proof = "reset-once-proof"
			require.NoError(t, common.RegisterVerificationCodeWithKey(owner.Email, proof, common.PasswordResetPurpose))
			t.Cleanup(func() { common.DeleteKey(owner.Email, common.PasswordResetPurpose) })

			passwordWritten, injected, failQuery := false, false, true
			if failure == "publish_query" || failure == "session_revoke_query" {
				writeCallback, queryCallback := "test:reset-password-written", "test:reset-postcommit-query"
				require.NoError(t, db.Callback().Update().After("gorm:update").Register(writeCallback, func(tx *gorm.DB) {
					changes, ok := tx.Statement.Dest.(map[string]any)
					if _, hasPassword := changes["password"]; ok && hasPassword && tx.Statement.Table == "users" && tx.Error == nil && tx.RowsAffected == 1 {
						passwordWritten = true
					}
				}))
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
					table := "users"
					if failure == "session_revoke_query" {
						table = "user_sessions"
					}
					if failQuery && passwordWritten && !injected && tx.Statement.Table == table {
						injected = true
						tx.AddError(assert.AnError)
					}
				}))
				t.Cleanup(func() {
					require.NoError(t, db.Callback().Update().Remove(writeCallback))
					require.NoError(t, db.Callback().Query().Remove(queryCallback))
				})
			}
			committedWithoutAcknowledgement := false
			if failure == "commit_acknowledgement" {
				connection, err := db.DB()
				require.NoError(t, err)
				pool := &resetCommitErrorPool{DB: connection, committed: &committedWithoutAcknowledgement}
				faultDB := db.Session(&gorm.Session{NewDB: true, Context: context.Background()})
				faultDB.ConnPool, faultDB.Statement.ConnPool = pool, pool
				model.DB = faultDB
				t.Cleanup(func() { model.DB = db })
			}

			response := postPasswordReset(owner.Email, proof)
			failQuery = false
			model.DB = db
			if failure == "none" || failure == "notification" {
				assert.Contains(t, response, `"success":true`)
			} else {
				assert.NotContains(t, response, `"success":true`)
			}
			if failure == "publish_query" || failure == "session_revoke_query" {
				require.True(t, injected, "the failure must occur after the password UPDATE")
			}
			if failure == "commit_acknowledgement" {
				require.True(t, committedWithoutAcknowledgement, "the real SQL transaction must commit before its acknowledgement is lost")
			}
			var stored model.User
			require.NoError(t, db.First(&stored, owner.Id).Error)
			require.True(t, common.ValidatePasswordAndHash("reset-pass-1", stored.Password), "the first request committed the chosen password")
			assert.Equal(t, owner.AuthVersion+1, stored.AuthVersion)
			var accessTokens int64
			require.NoError(t, db.Model(&model.UserAccessToken{}).Where("user_id = ?", owner.Id).Count(&accessTokens).Error)
			assert.Zero(t, accessTokens, "personal access tokens were removed in the password transaction")
			_, _, err := service.ValidateLoginSession(service.AuthIdentity{
				UserID: owner.Id, SessionID: "reset-existing-session", SessionVersion: 1, UserAuthVersion: owner.AuthVersion,
			})
			assert.ErrorIs(t, err, service.ErrLoginSessionRevoked, "old sessions cannot survive the committed auth-version change")

			replayed := postPasswordResetWithPassword(owner.Email, proof, "replayed-password-2")
			assert.Contains(t, replayed, "Password reset link is invalid or has expired", "a committed or uncertain reset must not restore its proof")
			require.NoError(t, db.First(&stored, owner.Id).Error)
			assert.True(t, common.ValidatePasswordAndHash("reset-pass-1", stored.Password), "a replay cannot choose another password")
			assert.Equal(t, owner.AuthVersion+1, stored.AuthVersion, "the reset proof permits one committed password change")

			mailbox.mutex.Lock()
			mails := append([]string(nil), mailbox.mail[owner.Email]...)
			mailbox.mutex.Unlock()
			var audits []model.AuditLog
			require.NoError(t, model.LOG_DB.Where("user_id = ? AND action = ?", owner.Id, "user.password_change").Find(&audits).Error)
			if failure == "commit_acknowledgement" {
				assert.Empty(t, mails, "an unknown commit outcome must not claim confirmed success")
				assert.Len(t, audits, 1, "an uncertain reset is still recorded once without claiming success")
				for _, audit := range audits {
					assert.False(t, audit.Success)
				}
				return
			}
			if failure == "notification" {
				assert.Empty(t, mails)
			} else {
				assert.Len(t, mails, 1, "every confirmed password change sends one security notice")
				if len(mails) == 1 {
					assert.Contains(t, mails[0], "Password updated")
					assert.NotContains(t, mails[0], proof)
					assert.NotContains(t, mails[0], "reset-pass-1")
				}
			}
			assert.Len(t, audits, 1, "the committed reset is recorded once even when finalization fails")
			if len(audits) == 1 {
				audit := audits[0]
				assert.Equal(t, model.AuditCategorySecurity, audit.Category)
				assert.Equal(t, common.RoleCommonUser, audit.ActorRole)
				assert.True(t, audit.Success)
				require.NotNil(t, audit.Other.Op)
				encoded, err := common.Marshal(audit.Other.Op.Params)
				require.NoError(t, err)
				var params struct {
					Success            bool `json:"success"`
					PasswordReset      bool `json:"password_reset"`
					NotificationFailed bool `json:"notification_failed"`
				}
				require.NoError(t, common.Unmarshal(encoded, &params))
				assert.True(t, params.Success)
				assert.True(t, params.PasswordReset)
				assert.Equal(t, failure == "notification", params.NotificationFailed)
				assert.NotContains(t, string(encoded), proof)
				assert.NotContains(t, string(encoded), "reset-pass-1")
				assert.NotContains(t, audit.Content, proof)
			}
		})
	}
}

func TestPasswordResetRejectsChangedEmailOwner(t *testing.T) {
	for _, mutation := range []string{"soft_deleted", "email_reassigned"} {
		t.Run(mutation, func(t *testing.T) {
			mailbox := emailEnumDB(t)
			db := model.DB
			var owner model.User
			require.NoError(t, db.Where("email = ?", "taken@example.com").First(&owner).Error)
			other := model.User{
				Username: "replacement-email-owner", Password: "other-original-password-hash", Email: "other@example.com",
				Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
			}
			require.NoError(t, db.Create(&other).Error)
			require.NoError(t, db.Create(&model.UserAccessToken{UserId: owner.Id, TokenHash: strings.Repeat("c", 64)}).Error)
			const proof = "proof-for-original-email-owner"
			require.NoError(t, common.RegisterVerificationCodeWithKey(owner.Email, proof, common.PasswordResetPurpose))
			t.Cleanup(func() { common.DeleteKey(owner.Email, common.PasswordResetPurpose) })

			connectionPool, err := db.DB()
			require.NoError(t, err)
			connection, err := connectionPool.Conn(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, connection.Close()) })
			callback := "test:reset-email-owner-changed"
			mutated, changedRows := false, int64(0)
			var mutationErr error
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				users, ok := tx.Statement.Dest.(*[]model.User)
				if mutated || !ok || tx.Error != nil || len(*users) != 1 || (*users)[0].Id != owner.Id || (*users)[0].Email != owner.Email {
					return
				}
				mutated = true
				// This connection is reserved before the lookup. It is distinct
				// from the connection executing GetUniqueUserByEmail.
				var result sql.Result
				if mutation == "soft_deleted" {
					result, mutationErr = connection.ExecContext(t.Context(), "UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1", owner.Id)
				} else {
					result, mutationErr = connection.ExecContext(t.Context(),
						"UPDATE users SET email = CASE WHEN id = $1 THEN $2 ELSE $3 END WHERE id IN ($1, $4)",
						owner.Id, "moved@example.com", owner.Email, other.Id)
				}
				if mutationErr == nil {
					changedRows, mutationErr = result.RowsAffected()
				}
				if mutationErr != nil {
					tx.AddError(mutationErr)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

			response := postPasswordReset(owner.Email, proof)
			require.True(t, mutated, "the account must change after the reset lookup")
			require.NoError(t, mutationErr)
			if mutation == "soft_deleted" {
				require.EqualValues(t, 1, changedRows)
			} else {
				require.EqualValues(t, 2, changedRows)
			}
			assert.NotContains(t, response, `"success":true`, "the reset cannot use an account snapshot whose email ownership changed")
			replayed := postPasswordResetWithPassword(owner.Email, proof, "replayed-password-2")
			assert.NotContains(t, replayed, `"success":true`, "an old proof cannot be retargeted to the new owner of the same email")

			for _, original := range []model.User{owner, other} {
				var stored model.User
				require.NoError(t, db.Unscoped().First(&stored, original.Id).Error)
				assert.Equal(t, original.Password, stored.Password, "neither account may receive a new password from the stale proof")
				assert.Equal(t, original.AuthVersion, stored.AuthVersion, "a rejected stale proof must not commit an auth-version change")
			}
			var tokens int64
			require.NoError(t, db.Model(&model.UserAccessToken{}).Where("user_id = ?", owner.Id).Count(&tokens).Error)
			assert.EqualValues(t, 1, tokens, "rejecting a stale subject must not delete its credentials")
			mailbox.mutex.Lock()
			assert.Empty(t, mailbox.mail, "neither the old nor the replacement email owner receives a success notice")
			mailbox.mutex.Unlock()
			var successes int64
			require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("action = ? AND success = ?", "user.password_change", true).Count(&successes).Error)
			assert.Zero(t, successes, "a missing or reassigned account must not be audited as a committed password change")
		})
	}
}

// This fault boundary keeps the real database commit and loses only the reply
// observed by the application, as can happen when the connection drops at COMMIT.
type resetCommitErrorPool struct {
	*sql.DB
	committed *bool
}

func (pool *resetCommitErrorPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := pool.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &resetCommitErrorTx{Tx: tx, committed: pool.committed}, nil
}

type resetCommitErrorTx struct {
	*sql.Tx
	committed *bool
}

func (tx *resetCommitErrorTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	*tx.committed = true
	return assert.AnError
}

func resetTokenFromMail(t *testing.T, mails []sentMail) string {
	t.Helper()
	require.Len(t, mails, 1)
	_, after, found := strings.Cut(mails[0].content, "token=")
	require.True(t, found)
	return after[:strings.IndexAny(after, `"&<' `)]
}

func postPasswordReset(email, token string) string {
	return postPasswordResetWithPassword(email, token, "reset-pass-1")
}

func postPasswordResetWithPassword(email, token, password string) string {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, _ := common.Marshal(map[string]string{"email": email, "token": token, "password": password})
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

func TestRegisterDefaultTokenRespectsOwnerLimit(t *testing.T) {
	for _, competing := range []bool{false, true} {
		name := "available"
		if competing {
			name = "competing_create"
		}
		t.Run(name, func(t *testing.T) {
			emailEnumDB(t)
			db := model.DB
			require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}, &model.Option{}))
			require.NoError(t, model.MigrateAdtyTables())
			oldRegister, oldPassword, oldVerify := common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled
			oldDefault, oldLimit := constant.GenerateDefaultToken, operation_setting.GetTokenSetting().MaxUserTokens
			common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = true, true, false
			constant.GenerateDefaultToken, operation_setting.GetTokenSetting().MaxUserTokens = true, 1
			t.Cleanup(func() {
				common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = oldRegister, oldPassword, oldVerify
				constant.GenerateDefaultToken, operation_setting.GetTokenSetting().MaxUserTokens = oldDefault, oldLimit
			})
			const username = "default-token-owner"
			var competingResponse *httptest.ResponseRecorder
			injected := false
			if competing {
				callback := "test:register-default-token-competitor"
				require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
					owner, ok := tx.Statement.Dest.(*model.User)
					if injected || !ok || tx.Error != nil || owner.Id <= 0 || owner.Username != username {
						return
					}
					injected = true
					ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
						"name": "competing-token", "expired_time": -1, "unlimited_quota": true, "group": "auto",
					}, owner.Id)
					common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
					AddToken(ctx)
					competingResponse = recorder
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"username":"`+username+`","password":"register-password"}`))
			Register(ctx)
			response := decodeAPIResponse(t, recorder)
			require.True(t, response.Success, "the committed account remains a successful registration: %s", recorder.Body.String())
			if competing {
				require.True(t, injected, "the competing creation must run after account insertion")
				require.NotNil(t, competingResponse)
				require.True(t, decodeAPIResponse(t, competingResponse).Success, competingResponse.Body.String())
			}
			var owner model.User
			require.NoError(t, db.Where("username = ?", username).First(&owner).Error)
			var tokens []model.Token
			require.NoError(t, db.Where("user_id = ?", owner.Id).Find(&tokens).Error)
			require.Equal(t, 1, len(tokens), "registration and AddToken share the owner limit")
			if competing {
				assert.Equal(t, "competing-token", tokens[0].Name, "a full account must not receive a second, default token")
			} else {
				assert.True(t, tokens[0].AllowUserQuotaQuery)
			}
		})
	}
}
