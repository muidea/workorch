package orchestrator

import (
	"errors"
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
