package orchestrator

import (
	"context"
	"errors"
	"testing"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
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

func TestClassifyRuntimeV2DriveFailureUsesTypedRecoveryOutcomes(t *testing.T) {
	unknown := classifyRuntimeV2DriveFailure(&runtimev2.OperationOutcomeUnknownError{OperationID: "write-1", Reason: "receipt missing"})
	if unknown.Class != runtimecontract.FailureClassOperationOutcomeUnknown {
		t.Fatalf("typed outcome-unknown classification = %+v", unknown)
	}
	transient := classifyRuntimeV2DriveFailure(context.DeadlineExceeded)
	if transient.Class != runtimecontract.FailureClassOwnerTransientUnavailable {
		t.Fatalf("typed timeout classification = %+v", transient)
	}
	textOnly := classifyRuntimeV2DriveFailure(errors.New("owner timeout outcome_unknown projection"))
	if textOnly.Class != runtimecontract.FailureClassRuntimeContractMisalignment {
		t.Fatalf("error text changed classification = %+v", textOnly)
	}
}
