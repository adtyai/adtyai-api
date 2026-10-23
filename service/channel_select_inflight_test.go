package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestAutoGroupEmptyInflightSwitchesGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	retry := 2
	param := &RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "m", Retry: &retry}
	prev := selectSatisfiedChannel
	calls := 0
	selectSatisfiedChannel = func(group, modelName string, start int, filters []dto.ChannelFilter, caps model.RouteCaps) (*model.Channel, model.RouteBandDecision, error) {
		calls++
		if calls == 1 {
			if group != "g1" || start != 2 {
				t.Errorf("select group=%s start=%d", group, start)
			}
			return nil, model.RouteBandDecision{Group: group, Reason: "inflight_rejected"}, nil
		}
		return &model.Channel{Id: 8}, model.RouteBandDecision{Group: group, Picked: 8}, nil
	}
	t.Cleanup(func() { selectSatisfiedChannel = prev })

	ch, group, err := selectAutoGroupChannel(param, []string{"g1", "g2"}, nil, 0, false)
	if err != nil || ch == nil || ch.Id != 8 || group != "g2" {
		t.Fatalf("empty inflight must try the next group, channel=%v group=%s err=%v", ch, group, err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBandStartForUsesStoredDecisionBeforeOverwrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyRouteDecision, model.RouteBandDecision{
		Group: "g1", BandIndex: 1, Picked: 9, Reason: "pick",
	})
	if got := bandStartFor(c, "g1", 0); got != 0 {
		t.Fatalf("same group must use the official retry, got %d", got)
	}
	if got := bandStartFor(c, "g2", 4); got != 4 {
		t.Fatalf("group change must use official retry, got %d", got)
	}
}
