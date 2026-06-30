package orchestrator

import (
	"os"
	"path/filepath"
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
		Metadata: map[string]any{
			"evidenceQuality": string(common.EvidenceQualityValid),
			"targetRefs":      []string{"req-001", "req-002", "req-003"},
			"observedClaims":  []string{"all scoped requirements were validated"},
		},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if !changed || bound != 3 {
		t.Fatalf("reconcile changed/bound = %v/%d, want true/3", changed, bound)
	}
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if got := stringFromMap(item, "status"); got != "satisfied" {
			t.Fatalf("%s status = %q, want deterministic satisfied", stringFromMap(item, "id"), got)
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
	if action, ok := deriveMissingAssessmentNextAction(run, mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])); ok || action.Type != "" {
		t.Fatalf("derive assessment after deterministic satisfaction = %+v/%v, want no assessment", action, ok)
	}
	if action, ok := deriveAllSatisfiedCompleteNextAction(run, mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])); !ok ||
		action.Type != runtimemodel.NextActionTypeComplete ||
		action.SubjectID != string(run.ID) {
		t.Fatalf("derive complete after deterministic satisfaction = %+v/%v, want complete", action, ok)
	}
}

func TestAcceptedValidationEvidenceRecordWithoutProofIsSupportingOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-reuse-missing-proof",
		WorkspaceID: "ws-evidence-reuse-missing-proof",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{
				coverageReuseTestItem("cwi-001", "req-001"),
			},
		},
	}
	records := []*runtimemodel.EvidenceRecord{{
		ID:               "evidence-validation-weak",
		WorkspaceID:      run.WorkspaceID,
		RunID:            run.ID,
		StepID:           "step-validation-weak",
		Kind:             runtimemodel.EvidenceKindValidation,
		Status:           runtimemodel.EvidenceStatusAccepted,
		SourceRef:        "step-validation-weak/validation/runtime-step",
		CoverageItemRefs: []string{"cwi-001"},
		Metadata:         map[string]any{"evidenceQuality": string(common.EvidenceQualityValid)},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if !changed || bound != 0 {
		t.Fatalf("reconcile changed/bound = %v/%d, want supporting-only mutation true/0", changed, bound)
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted[runtimemodel.EvidenceKindValidation]) != 0 {
		t.Fatalf("accepted validation = %+v, want none", accepted[runtimemodel.EvidenceKindValidation])
	}
	if supporting := mapStringSliceValue(item["supportingEvidenceByKind"]); len(supporting[runtimemodel.EvidenceKindValidation]) != 2 {
		t.Fatalf("supporting validation = %+v, want source ref and record id", supporting[runtimemodel.EvidenceKindValidation])
	}
}

func TestAcceptedValidationEvidenceRecordTextOnlyTargetRefsAreSupportingOnly(t *testing.T) {
	textTarget := "deliverable must expose runtime health behavior"
	run := &runmodel.Run{
		ID:          "run-evidence-reuse-text-only",
		WorkspaceID: "ws-evidence-reuse-text-only",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-structured",
				"status":                "open",
				"sourceRequirementId":   "req-structured",
				"targetText":            textTarget,
				"targetRefs":            []string{textTarget},
				"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindValidation},
				"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindValidation},
			}},
		},
	}
	records := []*runtimemodel.EvidenceRecord{{
		ID:          "evidence-validation-text-only",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      "step-validation-text-only",
		Kind:        runtimemodel.EvidenceKindValidation,
		Status:      runtimemodel.EvidenceStatusAccepted,
		SourceRef:   "step-validation-text-only/validation/runtime-step",
		Metadata: map[string]any{
			"evidenceQuality": string(common.EvidenceQualityValid),
			"targetRefs":      []string{textTarget},
			"observedClaims":  []string{"health behavior was observed"},
		},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if changed || bound != 0 {
		t.Fatalf("reconcile changed/bound = %v/%d, want false/0 for text-only target refs", changed, bound)
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted[runtimemodel.EvidenceKindValidation]) != 0 {
		t.Fatalf("accepted validation refs = %+v, want none", accepted[runtimemodel.EvidenceKindValidation])
	}
}

func TestRuntimeStepCoverageScopeMetadataSeparatesStructuredTargetRefs(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-1/graph-1/001-validate-target",
		Input: map[string]any{
			"targetRefs": []string{"free text target with spaces", "cwi-001"},
		},
		Output: map[string]any{
			"validationEvidence": map[string]any{
				"targetRefs":     []string{"another text target with spaces", "req-001"},
				"observedClaims": []string{"runtime behavior observed"},
			},
		},
	}

	scope := runtimeStepCoverageScopeMetadata(step)
	if refs := stringSliceFromValue(scope["targetRefs"]); !stringSliceContains(refs, "cwi-001") || !stringSliceContains(refs, "req-001") {
		t.Fatalf("targetRefs = %+v, want structured refs", refs)
	}
	if refs := stringSliceFromValue(scope["targetRefs"]); stringSliceContains(refs, "free text target with spaces") || stringSliceContains(refs, "another text target with spaces") {
		t.Fatalf("targetRefs = %+v, want text refs excluded from binding refs", refs)
	}
	if refs := stringSliceFromValue(scope["structuredTargetRefs"]); !stringSliceContains(refs, "req-001") {
		t.Fatalf("structuredTargetRefs = %+v, want validation evidence structured ref", refs)
	}
	if claims := stringSliceFromValue(scope["observedClaims"]); !stringSliceContains(claims, "runtime behavior observed") {
		t.Fatalf("observedClaims = %+v, want validation evidence claim", claims)
	}
}

func TestRuntimeStepCoverageScopeMetadataFallsBackToCurrentRequiredOutputFocus(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-1/graph-1/001-validate-current-focus",
		Input: map[string]any{
			stepInputKeyStagePacket: map[string]any{
				"requiredOutput": map[string]any{
					"coverageWorkItemIds":                   []string{"cwi-085"},
					"structuredTargetRefs":                  []string{"cwi-085", "deliverable:source"},
					"targetRefs":                            []string{"source_change"},
					"observedClaims":                        []string{"runtime behavior was observed for the declared coverage focus"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
			"coverageFocus": map[string]any{
				"id":                  "cwi-085",
				"coverageWorkItemIds": []string{"cwi-085"},
				"structuredTargetRefs": []string{
					"cwi-085",
					"deliverable:source",
				},
			},
		},
		Output: map[string]any{
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
		},
	}

	scope := runtimeStepCoverageScopeMetadata(step)
	if ids := stringSliceFromValue(scope["coverageWorkItemIds"]); !stringSliceContains(ids, "cwi-085") {
		t.Fatalf("coverageWorkItemIds = %+v, want current focus id", ids)
	}
	if refs := stringSliceFromValue(scope["structuredTargetRefs"]); !stringSliceContains(refs, "deliverable:source") {
		t.Fatalf("structuredTargetRefs = %+v, want requiredOutput/focus refs", refs)
	}
	if claims := stringSliceFromValue(scope["observedClaims"]); !stringSliceContains(claims, "runtime behavior was observed for the declared coverage focus") {
		t.Fatalf("observedClaims = %+v, want requiredOutput observed claim", claims)
	}
}

func TestFinalizeManagedValidationResourcesReleasesActiveScratch(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	scratch := filepath.Join(root, "workorch-validation-scratch", "ws-a", "run-a", "step-a")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	step := &runmodel.Step{
		ID: "step-a",
		Input: map[string]any{
			"validationResources": []map[string]any{{
				"id":     "ws-a:run-a:step-a:scratch_dir",
				"kind":   "scratch_dir",
				"path":   scratch,
				"status": "active",
			}},
			"validationResourceOwnership": map[string]any{
				"persisted": true,
			},
			"managedValidation": map[string]any{
				"runtimeManaged": true,
			},
		},
	}

	if !finalizeManagedValidationResourcesForStep(step, "failed", "contract_violation") {
		t.Fatal("finalizeManagedValidationResourcesForStep returned false")
	}
	resources := mapSliceFromValue(step.Input["validationResources"])
	if len(resources) != 1 || stringFromMap(resources[0], "status") != "failed" || stringFromMap(resources[0], "cleanupStatus") != "removed" {
		t.Fatalf("validationResources = %+v, want failed removed scratch", resources)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch stat err = %v, want removed", err)
	}
	ownership := mapFromValue(step.Input["validationResourceOwnership"])
	if ownership["finalized"] != true || stringFromMap(ownership, "finalizedBy") != "contract_violation" {
		t.Fatalf("ownership = %+v, want finalized metadata", ownership)
	}
}

func TestAcceptedStateChangeEvidenceRecordDoesNotReuseByBroadCoverageTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-reuse-coverage-target",
		WorkspaceID: "ws-evidence-reuse-coverage-target",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-target",
				"status":                "open",
				"sourceRequirementId":   "target-001",
				"coverageTargetToken":   "target-001",
				"normalizedSourceKey":   "source:target-001",
				"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
				"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
			}},
		},
	}
	records := []*runtimemodel.EvidenceRecord{{
		ID:          "evidence-state-change-target-001",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      "step-state-change-001",
		Kind:        runtimemodel.EvidenceKindStateChange,
		Status:      runtimemodel.EvidenceStatusAccepted,
		SourceRef:   "step-state-change-001/output.json",
		Metadata: map[string]any{
			"coverageTargets": []string{"target-001"},
			"stateChanging":   true,
		},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if changed || bound != 0 {
		t.Fatalf("reconcile changed/bound = %v/%d, want false/0", changed, bound)
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if missing := coverageMissingEvidenceKinds(item); len(missing) != 1 || missing[0] != runtimemodel.EvidenceKindStateChange {
		t.Fatalf("missingEvidenceKinds = %+v, want state_change still missing", missing)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if refs := accepted[runtimemodel.EvidenceKindStateChange]; len(refs) != 0 {
		t.Fatalf("accepted state_change refs = %+v, want none", refs)
	}
	if edges := mapSliceFromValue(item["evidenceReuseRefs"]); len(edges) != 0 {
		t.Fatalf("evidenceReuseRefs = %+v, want none", edges)
	}
}

func TestAcceptedStateChangeEvidenceRecordReusesByExplicitSourceRequirement(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-reuse-source-requirement",
		WorkspaceID: "ws-evidence-reuse-source-requirement",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-target",
				"status":                "open",
				"sourceRequirementId":   "target-001",
				"coverageTargetToken":   "target-001",
				"normalizedSourceKey":   "source:target-001",
				"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
				"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
			}},
		},
	}
	records := []*runtimemodel.EvidenceRecord{{
		ID:          "evidence-state-change-target-001",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      "step-state-change-001",
		Kind:        runtimemodel.EvidenceKindStateChange,
		Status:      runtimemodel.EvidenceStatusAccepted,
		SourceRef:   "step-state-change-001/output.json",
		Metadata: map[string]any{
			"sourceRequirementIds": []string{"target-001"},
			"coverageTargets":      []string{"target-001"},
			"stateChanging":        true,
		},
	}}

	changed, bound := newCoverageSubsystem().reconcileAcceptedEvidenceRecords(run, records)
	if !changed || bound != 1 {
		t.Fatalf("reconcile changed/bound = %v/%d, want true/1", changed, bound)
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if missing := coverageMissingEvidenceKinds(item); len(missing) != 0 {
		t.Fatalf("missingEvidenceKinds = %+v, want none", missing)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if refs := accepted[runtimemodel.EvidenceKindStateChange]; !stringSliceContains(refs, "step-state-change-001/output.json") {
		t.Fatalf("accepted state_change refs = %+v, want source ref", refs)
	}
	if edges := mapSliceFromValue(item["evidenceReuseRefs"]); len(edges) != 1 || edges[0]["matchedBy"] != "source_requirement_id" {
		t.Fatalf("evidenceReuseRefs = %+v, want source_requirement_id edge", edges)
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
