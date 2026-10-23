package smartrouting

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestLoadBreakerStagesKeepsSnapshotOnErrorAndReplacesOnEmpty(t *testing.T) {
	breakerStageMu.Lock()
	prev := breakerStages
	breakerStages = map[int]string{7: "open"}
	breakerStageMu.Unlock()
	prevQuery := breakerStageQuery
	t.Cleanup(func() {
		breakerStageQuery = prevQuery
		breakerStageMu.Lock()
		breakerStages = prev
		breakerStageMu.Unlock()
	})

	breakerStageQuery = func() ([]model.ChannelBreakerState, error) {
		return nil, errors.New("db down")
	}
	loadBreakerStages()
	if breakerStageOf(7) != "open" {
		t.Fatalf("query error must keep the last stage, got %q", breakerStageOf(7))
	}

	breakerStageQuery = func() ([]model.ChannelBreakerState, error) {
		return nil, nil
	}
	loadBreakerStages()
	if breakerStageOf(7) != "" {
		t.Fatalf("successful empty load must clear stages, got %q", breakerStageOf(7))
	}
}
