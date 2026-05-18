package orchestrator

import (
	"context"
	"errors"
	"reflect"
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

	normalized, changed := (&RuntimeOrchestrator{}).normalizeDoDAssessmentEvidenceRefs(context.Background(), catalog, assessment)
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
