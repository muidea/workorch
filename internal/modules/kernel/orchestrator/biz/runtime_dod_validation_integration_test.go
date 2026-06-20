package orchestrator

import (
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidatedCompletionDoDAssessmentRejectsMissingCurrentDoDCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-coverage-validate",
		WorkspaceID:     "ws-dod-coverage-validate",
		DoDID:           "dod-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"### Acceptance",
					"- [ ] **build passes**",
					"- [ ] **error path covered**",
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "build passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-validate/step-1"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "does not cover all current DoD items") || !strings.Contains(err.Error(), "error path covered") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing DoD coverage", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("validatedCompletionDoDAssessment err = %T, want structured contract violation", err)
	}
	if violation.violationType != contractViolationCoverageMapIncomplete || violation.rule != contractViolationRuleCoverageMapIncomplete {
		t.Fatalf("contract violation = %+v, want coverage map incomplete", violation)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingGoalCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-validate",
		WorkspaceID:     "ws-goal-coverage-validate",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持同时代理不同类型的 LLM\n支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-validate/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.goalCoverage is required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing Goal coverage", err)
	}
}

func TestNormalizedRunGoalCoverageItemsIncludeStableRequirementIDs(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-goal-coverage-stable-id",
		WorkspaceID: "ws-goal-coverage-stable-id",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持流式响应 Token 统计\n支持配置化停用能力",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}

	items := normalizedRunGoalCoverageItems(run)
	if len(items) != 2 {
		t.Fatalf("goal coverage items = %+v, want two uncovered Goal leaves", items)
	}
	for _, item := range items {
		if !strings.HasPrefix(item.id, "goal-req-") {
			t.Fatalf("goal item id = %q, want stable goal requirement id", item.id)
		}
	}
	again := normalizedRunGoalCoverageItems(run)
	if len(again) != len(items) || again[0].id != items[0].id || again[1].id != items[1].id {
		t.Fatalf("goal item ids = %+v then %+v, want deterministic ids", items, again)
	}
}

func TestNormalizedRunGoalCoverageItemsIgnoreMarkdownTableHeader(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-goal-coverage-table",
		WorkspaceID: "ws-goal-coverage-table",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": strings.Join([]string{
					"| Capability | Parameters | Purpose |",
					"| --- | --- | --- |",
					"| alpha | path | resolve target |",
					"| beta | content | update target |",
				}, "\n"),
			},
		},
	}

	items := normalizedRunGoalCoverageItems(run)
	if len(items) != 2 {
		t.Fatalf("goal coverage items = %+v, want table data rows only", items)
	}
	if got := items[0].raw; got != "| alpha | path | resolve target |" {
		t.Fatalf("first goal coverage raw = %q, want alpha data row", got)
	}
	if got := items[1].raw; got != "| beta | content | update target |" {
		t.Fatalf("second goal coverage raw = %q, want beta data row", got)
	}
	for _, item := range items {
		if strings.Contains(item.raw, "Capability | Parameters") {
			t.Fatalf("table header was treated as goal coverage item: %+v", item)
		}
		if !strings.HasPrefix(item.id, "goal-req-") {
			t.Fatalf("goal item id = %q, want stable goal requirement id", item.id)
		}
	}
}

func TestValidatedCompletionDoDAssessmentRejectsImplicitGoalCoverageWithoutGoalCoverageMap(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-implicit",
		WorkspaceID:     "ws-goal-coverage-implicit",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "goal text appears in a DoD item but no goalCoverage map exists",
			"items": []map[string]any{
				{
					"item":         "支持通过配置单独停用某个 LLM",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-implicit/block/build"},
				},
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-implicit/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.goalCoverage is required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want explicit goalCoverage requirement", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsGoalCoverageWithoutEvidenceRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-no-evidence",
		WorkspaceID:     "ws-goal-coverage-no-evidence",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "goal coverage lacks evidence",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-no-evidence/block/build"},
				},
			},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"goal":  "支持通过配置单独停用某个 LLM",
						"state": "satisfied",
					},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "satisfied dodAssessment.goalCoverage item requires evidenceRefs") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want goalCoverage evidenceRefs requirement", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("validatedCompletionDoDAssessment err = %T, want structured contract violation", err)
	}
	if violation.violationType != contractViolationDoDAssessmentEvidenceMissing || violation.rule != contractViolationRuleDoDAssessmentEvidenceMissing {
		t.Fatalf("contract violation = %+v, want missing evidence refs", violation)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsExplicitGoalCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-complete",
		WorkspaceID:     "ws-goal-coverage-complete",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持同时代理不同类型的 LLM\n支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, summary, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and Goal coverage passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-complete/block/build"},
				},
			},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"goal":         "支持同时代理不同类型的 LLM",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-goal-coverage-complete/block/patch"},
					},
					{
						"goal":         "支持通过配置单独停用某个 LLM",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-goal-coverage-complete/block/test-disabled"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if summary != "DoD and Goal coverage passed" {
		t.Fatalf("summary = %q, want Goal-aware summary", summary)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsGoalCoverageWithSectionPrefix(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-section-prefix",
		WorkspaceID:     "ws-goal-coverage-section-prefix",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "针对非流式响应，解析返回包中的 usage 字段。",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, summary, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and Goal coverage passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-section-prefix/block/build"},
				},
			},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"item":         "Token 自动审计：针对非流式响应，解析返回包中的 usage 字段。",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-goal-coverage-section-prefix/block/test"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if summary != "DoD and Goal coverage passed" {
		t.Fatalf("summary = %q, want Goal-aware summary", summary)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingRequiredAcceptanceDimension(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-dimension-missing",
		WorkspaceID:     "ws-required-dimension-missing",
		DoDID:           "dod-required-dimension",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"acceptanceDimensions": []map[string]any{
							{"id": "runtime_entrypoint", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-missing/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "acceptanceDimensions missing required dimensions: runtime_entrypoint") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing acceptance dimension", err)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsRequiredAcceptanceDimension(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-dimension-complete",
		WorkspaceID:     "ws-required-dimension-complete",
		DoDID:           "dod-required-dimension",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"acceptanceDimensions": []map[string]any{
							{"id": "runtime_entrypoint", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD and required dimension passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-complete/block/build"},
				},
			},
			"acceptanceDimensions": []map[string]any{
				{
					"id":           "runtime_entrypoint",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-complete/block/entrypoint"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingRequiredDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-deliverable-missing",
		WorkspaceID:     "ws-required-deliverable-missing",
		DoDID:           "dod-required-deliverable",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "single_binary", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-deliverable-missing/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "deliverableCoverage missing required deliverables: single_binary") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing deliverable coverage", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validatedCompletionDoDAssessment err = %T, want runtime diagnostics", err)
	}
	diagnostics := diagnostic.RuntimeFailureDiagnostics()
	if diagnostics["kind"] != failureClassDoDCoverageMissing {
		t.Fatalf("runtimeDiagnostics = %+v, want DoD coverage correction kind", diagnostics)
	}
	missing := mapSliceFromValue(diagnostics["missingDeliverableCoverage"])
	if len(missing) != 1 || missing[0]["id"] != "single_binary" {
		t.Fatalf("missingDeliverableCoverage = %+v, want single_binary", diagnostics["missingDeliverableCoverage"])
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsRequiredDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-deliverable-complete",
		WorkspaceID:     "ws-required-deliverable-complete",
		DoDID:           "dod-required-deliverable",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "single_binary", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD and required deliverable passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-deliverable-complete/block/build"},
				},
			},
			"deliverableCoverage": map[string]any{
				"items": []map[string]any{
					{
						"id":           "single_binary",
						"state":        "satisfied",
						"artifactRefs": []string{"run-required-deliverable-complete/artifact/result-summary"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsRequiredDeliverableCoverageItemField(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-deliverable-item-field",
		WorkspaceID:     "ws-required-deliverable-item-field",
		DoDID:           "dod-required-deliverable-item-field",
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
							{"id": "user_deliverable", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD and required deliverables passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-deliverable-item-field/block/build"},
				},
			},
			"deliverableCoverage": map[string]any{
				"items": []map[string]any{
					{
						"item":         "source_change",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-required-deliverable-item-field/block/patch"},
					},
					{
						"item":         "validation_report",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-required-deliverable-item-field/block/test"},
					},
					{
						"item":         "coverage_map",
						"state":        "satisfied",
						"artifactRefs": []string{"run-required-deliverable-item-field/artifact/result-summary"},
					},
					{
						"item":         "user_deliverable",
						"state":        "satisfied",
						"artifactRefs": []string{"run-required-deliverable-item-field/artifact/manifest"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsOpenEvidenceGaps(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-open-evidence-gap",
		WorkspaceID:     "ws-open-evidence-gap",
		DoDID:           "dod-open-evidence-gap",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed but evidence gap remains",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-open-evidence-gap/block/build"},
				},
			},
			"evidenceGaps": []map[string]any{
				{"id": "missing-error-path", "reason": "error branch not verified"},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.evidenceGaps must be empty") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want open evidence gap rejection", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsRequiredBranchCoverageWithoutEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-branch-coverage-no-evidence",
		WorkspaceID:     "ws-branch-coverage-no-evidence",
		DoDID:           "dod-branch-coverage-no-evidence",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed but branch evidence missing",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-branch-coverage-no-evidence/block/build"},
				},
			},
			"branchCoverage": map[string]any{
				"items": []map[string]any{
					{
						"id":       "error_path",
						"required": true,
						"state":    "satisfied",
					},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "satisfied dodAssessment.branchCoverage item") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want branch coverage evidence rejection", err)
	}
}

func TestValidatedCompletionDoDAssessmentIgnoresGoalParentItems(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-parent-coverage",
		WorkspaceID:     "ws-goal-parent-coverage",
		DoDID:           "dod-goal-parent-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "### Goal\n- Token 自动审计：\n  - 非流式 usage 字段解析",
			},
			"dod": map[string]any{
				"items": []string{"非流式 usage 字段解析"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal parent is a grouping label; leaf DoD is covered.",
			"items": []map[string]any{{
				"item":         "非流式 usage 字段解析",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-parent-coverage/block/test"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}
