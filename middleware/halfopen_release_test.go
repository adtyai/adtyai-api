package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
)

func TestDistributeReleasesHalfOpenWhenHandlerDoesNotNote(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	smartrouting.ResetHalfOpenInflightForTest()
	if !smartrouting.TryIncHalfOpenInflight(42, 1) {
		t.Fatal("seed half-open slot")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 42)
		c.Next()
	})
	router.Use(Distribute())
	router.GET("/v1/videos/:task_id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/videos/abc", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)
	if smartrouting.HalfOpenInflightFull(42, 1) {
		t.Fatal("request end must release a half-open slot the handler did not note")
	}
}
