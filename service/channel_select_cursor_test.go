package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestPinRetryRestartsAtHighestBand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{Id: 11, Name: "pinned", Status: common.ChannelStatusEnabled, Group: "vip", Models: "gpt-5"}
	restore := model.InstallChannelCacheForTest([]*model.Channel{channel})
	t.Cleanup(restore)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
	GetChannelConstraints(c).AddPin(dto.ChannelPin{
		ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt,
	})

	picked, _, err := SelectChannelForRequest(c, "gpt-5", &RetryParam{Ctx: c, TokenGroup: "vip", ModelName: "gpt-5", Retry: common.GetPointer(0)})
	if err != nil || picked == nil || picked.Id != channel.Id {
		t.Fatalf("pin pick=%v err=%v", picked, err)
	}
	if start := bandStartFor(c, "vip", 1); start != 1 {
		t.Fatalf("pinned retry must use the official retry index, start=%d", start)
	}
}

func TestAffinityRetryRestartsAtHighestBand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{Id: 12, Name: "sticky", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-5"}
	restore := model.InstallChannelCacheForTest([]*model.Channel{channel})
	t.Cleanup(restore)
	model.SetGroupModelChannelsForTest("default", "gpt-5", []int{channel.Id})

	affinityValue := fmt.Sprintf("header-hit-%d", time.Now().UnixNano())
	rule := operation_setting.ChannelAffinityRule{
		Name:       "header-affinity",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "request_header", Key: "X-Affinity-Key"},
		},
		IncludeRuleName:  true,
		IncludeModelName: true,
	}
	setting := operation_setting.GetChannelAffinitySetting()
	prevEnabled := setting.Enabled
	originalRules := setting.Rules
	setting.Enabled = true
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	cacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", affinityValue)
	cache := getChannelAffinityCache()
	if err := cache.SetWithTTL(cacheKey, channel.Id, time.Minute); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		setting.Enabled = prevEnabled
		setting.Rules = originalRules
		_, _ = cache.DeleteMany([]string{cacheKey})
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("X-Affinity-Key", affinityValue)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")

	picked, selectGroup, err := SelectChannelForRequest(c, "gpt-5", &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "gpt-5", Retry: common.GetPointer(0)})
	if err != nil || picked == nil || picked.Id != channel.Id || selectGroup != "default" {
		t.Fatalf("affinity pick=%v group=%s err=%v", picked, selectGroup, err)
	}
	if start := bandStartFor(c, "default", 1); start != 1 {
		t.Fatalf("affinity retry must use the official retry index, start=%d", start)
	}
}
