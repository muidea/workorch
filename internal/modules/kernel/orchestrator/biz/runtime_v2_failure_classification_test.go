package orchestrator

import (
	"context"
	"errors"
	"testing"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestClassifyRuntimeV2DriveFailureKeepsUnsupportedCapabilityOutOfGovernance(t *testing.T) {
	failure := classifyRuntimeV2DriveFailure(&orchestratorevents.RuntimeCapabilityUnsupportedError{
		ModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindPatchRevert}, Reason: "unsupported_kind",
	})
	if failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment || failure.Code != "runtime_capability_unsupported" ||
		runtimev2.FailureClassDisposition(failure.Class) != runtimev2.FailureDispositionTerminalDefect {
		t.Fatalf("unsupported capability classification = %+v", failure)
	}
}

func TestClassifyRuntimeV2DriveFailureUsesTypedRecoveryOutcomes(t *testing.T) {
	cas := classifyRuntimeV2DriveFailure(&runtimev2.RuntimeCASConflictError{WorkspaceID: "ws-v2", RunID: "run-v2", ExpectedRevision: 4, ObservedRevision: 5})
	if cas.Class != runtimecontract.FailureClassCASConflict {
		t.Fatalf("typed CAS classification = %+v", cas)
	}
	semanticConflict := classifyRuntimeV2DriveFailure(common.ErrConflict)
	if semanticConflict.Class != runtimecontract.FailureClassRuntimeContractMisalignment || semanticConflict.Code != "runtime_semantic_conflict" {
		t.Fatalf("untyped conflict must not enter CAS recovery: %+v", semanticConflict)
	}
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
