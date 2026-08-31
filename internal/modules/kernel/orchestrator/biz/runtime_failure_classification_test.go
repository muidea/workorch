package orchestrator

import (
	"context"
	"errors"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestClassifyRuntimeDriveFailureKeepsUnsupportedCapabilityOutOfGovernance(t *testing.T) {
	failure := classifyRuntimeDriveFailure(&runtimecontract.RuntimeCapabilityUnsupportedError{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindPatchRevert}, Reason: "unsupported_kind",
	})
	if failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment || failure.Code != "runtime_capability_unsupported" ||
		runtime.FailureClassDisposition(failure.Class) != runtime.FailureDispositionTerminalDefect {
		t.Fatalf("unsupported capability classification = %+v", failure)
	}
}

func TestClassifyRuntimeDriveFailureKeepsCapabilityCatalogClassificationOwnerNeutral(t *testing.T) {
	failure := classifyRuntimeDriveFailure(&runtimecontract.RuntimeCapabilityCatalogUnavailableError{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		SubjectRef:    "server-catalog",
		Reason:        "owner_unavailable",
	})
	if failure.Class != runtimecontract.FailureClassCapabilityUnavailable || failure.Code != "runtime_capability_catalog_unavailable" ||
		failure.Signature != "runtime-capability-catalog-unavailable/blocks/mcp/server-catalog" {
		t.Fatalf("capability catalog classification = %+v", failure)
	}
}

func TestClassifyRuntimeDriveFailureUsesTypedRecoveryOutcomes(t *testing.T) {
	cas := classifyRuntimeDriveFailure(&runtime.RuntimeCASConflictError{WorkspaceID: "ws-v2", RunID: "run-v2", ExpectedRevision: 4, ObservedRevision: 5})
	if cas.Class != runtimecontract.FailureClassCASConflict {
		t.Fatalf("typed CAS classification = %+v", cas)
	}
	semanticConflict := classifyRuntimeDriveFailure(common.ErrConflict)
	if semanticConflict.Class != runtimecontract.FailureClassRuntimeContractMisalignment || semanticConflict.Code != "runtime_semantic_conflict" {
		t.Fatalf("untyped conflict must not enter CAS recovery: %+v", semanticConflict)
	}
	semantic := classifyRuntimeDriveFailure(&runtime.RuntimeSemanticRejectionError{
		WorkspaceID: "ws-v2", RunID: "run-v2", ExpectedRevision: 4, ObservedRevision: 4,
		Feedback: runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainPersistence, StableCategory: runtimecontract.RuntimeFeedbackCategorySemanticRejection,
			WorkspaceID: "ws-v2", RunID: "run-v2", OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/run",
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
	unknown := classifyRuntimeDriveFailure(&runtime.OperationOutcomeUnknownError{OperationID: "write-1", Reason: "receipt missing"})
	if unknown.Class != runtimecontract.FailureClassOperationOutcomeUnknown {
		t.Fatalf("typed outcome-unknown classification = %+v", unknown)
	}
	transient := classifyRuntimeDriveFailure(context.DeadlineExceeded)
	if transient.Class != runtimecontract.FailureClassOwnerTransientUnavailable {
		t.Fatalf("typed timeout classification = %+v", transient)
	}
	textOnly := classifyRuntimeDriveFailure(errors.New("owner timeout outcome_unknown projection"))
	if textOnly.Class != runtimecontract.FailureClassRuntimeContractMisalignment {
		t.Fatalf("error text changed classification = %+v", textOnly)
	}
}
