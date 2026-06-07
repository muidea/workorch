package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGoalCoverageRequiredFailureDispatchesAssessmentOnlyRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-required-repair",
		WorkspaceID:     "ws-goal-coverage-required-repair",
		DoDID:           "dod-goal-coverage-required-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "查询最新天气信息",
			},
			"dod": map[string]any{
				"items": []string{"给出浙江杭州滨江区最新的天气信息"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "已获取天气信息。",
			"items": []map[string]any{{
				"item":         "给出浙江杭州滨江区最新的天气信息",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-required-repair/graph-001/001-fetch"},
			}},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "dodAssessment.goalCoverage is required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want goal coverage required error", err)
	}
	failed := &runmodel.Step{
		ID:    "run-goal-coverage-required-repair/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeySoulStage:    soulStageValidation,
		},
		Output: runtimeFailureOutput(outputWithDoDAssessmentDiagnostics(output), err),
	}
	attachRuntimeFailureClassification(failed.Output, classifyRuntimeFailure(failed, failed.Output, "runtime completion gate failed", err))

	input := (&RuntimeOrchestrator{steps: memory.NewStepRepository()}).runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, err)
	if input[stepInputKeyAssessmentOnly] != true ||
		input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input[stepInputKeySoulStage] != soulStageValidation ||
		input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repair input = %+v, want assessment-only goal coverage correction", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 ||
		stringFromMap(missing[0], "goalRequirementId") == "" ||
		missing[0]["raw"] != "查询最新天气信息" {
		t.Fatalf("missingGoalCoverage = %+v, want Goal requirement diagnostics", input["missingGoalCoverage"])
	}
}

func TestGoalCoverageItemShapeFailureDispatchesAssessmentOnlyRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-shape-repair",
		WorkspaceID:     "ws-goal-coverage-shape-repair",
		DoDID:           "dod-goal-coverage-shape-repair",
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
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal and DoD requirements are covered.",
			"items": []map[string]any{{
				"item":         "verification passes",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-coverage-shape-repair/graph-001/002-verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-shape-repair/graph-001/002-verify"},
				}},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "dodAssessment.goalCoverage item and state are required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want goal coverage item shape error", err)
	}
	failed := &runmodel.Step{
		ID:    "run-goal-coverage-shape-repair/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeySoulStage:    soulStageValidation,
		},
		Output: runtimeFailureOutput(output, err),
	}
	attachRuntimeFailureClassification(failed.Output, classifyRuntimeFailure(failed, failed.Output, "runtime completion gate failed", err))
	if got := stringFromMap(failed.Output, failureOutputKeyClass); got != failureClassDoDCoverageMissing {
		t.Fatalf("failureClass = %q, want %q", got, failureClassDoDCoverageMissing)
	}

	input := (&RuntimeOrchestrator{steps: memory.NewStepRepository()}).runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, err)
	if input[stepInputKeyAssessmentOnly] != true ||
		input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != "complete_dod_goal_coverage_assessment" ||
		boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want non-state-changing assessment-only coverage correction", input)
	}
}

func TestValidateLLMExecutionOutputContractRejectsGraphNodeOutsideCurrentSoulToolKinds(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-soul-tool-kind-contract",
		WorkspaceID:     "ws-soul-tool-kind-contract",
		DoDID:           "dod-soul-tool-kind-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"soulRouting": map[string]any{
				"bindings": map[string]any{
					soulStageExecution: map[string]any{
						"soul": map[string]any{
							"id":               "soul-mcp-only",
							"allowedToolKinds": []string{"mcp/tool_call"},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-soul-tool-kind-contract/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageRepair,
			stepInputKeySoulStage:    soulStageExecution,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{
			ID:   "fallback-shell",
			Kind: "shell/command",
			Input: map[string]any{
				"command": "curl",
			},
		}}},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "allowedToolKinds") ||
		!strings.Contains(err.Error(), "shell/command") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want current soul tool kind rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsGraphMCPServerOutsideCurrentSoulRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-soul-mcp-server-contract",
		WorkspaceID:     "ws-soul-mcp-server-contract",
		DoDID:           "dod-soul-mcp-server-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"soulRouting": map[string]any{
				"bindings": map[string]any{
					soulStageExecution: map[string]any{
						"soul": map[string]any{
							"id":                   "soul-weather-only",
							"allowedToolKinds":     []string{"mcp/tool_call"},
							"allowedMCPServerRefs": []string{"weather"},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-soul-mcp-server-contract/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageRepair,
			stepInputKeySoulStage:    soulStageExecution,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{
			ID:   "other-mcp",
			Kind: "mcp/tool_call",
			Input: map[string]any{
				"serverId": "other",
				"tool":     "weather.current",
			},
		}}},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "allowedMCPServerRefs") ||
		!strings.Contains(err.Error(), "other") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want current soul MCP server rejection", err)
	}
}
