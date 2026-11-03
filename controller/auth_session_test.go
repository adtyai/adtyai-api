package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuthLogoutRejectsRefreshCookieSessionMismatch(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "auth-logout-mismatch-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})

	user := &model.User{
		Username: "logout-mismatch-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	sessionA, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent-a")
	require.NoError(t, err)
	sessionB, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent-b")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/logout", nil)
	c.Request.Header.Set("Authorization", "Bearer "+sessionA.AccessToken)
	c.Request.Header.Set("X-Auth-Session", sessionA.Session.SID)
	c.Request.AddCookie(&http.Cookie{Name: service.RefreshCookieName, Value: sessionB.RefreshToken})

	AuthLogout(c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "AUTH_SESSION_MISMATCH", response.Code)
	for _, sid := range []string{sessionA.Session.SID, sessionB.Session.SID} {
		stored, err := model.GetUserSessionBySID(sid)
		require.NoError(t, err)
		assert.Equal(t, model.UserSessionStatusActive, stored.Status)
	}
}

func TestAuthLogoutRejectsStaleAccessTokenAfterSessionRotation(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "auth-logout-stale-token-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})

	user := &model.User{
		Username: "logout-stale-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent")
	require.NoError(t, err)
	_, err = model.BumpUserAuthVersion(user.Id)
	require.NoError(t, err)
	rotated, err := service.AdvanceCurrentSessionToUserVersion(service.AuthIdentity{
		UserID: user.Id, SessionID: bundle.Session.SID, UserAuthVersion: 1, SessionVersion: 1,
	}, "password_change")
	require.NoError(t, err)
	require.NotEmpty(t, rotated.AccessToken)

	gin.SetMode(gin.TestMode)
	stale := httptest.NewRecorder()
	staleCtx, _ := gin.CreateTestContext(stale)
	staleCtx.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/logout", nil)
	staleCtx.Request.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	staleCtx.Request.Header.Set("X-Auth-Session", bundle.Session.SID)
	AuthLogout(staleCtx)

	assert.Equal(t, http.StatusUnauthorized, stale.Code)
	var rejected struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(stale.Body.Bytes(), &rejected))
	assert.False(t, rejected.Success)
	assert.Equal(t, "AUTH_SESSION_REVOKED", rejected.Code)
	stored, err := model.GetUserSessionBySID(bundle.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, stored.Status)
	assert.Equal(t, int64(2), stored.Version)

	current := httptest.NewRecorder()
	currentCtx, _ := gin.CreateTestContext(current)
	currentCtx.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/logout", nil)
	currentCtx.Request.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	currentCtx.Request.Header.Set("X-Auth-Session", bundle.Session.SID)
	AuthLogout(currentCtx)
	assert.Equal(t, http.StatusOK, current.Code)
	stored, err = model.GetUserSessionBySID(bundle.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, stored.Status)
}

func TestRefreshAuthStoresNewClientAddress(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "auth-refresh-client-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})

	user := &model.User{
		Username: "refresh-client-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	bundle, err := service.CreateLoginSession(user.Id, "password", "203.0.113.8", "OldAgent/1")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/refresh", nil)
	c.Request.RemoteAddr = "198.51.100.20:443"
	c.Request.Header.Set("User-Agent", "NewAgent/2")
	c.Request.Header.Set("X-Auth-Session", bundle.Session.SID)
	c.Request.AddCookie(&http.Cookie{Name: service.RefreshCookieName, Value: bundle.RefreshToken})

	RefreshAuth(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	stored, err := model.GetUserSessionBySID(bundle.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, "198.51.100.20", stored.IP)
	assert.Equal(t, "NewAgent/2", stored.UserAgent)
	sessions, err := service.ListLoginSessions(user.Id, bundle.Session.SID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "198.51.100.20", sessions[0].IP)
	assert.Equal(t, "NewAgent/2", sessions[0].UserAgent)
}

func TestWriteAuthSessionErrorMapsSessionGrowthLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "active session limit",
			err:            model.ErrUserSessionLimit,
			expectedStatus: http.StatusConflict,
			expectedCode:   "AUTH_SESSION_LIMIT",
		},
		{
			name:           "issuance limit",
			err:            model.ErrUserSessionIssuanceLimit,
			expectedStatus: http.StatusTooManyRequests,
			expectedCode:   "AUTH_SESSION_ISSUANCE_LIMIT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			writeAuthSessionError(c, test.err)

			assert.Equal(t, test.expectedStatus, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.expectedCode, response.Code)
		})
	}
}

func TestSessionLimitDoesNotRecordRejectedLoginAsSuccessful(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousActiveLimit := common.UserSessionActiveLimit
	previousIssuanceLimit := common.UserSessionIssuanceLimit
	previousIssuanceWindow := common.UserSessionIssuanceWindowSeconds
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.TwoFA{}, &model.PasskeyCredential{}))
	model.DB = db
	common.RedisEnabled = false
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 100
	common.UserSessionIssuanceWindowSeconds = int64(common.DefaultUserSessionIssuanceWindowSeconds)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.UserSessionActiveLimit = previousActiveLimit
		common.UserSessionIssuanceLimit = previousIssuanceLimit
		common.UserSessionIssuanceWindowSeconds = previousIssuanceWindow
	})

	const previousLastLoginAt = int64(123)
	user := &model.User{
		Username: "rejected-login-audit-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, LastLoginAt: previousLastLoginAt,
	}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: "existing-active-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
	setupLogin(user, nil, c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, previousLastLoginAt, stored.LastLoginAt)
}
