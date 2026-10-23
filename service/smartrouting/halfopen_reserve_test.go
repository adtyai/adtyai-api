package smartrouting

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestNormalPickDoesNotReserveHalfOpenSlot(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)

	prevHook := model.OnChannelSelectedHook
	model.RegisterOnChannelSelectedHook(OnChannelSelected)
	t.Cleanup(func() { model.OnChannelSelectedHook = prevHook })
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: 42, Stage: "half_2"}})

	weight := uint(1)
	ch := &model.Channel{Id: 42, Status: common.ChannelStatusEnabled, Weight: &weight}
	picked, decision, err := model.SelectFromPriorityBands([]model.PriorityBand{{
		Priority: 10,
		Channels: []*model.Channel{ch},
	}}, 0, "m", "g", model.RouteCaps{})
	if err != nil || picked == nil || picked.Id != 42 {
		t.Fatalf("normal pick failed: channel=%v err=%v", picked, err)
	}
	if decision.HalfOpen {
		t.Fatal("official pick must not be marked as a half-open probe")
	}
	if got := halfOpenInflightCount(42); got != 0 {
		t.Fatalf("normal pick reserved a probe slot, count=%d", got)
	}
}

func TestHalfOpenProbeStillReservesSlot(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)
	prevSelected, prevProbe := model.OnChannelSelectedHook, model.OnHalfOpenProbeHook
	model.RegisterOnChannelSelectedHook(OnChannelSelected)
	model.RegisterOnHalfOpenProbeHook(ReserveHalfOpenProbe)
	t.Cleanup(func() {
		model.OnChannelSelectedHook = prevSelected
		model.OnHalfOpenProbeHook = prevProbe
	})
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: 9, Stage: "half_2"}})
	prevWeight := model.SmartWeightHook
	model.SmartWeightHook = func(channels []*model.Channel, modelName, group string, caps model.RouteCaps) model.SmartWeightResult {
		return model.SmartWeightResult{Skip: true, HalfOpen: &model.Channel{Id: 9}, Decision: model.RouteBandDecision{Reason: "skip"}}
	}
	t.Cleanup(func() { model.SmartWeightHook = prevWeight })

	picked, decision, err := model.SelectFromPriorityBands([]model.PriorityBand{{
		Priority: 10,
		Channels: []*model.Channel{{Id: 1}},
	}}, 0, "m", "g", model.RouteCaps{})
	if err != nil || picked == nil || picked.Id != 9 || !decision.HalfOpen {
		t.Fatalf("probe pick failed: channel=%v decision=%+v err=%v", picked, decision, err)
	}
	if got := halfOpenInflightCount(9); got != 1 {
		t.Fatalf("probe must still reserve a slot, count=%d", got)
	}
}

func TestUserErrorDoesNotReopenHalfOpenProbe(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prev := GetConfig()
	cfg := prev
	cfg.Mode = "off"
	cfg.Enabled = false
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: 42, Stage: "half_2"}})
	if !TryIncHalfOpenInflight(42, 1) {
		t.Fatal("seed slot")
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 42)

	defer func() {
		if recover() != nil {
			t.Fatal("user error must not report a half-open probe failure")
		}
	}()
	OnAttemptEnd(c, AttemptEvent{ChannelID: 42, Model: "m", Success: false, Class: ClassUserError})
	if got := halfOpenInflightCount(42); got != 0 {
		t.Fatalf("user error must still release the slot, count=%d", got)
	}
}

func TestProbeFailureIgnoresClientAndBalanceErrors(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prev := GetConfig()
	cfg := prev
	cfg.Mode = "off"
	cfg.Enabled = false
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: 42, Stage: "half_2"}})

	prevReport := reportHalfOpenResult
	t.Cleanup(func() { reportHalfOpenResult = prevReport })

	gin.SetMode(gin.TestMode)
	report := func(class ErrClass, success bool) (called bool, reportedSuccess bool) {
		ResetHalfOpenInflightForTest()
		if !TryIncHalfOpenInflight(42, 1) {
			t.Fatal("seed slot")
		}
		called = false
		reportedSuccess = true
		reportHalfOpenResult = func(id int, ok bool) {
			called = true
			reportedSuccess = ok
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 42)
		OnAttemptEnd(c, AttemptEvent{ChannelID: 42, Model: "m", Success: success, Class: class})
		if got := halfOpenInflightCount(42); got != 0 {
			t.Fatalf("class %q must release the slot, count=%d", class, got)
		}
		return called, reportedSuccess
	}

	for _, class := range []ErrClass{ClassUserError, ClassAuthMismatch, ClassBalance} {
		if called, _ := report(class, false); called {
			t.Fatalf("class %q must not reopen a half-open probe", class)
		}
	}
	if called, ok := report(ClassServer, false); !called || ok {
		t.Fatal("connectivity failure must still report a probe failure")
	}
	if called, ok := report(ClassOK, true); !called || !ok {
		t.Fatal("probe success must still be reported")
	}
}

func TestRedisOffHalfOpenSuccessPersists(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prev := GetConfig()
	cfg := prev
	cfg.HalfOpenSuccessNeeded = 5
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })

	store := &model.ChannelBreakerState{ChannelID: 42, Stage: "half_2"}
	prevLoad := loadBreakerStateFor
	prevSave := saveHalfOpenProgress
	loadBreakerStateFor = func(id int) *model.ChannelBreakerState {
		cp := *store
		return &cp
	}
	saved := 0
	saveHalfOpenProgress = func(st *model.ChannelBreakerState) {
		if st == nil {
			return
		}
		saved++
		store.HalfOpenOK = st.HalfOpenOK
		store.Stage = st.Stage
	}
	t.Cleanup(func() {
		loadBreakerStateFor = prevLoad
		saveHalfOpenProgress = prevSave
	})

	OnHalfOpenResult(42, true)
	OnHalfOpenResult(42, true)
	if saved < 2 || store.HalfOpenOK != 2 || store.Stage != "half_2" {
		t.Fatalf("partial success must persist below the ramp threshold, ok=%d stage=%s saved=%d", store.HalfOpenOK, store.Stage, saved)
	}
}
