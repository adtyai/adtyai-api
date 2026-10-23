package smartrouting

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRouteWeightFromScore(t *testing.T) {
	if got := RouteWeightFromScore(0, "normal"); got != 1 {
		t.Fatalf("non-positive score must stay routable, got %d", got)
	}
	if got := RouteWeightFromScore(70, "normal"); got != 100 {
		t.Fatalf("70 => 100, got %d", got)
	}
	rw100 := RouteWeightFromScore(100, "normal")
	if rw100 < 200 || rw100 > 210 {
		t.Fatalf("100 => ~204, got %d", rw100)
	}
	deg := RouteWeightFromScore(70, "degraded")
	if deg != 50 {
		t.Fatalf("degraded 70 => 50, got %d", deg)
	}
}

func TestApplySampleCap(t *testing.T) {
	ok, fail := 100.0, 0.0
	tok := map[int]float64{1: 100, 2: 10}
	// total 110, token1 share ~0.91 > 0.2
	ok2, fail2, capped := ApplySampleCap(ok, fail, map[int]float64{1: 100, 2: 10}, 0.20)
	if !capped {
		t.Fatal("expected capped")
	}
	if ok2 >= ok {
		t.Fatalf("expected ok reduced, got %v vs %v", ok2, ok)
	}
	_ = fail2
	_ = tok
}

func TestComputeStabScoreUnknownAndFormula(t *testing.T) {
	cfg := DefaultConfig()
	score, known := ComputeStabScore(424242, "stab-none", cfg)
	if known || score != 0 {
		t.Fatalf("missing windows must be unknown, score=%v known=%v", score, known)
	}
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	id := 515151
	modelName := "stab-model"
	now := time.Now().UTC()
	localBucketsMu.Lock()
	localBuckets[bucketKeyLocal(id, modelName, minuteFloor(now))] = &MinuteBucket{OK: 5}
	localBuckets[bucketKeyLocal(id, modelName, minuteFloor(now.Add(-10*time.Minute)))] = &MinuteBucket{OK: 1, Fail: 4}
	localBucketsMu.Unlock()
	t.Cleanup(func() {
		localBucketsMu.Lock()
		delete(localBuckets, bucketKeyLocal(id, modelName, minuteFloor(now)))
		delete(localBuckets, bucketKeyLocal(id, modelName, minuteFloor(now.Add(-10*time.Minute))))
		localBucketsMu.Unlock()
	})
	cfg.EWMAShortMin = 5
	cfg.EWMALongMin = 60
	score, known = ComputeStabScore(id, modelName, cfg)
	if !known || math.Abs(score-60) > 0.01 {
		t.Fatalf("expected 60 known, got %v known=%v", score, known)
	}
}

func TestComputeConnScoreInsufficient(t *testing.T) {
	cfg := DefaultConfig()
	s, samples, insuff := ComputeConnScore(999999, "no-such-model", cfg)
	if !insuff || s != 0 {
		t.Fatalf("expected insufficient 0, got score=%v samples=%d insuff=%v", s, samples, insuff)
	}
}

func TestClassify(t *testing.T) {
	// nil => ok
	if Classify(nil) != ClassOK {
		t.Fatal("nil")
	}
}

func TestPickWeighted(t *testing.T) {
	idx := PickWeighted([]int{100, 0, 0})
	if idx != 0 {
		t.Fatalf("expected 0, got %d", idx)
	}
	counts := map[int]int{}
	for i := 0; i < 1000; i++ {
		counts[PickWeighted([]int{50, 50})]++
	}
	if counts[0] < 300 || counts[1] < 300 {
		t.Fatalf("distribution odd: %v", counts)
	}
}

func TestPriceClamp(t *testing.T) {
	s := clamp(100*10/5, 10, 100)
	if math.Abs(s-100) > 1e-9 {
		t.Fatalf("got %v", s)
	}
}

func TestFailWeight(t *testing.T) {
	if FailWeight(ClassUserError) != 0 {
		t.Fatal("user error")
	}
	if FailWeight(ClassRateLimit) != 0.5 {
		t.Fatal("429")
	}
	if FailWeight(ClassServer) != 1 {
		t.Fatal("5xx")
	}
}

func TestDefaultConfigEnabled(t *testing.T) {
	c := DefaultConfig()
	if !c.Enabled {
		t.Fatal("enabled default true")
	}
	if c.Weights.Conn != 30 || c.HalfOpenSuccessNeeded != 5 {
		t.Fatalf("defaults %+v", c)
	}
	if len(c.HalfOpenShareStage) != 3 || c.HalfOpenShareStage[0] != 0.02 {
		t.Fatal("halfopen stages")
	}
}

func TestFloorShareMath(t *testing.T) {
	cfg := DefaultConfig()
	// 12 channels each 3% = 36% > 30% → each 2.5%
	n := 12
	total := cfg.FloorShare * float64(n)
	each := cfg.FloorShare
	if total > cfg.FloorTotalMax {
		each = cfg.FloorTotalMax / float64(n)
	}
	if math.Abs(each-0.025) > 1e-9 {
		t.Fatalf("got %v", each)
	}
	// 40 channels → 0.75%
	n = 40
	total = cfg.FloorShare * float64(n)
	each = cfg.FloorShare
	if total > cfg.FloorTotalMax {
		each = cfg.FloorTotalMax / float64(n)
	}
	if math.Abs(each-0.0075) > 1e-9 {
		t.Fatalf("got %v", each)
	}
}

func TestIsNonTokenPricedModelByName(t *testing.T) {
	if !IsNonTokenPricedModel("dall-e-3", 0) {
		t.Fatal("dall-e")
	}
	if IsNonTokenPricedModel("gpt-4o", 0) {
		t.Fatal("gpt should be token")
	}
}

func TestIsNonTokenPricedModelDoesNotQueryWhenCacheOff(t *testing.T) {
	prevCache := common.MemoryCacheEnabled
	prevDB := model.DB
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open("file:score-cache-off?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatal(err)
	}
	ch := &model.Channel{Id: 88, Name: "db-only", Type: constant.ChannelTypeMidjourney, Status: common.ChannelStatusEnabled}
	if err := db.Create(ch).Error; err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prevCache
		model.DB = prevDB
	})
	if IsNonTokenPricedModel("gpt-4o", ch.Id) {
		t.Fatal("score path must not load channel type from the database")
	}
}
