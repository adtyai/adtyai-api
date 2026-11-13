package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKeyRateLimitRejectsTheNextRequestOverTheCap(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AdtyKeyLimit{}))
	oldDB := model.DB
	model.DB = db
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.RedisEnabled = previousRedis
	})
	require.NoError(t, model.SaveAdtyKeyLimit(11, 1))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("token_id", 11)
		c.Next()
	}, AdtyKeyRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	other := gin.New()
	other.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("token_id", 12)
		c.Next()
	}, AdtyKeyRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	engine.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code, second.Body.String())

	untouched := httptest.NewRecorder()
	other.ServeHTTP(untouched, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusOK, untouched.Code, "a key without a cap is not limited")
	_ = time.Minute
}
