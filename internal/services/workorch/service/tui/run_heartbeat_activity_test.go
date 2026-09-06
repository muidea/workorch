package tui

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestRunHeartbeatActivityRefreshDoesNotRequireEffectiveProgress(t *testing.T) {
	before := projectioncontract.RunHeartbeat{RunID: "run-a", LatestEffectiveProgressAt: "2026-09-06T00:00:00Z", LatestActivityAt: "2026-09-06T00:01:00Z"}
	after := before
	after.LatestActivityAt = "2026-09-06T00:02:00Z"
	if runHeartbeatEqual(before, after) || !runHeartbeatEqual(after, after) {
		t.Fatal("TUI dropped an activity-only display update")
	}
}
