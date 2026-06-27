package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcceptedEvidenceRecordReusesAcrossMatchingCoverageItems(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-reuse",
		WorkspaceID: "ws-evidence-reuse",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{
				coverageReuseTestItem("cwi-001", "req-001"),
				coverageReuseTestItem("cwi-002", "req-002"),
				coverageReuseTestItem("cwi-003", "req-003"),
			},
		},
	}
	records := []*runtimemodel.EvidenceRecord{{
		ID:               "evidence-validation-001",
		WorkspaceID:      run.WorkspaceID,
		RunID:            run.ID,
		StepID:           "step-validation-001",
		Kind:             runtimemodel.EvidenceKindValidation,
		Status:           runtimemodel.EvidenceStatusAccepted,
		SourceRef:        "step-validation-001/validation/runtime-step",
		CoverageItemRefs: []string{"cwi-001", "cwi-002", "cwi-003"},
		Metadata:         map[string]any{"evidenceQuality": string(common.EvidenceQualityValid)},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if !changed || bound != 3 {
		t.Fatalf("reconcile changed/bound = %v/%d, want true/3", changed, bound)
	}
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if got := stringFromMap(item, "status"); got != "evidence_ready" {
			t.Fatalf("%s status = %q, want evidence_ready", stringFromMap(item, "id"), got)
		}
		accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
		if refs := accepted[runtimemodel.EvidenceKindValidation]; !stringSliceContains(refs, "step-validation-001/validation/runtime-step") {
			t.Fatalf("%s accepted validation refs = %+v, want reused source ref", stringFromMap(item, "id"), refs)
		}
		if edges := mapSliceFromValue(item["evidenceReuseRefs"]); len(edges) != 1 || edges[0]["matchedBy"] != "coverage_item_ref" {
			t.Fatalf("%s evidenceReuseRefs = %+v, want one coverage_item_ref edge", stringFromMap(item, "id"), edges)
		}
	}
	if action, ok := deriveMissingValidationNextAction(run, mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])); ok || action.Type != "" {
		t.Fatalf("derive missing validation after reuse = %+v/%v, want no validation action", action, ok)
	}
	if action, ok := deriveMissingAssessmentNextAction(run, mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])); !ok ||
		action.Type != runtimemodel.NextActionTypeAssessment ||
		action.SubjectID != "cwi-001" ||
		action.RequiredOutput["mustAssessAcceptedEvidence"] != true {
		t.Fatalf("derive assessment after reuse = %+v/%v, want assessment for first evidence-ready item", action, ok)
	}
}

func TestAcceptedEvidenceRecordReuseSkipsNonMatchingRejectedAndSupersededRecords(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-reuse-skip",
		WorkspaceID: "ws-evidence-reuse-skip",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{coverageReuseTestItem("cwi-001", "req-001")},
		},
	}
	records := []*runtimemodel.EvidenceRecord{
		{
			ID:               "evidence-nonmatching",
			WorkspaceID:      run.WorkspaceID,
			RunID:            run.ID,
			Kind:             runtimemodel.EvidenceKindValidation,
			Status:           runtimemodel.EvidenceStatusAccepted,
			SourceRef:        "validation/nonmatching",
			CoverageItemRefs: []string{"cwi-other"},
		},
		{
			ID:               "evidence-rejected",
			WorkspaceID:      run.WorkspaceID,
			RunID:            run.ID,
			Kind:             runtimemodel.EvidenceKindValidation,
			Status:           runtimemodel.EvidenceStatusRejected,
			SourceRef:        "validation/rejected",
			CoverageItemRefs: []string{"cwi-001"},
		},
		{
			ID:               "evidence-superseded",
			WorkspaceID:      run.WorkspaceID,
			RunID:            run.ID,
			Kind:             runtimemodel.EvidenceKindValidation,
			Status:           runtimemodel.EvidenceStatusAccepted,
			SourceRef:        "validation/superseded",
			CoverageItemRefs: []string{"cwi-001"},
		},
		{
			ID:          "evidence-current",
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Kind:        runtimemodel.EvidenceKindDiagnostic,
			Status:      runtimemodel.EvidenceStatusAccepted,
			SourceRef:   "diagnostic/current",
			Supersedes:  []common.EvidenceRecordID{"evidence-superseded"},
		},
	}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if changed || bound != 0 {
		t.Fatalf("reconcile changed/bound = %v/%d, want false/0", changed, bound)
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if refs := stringSliceFromValue(item["acceptedEvidenceRefs"]); len(refs) > 0 {
		t.Fatalf("acceptedEvidenceRefs = %+v, want empty", refs)
	}
}

func TestValidationNextActionRoutesFromPolicyWithoutBusinessSpecialCases(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-validation-route",
		WorkspaceID: "ws-validation-route",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"validationPolicies": []map[string]any{{
				"id":                     "policy-managed",
				"appliesToEvidenceKinds": []string{runtimemodel.EvidenceKindValidation},
				"sourceRequirementIds":   []string{"req-managed"},
				"allowedExecutionModes":  []string{"managed_harness"},
				"managedHarnessRef":      "managed-harness-ref",
			}, {
				"id":                     "policy-deferred",
				"appliesToEvidenceKinds": []string{runtimemodel.EvidenceKindValidation},
				"sourceRequirementIds":   []string{"req-deferred"},
				"validationSafetyClass":  "ci_only",
				"allowedExecutionModes":  []string{"deferred"},
				"deferredEvidenceKind":   "external_validation",
			}},
		},
	}

	managed, ok := deriveMissingValidationNextAction(run, []map[string]any{coverageReuseTestItem("cwi-managed", "req-managed")})
	if !ok || managed.Type != runtimemodel.NextActionTypeValidate ||
		managed.RequiredOutput["validationRoute"] != runtimemodel.ValidationRouteManagedHarness ||
		managed.StageMode != "managed_harness" ||
		managed.RequiredOutput["managedHarnessRef"] != "managed-harness-ref" {
		t.Fatalf("managed validation action = %+v, want managed harness route", managed)
	}

	deferred, ok := deriveMissingValidationNextAction(run, []map[string]any{coverageReuseTestItem("cwi-deferred", "req-deferred")})
	if !ok || deferred.Type != runtimemodel.NextActionTypeValidate ||
		deferred.RequiredOutput["validationRoute"] != runtimemodel.ValidationRouteDeferred ||
		deferred.StageMode != "deferred" ||
		deferred.RequiredOutput["deferredEvidenceKind"] != "external_validation" {
		t.Fatalf("deferred validation action = %+v, want deferred route", deferred)
	}

	defaultAction, ok := deriveMissingValidationNextAction(run, []map[string]any{coverageReuseTestItem("cwi-default", "req-default")})
	if !ok || defaultAction.RequiredOutput["validationRoute"] != runtimemodel.ValidationRouteLLMGraph {
		t.Fatalf("default validation action = %+v, want llm graph route", defaultAction)
	}
}

func TestGapBaselineUsesFocusAndLedgerRefsInsteadOfFullOpenLedger(t *testing.T) {
	unresolved := []map[string]any{
		coverageReuseTestItem("cwi-001", "req-001"),
		coverageReuseTestItem("cwi-002", "req-002"),
		coverageReuseTestItem("cwi-003", "req-003"),
		coverageReuseTestItem("cwi-004", "req-004"),
	}
	run := &runmodel.Run{
		ID:          "run-gap-baseline-narrow",
		WorkspaceID: "ws-gap-baseline-narrow",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-narrow",
				"coverageBaselineHash": "sha256:narrow",
				"currentFocus":         unresolved[0],
				"currentFocusItems":    []map[string]any{unresolved[0]},
				"unresolvedItems":      unresolved,
				"fullLedgerRef":        "run.executionContext.coverageWorkItems",
			},
		},
	}

	baseline := gapBaselineFromRun(run)
	if got := len(mapSliceFromValue(baseline["openCoverageWorkItems"])); got != 1 {
		t.Fatalf("openCoverageWorkItems length = %d, want current focus only", got)
	}
	if baseline["openCoverageWorkItemCount"] != 4 || baseline["openCoverageItemsTruncated"] != true {
		t.Fatalf("baseline open count/truncated = %+v/%+v, want 4/true", baseline["openCoverageWorkItemCount"], baseline["openCoverageItemsTruncated"])
	}
	if baseline["ledgerRef"] != "run.executionContext.coverageWorkItems" || baseline["ledgerSummaryRef"] == "" {
		t.Fatalf("baseline refs = ledger:%+v summary:%+v, want refs", baseline["ledgerRef"], baseline["ledgerSummaryRef"])
	}
	if todos := mapSliceFromValue(baseline["todoItems"]); len(todos) != 1 {
		t.Fatalf("todoItems = %+v, want focus todo only", todos)
	}
}

func coverageReuseTestItem(id string, sourceRequirementID string) map[string]any {
	return map[string]any{
		"id":                    id,
		"status":                "open",
		"sourceRequirementId":   sourceRequirementID,
		"normalizedSourceKey":   "source:" + sourceRequirementID,
		"sourcePath":            "requirements." + sourceRequirementID,
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindValidation},
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindValidation},
	}
}
