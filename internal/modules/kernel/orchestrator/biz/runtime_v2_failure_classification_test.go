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
	semantic := classifyRuntimeV2DriveFailure(&runtimev2.RuntimeSemanticRejectionError{
		WorkspaceID: "ws-v2", RunID: "run-v2", ExpectedRevision: 4, ObservedRevision: 4,
		Feedback: runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainPersistence, StableCategory: runtimecontract.RuntimeFeedbackCategorySemanticRejection,
			WorkspaceID: "ws-v2", RunID: "run-v2", OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaV1, OwnerModuleID: "blocks/run",
				OriginalClass: "semantic_rejection", OriginalCode: "definition_mismatch", OriginalMessage: "definition revision is stale",
				Signature: "runtime-state-update/ws-v2/run-v2/4/4/semantic_rejection", ExpectedRevision: "4", ObservedRevision: "4",
			},
		},
	})
	if semantic.Class != runtimecontract.FailureClassRuntimeContractMisalignment || semantic.Code != "definition_mismatch" ||
		semantic.OriginalDiagnostic == nil || semantic.OriginalDiagnostic.OriginalCode != "definition_mismatch" ||
		semantic.OriginalDiagnostic.ObservedRevision != "4" {
		t.Fatalf("typed semantic rejection lost original diagnostic: %+v", semantic)
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
