package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWeChatSignupBindsAffiliateCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousEnabled := common.WeChatAuthEnabled
	previousAddress := common.WeChatServerAddress
	previousToken := common.WeChatServerToken
	previousRegister := common.RegisterEnabled
	previousType := common.MainDatabaseType()
	common.RedisEnabled = false
	common.WeChatAuthEnabled = true
	common.WeChatServerToken = "wechat-token"
	common.RegisterEnabled = true
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.TwoFA{},
		&model.PasskeyCredential{},
		&model.AuthFlow{},
	))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.WeChatAuthEnabled = previousEnabled
		common.WeChatServerAddress = previousAddress
		common.WeChatServerToken = previousToken
		common.RegisterEnabled = previousRegister
		common.SetMainDatabaseType(previousType)
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	inviter := model.User{
		Username: "wechat-inviter", Password: "hashed-password-value", DisplayName: "inviter",
		AffCode: "WXINVITE", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&inviter).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "new-wechat-code", r.URL.Query().Get("code"))
		_, _ = w.Write([]byte(`{"success":true,"data":"new-wechat-id"}`))
	}))
	t.Cleanup(upstream.Close)
	common.WeChatServerAddress = upstream.URL

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code=new-wechat-code&aff=WXINVITE", nil)
	c.Request.RemoteAddr = "203.0.113.9:1234"
	c.Request.Header.Set("X-Device-FP", "device-fp-wechat")
	WeChatAuth(c)

	var created model.User
	require.NoError(t, db.Where("wechat_id = ?", "new-wechat-id").First(&created).Error)
	require.Equal(t, inviter.Id, created.InviterId)
	require.Equal(t, "203.0.113.9", created.RegisterIp)
	require.Equal(t, "device-fp-wechat", created.RegisterDeviceFp)
}
