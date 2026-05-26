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
