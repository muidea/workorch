package orchestrator

import (
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestClassifyRuntimeV2DriveFailureKeepsUnsupportedCapabilityOutOfGovernance(t *testing.T) {
	failure := classifyRuntimeV2DriveFailure(&orchestratorevents.RuntimeCapabilityUnsupportedError{
		ModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindPatchRevert}, Reason: "unsupported_kind",
	})
	if failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment || failure.Code != "runtime_capability_unsupported" ||
		runtimecontract.FailureClassDisposition(failure.Class) != runtimecontract.RuntimeFailureDispositionTerminalDefect {
		t.Fatalf("unsupported capability classification = %+v", failure)
	}
}
