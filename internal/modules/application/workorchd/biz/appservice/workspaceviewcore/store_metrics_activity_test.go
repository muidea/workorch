package workspaceviewcore

import (
	"testing"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestHeartbeatAgeUsesActivityRatherThanAcceptedProgress(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 20, 0, 0, time.UTC)
	heartbeat := projectioncontract.RunHeartbeat{
		LatestEffectiveProgressAt: now.Add(-15 * time.Minute).Format(time.RFC3339Nano),
		LatestActivityAt:          now.Add(-time.Minute).Format(time.RFC3339Nano),
	}
	if got := runHeartbeatAgeSeconds(heartbeat, now); got != 60 {
		t.Fatalf("heartbeat age = %d, want activity age 60, not proof age 900", got)
	}
}
