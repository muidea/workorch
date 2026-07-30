package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapRuntimeV2AuditSeparatesActiveAndRetiredPrerequisiteLifecycle(t *testing.T) {
	run := readmodelcore.Run{RuntimeState: &runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus: runtimecontract.CoverageFocus{ID: "focus-current"},
		Prerequisites: runtimecontract.PrerequisiteState{
			Pending: []runtimecontract.RuntimePrerequisite{{
				ID: "active-selection", Kind: runtimecontract.PrerequisiteKindTargetSelection,
				Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-current", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			}},
			Retired: []runtimecontract.RuntimePrerequisite{{
				ID: "retired-selection", Kind: runtimecontract.PrerequisiteKindTargetSelection,
				Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-prior", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			}},
		},
	}}}

	audit := MapRuntimeV2Audit(run)
	if audit == nil || len(audit.Grounding.ActivePrerequisites) != 1 || len(audit.Grounding.RetiredPrerequisites) != 1 {
		t.Fatalf("prerequisite lifecycle audit = %+v", audit)
	}
	if active := audit.Grounding.ActivePrerequisites[0]; active.ID != "active-selection" || active.Scope != string(runtimecontract.RuntimePrerequisiteScopeFocus) || active.FocusID != "focus-current" {
		t.Fatalf("active prerequisite audit = %+v", active)
	}
	if retired := audit.Grounding.RetiredPrerequisites[0]; retired.ID != "retired-selection" || retired.FocusID != "focus-prior" {
		t.Fatalf("retired prerequisite audit = %+v", retired)
	}
}

func TestMapRuntimeV2AuditPreservesFailureOriginalDiagnosticWithoutAction(t *testing.T) {
	run := readmodelcore.Run{RuntimeState: &runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-audit", RunID: "run-audit", DefinitionRevision: "definition-audit", EvidenceRevision: "evidence-audit",
		Focus: runtimecontract.CoverageFocus{ID: "focus-current"},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "definition_mismatch",
			Signature: "runtime-state-update/ws-audit/run-audit/4/4/semantic_rejection", Message: "definition revision is stale",
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaV1, OwnerModuleID: "blocks/run",
				OriginalClass: "semantic_rejection", OriginalCode: "definition_mismatch", OriginalMessage: "definition revision is stale",
				Signature: "runtime-state-update/ws-audit/run-audit/4/4/semantic_rejection", ExpectedRevision: "4", ObservedRevision: "4",
			},
		},
	}}}

	audit := MapRuntimeV2Audit(run)
	if audit == nil || audit.Failure == nil || audit.Failure.Code != "definition_mismatch" ||
		audit.Failure.OriginalDiagnostic == nil || audit.Failure.OriginalDiagnostic.OwnerModuleID != "blocks/run" ||
		audit.Failure.OriginalDiagnostic.ObservedRevision != "4" {
		t.Fatalf("failure audit lost original diagnostic: %+v", audit)
	}
}
