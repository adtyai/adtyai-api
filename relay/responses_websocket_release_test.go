package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
)

func TestResponsesWSRunCallReleasesHalfOpenWithoutAttempt(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	smartrouting.ResetHalfOpenInflightForTest()
	if !smartrouting.TryIncHalfOpenInflight(42, 1) {
		t.Fatal("seed half-open slot")
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 42)

	err := (&responsesWSSession{}).runCall(c, nil, responsesWSCreateRequest{})
	if err == nil {
		t.Fatal("empty model should fail the call")
	}
	if smartrouting.HalfOpenInflightFull(42, 1) {
		t.Fatal("websocket return must release a reserved half-open slot even when no attempt is noted")
	}
}

func TestResponsesWSSetupFailureDoesNotNoteProbe(t *testing.T) {
	noted := 0
	prev := noteResponsesWSAttempt
	noteResponsesWSAttempt = func(c *gin.Context, channelID int, modelName string, err *types.NewAPIError, latencyMs int64) {
		noted++
	}
	t.Cleanup(func() { noteResponsesWSAttempt = prev })
	restore := model.InstallChannelCacheForTest([]*model.Channel{{
		Id:     42,
		Status: common.ChannelStatusManuallyDisabled,
	}})
	t.Cleanup(restore)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	err := (&responsesWSSession{lockedChannelID: 42}).runCall(c, nil, responsesWSCreateRequest{
		Request: dto.OpenAIResponsesRequest{Model: "m"},
	})
	if err == nil {
		t.Fatal("disabled locked channel must fail before upstream")
	}
	if noted != 0 {
		t.Fatalf("setup failure before upstream was recorded as a probe, notes=%d", noted)
	}
}
