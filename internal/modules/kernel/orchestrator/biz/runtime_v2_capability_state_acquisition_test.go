package orchestrator

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2CapabilityStateAcquisitionGenerationTracksFreshnessOnly(t *testing.T) {
	base := runtimecontract.RuntimeRecoveryLedger{}
	baseGeneration := runtimeV2CapabilityStateAcquisitionGeneration(base)
	baseKey := stableRuntimeV2CapabilityStateAcquisitionKey("ws-v2", "run-v2", "repository-state", baseGeneration)
	if baseGeneration != 0 || baseKey != "capability-state/ws-v2/run-v2/repository-state/0" {
		t.Fatalf("base generation=%d key=%q", baseGeneration, baseKey)
	}

	stale := base
	stale.PrerequisiteRefreshes = 1
	if got := runtimeV2CapabilityStateAcquisitionGeneration(stale); got != baseGeneration+1 {
		t.Fatalf("stale generation=%d, want %d", got, baseGeneration+1)
	}

	resourceChange := base
	resourceChange.PrerequisiteResourceChangeRefreshes = 1
	if got := runtimeV2CapabilityStateAcquisitionGeneration(resourceChange); got != baseGeneration+1 {
		t.Fatalf("resource-change generation=%d, want %d", got, baseGeneration+1)
	}

	both := stale
	both.PrerequisiteResourceChangeRefreshes = 1
	if got := runtimeV2CapabilityStateAcquisitionGeneration(both); got != 2 {
		t.Fatalf("combined generation=%d, want 2", got)
	}

	unrelated := base
	unrelated.ProviderTransportAttempts = 3
	unrelated.OperationRecoveryAttempts = 2
	unrelated.CASReloadAttempts = 4
	unrelated.TerminalDeliveryAttempts = 1
	if got := runtimeV2CapabilityStateAcquisitionGeneration(unrelated); got != baseGeneration {
		t.Fatalf("unrelated recovery changed generation to %d", got)
	}

	legacy := runtimecontract.RuntimeRecoveryLedger{CapabilityStateRefreshes: 2}
	if got := runtimeV2CapabilityStateAcquisitionGeneration(legacy); got != 2 {
		t.Fatalf("legacy normalized generation=%d, want 2", got)
	}
}
