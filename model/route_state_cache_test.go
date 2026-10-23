package model

import (
	"errors"
	"testing"
)

func TestRefreshRouteStateKeepsLastSnapshotOnQueryError(t *testing.T) {
	ReplaceRouteStateForTest([]ChannelRouteTag{{ChannelID: 7, Model: "m", Tag: "excluded"}}, nil, nil)
	prev := routeStateQuery
	routeStateQuery = func() (*routeStateSnap, error) {
		return nil, errors.New("db down")
	}
	t.Cleanup(func() { routeStateQuery = prev })
	RefreshRouteState()
	if !CachedHasRouteTag(7, "m", "excluded") {
		t.Fatal("a failed reload must keep the last snapshot")
	}
}
