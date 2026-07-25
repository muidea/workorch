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
