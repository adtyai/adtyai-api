package smartrouting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func seedHalfOpenCache(t *testing.T, items []HalfOpenInfo) {
	t.Helper()
	prevRunning := halfOpenLoopRunning.Load()
	halfOpenCacheMu.Lock()
	prevCache, prevAt := halfOpenCache, halfOpenCacheAt
	halfOpenCache = items
	halfOpenCacheAt = time.Now()
	halfOpenCacheMu.Unlock()
	halfOpenLoopRunning.Store(true)
	t.Cleanup(func() {
		halfOpenLoopRunning.Store(prevRunning)
		halfOpenCacheMu.Lock()
		halfOpenCache, halfOpenCacheAt = prevCache, prevAt
		halfOpenCacheMu.Unlock()
	})
}

func TestHalfOpenFromReadySkipsDisabled(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	ResetHalfOpenInflightForTest()
	disabled := &model.Channel{Id: 2, Name: "off", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Group: "g", Models: "m", Priority: common.GetPointer(int64(10))}
	broken := &model.Channel{Id: 4, Name: "probe", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusScoreBroken, Group: "g", Models: "m", Priority: common.GetPointer(int64(10))}
	restore := model.InstallChannelCacheForTest([]*model.Channel{disabled, broken})
	t.Cleanup(restore)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: disabled.Id, Model: "m"}})
	if got := halfOpenFromReady([]*model.Channel{disabled}, "m", "g", model.RouteCaps{}, DefaultConfig()); got != nil {
		t.Fatalf("disabled half-open must not be offered, got id %d status %d", got.Id, got.Status)
	}
	halfOpenCacheMu.Lock()
	halfOpenCache = []HalfOpenInfo{{ChannelID: broken.Id, Model: "m"}}
	halfOpenCacheAt = time.Now()
	halfOpenCacheMu.Unlock()
	got := halfOpenFromReady([]*model.Channel{broken}, "m", "g", model.RouteCaps{}, DefaultConfig())
	if got == nil || got.Id != broken.Id {
		t.Fatalf("score-broken probe should still be offered, got %#v", got)
	}
}

func TestHalfOpenMatchesBandPriorityNotChannelColumn(t *testing.T) {
	probe := &model.Channel{Id: 8, Name: "probe", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "g", Models: "m", Priority: common.GetPointer(int64(1))}
	restore := model.InstallChannelCacheForTest([]*model.Channel{probe})
	t.Cleanup(restore)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: probe.Id, Model: "m"}})
	band := []*model.Channel{{
		Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Group: "g", Models: "m", Priority: common.GetPointer(int64(10)),
	}}
	caps := model.RouteCaps{
		HasBandPriority: true,
		BandPriority:    10,
		PriorityOf: func(channelID int) (int64, bool) {
			if channelID == probe.Id {
				return 10, true
			}
			return 0, false
		},
	}
	got := halfOpenFromReady(band, "m", "g", caps, DefaultConfig())
	if got == nil || got.Id != probe.Id {
		t.Fatalf("probe must follow the ability band key, not channel.priority, got %#v", got)
	}
}

func TestStreamRequestStillOffersHalfOpen(t *testing.T) {
	probe := &model.Channel{Id: 6, Name: "probe", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "g", Models: "m", Priority: common.GetPointer(int64(10))}
	restore := model.InstallChannelCacheForTest([]*model.Channel{probe})
	t.Cleanup(restore)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: probe.Id, Model: "m"}})
	cfg := DefaultConfig()
	if !cfg.HalfOpenPreferNonStream {
		t.Fatal("default must prefer non-stream")
	}
	got := halfOpenFromReady([]*model.Channel{probe}, "m", "g", model.RouteCaps{IsStream: true}, cfg)
	if got == nil || got.Id != probe.Id {
		t.Fatalf("stream request must still be able to probe a half-open channel, got %#v", got)
	}
}

func TestHalfOpenFromReadyHonorsFilters(t *testing.T) {
	priority := int64(10)
	blocked := &model.Channel{
		Id:       2,
		Type:     constant.ChannelTypeMidjourney,
		Models:   "m",
		Group:    "g",
		Priority: &priority,
		Status:   1,
	}
	restore := model.InstallChannelCacheForTest([]*model.Channel{blocked})
	t.Cleanup(restore)
	prevRunning := halfOpenLoopRunning.Load()
	halfOpenCacheMu.Lock()
	prevCache, prevAt := halfOpenCache, halfOpenCacheAt
	halfOpenCache = []HalfOpenInfo{{ChannelID: blocked.Id, Model: "m"}}
	halfOpenCacheAt = time.Now()
	halfOpenCacheMu.Unlock()
	halfOpenLoopRunning.Store(true)
	t.Cleanup(func() {
		halfOpenLoopRunning.Store(prevRunning)
		halfOpenCacheMu.Lock()
		halfOpenCache, halfOpenCacheAt = prevCache, prevAt
		halfOpenCacheMu.Unlock()
	})

	caps := model.RouteCaps{Filters: []dto.ChannelFilter{{Kind: dto.FilterResponsesWebSocket}}}
	if got := halfOpenFromReady([]*model.Channel{blocked}, "m", "g", caps, DefaultConfig()); got != nil {
		t.Fatalf("filtered half-open channel must not be offered, got id %d", got.Id)
	}

	allowed := &model.Channel{
		Id:       8,
		Type:     constant.ChannelTypeOpenAI,
		Models:   "m",
		Group:    "g",
		Priority: &priority,
		Status:   1,
	}
	allowed.SetSetting(relaydto.ChannelSettings{ResponsesWebSocketEnabled: true})
	restoreAllowed := model.InstallChannelCacheForTest([]*model.Channel{allowed})
	t.Cleanup(restoreAllowed)
	halfOpenCacheMu.Lock()
	halfOpenCache = []HalfOpenInfo{{ChannelID: allowed.Id, Model: "m"}}
	halfOpenCacheAt = time.Now()
	halfOpenCacheMu.Unlock()
	got := halfOpenFromReady([]*model.Channel{allowed}, "m", "g", caps, DefaultConfig())
	if got == nil || got.Id != allowed.Id {
		t.Fatalf("matching half-open channel should be offered, got %#v", got)
	}
}

func TestHalfOpenLookupDoesNotQueryWhenCacheOff(t *testing.T) {
	prevCache := common.MemoryCacheEnabled
	prevDB := model.DB
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open("file:halfopen-cache-off?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatal(err)
	}
	ch := &model.Channel{
		Id: 77, Name: "db-only", Type: constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled, Group: "g", Models: "m",
		Priority: common.GetPointer(int64(10)),
	}
	if err := db.Create(ch).Error; err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prevCache
		model.DB = prevDB
	})
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: ch.Id, Model: "m"}})
	band := []*model.Channel{{
		Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Group: "g", Models: "m", Priority: common.GetPointer(int64(10)),
	}}
	if got := halfOpenFromReady(band, "m", "g", model.RouteCaps{}, DefaultConfig()); got != nil {
		t.Fatalf("select path must not load a half-open channel from the database, got id %d", got.Id)
	}
	if got := HalfOpenFallback("m", "g", model.RouteCaps{}, func(*model.Channel) bool { return true }); got != nil {
		t.Fatalf("empty-candidate half-open must not load from the database, got id %d", got.Id)
	}
	if scoreBreakEligible(ch.Id) {
		t.Fatal("breaker eligibility must not read the database when the memory cache is off")
	}
	if TripImmediate(ch.Id, "banned") {
		t.Fatal("immediate trip must not read the database when the memory cache is off")
	}
}
