package readmodelprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProgressKeepsWholeRunValidationWhenFocusAdvances(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{ID: "focus-b", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{
				{ID: "implementation", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}},
				{ID: "proved-a", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}},
				{ID: "uncredited-aux", Kind: runtimecontract.EvidenceKindValidation},
			},
			Rejected: []runtimecontract.EvidenceFact{{ID: "failed-a", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}}},
		},
	}}
	before, _ := json.Marshal(state)
	progress := mapRuntimeProgress(state)
	want := &projectioncontract.RuntimeEvidenceCounts{Accepted: 3, AcceptedValidation: 2, Rejected: 1, RejectedValidation: 1}
	if !reflect.DeepEqual(progress.RunEvidence, want) || progress.AcceptedValidationEvidenceCount != 0 {
		t.Fatalf("focus/Run counters disagree with their scope: %+v", progress)
	}
	text := runtimeProgressSummary(&progress)
	for _, fragment := range []string{"whole Run:", "validation accepted 2 rejected 1", "current focus:", "CWI-credited validation 0"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("summary omitted %q: %s", fragment, text)
		}
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("read model mutated Runtime authority")
	}
	clone := projectioncontract.CloneRuntimeProgress(&progress)
	clone.RunEvidence.AcceptedValidation = 100
	if progress.RunEvidence.AcceptedValidation != 2 {
		t.Fatal("run totals alias their clone")
	}
	legacy := projectioncontract.RuntimeProgress{AcceptedValidationEvidenceCount: 2}
	if text := runtimeProgressSummary(&legacy); strings.Contains(text, "validation accepted 0 rejected 0") {
		t.Fatal("legacy missing totals became zero progress")
	}
}

func TestTerminalReadinessCauseProjectsWithoutFailureFactOrAuthorityMutation(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OriginalClass: "validation_readiness", OriginalCode: "native_check_missing", OriginalMessage: "missing exact native check", Signature: "readiness/check", TargetRefs: []string{"checks/cwi.test"}}
	state := runtimecontract.RuntimeStateRecord{Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed,
		Terminal: &runtimecontract.RuntimeTerminalDecision{Code: "owner_fact_convergence_exhausted", OriginalDiagnostic: diagnostic}}}
	progress := mapRuntimeProgress(state)
	audit := mapRuntimeAudit(&state)
	for _, got := range []*runtimecontract.RuntimeOriginalDiagnostic{progress.OriginalDiagnostic, audit.Terminal.OriginalDiagnostic} {
		if !reflect.DeepEqual(got, diagnostic) || got == diagnostic {
			t.Fatal("terminal projection lost/aliased cause")
		}
	}
	if progress.TerminalCode != "owner_fact_convergence_exhausted" || state.Facts.Failure != nil || audit.Failure != nil {
		t.Fatal("projection rebuilt a FailureFact")
	}
	clone := projectioncontract.CloneRuntimeAudit(audit)
	clone.Terminal.OriginalDiagnostic.TargetRefs[0] = "other"
	if audit.Terminal.OriginalDiagnostic.TargetRefs[0] != "checks/cwi.test" {
		t.Fatal("terminal audit clone aliased cause")
	}
}
