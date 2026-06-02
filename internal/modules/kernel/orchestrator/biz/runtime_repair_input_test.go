package orchestrator

import (
	"errors"
	"fmt"
	"testing"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeRepairBaseInputProjectsFailedStepContract(t *testing.T) {
	run := &runmodel.Run{ID: "run-repair-base", WorkspaceID: "ws-repair-base"}
	failed := &runmodel.Step{
		ID:    "run-repair-base/graph-001-build",
		RunID: run.ID,
		Kind:  "tool_call",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{"large": "omitted"},
			"target":                            "src/app.go",
		},
		Output: map[string]any{
			"stderr": "compile failed",
		},
	}

	input := runtimeRepairBaseInput(
		runtimeRepairBaseInputDeps{policy: RuntimePolicy{MaxRuntimeRepairAttempts: 7}},
		run,
		failed,
		2,
		errors.New("exit status 1"),
	)

	if input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("runtimeRepairFollowUp = %v, want true", input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input[stepInputKeyRuntimeRepairAttempt] != 2 {
		t.Fatalf("runtimeRepairAttempt = %v, want 2", input[stepInputKeyRuntimeRepairAttempt])
	}
	if input["maxRuntimeRepairAttempts"] != 7 {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want 7", input["maxRuntimeRepairAttempts"])
	}
	if input["failureReason"] != "exit status 1" {
		t.Fatalf("failureReason = %v, want exit status 1", input["failureReason"])
	}
	if input["failedStepId"] != string(failed.ID) || input[stepInputKeyRepairOfStepID] != string(failed.ID) {
		t.Fatalf("failed step lineage not projected: %v", input)
	}
	if input[stepInputKeyRepairOfRootStepID] != string(failed.ID) {
		t.Fatalf("repair root = %v, want failed step id", input[stepInputKeyRepairOfRootStepID])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionRuntimeRepair) {
		t.Fatalf("recovery action = %v, want runtime repair", input[stepInputKeyRecoveryAction])
	}
	failedInput := mapFromValue(input["failedStepInput"])
	if failedInput["target"] != "src/app.go" {
		t.Fatalf("failedStepInput target = %v, want src/app.go", failedInput["target"])
	}
	if _, ok := failedInput[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("failedStepInput retained execution context package: %v", failedInput)
	}
	failedOutput := mapFromValue(input["failedStepOutput"])
	if failedOutput["stderr"] != "compile failed" {
		t.Fatalf("failedStepOutput stderr = %v, want compile failed", failedOutput["stderr"])
	}
}

func TestRuntimeRepairBaseInputWithoutFailedStepUsesDefaults(t *testing.T) {
	input := runtimeRepairBaseInput(runtimeRepairBaseInputDeps{}, &runmodel.Run{ID: "run-repair-base-default"}, nil, 0, nil)

	if input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("runtimeRepairFollowUp = %v, want true", input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input["maxRuntimeRepairAttempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want default %d", input["maxRuntimeRepairAttempts"], defaultMaxRuntimeRepairAttempts)
	}
	if _, ok := input["failedStepId"]; ok {
		t.Fatalf("failedStepId should be absent without failed step: %v", input)
	}
}

func TestApplyRuntimeRepairFailureProjectionAddsDiagnosticsAndCoverageContext(t *testing.T) {
	coverageContext := coverageWorkItemContextFromItems("rev-1", []map[string]any{{
		"id":                    "cwi-001",
		"definitionRevision":    "rev-1",
		"source":                "goal",
		"sourceScope":           "goal_leaf",
		"sourceRequirementId":   "goal-req-001",
		"sourcePath":            "goal.leaves[0]",
		"targetText":            "must produce a bounded state change",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"status":                "open",
	}}, nil, nil)
	input := map[string]any{}
	failed := &runmodel.Step{
		ID:    "run-repair-failure/step-1",
		RunID: "run-repair-failure",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"kind":                     "dod_evidence_quality_gap",
				coverageWorkItemContextKey: coverageContext,
			},
		},
	}

	applyRuntimeRepairFailureProjection(input, failed)

	if stringFromMap(input, "repairObjective") != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want concrete validation evidence", stringFromMap(input, "repairObjective"))
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	if len(mapFromValue(input["verificationEvidenceGap"])) == 0 {
		t.Fatalf("verificationEvidenceGap missing from input: %v", input)
	}
	if context := mapFromValue(input[coverageWorkItemContextKey]); len(mapSliceFromValue(context["unresolvedItems"])) != 1 {
		t.Fatalf("coverageWorkItemContext = %+v, want one unresolved item", input[coverageWorkItemContextKey])
	}
	unresolved := mapSliceFromValue(input["unresolvedCoverageWorkItems"])
	if len(unresolved) != 1 || stringFromMap(unresolved[0], "sourceRequirementId") != "goal-req-001" {
		t.Fatalf("unresolvedCoverageWorkItems = %+v, want goal-req-001", unresolved)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	if len(guidance) == 0 {
		t.Fatalf("repairGuidance missing after projection")
	}
}

func TestApplyRuntimeRepairMissingCoverageProjectionPreservesExistingDeliverableCoverage(t *testing.T) {
	input := map[string]any{
		"missingDeliverableCoverage": []map[string]any{{
			"id": "already-present",
		}},
	}
	failed := &runmodel.Step{
		ID:    "run-repair-coverage/step-1",
		RunID: "run-repair-coverage",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"missingDoDItems": []map[string]any{{
					"id":   "dod-001",
					"item": "must verify the bounded outcome",
				}},
				"missingDeliverableCoverage": []map[string]any{{
					"id": "from-failure",
				}},
			},
		},
	}

	applyRuntimeRepairMissingCoverageProjection(t.Context(), nil, input, &runmodel.Run{ID: "run-repair-coverage"}, failed, false)

	missingDoD := mapSliceFromValue(input["missingDoDItems"])
	if len(missingDoD) != 1 || stringFromMap(missingDoD[0], "id") != "dod-001" {
		t.Fatalf("missingDoDItems = %+v, want dod-001", missingDoD)
	}
	deliverable := mapSliceFromValue(input["missingDeliverableCoverage"])
	if len(deliverable) != 1 || stringFromMap(deliverable[0], "id") != "already-present" {
		t.Fatalf("missingDeliverableCoverage = %+v, want preserved existing item", deliverable)
	}
}

func TestApplyRuntimeRepairAssessmentOnlyCoverageProjectionSetsValidationContract(t *testing.T) {
	input := map[string]any{}
	run := &runmodel.Run{
		ID: "run-repair-assessment-only",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"profile-a": map[string]any{
						"deliverableExpectations": []map[string]any{{
							"id":       "deliverable-001",
							"required": true,
						}},
					},
				},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-repair-assessment-only/step-1",
		RunID: run.ID,
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"missingDeliverableCoverage": []map[string]any{{
					"id": "deliverable-001",
				}},
			},
		},
	}

	applyRuntimeRepairAssessmentOnlyCoverageProjection(t.Context(), nil, RuntimeEvidenceRepositories{}, input, run, failed, true, false, false)

	if input[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("soul stage = %v, want validation", input[stepInputKeySoulStage])
	}
	if runtimeStageFromStepInput(input) != runtimeStageVerification {
		t.Fatalf("runtime stage = %v, want verification", runtimeStageFromStepInput(input))
	}
	if input[stepInputKeyAssessmentOnly] != true || input["completionFollowUp"] != true {
		t.Fatalf("assessment-only flags missing: %v", input)
	}
	if stringFromMap(input, "repairObjective") != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repairObjective = %q, want assessment coverage correction", stringFromMap(input, "repairObjective"))
	}
	if required := stringSliceFromValue(input["requiredDeliverableCoverage"]); len(required) != 1 || required[0] != "deliverable-001" {
		t.Fatalf("requiredDeliverableCoverage = %+v, want deliverable-001", input["requiredDeliverableCoverage"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) == 0 {
		t.Fatalf("repairGuidance missing")
	}
}

func TestApplyRuntimeRepairConcreteEvidenceProjectionClearsStateChangingRequirement(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
	}
	failed := &runmodel.Step{
		ID:    "run-repair-concrete/step-1",
		RunID: "run-repair-concrete",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"claimAlignedEvidenceNeeded": true,
				"missingDoDItems": []map[string]any{{
					"id":   "dod-001",
					"item": "must cite concrete evidence",
				}},
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-001",
					"item":              "must cover goal",
				}},
			},
		},
	}

	applyRuntimeRepairConcreteEvidenceProjection(input, failed, true)

	if stringFromMap(input, "repairObjective") != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want concrete validation evidence", stringFromMap(input, "repairObjective"))
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	if _, ok := input["stateChangingOutputRequired"]; ok {
		t.Fatalf("stateChangingOutputRequired should be cleared: %v", input)
	}
	gap := mapFromValue(input["verificationEvidenceGap"])
	if gap["claimAlignedEvidenceNeeded"] != true {
		t.Fatalf("claimAlignedEvidenceNeeded = %v, want true", gap["claimAlignedEvidenceNeeded"])
	}
	if len(mapSliceFromValue(gap["missingDoD"])) != 1 || len(mapSliceFromValue(gap["missingGoal"])) != 1 {
		t.Fatalf("verificationEvidenceGap = %+v, want missing DoD and Goal", gap)
	}
}

func TestApplyRuntimeRepairCoverageNeedsConcreteEvidenceProjectionClearsAssessmentOnly(t *testing.T) {
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"assessmentOnlyReason":     "dod_coverage_contract_correction",
	}
	failed := &runmodel.Step{
		ID:    "run-repair-coverage-needs-evidence/step-1",
		RunID: "run-repair-coverage-needs-evidence",
		Kind:  "llm",
		State: common.StepStateFailed,
	}

	applyRuntimeRepairCoverageNeedsConcreteEvidenceProjection(t.Context(), nil, RuntimeEvidenceRepositories{}, input, &runmodel.Run{ID: "run-repair-coverage-needs-evidence"}, failed, true, true)

	if _, ok := input[stepInputKeyAssessmentOnly]; ok {
		t.Fatalf("assessment-only flag should be cleared: %v", input)
	}
	if _, ok := input["assessmentOnlyReason"]; ok {
		t.Fatalf("assessmentOnlyReason should be cleared: %v", input)
	}
	if input["completionFollowUp"] != false {
		t.Fatalf("completionFollowUp = %v, want false", input["completionFollowUp"])
	}
	if stringFromMap(input, "evidenceRepairSource") != "failed_assessment_only_coverage_gap" {
		t.Fatalf("evidenceRepairSource = %q, want failed assessment-only source", stringFromMap(input, "evidenceRepairSource"))
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 3 {
		t.Fatalf("repairGuidance = %+v, want three coverage evidence guidance items", guidance)
	}
}

func TestApplyRuntimeRepairValidationEvidenceProjectionSetsBoundedValidation(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairValidationEvidenceProjection(input, true)

	if input["boundedValidationRequired"] != true {
		t.Fatalf("boundedValidationRequired = %v, want true", input["boundedValidationRequired"])
	}
	if stringFromMap(input, "evidenceRepairSource") != "coverage_gap_without_implementation_target" {
		t.Fatalf("evidenceRepairSource = %q, want validation evidence source", stringFromMap(input, "evidenceRepairSource"))
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 4 {
		t.Fatalf("repairGuidance = %+v, want four validation evidence guidance items", guidance)
	}
}

func TestApplyRuntimeRepairValidationEscalationProjectionSwitchesToStateChanging(t *testing.T) {
	input := map[string]any{
		stepInputKeyAssessmentOnly:  true,
		"assessmentOnlyReason":      "previous-assessment",
		"boundedValidationRequired": true,
		"evidenceRepairSource":      "validation-only",
	}

	applyRuntimeRepairValidationEscalationProjection(input, true)

	if input["validationRepairEscalatedToImplementation"] != true {
		t.Fatalf("validationRepairEscalatedToImplementation = %v, want true", input["validationRepairEscalatedToImplementation"])
	}
	if stringFromMap(input, "repairObjective") != repairObjectiveStateChangingOutput || input["stateChangingOutputRequired"] != true {
		t.Fatalf("state-changing contract missing: %+v", input)
	}
	for _, key := range []string{stepInputKeyAssessmentOnly, "assessmentOnlyReason", "boundedValidationRequired", "evidenceRepairSource"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be cleared after validation escalation: %+v", key, input)
		}
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 3 {
		t.Fatalf("repairGuidance = %+v, want three escalation guidance items", guidance)
	}
}

func TestApplyRuntimeRepairGovernanceHistoryProjectionCompactsExecutionContextHistory(t *testing.T) {
	input := map[string]any{}
	history := make([]map[string]any, 0, 7)
	for idx := 0; idx < 7; idx++ {
		history = append(history, map[string]any{
			"id":       fmt.Sprintf("ticket-%03d", idx+1),
			"kind":     "decision",
			"state":    "resolved",
			"reason":   "operator supplied required context",
			"question": "provide context",
		})
	}
	run := &runmodel.Run{
		ID: "run-repair-governance-history",
		ExecutionContext: map[string]any{
			"governanceHistory": history,
		},
	}

	applyRuntimeRepairGovernanceHistoryProjection(t.Context(), nil, nil, nil, input, run)

	projected := mapSliceFromValue(input["governanceHistory"])
	if len(projected) != 6 {
		t.Fatalf("governanceHistory len = %d, want 6", len(projected))
	}
	if stringFromMap(projected[0], "id") != "ticket-001" || stringFromMap(projected[5], "id") != "ticket-006" {
		t.Fatalf("governanceHistory = %+v, want first six entries preserved", projected)
	}
}

func TestApplyRuntimeRepairGraphEvidenceProjectionAddsTargetSnapshots(t *testing.T) {
	ctx := t.Context()
	stepRepo := memory.NewStepRepository()
	run := &runmodel.Run{ID: "run-repair-graph-evidence"}
	root := &runmodel.Step{
		ID:    "run-repair-graph-evidence/root",
		RunID: run.ID,
		Kind:  "llm",
		State: common.StepStateCompleted,
		Output: map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{{
					"id":   "read-config",
					"kind": "file/read",
				}},
			},
		},
	}
	readStep := &runmodel.Step{
		ID:    "run-repair-graph-evidence/read-config",
		RunID: run.ID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(root.ID),
			stepInputKeyGraphNodeID:     "read-config",
		},
		Output: map[string]any{
			"stage":       "file_read",
			"path":        "config/app.toml",
			"exists":      true,
			"content":     "enabled = true\n",
			"contentHash": "hash-001",
			"size":        15,
		},
	}
	failed := &runmodel.Step{
		ID:    "run-repair-graph-evidence/failed",
		RunID: run.ID,
		Kind:  "llm",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(root.ID),
		},
	}
	if err := stepRepo.Create(ctx, root); err != nil {
		t.Fatalf("create root step: %v", err)
	}
	if err := stepRepo.Create(ctx, readStep); err != nil {
		t.Fatalf("create read step: %v", err)
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("create failed step: %v", err)
	}
	input := map[string]any{}

	applyRuntimeRepairGraphEvidenceProjection(ctx, newGraphSubsystem(stepRepo, RuntimeEvidenceRepositories{}), input, run, failed)

	evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || stringFromMap(evidence[0], "nodeId") != "read-config" {
		t.Fatalf("graphEvidence = %+v, want read-config evidence", evidence)
	}
	targets := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targets) != 1 || stringFromMap(targets[0], "targetRef") != "config/app.toml" {
		t.Fatalf("failureTargetRepairContext = %+v, want config/app.toml snapshot", targets)
	}
}

func TestApplyRuntimeRepairPatchContextProjectionRequiresStateChangingRepair(t *testing.T) {
	input := map[string]any{}
	patchContext := []map[string]any{{
		"targetRef":        "src/app.go",
		"contentHash":      "hash-current",
		"contentComplete":  true,
		"safeForFileWrite": true,
	}}
	patchSuggestions := []map[string]any{{
		"operation": "text_replace",
		"baseHash":  "hash-current",
	}}

	applyRuntimeRepairPatchContextProjection(input, patchContext, patchSuggestions, nil)

	if stringFromMap(input, "repairObjective") != repairObjectiveStateChangingOutput {
		t.Fatalf("repairObjective = %q, want state-changing output", stringFromMap(input, "repairObjective"))
	}
	if input["stateChangingOutputRequired"] != true {
		t.Fatalf("stateChangingOutputRequired = %v, want true", input["stateChangingOutputRequired"])
	}
	if len(mapSliceFromValue(input["patchRepairContext"])) != 1 {
		t.Fatalf("patchRepairContext = %+v, want one snapshot", input["patchRepairContext"])
	}
	if len(mapSliceFromValue(input["patchRepairSuggestions"])) != 1 {
		t.Fatalf("patchRepairSuggestions = %+v, want one suggestion", input["patchRepairSuggestions"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) < 10 {
		t.Fatalf("repairGuidance len = %d, want patch guidance", len(guidance))
	}
}

func TestApplyRuntimeRepairPatchContextProjectionKeepsSatisfiedPatchAsValidation(t *testing.T) {
	input := map[string]any{}
	patchContext := []map[string]any{{
		"targetRef":   "src/app.go",
		"contentHash": "hash-current",
	}}
	satisfiedSignals := []map[string]any{{
		"targetRef": "src/app.go",
		"reason":    "current target already contains intended change",
	}}

	applyRuntimeRepairPatchContextProjection(input, patchContext, nil, satisfiedSignals)

	if input["stateChangeAlreadySatisfiedByCurrentTarget"] != true {
		t.Fatalf("stateChangeAlreadySatisfiedByCurrentTarget = %v, want true", input["stateChangeAlreadySatisfiedByCurrentTarget"])
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) != 1 {
		t.Fatalf("currentTargetAlreadySatisfiesFailedPatch = %+v, want one signal", input["currentTargetAlreadySatisfiesFailedPatch"])
	}
	if _, ok := input["stateChangingOutputRequired"]; ok {
		t.Fatalf("stateChangingOutputRequired should not be set when patch is already satisfied: %v", input)
	}
	if stringFromMap(input, "repairObjective") != "" {
		t.Fatalf("repairObjective = %q, want unchanged objective", stringFromMap(input, "repairObjective"))
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	if len(guidance) < 12 {
		t.Fatalf("repairGuidance len = %d, want patch guidance plus satisfied guidance", len(guidance))
	}
}

func TestApplyRuntimeRepairFailureTargetContextProjectionAddsTargetGuidance(t *testing.T) {
	input := map[string]any{}
	targetContext := []map[string]any{{
		"targetRef":   "src/app.go",
		"contentHash": "hash-current",
		"source":      "failure_target_current_workspace_file",
	}}

	applyRuntimeRepairFailureTargetContextProjection(input, targetContext)

	if len(mapSliceFromValue(input["failureTargetRepairContext"])) != 1 {
		t.Fatalf("failureTargetRepairContext = %+v, want one target snapshot", input["failureTargetRepairContext"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 5 {
		t.Fatalf("repairGuidance = %+v, want five target context guidance items", guidance)
	}
}

func TestApplyRuntimeRepairResourceShapeProjectionAddsShapeGuidance(t *testing.T) {
	input := map[string]any{}
	shapeContext := []map[string]any{{
		"targetRef": "src/config.go",
		"source":    "same_directory_resource_shape",
	}}

	applyRuntimeRepairResourceShapeProjection(input, shapeContext)

	if len(mapSliceFromValue(input["resourceShapeAlignmentContext"])) != 1 {
		t.Fatalf("resourceShapeAlignmentContext = %+v, want one shape item", input["resourceShapeAlignmentContext"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 3 {
		t.Fatalf("repairGuidance = %+v, want three shape guidance items", guidance)
	}
}

func TestApplyRuntimeRepairRootCauseCandidateProjectionAddsRootCauseGuidance(t *testing.T) {
	input := map[string]any{}
	rootCauseCandidates := []map[string]any{{
		"targetRef": "src/dependency.go",
		"source":    "adjacent_resource",
	}}

	applyRuntimeRepairRootCauseCandidateProjection(input, rootCauseCandidates)

	if len(mapSliceFromValue(input["rootCauseCandidateContext"])) != 1 {
		t.Fatalf("rootCauseCandidateContext = %+v, want one root cause candidate", input["rootCauseCandidateContext"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 3 {
		t.Fatalf("repairGuidance = %+v, want three root cause guidance items", guidance)
	}
}

func TestRuntimeRepairContextProjectionHelpersIgnoreEmptyContext(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairFailureTargetContextProjection(input, nil)
	applyRuntimeRepairResourceShapeProjection(input, nil)
	applyRuntimeRepairRootCauseCandidateProjection(input, nil)

	if len(input) != 0 {
		t.Fatalf("input = %+v, want no fields for empty contexts", input)
	}
}

func TestApplyRuntimeRepairReadableResourceProjectionAddsGuidance(t *testing.T) {
	input := map[string]any{}
	context := []map[string]any{{
		"path":    "README.md",
		"content": "current readable content",
	}}

	applyRuntimeRepairReadableResourceProjection(input, context)

	if len(mapSliceFromValue(input["readableResourceRepairContext"])) != 1 {
		t.Fatalf("readableResourceRepairContext = %+v, want one readable resource", input["readableResourceRepairContext"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 2 {
		t.Fatalf("repairGuidance = %+v, want two readable resource guidance items", guidance)
	}
}

func TestApplyRuntimeRepairWorkspaceManifestProjectionAddsOnlyNonEmptyManifest(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairWorkspaceManifestProjection(input, nil)
	if _, ok := input[stepInputKeyWorkspaceFileManifest]; ok {
		t.Fatalf("workspaceFileManifest should be absent for empty manifest: %v", input)
	}

	applyRuntimeRepairWorkspaceManifestProjection(input, []map[string]any{{
		"path": "README.md",
	}})

	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 1 || stringFromMap(manifest[0], "path") != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want README.md", input[stepInputKeyWorkspaceFileManifest])
	}
}

func TestApplyRuntimeRepairStateChangingActionPackageProjectionRequiresStateChangingFlag(t *testing.T) {
	input := map[string]any{}
	failed := &runmodel.Step{
		ID:    "run-repair-action-package/step-1",
		RunID: "run-repair-action-package",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stderr": "target file mismatch",
		},
	}

	applyRuntimeRepairStateChangingActionPackageProjection(input, failed, errors.New("exit status 1"))
	if len(input) != 0 {
		t.Fatalf("input = %+v, want no projection without stateChangingOutputRequired", input)
	}

	input["stateChangingOutputRequired"] = true
	applyRuntimeRepairStateChangingActionPackageProjection(input, failed, errors.New("exit status 1"))

	if len(mapFromValue(input["lastFailureDiagnostics"])) == 0 {
		t.Fatalf("lastFailureDiagnostics missing from state-changing projection: %+v", input)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	if len(guidance) < 2 {
		t.Fatalf("repairGuidance = %+v, want action package guidance", guidance)
	}
}

func TestApplyRuntimeRepairFailureScopeProjectionAddsClassAndSignature(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairFailureScopeProjection(input, failureClassShellCommand, "class=shell_command_failed|failureTargets=main.go")

	if input[failureOutputKeyClass] != failureClassShellCommand || input[stepInputKeyRepairOfFailureClass] != failureClassShellCommand {
		t.Fatalf("failure class fields = %+v, want shell command class", input)
	}
	if input[failureOutputKeySignature] != "class=shell_command_failed|failureTargets=main.go" ||
		input[stepInputKeyRepairOfFailureSig] != "class=shell_command_failed|failureTargets=main.go" {
		t.Fatalf("failure signature fields = %+v, want signature copied", input)
	}
}

func TestApplyRuntimeRepairStructuredFailureSignalsProjectionAddsPrimaryDiagnostic(t *testing.T) {
	input := map[string]any{}
	signals := map[string]any{
		"targetRefs":  []string{"main.go"},
		"lineRefs":    []map[string]any{{"targetRef": "main.go", "line": 12}},
		"identifiers": []string{"MissingType"},
		"messages":    []string{"undefined: MissingType"},
	}

	applyRuntimeRepairStructuredFailureSignalsProjection(input, signals)

	projected := mapFromValue(input["structuredFailureSignals"])
	if len(projected) == 0 {
		t.Fatalf("structuredFailureSignals missing: %+v", input)
	}
	if primary := mapFromValue(input["primaryDiagnosticRepair"]); len(primary) == 0 {
		t.Fatalf("primaryDiagnosticRepair missing for shape conflict signals: %+v", input)
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 4 {
		t.Fatalf("repairGuidance = %+v, want four structured signal guidance items", guidance)
	}
}

func TestApplyRuntimeRepairFailureTargetScopeProjectionAddsScopedGuidanceForStateChangingRepair(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
	}

	applyRuntimeRepairFailureTargetScopeProjection(input, "failureTargets=main.go", 3)

	if stringFromMap(input, "failureTargetScope") != "failureTargets=main.go" {
		t.Fatalf("failureTargetScope = %q, want main.go scope", stringFromMap(input, "failureTargetScope"))
	}
	if input["targetRepairAttempts"] != 3 {
		t.Fatalf("targetRepairAttempts = %v, want 3", input["targetRepairAttempts"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 2 {
		t.Fatalf("repairGuidance = %+v, want two scoped verification guidance items", guidance)
	}
}

func TestApplyRuntimeRepairTransientRetryProjectionSetsRetryContract(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairTransientRetryProjection(input, true)

	if stringFromMap(input, "repairObjective") != repairObjectiveTransientExecutionRetry {
		t.Fatalf("repairObjective = %q, want transient retry", stringFromMap(input, "repairObjective"))
	}
	if input["executionTimeoutRetryAllowed"] != true {
		t.Fatalf("executionTimeoutRetryAllowed = %v, want true", input["executionTimeoutRetryAllowed"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) == 0 {
		t.Fatalf("repairGuidance missing transient retry guidance")
	}
}

func TestApplyRuntimeRepairCapabilityPolicyProjectionClearsAssessmentOnly(t *testing.T) {
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"assessmentOnlyReason":     "coverage",
	}
	contract := map[string]any{
		"policies": []map[string]any{{"policyId": "policy-001"}},
	}

	applyRuntimeRepairCapabilityPolicyProjection(input, contract)

	if input["capabilityPolicyRepairRequired"] != true || stringFromMap(input, "repairObjective") != "repair_capability_policy_follow_up" {
		t.Fatalf("capability policy contract missing: %+v", input)
	}
	if input["stateChangingOutputRequired"] != true {
		t.Fatalf("stateChangingOutputRequired = %v, want true", input["stateChangingOutputRequired"])
	}
	if len(mapFromValue(input["capabilityPolicyRepairContract"])) == 0 {
		t.Fatalf("capabilityPolicyRepairContract missing: %+v", input)
	}
	for _, key := range []string{stepInputKeyAssessmentOnly, "assessmentOnlyReason"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be cleared for capability policy repair: %+v", key, input)
		}
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 3 {
		t.Fatalf("repairGuidance = %+v, want three capability policy guidance items", guidance)
	}
}

func TestApplyRuntimeRepairStateChangingCoverageTargetGuidanceRequiresCoverageGap(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
	}

	applyRuntimeRepairStateChangingCoverageTargetGuidance(input)
	if _, ok := input["repairGuidance"]; ok {
		t.Fatalf("repairGuidance should be absent without coverage gaps: %+v", input)
	}

	input["missingDoDItems"] = []map[string]any{{"id": "dod-001"}}
	applyRuntimeRepairStateChangingCoverageTargetGuidance(input)
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 2 {
		t.Fatalf("repairGuidance = %+v, want two coverage target guidance items", guidance)
	}
}

func TestApplyRuntimeRepairUnresolvedCoverageWorkItemGuidanceAddsResolutionRules(t *testing.T) {
	input := map[string]any{
		"unresolvedCoverageWorkItems": []map[string]any{{
			"id":     "cwi-001",
			"status": "open",
		}},
	}

	applyRuntimeRepairUnresolvedCoverageWorkItemGuidance(input)

	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 4 {
		t.Fatalf("repairGuidance = %+v, want four unresolved work item guidance items", guidance)
	}
}

func TestApplyRuntimeRepairStrategyReassessmentProjectionAddsDecisionGuidance(t *testing.T) {
	input := map[string]any{}
	reassessment := map[string]any{
		"required":      true,
		"failedSurface": "target.go",
	}

	applyRuntimeRepairStrategyReassessmentProjection(input, reassessment)

	if projected := mapFromValue(input["strategyReassessment"]); projected["failedSurface"] != "target.go" {
		t.Fatalf("strategyReassessment = %+v, want target.go failed surface", projected)
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 5 {
		t.Fatalf("repairGuidance = %+v, want five reassessment guidance items", guidance)
	}
}

func TestApplyRuntimeRepairPreviousGraphMetadataProjectionAddsPreviousAndGraphFields(t *testing.T) {
	failed := &runmodel.Step{
		ID:    "run-repair-graph-metadata/graph-001-plan",
		RunID: "run-repair-graph-metadata",
		Kind:  "llm",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:         "node-001",
			stepInputKeyGraphRootStepID:     "run-repair-graph-metadata/root",
			stepInputKeyGraphCompletedNodes: []string{"node-000"},
		},
		Output: map[string]any{
			"stage":                  "llm_execution_graph_plan",
			stepOutputKeyGraphBranch: "main",
		},
	}
	input := map[string]any{}
	previousOutput := map[string]any{"stage": "compact"}
	graph := runtimemodel.BlockExecutionGraph{
		Nodes: []runtimemodel.BlockExecutionNode{{ID: "node-001", Kind: "shell/command"}},
	}

	applyRuntimeRepairPreviousGraphMetadataProjection(input, failed, previousOutput, graph, true)

	if input["previousStepKind"] != failed.Kind {
		t.Fatalf("previousStepKind = %v, want %v", input["previousStepKind"], failed.Kind)
	}
	if mapFromValue(input["previousStepOutput"])["stage"] != "compact" {
		t.Fatalf("previousStepOutput = %+v, want compact output", input["previousStepOutput"])
	}
	if input[stepInputKeyGraphPreviousNodeID] != "node-001" || input["failedGraphNodeId"] != "node-001" {
		t.Fatalf("graph previous node fields missing: %+v", input)
	}
	if completed := stringSliceFromValue(input[stepInputKeyGraphCompletedNodes]); len(completed) != 1 || completed[0] != "node-000" {
		t.Fatalf("graph completed nodes = %+v, want one node", input[stepInputKeyGraphCompletedNodes])
	}
	if input[stepOutputKeyGraphBranch] != "main" {
		t.Fatalf("graph branch = %v, want main", input[stepOutputKeyGraphBranch])
	}
	if projected, ok := input["failedExecutionGraph"].(runtimemodel.BlockExecutionGraph); !ok || len(projected.Nodes) != 1 || input[stepInputKeyGraphRootStepID] == "" {
		t.Fatalf("failed graph metadata missing: %+v", input)
	}
}

func TestApplyRuntimeRepairValidationEscalationFollowUpGuidanceRequiresEscalationFlag(t *testing.T) {
	input := map[string]any{}

	applyRuntimeRepairValidationEscalationFollowUpGuidance(input)
	if _, ok := input["repairGuidance"]; ok {
		t.Fatalf("repairGuidance should be absent without escalation flag: %+v", input)
	}

	input["validationRepairEscalatedToImplementation"] = true
	applyRuntimeRepairValidationEscalationFollowUpGuidance(input)
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) != 2 {
		t.Fatalf("repairGuidance = %+v, want two escalation follow-up guidance items", guidance)
	}
}

func TestResetRuntimeRepairInputToRejectedResultContinuationSupersedesHistoricalReplan(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-rejected-continuation-reset",
		WorkspaceID: "ws-rejected-continuation-reset",
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"resultId":         "result-001",
				"summary":          "latest result still misses required evidence",
				"requiredEvidence": []string{"read current target", "verify current behavior"},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-rejected-continuation-reset/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"reason": "historical replan target failed",
		},
	}
	input := map[string]any{
		"replanStateChangingContinuation": true,
		"stateChangingOutputRequired":     true,
		"patchRepairContext":              []map[string]any{{"targetRef": "old.go"}},
		"failureTargetRepairContext":      []map[string]any{{"targetRef": "old.go"}},
		"targetRepairAttempts":            3,
		"strategyReassessment":            map[string]any{"required": true},
	}

	input = resetRuntimeRepairInputToRejectedResultContinuation(input, run, failed)

	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence {
		t.Fatalf("repairObjective = %v, want %s", input["repairObjective"], repairObjectiveAcquireFailureEvidence)
	}
	if input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("concreteFailureEvidenceRequired = %v, want true", input["concreteFailureEvidenceRequired"])
	}
	for _, key := range []string{"replanStateChangingContinuation", "stateChangingOutputRequired", "patchRepairContext", "failureTargetRepairContext", "targetRepairAttempts", "strategyReassessment"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be removed when rejected result continuation supersedes historical replan: %+v", key, input[key])
		}
	}
	progress := mapFromValue(input[stepInputKeyCurrentProgressDigest])
	if stringFromMap(progress, "source") != "rejectedResultContinuation" {
		t.Fatalf("currentProgressDigest = %+v, want rejected result continuation source", progress)
	}
	currentFailure := mapFromValue(input["currentFailure"])
	if stringFromMap(currentFailure, "failureClass") != "rejected_result_continuation" {
		t.Fatalf("currentFailure = %+v, want rejected result continuation failure", currentFailure)
	}
	historical := mapFromValue(input["historicalRepairAttempt"])
	if historical["supersededBy"] != "rejectedResultContinuation" || historical["stepId"] != string(failed.ID) {
		t.Fatalf("historicalRepairAttempt = %+v, want failed step lineage superseded by rejected result", historical)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if pkg["evidenceAcquisitionRequired"] != true || required["mustProduceConcreteFailureEvidence"] != true || required["mustIncludeStateChangingNode"] != false {
		t.Fatalf("repairActionPackage = %+v, requiredOutput = %+v, want evidence acquisition contract", pkg, required)
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) < 3 {
		t.Fatalf("repairGuidance = %+v, want base evidence acquisition and rejected result continuation guidance", guidance)
	}
}

func TestPreserveStateChangingSchemaInvalidRepairContractKeepsExecutableRepair(t *testing.T) {
	failed := &runmodel.Step{
		ID:    "run-schema-invalid-contract/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
			"error":               "invalid llm decision json",
		},
	}
	input := map[string]any{
		"boundedValidationRequired": true,
		"evidenceRepairRequired":    true,
		stepInputKeyAssessmentOnly:  true,
		"assessmentOnlyReason":      "stale_assessment",
	}

	if !preserveStateChangingSchemaInvalidRepairContract(input, failed) {
		t.Fatalf("preserveStateChangingSchemaInvalidRepairContract returned false")
	}
	if input["schemaInvalidRepairMode"] != "single_valid_json_decision" ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true {
		t.Fatalf("input = %+v, want state-changing schema invalid contract", input)
	}
	for _, key := range []string{"boundedValidationRequired", "evidenceRepairRequired", stepInputKeyAssessmentOnly, "assessmentOnlyReason"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be removed from state-changing schema invalid contract: %+v", key, input[key])
		}
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if recovery := mapFromValue(pkg["schemaInvalidRecovery"]); recovery["required"] != true {
		t.Fatalf("schemaInvalidRecovery = %+v, want required", recovery)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustReturnValidJSON"] != true || required["mustNotReuseRejectedJSON"] != true || required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("requiredOutput = %+v, want valid JSON state-changing contract", required)
	}
}

func TestPreserveAssessmentOnlySchemaInvalidRepairContractKeepsAssessmentScope(t *testing.T) {
	failed := &runmodel.Step{
		ID:    "run-assessment-schema-invalid/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"assessmentOnlyReason":     "dod_coverage_contract_correction",
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-001",
				"raw":               "goal coverage gap",
			}},
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
			"error":               "invalid llm decision json",
		},
	}
	input := map[string]any{
		"stateChangingOutputRequired":     true,
		"boundedValidationRequired":       true,
		"concreteFailureEvidenceRequired": true,
	}

	if !preserveAssessmentOnlySchemaInvalidRepairContract(input, failed) {
		t.Fatalf("preserveAssessmentOnlySchemaInvalidRepairContract returned false")
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input[stepInputKeySoulStage] != soulStageValidation ||
		input[stepInputKeyAssessmentOnly] != true ||
		input["completionFollowUp"] != true ||
		input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("input = %+v, want assessment-only schema invalid recovery contract", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["goalRequirementId"] != "goal-001" {
		t.Fatalf("missingGoalCoverage = %+v, want copied assessment scope", input["missingGoalCoverage"])
	}
	for _, key := range []string{"stateChangingOutputRequired", "boundedValidationRequired", "concreteFailureEvidenceRequired"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be removed from assessment-only schema invalid contract: %+v", key, input[key])
		}
	}
	if recovery := mapFromValue(input["assessmentSchemaInvalidRecovery"]); recovery["required"] != true {
		t.Fatalf("assessmentSchemaInvalidRecovery = %+v, want required", recovery)
	}
}

func TestPreserveConcreteValidationSchemaInvalidRepairContractKeepsValidationEvidence(t *testing.T) {
	failed := &runmodel.Step{
		ID:    "run-validation-schema-invalid/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "verify runtime behavior",
			}},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
			"error":               "invalid llm decision json",
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
	}

	if !preserveConcreteValidationSchemaInvalidRepairContract(input, failed) {
		t.Fatalf("preserveConcreteValidationSchemaInvalidRepairContract returned false")
	}
	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["evidenceRepairRequired"] != true ||
		input["schemaInvalidRepairMode"] != "single_valid_json_decision" {
		t.Fatalf("input = %+v, want concrete validation schema invalid recovery contract", input)
	}
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 || targets[0]["targetText"] != "verify runtime behavior" {
		t.Fatalf("coverageTargetSet = %+v, want inherited validation target", input["coverageTargetSet"])
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if recovery := mapFromValue(pkg["schemaInvalidRecovery"]); recovery["required"] != true {
		t.Fatalf("schemaInvalidRecovery = %+v, want required", recovery)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		required["mustReturnValidJSON"] != true ||
		required["mustNotReuseRejectedJSON"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want valid JSON concrete validation contract", required)
	}
}

func TestInheritPriorRepairContextFillsMissingContextWithoutOverwritingCurrentInput(t *testing.T) {
	failed := &runmodel.Step{
		ID:   "run-inherit-prior/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":   "old-target.go",
				"contentHash": "hash-old",
			}},
			"patchRepairContext": []map[string]any{{
				"targetRef": "patch-target.go",
			}},
			"readableResourceRepairContext": []map[string]any{{
				"targetRef": "readable.md",
			}},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id": "cwi-001",
				},
			},
			stepInputKeyWorkspaceFileManifest: []map[string]any{{
				"path": "old-target.go",
			}},
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "targetRef=old-target.go",
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
			"rootCauseCandidateContext": []map[string]any{{
				"targetRef": "candidate.go",
			}},
			"targetRepairAttempts": int64(4),
			"failureTargetScope":   "targetRef=old-target.go",
		},
	}
	input := map[string]any{
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":   "current-target.go",
			"contentHash": "hash-current",
		}},
	}

	inheritPriorRepairContext(input, failed)

	targets := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targets) != 1 || targets[0]["targetRef"] != "current-target.go" {
		t.Fatalf("failureTargetRepairContext = %+v, want current input preserved", input["failureTargetRepairContext"])
	}
	if patches := mapSliceFromValue(input["patchRepairContext"]); len(patches) != 1 || patches[0]["targetRef"] != "patch-target.go" {
		t.Fatalf("patchRepairContext = %+v, want inherited patch context", input["patchRepairContext"])
	}
	if readable := mapSliceFromValue(input["readableResourceRepairContext"]); len(readable) != 1 || readable[0]["targetRef"] != "readable.md" {
		t.Fatalf("readableResourceRepairContext = %+v, want inherited readable context", input["readableResourceRepairContext"])
	}
	if context := mapFromValue(input[coverageWorkItemContextKey]); stringFromMap(mapFromValue(context["currentFocus"]), "id") != "cwi-001" {
		t.Fatalf("coverageWorkItemContext = %+v, want inherited current focus", input[coverageWorkItemContextKey])
	}
	if manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest]); len(manifest) != 1 || manifest[0]["path"] != "old-target.go" {
		t.Fatalf("workspaceFileManifest = %+v, want inherited manifest", input[stepInputKeyWorkspaceFileManifest])
	}
	if reassessment := mapFromValue(input["strategyReassessment"]); !boolValue(reassessment["required"]) {
		t.Fatalf("strategyReassessment = %+v, want inherited reassessment", input["strategyReassessment"])
	}
	if pkg := mapFromValue(input["repairActionPackage"]); !boolValue(mapFromValue(pkg["requiredOutput"])["mustIncludeStateChangingNode"]) {
		t.Fatalf("repairActionPackage = %+v, want inherited package", input["repairActionPackage"])
	}
	if candidates := mapSliceFromValue(input["rootCauseCandidateContext"]); len(candidates) != 1 || candidates[0]["targetRef"] != "candidate.go" {
		t.Fatalf("rootCauseCandidateContext = %+v, want inherited root-cause candidates", input["rootCauseCandidateContext"])
	}
	if input["targetRepairAttempts"] != int64(4) || input["failureTargetScope"] != "targetRef=old-target.go" {
		t.Fatalf("attempt/scope = %v/%v, want inherited values", input["targetRepairAttempts"], input["failureTargetScope"])
	}
	if guidance := stringSliceFromValue(input["repairGuidance"]); len(guidance) < 6 {
		t.Fatalf("repairGuidance = %+v, want inherited context and strategy guidance", guidance)
	}
}

func TestInheritCoverageRepairContextReadsInputOutputAndDiagnosticsWithoutOverwritingCurrentInput(t *testing.T) {
	failed := &runmodel.Step{
		ID:   "run-inherit-coverage/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"missingDoDItems": []map[string]any{{
				"id": "dod-from-input",
			}},
		},
		Output: map[string]any{
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-from-output",
			}},
			"failedStepOutput": map[string]any{
				"missingDeliverableCoverage": []map[string]any{{
					"id": "deliverable-from-failed-output",
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-from-diagnostics",
				}},
			},
		},
	}
	input := map[string]any{
		"missingGoalCoverage": []map[string]any{{
			"goalRequirementId": "goal-current",
		}},
	}

	if !inheritCoverageRepairContext(input, failed) {
		t.Fatalf("inheritCoverageRepairContext returned false")
	}
	if input["coverageRepairContextInherited"] != true || input["coverageRepairRequired"] != true {
		t.Fatalf("input = %+v, want inherited coverage repair flags", input)
	}
	if items := mapSliceFromValue(input["missingDoDItems"]); len(items) != 1 || items[0]["id"] != "dod-from-input" {
		t.Fatalf("missingDoDItems = %+v, want input coverage inherited", input["missingDoDItems"])
	}
	if items := mapSliceFromValue(input["missingGoalCoverage"]); len(items) != 1 || items[0]["goalRequirementId"] != "goal-current" {
		t.Fatalf("missingGoalCoverage = %+v, want current input preserved", input["missingGoalCoverage"])
	}
	if items := mapSliceFromValue(input["missingDeliverableCoverage"]); len(items) != 1 || items[0]["id"] != "deliverable-from-failed-output" {
		t.Fatalf("missingDeliverableCoverage = %+v, want failed output coverage inherited", input["missingDeliverableCoverage"])
	}
}
