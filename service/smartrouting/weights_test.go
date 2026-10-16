package smartrouting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestSmartWeightsDisabledFallsBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	SetConfig(cfg)
	w := uint(10)
	p := int64(1)
	ch := &model.Channel{Id: 1, Name: "a", Weight: &w, Priority: &p}
	out, weights := SmartWeights([]*model.Channel{ch}, "gpt-test", "default")
	if len(out) != 1 || weights[0] != 10 {
		t.Fatalf("got out=%v weights=%v", out, weights)
	}
}

func TestSmartWeightsEmpty(t *testing.T) {
	out, w := SmartWeights(nil, "m", "g")
	if out != nil || w != nil {
		t.Fatalf("expected nil")
	}
}

func TestChannelStatusConstant(t *testing.T) {
	if common.ChannelStatusScoreBroken != 4 {
		t.Fatalf("status 4 = %d", common.ChannelStatusScoreBroken)
	}
}
