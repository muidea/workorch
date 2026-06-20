package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeCompletionGateDoDAssessmentOutputCompletesAssessmentOnlyGoalCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-normalize",
		WorkspaceID:     "ws-goal-coverage-normalize",
		DoDID:           "dod-goal-coverage-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "record externally visible state change",
			},
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-normalize/repair-001-of-graph-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-neutral",
				"raw":               "record externally visible state change",
				"evidenceCandidates": []map[string]any{{
					"stepId": "run-goal-coverage-normalize/graph-001/001-change",
				}},
			}},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-normalize/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"state":        "satisfied",
					"reason":       "The completed graph produced the required state change.",
					"evidenceRefs": []string{"workspace file", "run-goal-coverage-normalize/graph-001/001-change"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("goalCoverage.items = %+v, want one normalized item", coverage["items"])
	}
	if got := stringFromMap(items[0], "goalRequirementId"); got != "goal-req-001-neutral" {
		t.Fatalf("goalRequirementId = %q, want copied missingGoalCoverage id", got)
	}
	if got := stringFromMap(items[0], "requirement"); got != "record externally visible state change" {
		t.Fatalf("requirement = %q, want copied missingGoalCoverage requirement", got)
	}
	refs := evidenceRefsFromValue(items[0]["evidenceRefs"])
	if len(refs) != 1 || refs[0] != "run-goal-coverage-normalize/graph-001/001-change" {
		t.Fatalf("evidenceRefs = %+v, want only traceable refs", refs)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputPreservesGovernanceOwnerEvidenceRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-governance-owner-ref-normalize",
		WorkspaceID:     "ws-governance-owner-ref-normalize",
		DoDID:           "dod-governance-owner-ref-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"two Decision tickets are resolved",
					"one Approval is approved",
					"MCP loadtest.governed_result evidence is collected",
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-governance-owner-ref-normalize/follow-up-graph",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Governance and MCP validation evidence are complete.",
			"items": []map[string]any{
				{
					"item":         "two Decision tickets are resolved",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001-step-1-decision", "run-001-llm-after-decision-decision"},
				},
				{
					"item":         "one Approval is approved",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001-llm-after-decision-2-approval"},
				},
				{
					"item":         "MCP loadtest.governed_result evidence is collected",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001/graph-001/001-loadtest-tool"},
				},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(items) != 3 {
		t.Fatalf("dodAssessment.items = %+v, want 3 items", assessment["items"])
	}
	if refs := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(refs, []string{"run-001-step-1-decision", "run-001-llm-after-decision-decision"}) {
		t.Fatalf("decision evidenceRefs = %+v, want owner decision refs", refs)
	}
	if refs := evidenceRefsFromValue(items[1]["evidenceRefs"]); !reflect.DeepEqual(refs, []string{"run-001-llm-after-decision-2-approval"}) {
		t.Fatalf("approval evidenceRefs = %+v, want owner approval ref", refs)
	}
}

func TestDoDAssessmentItemMapsUnwrapsNestedItemsContainer(t *testing.T) {
	items, ok := doDAssessmentItemMaps(map[string]any{
		"satisfied": true,
		"items": []map[string]any{{
			"id":           "source_change",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-nested-items/graph-001/001-verify"},
		}},
	})

	if !ok || len(items) != 1 {
		t.Fatalf("items = %+v ok=%v, want unwrapped nested items", items, ok)
	}
	if items[0]["id"] != "source_change" {
		t.Fatalf("items[0] = %+v, want source_change", items[0])
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputRepairsContradictoryTopLevelSatisfied(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-top-level-satisfied-normalize",
		WorkspaceID:     "ws-dod-top-level-satisfied-normalize",
		DoDID:           "dod-top-level-satisfied-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-dod-top-level-satisfied-normalize/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "All required items are structurally satisfied.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-dod-top-level-satisfied-normalize/graph-001/001-verify"},
			}},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if assessment["satisfied"] != true {
		t.Fatalf("dodAssessment.satisfied = %v, want normalized true", assessment["satisfied"])
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputRepairsContradictoryDeliverableParentSatisfied(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-deliverable-parent-satisfied-normalize",
		WorkspaceID:     "ws-dod-deliverable-parent-satisfied-normalize",
		DoDID:           "dod-deliverable-parent-satisfied-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
							{"id": "validation_report", "required": true},
							{"id": "coverage_map", "required": true},
							{"id": "version_control_snapshot", "required": true},
							{"id": "user_deliverable", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-dod-deliverable-parent-satisfied-normalize/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "All DoD and deliverable items are satisfied, but the parent flags were stale.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/graph-001/002-test"},
			}},
			"deliverableCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{
					{
						"id":           "source_change",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/graph-001/001-change"},
					},
					{
						"id":           "validation_report",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/graph-001/002-test"},
					},
					{
						"id":           "coverage_map",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/graph-001/003-coverage"},
					},
					{
						"id":           "version_control_snapshot",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/graph-001/004-git"},
					},
					{
						"id":           "user_deliverable",
						"state":        "produced",
						"artifactRefs": []string{"run-dod-deliverable-parent-satisfied-normalize/artifacts/result"},
					},
				},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if assessment["satisfied"] != true {
		t.Fatalf("dodAssessment.satisfied = %v, want normalized true", assessment["satisfied"])
	}
	coverage := mapFromValue(assessment["deliverableCoverage"])
	if coverage["satisfied"] != true {
		t.Fatalf("deliverableCoverage.satisfied = %v, want normalized true", coverage["satisfied"])
	}
}

func TestEvidenceQualityDiagnosticsUsesScopeSpecificCoverageGap(t *testing.T) {
	err := evidenceKindRequirementError(profileEvidenceKindRequirement{
		Scope:                "goal_coverage",
		ID:                   "*",
		RequireClaimEvidence: true,
	}, "runtime behavior covered", "run-a/graph-001/001-verify, run-b/graph-001/002-test", "", "requires evidence text or coverageTargets aligned with the satisfied claim")
	qualityErr, ok := err.(*doDAssessmentEvidenceQualityError)
	if !ok {
		t.Fatalf("error type = %T, want doDAssessmentEvidenceQualityError", err)
	}
	diagnostics := qualityErr.RuntimeFailureDiagnostics()
	if diagnostics["scope"] != "goal_coverage" {
		t.Fatalf("scope = %v, want goal_coverage", diagnostics["scope"])
	}
	if len(mapSliceFromValue(diagnostics["missingGoalCoverage"])) != 1 {
		t.Fatalf("missingGoalCoverage = %+v, want scoped gap", diagnostics["missingGoalCoverage"])
	}
	if len(mapSliceFromValue(diagnostics["missingDoDItems"])) != 0 {
		t.Fatalf("missingDoDItems = %+v, want no dod-item gap for goal coverage", diagnostics["missingDoDItems"])
	}
	refs := stringSliceFromValue(mapSliceFromValue(diagnostics["missingGoalCoverage"])[0]["evidenceRefs"])
	if !reflect.DeepEqual(refs, []string{"run-a/graph-001/001-verify", "run-b/graph-001/002-test"}) {
		t.Fatalf("evidenceRefs = %+v, want split refs", refs)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputFillsGoalCoverageItemFromRunRequirementID(t *testing.T) {
	goalText := "publish completion evidence"
	requirementID := goalCoverageRequirementID(1, normalizeDoDCoverageText(goalText))
	run := &runmodel.Run{
		ID:              "run-goal-coverage-id-normalize",
		WorkspaceID:     "ws-goal-coverage-id-normalize",
		DoDID:           "dod-goal-coverage-id-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": goalText,
			},
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-id-normalize/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-id-normalize/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"goalRequirementId": requirementID,
					"state":             "satisfied",
					"evidenceRefs":      []string{"run-goal-coverage-id-normalize/graph-001/002-verify"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("goalCoverage.items = %+v, want one normalized item", coverage["items"])
	}
	if got := stringFromMap(items[0], "item"); got != goalText {
		t.Fatalf("goalCoverage item = %q, want current Goal requirement text", got)
	}
	if got := stringFromMap(items[0], "goalRequirementId"); got != requirementID {
		t.Fatalf("goalRequirementId = %q, want %q", got, requirementID)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputFillsDeliverableCoverageIDsByRequiredOrder(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-id-normalize",
		WorkspaceID:     "ws-deliverable-coverage-id-normalize",
		DoDID:           "dod-deliverable-coverage-id-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
							{"id": "validation_report", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-id-normalize/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and deliverable requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-id-normalize/graph-001/002-verify"},
			}},
			"deliverableCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-id-normalize/graph-001/001-change"},
					},
					{
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-id-normalize/graph-001/002-verify"},
					},
				},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) != 2 || stringFromMap(items[0], "id") != "source_change" || stringFromMap(items[1], "id") != "validation_report" {
		t.Fatalf("deliverableCoverage.items = %+v, want required ids filled by order", items)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputFillsDeliverableCoverageIDsWhenItemsHaveDescriptions(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-description-normalize",
		WorkspaceID:     "ws-deliverable-coverage-description-normalize",
		DoDID:           "dod-deliverable-coverage-description-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"validation passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
							{"id": "validation_report", "required": true},
							{"id": "coverage_map", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-description-normalize/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and deliverable requirements are covered.",
			"items": []map[string]any{{
				"item":         "validation passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-description-normalize/graph-001/verify"},
			}},
			"deliverableCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"item":         "完整的 Go 源代码",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-description-normalize/graph-001/source"},
					},
					{
						"item":         "验证报告",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-description-normalize/graph-001/report"},
					},
					{
						"item":         "覆盖映射",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-description-normalize/graph-001/map"},
					},
				},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	wantIDs := []string{"source_change", "validation_report", "coverage_map"}
	if len(items) != len(wantIDs) {
		t.Fatalf("deliverableCoverage.items = %+v, want %d items", items, len(wantIDs))
	}
	for idx, want := range wantIDs {
		if got := stringFromMap(items[idx], "id"); got != want {
			t.Fatalf("deliverableCoverage.items[%d].id = %q, want %q; items=%+v", idx, got, want, items)
		}
		if got := stringFromMap(items[idx], "item"); got == "" || got == want {
			t.Fatalf("deliverableCoverage.items[%d].item = %q, want original descriptive item", idx, got)
		}
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputCompletesMissingDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-missing-normalize",
		WorkspaceID:     "ws-deliverable-coverage-missing-normalize",
		DoDID:           "dod-deliverable-coverage-missing-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
							{"id": "validation_report", "required": true},
							{"id": "coverage_map", "required": true},
							{"id": "version_control_snapshot", "required": true},
							{"id": "user_deliverable", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-missing-normalize/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and goal requirements are covered by the recorded evidence.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-missing-normalize/graph-001/002-test"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"item":         "ship the requested code change",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-deliverable-coverage-missing-normalize/graph-001/001-change"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) != 5 {
		t.Fatalf("deliverableCoverage.items = %+v, want all required deliverables synthesized", items)
	}
	wantIDs := []string{"source_change", "validation_report", "coverage_map", "version_control_snapshot", "user_deliverable"}
	for idx, want := range wantIDs {
		if got := stringFromMap(items[idx], "id"); got != want {
			t.Fatalf("deliverableCoverage.items[%d].id = %q, want %q; items=%+v", idx, got, want, items)
		}
		if got := stringFromMap(items[idx], "state"); got != "satisfied" {
			t.Fatalf("deliverableCoverage.items[%d].state = %q, want satisfied", idx, got)
		}
		refs := evidenceRefsFromValue(items[idx]["evidenceRefs"])
		if len(refs) == 0 {
			t.Fatalf("deliverableCoverage.items[%d].evidenceRefs is empty", idx)
		}
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputDoesNotMaskFailedDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-failed-normalize",
		WorkspaceID:     "ws-deliverable-coverage-failed-normalize",
		DoDID:           "dod-deliverable-coverage-failed-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-failed-normalize/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD item is covered, but deliverable coverage is explicitly failed.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-failed-normalize/graph-001/002-test"},
			}},
			"deliverableCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{{
					"id":            "source_change",
					"state":         "failed",
					"failureReason": "source change was not produced",
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	_, _, err := validatedCompletionDoDAssessment(run, normalized)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want explicit deliverable failure preserved", err)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputCompletesRepairFollowUpSupplementalDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-repair-normalize",
		WorkspaceID:     "ws-deliverable-coverage-repair-normalize",
		DoDID:           "dod-deliverable-coverage-repair-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true, "evidenceRole": "primary_state_change"},
							{"id": "validation_report", "required": true, "evidenceRole": "supplemental_evidence"},
							{"id": "coverage_map", "required": true, "evidenceRole": "supplemental_evidence"},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-repair-normalize/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD item is covered, but repair omitted required deliverable coverage.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-repair-normalize/graph-001/002-test"},
			}},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment := doDAssessmentFromOutput(normalized)
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) != 2 {
		t.Fatalf("deliverableCoverage.items = %+v, want supplemental deliverables only", items)
	}
	wantIDs := []string{"validation_report", "coverage_map"}
	for idx, want := range wantIDs {
		if got := stringFromMap(items[idx], "id"); got != want {
			t.Fatalf("deliverableCoverage.items[%d].id = %q, want %q; items=%+v", idx, got, want, items)
		}
		if got := stringFromMap(items[idx], "state"); got != "satisfied" {
			t.Fatalf("deliverableCoverage.items[%d].state = %q, want satisfied", idx, got)
		}
		if refs := stringSliceFromValue(items[idx]["artifactRefs"]); len(refs) == 0 {
			t.Fatalf("deliverableCoverage.items[%d].artifactRefs is empty", idx)
		}
	}
	_, _, err := validatedCompletionDoDAssessment(run, normalized)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "deliverableCoverage missing required deliverables: source_change") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want primary deliverable gap preserved", err)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputCompletesRepairFollowUpPrimaryDeliverableWithAcceptedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-repair-primary-normalize",
		WorkspaceID:     "ws-deliverable-coverage-repair-primary-normalize",
		DoDID:           "dod-deliverable-coverage-repair-primary-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true, "evidenceRole": "primary_state_change"},
							{"id": "validation_report", "required": true, "evidenceRole": "supplemental_evidence"},
							{"id": "coverage_map", "required": true, "evidenceRole": "supplemental_evidence"},
						},
					},
				},
			},
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-001",
				"sourceRequirementId":   "source_change",
				"sourceScope":           "deliverable",
				"evidenceRole":          "primary_state_change",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"state_change": {"run-deliverable-coverage-repair-primary-normalize/graph-001/001-change"},
				},
				"acceptedEvidenceRefs": []string{"run-deliverable-coverage-repair-primary-normalize/graph-001/001-change"},
				"missingEvidenceKinds": []string{"validation"},
				"status":               "in_progress",
			}},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-repair-primary-normalize/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD item is covered and the repair assessment maps the existing evidence.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-repair-primary-normalize/graph-001/002-test"},
			}},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) != 3 {
		t.Fatalf("deliverableCoverage.items = %+v, want primary plus supplemental deliverables", items)
	}
	wantIDs := []string{"source_change", "validation_report", "coverage_map"}
	for idx, want := range wantIDs {
		if got := stringFromMap(items[idx], "id"); got != want {
			t.Fatalf("deliverableCoverage.items[%d].id = %q, want %q; items=%+v", idx, got, want, items)
		}
		if got := stringFromMap(items[idx], "state"); got != "satisfied" {
			t.Fatalf("deliverableCoverage.items[%d].state = %q, want satisfied", idx, got)
		}
		if refs := evidenceRefsFromValue(items[idx]["evidenceRefs"]); len(refs) == 0 {
			t.Fatalf("deliverableCoverage.items[%d].evidenceRefs is empty", idx)
		}
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputRecoversMissingSupplementalDeliverables(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-deliverable-coverage-repair-supplemental-missing-normalize",
		WorkspaceID:     "ws-deliverable-coverage-repair-supplemental-missing-normalize",
		DoDID:           "dod-deliverable-coverage-repair-supplemental-missing-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true, "evidenceRole": "primary_state_change"},
							{"id": "validation_report", "required": true, "evidenceRole": "supplemental_evidence"},
							{"id": "coverage_map", "required": true, "evidenceRole": "supplemental_evidence"},
							{"id": "version_control_snapshot", "required": true, "evidenceRole": "supplemental_evidence"},
							{"id": "user_deliverable", "required": true, "evidenceRole": "supplemental_evidence"},
						},
						"evidenceRequirements": []map[string]any{
							{
								"id":                "source_change",
								"scope":             "deliverable",
								"acceptedStepKinds": []string{"patch/apply"},
								"requireDirect":     true,
							},
							{
								"id":                "version_control_snapshot",
								"scope":             "deliverable",
								"acceptedStepKinds": []string{"git/write"},
								"acceptedOperations": []string{
									"commit",
								},
								"requireDirect": true,
							},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-deliverable-coverage-repair-supplemental-missing-normalize/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageAcceptance,
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "DoD items passed, but supplemental deliverable coverage was reported missing.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-deliverable-coverage-repair-supplemental-missing-normalize/graph-001/test"},
			}},
			"deliverableCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{
					{
						"id":           "source_change",
						"item":         "source_change",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-repair-supplemental-missing-normalize/graph-001/patch"},
					},
					{
						"id":           "version_control_snapshot",
						"item":         "version_control_snapshot",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-deliverable-coverage-repair-supplemental-missing-normalize/graph-001/commit"},
					},
					{
						"id":    "validation_report",
						"item":  "coverage_map",
						"state": "missing",
					},
					{
						"id":    "coverage_map",
						"item":  "version_control_snapshot",
						"state": "missing",
					},
					{
						"id":    "user_deliverable",
						"item":  "user_deliverable",
						"state": "missing",
					},
				},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if satisfied := assessment["satisfied"]; satisfied != true {
		t.Fatalf("dodAssessment.satisfied = %#v, want true", satisfied)
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) != 5 {
		t.Fatalf("deliverableCoverage.items = %+v, want five required deliverables", items)
	}
	wantIDs := []string{"source_change", "version_control_snapshot", "validation_report", "coverage_map", "user_deliverable"}
	for idx, want := range wantIDs {
		if got := stringFromMap(items[idx], "id"); got != want {
			t.Fatalf("deliverableCoverage.items[%d].id = %q, want %q; items=%+v", idx, got, want, items)
		}
		if got := stringFromMap(items[idx], "state"); got != "satisfied" {
			t.Fatalf("deliverableCoverage.items[%d].state = %q, want satisfied", idx, got)
		}
		if len(evidenceRefsFromValue(items[idx]["evidenceRefs"])) == 0 && len(stringSliceFromValue(items[idx]["artifactRefs"])) == 0 {
			t.Fatalf("deliverableCoverage.items[%d] lacks refs: %+v", idx, items[idx])
		}
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputMapsRepairGoalCoverageByEquivalentText(t *testing.T) {
	goalText := "针对**非流式响应**，解析返回包中的 `usage` 字段。"
	requirementID := goalCoverageRequirementID(1, normalizeDoDCoverageText(goalText))
	run := &runmodel.Run{
		ID:              "run-goal-coverage-equivalent-normalize",
		WorkspaceID:     "ws-goal-coverage-equivalent-normalize",
		DoDID:           "dod-goal-coverage-equivalent-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": goalText,
			},
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-equivalent-normalize/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": requirementID,
				"raw":               goalText,
			}},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-equivalent-normalize/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"item":         "Token 自动审计-非流式响应：解析返回包中的 usage 字段。",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-equivalent-normalize/graph-001/001-test"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("goalCoverage.items = %+v, want one normalized item", coverage["items"])
	}
	if got := stringFromMap(items[0], "goalRequirementId"); got != requirementID {
		t.Fatalf("goalRequirementId = %q, want repair input id", got)
	}
}

func TestNormalizeDoDAssessmentEvidenceRefsBackfillsEmptySatisfiedItemsFromConcreteRunEvidence(t *testing.T) {
	runID := common.RunID("run-assessment-empty-evidence-normalize")
	verifyStepID := common.StepID(runID + "/graph-001/002-run-tests")
	zeroWorkStepID := common.StepID(runID + "/graph-001/003-run-empty-tests")
	catalog := completionEvidenceRefCatalog{
		runID: runID,
		steps: map[string]*runmodel.Step{
			string(verifyStepID): {
				ID:    verifyStepID,
				RunID: runID,
				Kind:  "shell/command",
				State: common.StepStateCompleted,
				Output: map[string]any{
					"command":  "go",
					"args":     []string{"test", "./..."},
					"exitCode": 0,
					"stdout":   "ok example 0.1s",
				},
			},
			string(zeroWorkStepID): {
				ID:    zeroWorkStepID,
				RunID: runID,
				Kind:  "shell/command",
				State: common.StepStateCompleted,
				Output: map[string]any{
					"command":         "go",
					"args":            []string{"test", "./...", "-run", "NoMatch"},
					"exitCode":        0,
					"stdout":          "ok example [no tests to run]",
					"zeroWork":        true,
					"evidenceQuality": string(common.EvidenceQualityInconclusive),
				},
			},
		},
		stepOrder: map[string]int{
			string(verifyStepID):   1,
			string(zeroWorkStepID): 2,
		},
		aliases:                   map[string]string{},
		patches:                   map[string]bool{},
		stepScope:                 map[string]string{},
		latestUsableStepByScope:   map[string]string{},
		latestUsableOrderByScope:  map[string]int{},
		supersededFailureStepRefs: map[string]string{},
	}
	for ref := range catalog.steps {
		catalog.addAlias(ref, ref)
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "Requirements are satisfied.",
		"items": []map[string]any{{
			"item":         "runtime behavior verified",
			"state":        "satisfied",
			"evidenceRefs": []string{},
		}},
		"goalCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"item":         "goal behavior",
				"state":        "",
				"evidenceRefs": []string{},
			}},
		},
		"deliverableCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"item":         "validation_report",
				"state":        "",
				"evidenceRefs": []string{},
			}},
		},
	}

	normalized, changed := normalizeDoDAssessmentEvidenceRefs(context.Background(), nil, RuntimeEvidenceRepositories{}, catalog, assessment)
	if !changed {
		t.Fatalf("normalizeDoDAssessmentEvidenceRefs changed=false, want fallback evidence normalization")
	}
	items, _ := doDAssessmentItemMaps(normalized["items"])
	if got := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(verifyStepID)}) {
		t.Fatalf("DoD evidenceRefs = %+v, want concrete verification step only", got)
	}
	goalCoverage := mapFromValue(normalized["goalCoverage"])
	goalItems, _ := doDAssessmentItemMaps(goalCoverage["items"])
	if got := stringFromMap(goalItems[0], "state"); got != "satisfied" {
		t.Fatalf("goalCoverage item state = %q, want inferred satisfied", got)
	}
	if got := evidenceRefsFromValue(goalItems[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(verifyStepID)}) {
		t.Fatalf("goalCoverage evidenceRefs = %+v, want concrete verification step only", got)
	}
	deliverableCoverage := mapFromValue(normalized["deliverableCoverage"])
	deliverableItems, _ := doDAssessmentItemMaps(deliverableCoverage["items"])
	if got := stringFromMap(deliverableItems[0], "state"); got != "satisfied" {
		t.Fatalf("deliverableCoverage item state = %q, want inferred satisfied", got)
	}
	if got := evidenceRefsFromValue(deliverableItems[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(verifyStepID)}) {
		t.Fatalf("deliverableCoverage evidenceRefs = %+v, want concrete verification step only", got)
	}
}

func TestNormalizeDoDAssessmentEvidenceRefsMovesTraceableArtifactRefsIntoEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-assessment-artifact-evidence-normalize")
	patchStepID := common.StepID(runID + "/graph-001/001-state-change")
	verifyStepID := common.StepID(runID + "/graph-001/002-verify")
	stepRepo := newCountingStepRepository()
	for _, step := range []*runmodel.Step{
		{
			ID:    patchStepID,
			RunID: runID,
			Kind:  "patch/apply",
			State: common.StepStateCompleted,
			Input: map[string]any{
				"coverageTargets": []string{"produced_asset"},
			},
			Output: map[string]any{
				"status": "completed",
			},
		},
		{
			ID:    verifyStepID,
			RunID: runID,
			Kind:  "shell/command",
			State: common.StepStateCompleted,
			Input: map[string]any{
				"coverageTargets": []string{"produced_asset"},
			},
			Output: map[string]any{
				"status": "completed",
			},
		},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("Create step %s: %v", step.ID, err)
		}
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-assessment-artifact-evidence-normalize",
		DoDID:           "dod-assessment-artifact-evidence-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"scope":                 "deliverable",
							"id":                    "produced_asset",
							"acceptedStepKinds":     []string{"patch/apply"},
							"requireDirectEvidence": true,
						}},
					},
				},
			},
		},
	}
	catalog, err := completionEvidenceRefCatalogForRun(ctx, stepRepo, RuntimeEvidenceRepositories{}, run)
	if err != nil {
		t.Fatalf("completionEvidenceRefCatalogForRun: %v", err)
	}
	artifactRef := string(runID) + "/artifact/result-summary"
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "Requirements are satisfied.",
		"deliverableCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"id":           "produced_asset",
				"state":        "satisfied",
				"evidenceRefs": []string{string(verifyStepID)},
				"artifactRefs": []string{string(patchStepID), artifactRef},
			}},
		},
	}

	normalized, changed := normalizeDoDAssessmentEvidenceRefs(ctx, stepRepo, RuntimeEvidenceRepositories{}, catalog, assessment)
	if !changed {
		t.Fatalf("normalizeDoDAssessmentEvidenceRefs changed=false, want misplaced artifact evidence normalization")
	}
	deliverableCoverage := mapFromValue(normalized["deliverableCoverage"])
	items, ok := doDAssessmentItemMaps(deliverableCoverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("deliverableCoverage.items = %+v, want one item", deliverableCoverage["items"])
	}
	if got := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(verifyStepID), string(patchStepID)}) {
		t.Fatalf("evidenceRefs = %+v, want verification plus moved patch evidence", got)
	}
	if got := stringSliceFromValue(items[0]["artifactRefs"]); !reflect.DeepEqual(got, []string{artifactRef}) {
		t.Fatalf("artifactRefs = %+v, want only true artifact refs preserved", got)
	}
	if err := validateDoDAssessmentEvidenceKindRequirements(ctx, stepRepo, nil, run, normalized); err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceKindRequirements: %v", err)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputUsesEvidenceCandidatesWhenRefsAreNotTraceable(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-normalize-candidate",
		WorkspaceID:     "ws-goal-coverage-normalize-candidate",
		DoDID:           "dod-goal-coverage-normalize-candidate",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "publish completion evidence",
			},
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-normalize-candidate/repair-001-of-graph-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-candidate",
				"raw":               "publish completion evidence",
				"evidenceCandidates": []map[string]any{{
					"stepId": "run-goal-coverage-normalize-candidate/graph-001/002-verify",
				}},
			}},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-normalize-candidate/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"state":        "satisfied",
					"evidenceRefs": []string{"summary.md"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment, _, err := validatedCompletionDoDAssessment(run, normalized)
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("goalCoverage.items = %+v, want one normalized item", coverage["items"])
	}
	refs := evidenceRefsFromValue(items[0]["evidenceRefs"])
	if len(refs) != 1 || refs[0] != "run-goal-coverage-normalize-candidate/graph-001/002-verify" {
		t.Fatalf("evidenceRefs = %+v, want fallback candidate step ref", refs)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsNearEquivalentGoalCoverageText(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-near-equivalent",
		WorkspaceID:     "ws-goal-coverage-near-equivalent",
		DoDID:           "dod-goal-coverage-near-equivalent",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "* **统一协议转换**：作为本地网关，接收客户端请求，并透明转发至目标供应商（OpenAI, DeepSeek, Anthropic 等）。",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal coverage passed.",
			"items": []map[string]any{{
				"item":         "build passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-near-equivalent/graph-001/verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"item":         "统一协议转换：作为本地网关接收客户端请求并透明转发到目标供应商（OpenAI/DeepSeek/Anthropic）。",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-near-equivalent/graph-001/verify"},
				}},
			},
		},
	}
	if _, _, err := validatedCompletionDoDAssessment(run, output); err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputInfersSatisfiedStateFromParentAndCandidates(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-normalize-parent-satisfied",
		WorkspaceID:     "ws-goal-coverage-normalize-parent-satisfied",
		DoDID:           "dod-goal-coverage-normalize-parent-satisfied",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-normalize-parent-satisfied/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-parent-satisfied",
				"raw":               "publish completion evidence",
				"evidenceCandidates": []map[string]any{{
					"stepId": "run-goal-coverage-normalize-parent-satisfied/graph-001/002-verify",
				}},
			}},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-normalize-parent-satisfied/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"item":         "publish completion evidence",
					"evidenceRefs": []string{"summary.md"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	assessment := doDAssessmentFromOutput(normalized)
	coverage := mapFromValue(assessment["goalCoverage"])
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != 1 {
		t.Fatalf("goalCoverage.items = %+v, want one normalized item", coverage["items"])
	}
	if got := stringFromMap(items[0], "state"); got != "satisfied" {
		t.Fatalf("goalCoverage item state = %q, want inferred satisfied", got)
	}
	if got := stringFromMap(items[0], "goalRequirementId"); got != "goal-req-001-parent-satisfied" {
		t.Fatalf("goalRequirementId = %q, want copied requirement id", got)
	}
	refs := evidenceRefsFromValue(items[0]["evidenceRefs"])
	if len(refs) != 1 || refs[0] != "run-goal-coverage-normalize-parent-satisfied/graph-001/002-verify" {
		t.Fatalf("evidenceRefs = %+v, want fallback candidate step ref", refs)
	}
}

func TestNormalizeCompletionGateDoDAssessmentOutputDoesNotMaskNonAssessmentOnlyOutput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-normalize-scope",
		WorkspaceID:     "ws-goal-coverage-normalize-scope",
		DoDID:           "dod-goal-coverage-normalize-scope",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "record externally visible state change",
			},
			"dod": map[string]any{
				"items": []string{"verification passes"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-goal-coverage-normalize-scope/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"repairObjective": "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-neutral",
				"raw":               "record externally visible state change",
			}},
		},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-normalize-scope/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-normalize-scope/graph-001/001-change"},
				}},
			},
		},
	}

	normalized := normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	_, _, err := validatedCompletionDoDAssessment(run, normalized)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want unmasked invalid goal coverage", err)
	}
}
