package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDeriveCoverageWorkItemState(t *testing.T) {
	tests := []struct {
		name              string
		item              map[string]any
		wantAction        string
		wantActionableGap bool
		wantAssessment    bool
	}{
		{
			name: "open missing validation needs evidence",
			item: map[string]any{
				"id":                    "cwi-open",
				"status":                "open",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "in progress missing state change needs evidence",
			item: map[string]any{
				"id":                    "cwi-state-change",
				"status":                "in_progress",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/validate"},
				},
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "evidence ready with display gap needs assessment",
			item: map[string]any{
				"id":                    "cwi-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceRefs":  []string{"run/graph/patch", "run/graph/validate"},
				"acceptedEvidenceByKind": map[string][]string{
					"state_change": {"run/graph/patch"},
					"validation":   {"run/graph/validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"state_change": {"run/graph/patch"},
					"validation":   {"run/graph/validate"},
				},
				"currentGap": "required evidence kinds present; coverage assessment required",
			},
			wantAction:     coverageWorkItemActionNeedAssessment,
			wantAssessment: true,
		},
		{
			name: "open display gap with complete structured evidence needs assessment",
			item: map[string]any{
				"id":                    "cwi-complete-open",
				"status":                "open",
				"requiredEvidenceKinds": []string{"validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/validate"},
				},
				"currentGap": "required evidence kinds present; coverage assessment required",
			},
			wantAction:     coverageWorkItemActionNeedAssessment,
			wantAssessment: true,
		},
		{
			name: "evidence ready with structured missing evidence still needs evidence",
			item: map[string]any{
				"id":                    "cwi-inconsistent",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{"state_change"},
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "evidence ready without accepted evidence provenance needs evidence",
			item: map[string]any{
				"id":                   "cwi-ready-without-provenance",
				"status":               "evidence_ready",
				"missingEvidenceKinds": []string{},
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "open empty missing without accepted evidence needs evidence",
			item: map[string]any{
				"id":                    "cwi-empty-stale",
				"status":                "open",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"currentGap":            "display-only gap",
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "blocked needs decision",
			item: map[string]any{
				"id":     "cwi-blocked",
				"status": "blocked",
			},
			wantAction: coverageWorkItemActionNeedDecision,
		},
		{
			name: "satisfied has no action",
			item: map[string]any{
				"id":                    "cwi-satisfied",
				"status":                "satisfied",
				"requiredEvidenceKinds": []string{"validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/validate"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/validate"},
			},
			wantAction: coverageWorkItemActionNone,
		},
		{
			name: "stale satisfied without evidence reopens evidence gap",
			item: map[string]any{
				"id":                    "cwi-stale-satisfied",
				"status":                "satisfied",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
		{
			name: "failed item with stale empty missing derives validation gap",
			item: map[string]any{
				"id":                    "cwi-failed-stale-missing",
				"status":                "failed",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"rejectedEvidenceRefs":  []string{"run/graph/validate"},
				"currentGap":            "missing accepted evidence binding for required evidence kind: validation",
			},
			wantAction:        coverageWorkItemActionNeedEvidence,
			wantActionableGap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := deriveCoverageWorkItemState(tt.item)
			if state.NextAction != tt.wantAction {
				t.Fatalf("NextAction = %q, want %q; state=%+v", state.NextAction, tt.wantAction, state)
			}
			if got := state.HasActionableEvidenceGap(); got != tt.wantActionableGap {
				t.Fatalf("HasActionableEvidenceGap = %v, want %v; state=%+v", got, tt.wantActionableGap, state)
			}
			if got := state.NeedsAssessment(); got != tt.wantAssessment {
				t.Fatalf("NeedsAssessment = %v, want %v; state=%+v", got, tt.wantAssessment, state)
			}
			if got := coverageWorkItemHasOpenGap(tt.item); got != tt.wantActionableGap {
				t.Fatalf("coverageWorkItemHasOpenGap = %v, want %v", got, tt.wantActionableGap)
			}
		})
	}
}

func TestCoverageWorkItemRequiresEvidenceKindUsesDerivedActionableGap(t *testing.T) {
	satisfiedStateChange := map[string]any{
		"id":                    "cwi-state-change-ready",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceByKind": map[string][]string{
			"state_change": {"run/graph/patch"},
			"validation":   {"run/graph/validate"},
		},
		"acceptedEvidenceBindingByKind": map[string][]string{
			"state_change": {"run/graph/patch"},
			"validation":   {"run/graph/validate"},
		},
		"acceptedEvidenceRefs": []string{"run/graph/patch", "run/graph/validate"},
	}
	if coverageWorkItemRequiresEvidenceKind(satisfiedStateChange, "state_change") {
		t.Fatalf("coverageWorkItemRequiresEvidenceKind(state_change) = true, want false for evidence-ready item")
	}

	missingStateChange := map[string]any{
		"id":                    "cwi-state-change-missing",
		"status":                "open",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{"state_change"},
		"acceptedEvidenceByKind": map[string][]string{
			"validation": {"run/graph/validate"},
		},
	}
	if !coverageWorkItemRequiresEvidenceKind(missingStateChange, "state_change") {
		t.Fatalf("coverageWorkItemRequiresEvidenceKind(state_change) = false, want true for missing state_change")
	}
}

func TestCoverageWorkItemProjectionUsesDerivedMissingEvidence(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-stale-ready",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{},
		"currentGap":            "required evidence kinds present; coverage assessment required",
	}

	state := deriveCoverageWorkItemState(item)
	if !state.HasActionableEvidenceGap() || !stringSliceContains(state.MissingEvidenceKinds, "validation") {
		t.Fatalf("derived state = %+v, want validation evidence gap", state)
	}
	if states := coverageWorkItemNextStates(item); len(states) != 1 || states[0] != convergenceStateNeedValidation {
		t.Fatalf("coverageWorkItemNextStates = %+v, want [%s]", states, convergenceStateNeedValidation)
	}
	summary := coverageWorkItemSummary(item)
	if missing := stringSliceFromValue(summary["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("summary missingEvidenceKinds = %+v, want validation", missing)
	}
}

func TestDerivedMissingEvidenceDoesNotReopenAcceptedByKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-stale-missing-state-change",
		"status":                "in_progress",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{"state_change", "validation"},
		"acceptedEvidenceByKind": map[string][]string{
			"state_change": {"run/graph/patch"},
		},
		"acceptedEvidenceBindingByKind": map[string][]string{
			"state_change": {"run/graph/patch"},
		},
		"acceptedEvidenceRefs": []string{"run/graph/patch"},
	}

	state := deriveCoverageWorkItemState(item)
	if stringSliceContains(state.MissingEvidenceKinds, "state_change") {
		t.Fatalf("derived missingEvidenceKinds = %+v, should not reopen accepted state_change", state.MissingEvidenceKinds)
	}
	if !stringSliceContains(state.MissingEvidenceKinds, "validation") {
		t.Fatalf("derived missingEvidenceKinds = %+v, want validation still missing", state.MissingEvidenceKinds)
	}
}

func TestDerivedMissingEvidenceRequiresByKindEvidenceForMultipleRequiredKinds(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-flat-refs-multiple-kinds",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceRefs":  []string{"run/graph/ambiguous-evidence"},
		"currentGap":            "required evidence kinds present; coverage assessment required",
	}

	state := deriveCoverageWorkItemState(item)
	if state.NextAction != coverageWorkItemActionNeedEvidence {
		t.Fatalf("NextAction = %q, want %q; state=%+v", state.NextAction, coverageWorkItemActionNeedEvidence, state)
	}
	for _, kind := range []string{"state_change", "validation"} {
		if !stringSliceContains(state.MissingEvidenceKinds, kind) {
			t.Fatalf("derived missingEvidenceKinds = %+v, want %s missing without by-kind evidence", state.MissingEvidenceKinds, kind)
		}
	}
}

func TestCoverageMissingEvidenceKindsDelegatesToDerivedState(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-legacy-missing-wrapper",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceRefs":  []string{"run/graph/ambiguous-evidence"},
		"currentGap":            "required evidence kinds present; coverage assessment required",
	}

	missing := stringSet(coverageMissingEvidenceKinds(item))
	for _, kind := range []string{"state_change", "validation"} {
		if !missing[kind] {
			t.Fatalf("coverageMissingEvidenceKinds = %+v, want %s from derived state", missing, kind)
		}
	}
}

func TestDerivedMissingEvidenceRequiresBindingForSingleRequiredKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-flat-ref-single-kind",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceRefs":  []string{"run/graph/validate"},
	}

	state := deriveCoverageWorkItemState(item)
	if state.NextAction != coverageWorkItemActionNeedEvidence {
		t.Fatalf("NextAction = %q, want %q; state=%+v", state.NextAction, coverageWorkItemActionNeedEvidence, state)
	}
	if !stringSliceContains(state.MissingEvidenceKinds, "validation") {
		t.Fatalf("derived missingEvidenceKinds = %+v, want validation missing without binding", state.MissingEvidenceKinds)
	}
}

func TestCoverageTargetProfileStateChangeAllowsFlatAcceptedRefWhenStateChangeNotMissing(t *testing.T) {
	target := map[string]any{
		"id":                    "cwi-flat-state-change",
		"status":                "failed",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{"validation"},
		"acceptedEvidenceRefs":  []string{"run/graph/patch"},
	}

	if !coverageTargetProfileStateChangeEvidenceSatisfied(target) {
		t.Fatalf("coverageTargetProfileStateChangeEvidenceSatisfied = false, want true from explicit non-missing state_change with accepted ref")
	}
}

func TestCurrentFailureFromCoverageWorkItemContextUsesDerivedMissingEvidence(t *testing.T) {
	context := map[string]any{
		"currentFocus": map[string]any{
			"id":                    "cwi-stale-ready",
			"status":                "evidence_ready",
			"requiredEvidenceKinds": []string{"validation"},
			"missingEvidenceKinds":  []string{},
			"currentGap":            "required evidence kinds present; coverage assessment required",
		},
	}

	failure := currentFailureFromCoverageWorkItemContext(context)
	if len(failure) == 0 {
		t.Fatalf("currentFailureFromCoverageWorkItemContext returned nil, want actionable gap")
	}
	if missing := stringSliceFromValue(failure["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("failure missingEvidenceKinds = %+v, want validation", missing)
	}
}

func TestGapTodoItemsUseDerivedMissingEvidence(t *testing.T) {
	items := []map[string]any{
		{
			"id":                    "cwi-stale-open",
			"status":                "open",
			"requiredEvidenceKinds": []string{"validation"},
			"missingEvidenceKinds":  []string{},
			"currentGap":            "stale display gap",
		},
		{
			"id":                    "cwi-ready",
			"status":                "evidence_ready",
			"requiredEvidenceKinds": []string{"validation"},
			"missingEvidenceKinds":  []string{},
			"acceptedEvidenceRefs":  []string{"run/graph/validate"},
			"acceptedEvidenceByKind": map[string][]string{
				"validation": {"run/graph/validate"},
			},
			"acceptedEvidenceBindingByKind": map[string][]string{
				"validation": {"run/graph/validate"},
			},
			"currentGap": "required evidence kinds present; coverage assessment required",
		},
	}

	todos := gapTodoItemsFromCoverageContext(items)
	if len(todos) != 2 {
		t.Fatalf("todoItems = %+v, want two items", todos)
	}
	if missing := stringSliceFromValue(todos[0]["requiredEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("stale-open todo requiredEvidenceKinds = %+v, want validation", missing)
	}
	if missing := stringSliceFromValue(todos[1]["requiredEvidenceKinds"]); len(missing) != 0 {
		t.Fatalf("evidence-ready todo requiredEvidenceKinds = %+v, want empty assessment-ready todo", missing)
	}
}

func TestCoverageWorkItemsHaveAcceptedPrimaryDeliverableEvidenceUsesDerivedState(t *testing.T) {
	run := &runmodel.Run{
		ID:          common.RunID("run-primary-deliverable-derived"),
		WorkspaceID: common.WorkspaceID("ws-primary-deliverable-derived"),
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "deliverable-001",
				"sourceRequirementId":   "deliverable-001",
				"evidenceRole":          "primary_state_change",
				"status":                "open",
				"requiredEvidenceKinds": []string{"state_change"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceRefs":  []string{"run/graph/patch"},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"state_change": {"run/graph/patch"},
				},
			}},
		},
	}

	if !coverageWorkItemsHaveAcceptedPrimaryDeliverableEvidence(run, "deliverable-001") {
		t.Fatalf("coverageWorkItemsHaveAcceptedPrimaryDeliverableEvidence = false, want true from accepted binding refs")
	}
}

func TestCoverageTargetPredicatesUseDerivedState(t *testing.T) {
	staleReady := map[string]any{
		"id":                    "cwi-target-stale-ready",
		"status":                "evidence_ready",
		"state":                 "evidence_ready",
		"requiredEvidenceKinds": []string{"state_change"},
		"missingEvidenceKinds":  []string{},
	}
	if !coverageTargetSetHasMissingEvidence([]map[string]any{staleReady}) {
		t.Fatalf("coverageTargetSetHasMissingEvidence = false, want true when evidence_ready lacks accepted provenance")
	}
	if coverageTargetProfileStateChangeEvidenceSatisfied(staleReady) {
		t.Fatalf("coverageTargetProfileStateChangeEvidenceSatisfied = true, want false without accepted state_change evidence")
	}
	if coverageTargetClosedForPrimaryStateChange(staleReady) {
		t.Fatalf("coverageTargetClosedForPrimaryStateChange = true, want false without accepted state_change evidence")
	}

	ready := cloneStepPayload(staleReady)
	ready["acceptedEvidenceRefs"] = []string{"run/graph/patch"}
	ready["acceptedEvidenceByKind"] = map[string][]string{"state_change": {"run/graph/patch"}}
	ready["acceptedEvidenceBindingByKind"] = map[string][]string{"state_change": {"run/graph/patch"}}
	if coverageTargetSetHasMissingEvidence([]map[string]any{ready}) {
		t.Fatalf("coverageTargetSetHasMissingEvidence = true, want false for accepted state_change evidence")
	}
	if !coverageTargetProfileStateChangeEvidenceSatisfied(ready) {
		t.Fatalf("coverageTargetProfileStateChangeEvidenceSatisfied = false, want true for accepted state_change evidence")
	}
	if !coverageTargetClosedForPrimaryStateChange(ready) {
		t.Fatalf("coverageTargetClosedForPrimaryStateChange = false, want true for evidence-ready accepted state_change evidence")
	}
}
