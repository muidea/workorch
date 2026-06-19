package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLLMExecutionGraphNodeReturnedOnlyGraphViolatesContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-execution-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-execution-contract/block/implement",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyGraphNodeID:      "implement",
			stepInputKeyRuntimeStage:     runtimeStageExecution,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageExecution,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "nested-plan",
				"kind":  "llm/workspace_execution",
				"input": map[string]any{"task": "plan again"},
			}},
		},
	}
	if !llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = false, want contract violation")
	}
	output[stepOutputKeyPatches] = []map[string]any{{
		"patchType": "file_write",
		"targetRef": "main.go",
		"content":   "package main\n",
	}}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = true, want concrete patch output allowed")
	}
}

func TestLLMExecutionGraphNodeReturnedOnlyGraphAllowsConcretePatchGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-execution-concrete-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-execution-concrete-graph/graph-001/006-plan-impl",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyGraphNodeID:       "plan-impl",
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"completionState": "continue",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "fix-config",
					"kind": "patch/apply",
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
				{
					"id":        "verify",
					"kind":      "shell/command",
					"dependsOn": []string{"fix-config"},
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./..."},
					},
				},
			},
		},
	}

	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = true, want concrete patch graph allowed")
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract: %v, want concrete patch graph allowed", err)
	}
}

func TestRuntimeOrchestrator_DispatchesConcreteGraphReturnedByExecutionLLMNode(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	workspaceID := common.WorkspaceID("ws-execution-concrete-graph")
	runID := common.RunID("run-execution-concrete-graph-dispatch")
	rootStepID := common.StepID(string(runID) + "/step-1")
	llmStepID := common.StepID(string(runID) + "/graph-001/006-plan-impl")
	deliverableDirectory := t.TempDir()
	nestedGraph := map[string]any{
		"nodes": []map[string]any{
			{
				"id":   "fix-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "config.yaml",
					"content":   "enabled: true\n",
				},
			},
			{
				"id":        "verify",
				"kind":      "shell/command",
				"dependsOn": []string{"fix-config"},
				"input": map[string]any{
					"command": "go",
					"args":    []string{"test", "./..."},
				},
			},
		},
	}
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch request.StepID {
		case llmStepID:
			return map[string]any{
				"status":          "completed",
				"completionState": "continue",
				"executionGraph":  nestedGraph,
			}, nil
		case common.StepID(string(runID) + "/graph-002/001-fix-config"):
			return map[string]any{"status": "completed"}, nil
		case common.StepID(string(runID) + "/graph-002/002-verify"):
			return map[string]any{"status": "completed"}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     rootStepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{"executionGraph": map[string]any{"nodes": []map[string]any{{"id": "plan-impl", "kind": llmWorkspaceExecutionStepKind}}}},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}, llmStepID, llmWorkspaceExecutionStepKind, map[string]any{
		stepInputKeyGraphNodeID:       "plan-impl",
		stepInputKeyGraphRootStepID:   string(rootStepID),
		stepInputKeyRuntimeStage:      runtimeStageExecution,
		"stateChangingOutputRequired": true,
	}, nil); err != nil {
		t.Fatalf("dispatch execution llm node: %v", err)
	}

	for _, id := range []common.StepID{
		llmStepID,
		common.StepID(string(runID) + "/graph-002/001-fix-config"),
		common.StepID(string(runID) + "/graph-002/002-verify"),
	} {
		step, err := stepRepo.Get(ctx, id)
		if err != nil {
			t.Fatalf("get step %s: %v", id, err)
		}
		if step.State != common.StepStateCompleted {
			t.Fatalf("step %s state = %s, want completed", id, step.State)
		}
		if id != llmStepID && step.Input[stepInputKeyGraphRootStepID] != string(llmStepID) {
			t.Fatalf("step %s graph root = %v, want nested root %s", id, step.Input[stepInputKeyGraphRootStepID], llmStepID)
		}
	}
}

func TestValidateStateChangingExecutionGraphTreatsWorkspacePlanAliasAsLLMAncestor(t *testing.T) {
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{
			ID:   "plan-impl",
			Kind: llmcommon.StepKindWorkspacePlan,
		},
		{
			ID:        "write-config",
			Kind:      "patch/apply",
			DependsOn: []string{"plan-impl"},
			Input: map[string]any{
				"patchType": "file_write",
				"targetRef": "config.json",
				"content":   "{}",
			},
		},
	}}

	err := validateStateChangingExecutionGraph(graph)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "llm/workspace_execution before patch") {
		t.Fatalf("validateStateChangingExecutionGraph err = %v, want canonical llm ancestor rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsImplementationShellOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-implementation-state-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-implementation-state-contract/block/implement",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			stepInputKeyGraphNodeID:       "unit-001",
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"completionState": "continue",
		"shellCommands": []map[string]any{{
			"command": "go",
			"args":    []string{"test", "./..."},
		}},
		"nextStep": "run-implementation-state-contract/block/test",
		"nextKind": shellcommon.StepKindCommand,
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "no state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing output rejection", err)
	}

	output["patchRequests"] = []map[string]any{{
		"patchType": "file_write",
		"targetRef": "config.yaml",
		"content":   "providers: []\n",
	}}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract with patch err = %v, want allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairReadOnlyGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-contract/zz-llm-repair-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph without patch/apply") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing repair read-only graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairSatisfiedAssessmentOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-assessment-only",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-assessment-only/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "claimed state change without executable evidence",
			"items": []map[string]any{{
				"item":         "required state change",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-state-changing-repair-assessment-only/graph-001/001-claimed"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "dodAssessment without executable state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want satisfied assessment-only rejection", err)
	}
	output[stepOutputKeyDoDAssessment].(map[string]any)["satisfied"] = false
	err = validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "dodAssessment without executable state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract failed assessment err = %v, want failed assessment-only rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsDeclaredStateChangingShellGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-shell-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-shell-graph/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "prepare-state",
					"kind": "shell/command",
					"input": map[string]any{
						"command":               "tool",
						"args":                  []string{"sync"},
						"stateChanging":         true,
						"targetRef":             "manifest.lock",
						"stateChangingReason":   "refresh derived state from current workspace inputs",
						"relatedFailureTargets": []string{"manifest.lock"},
					},
				},
				{
					"id":        "verify-state",
					"kind":      "shell/command",
					"dependsOn": []string{"prepare-state"},
					"input": map[string]any{
						"command":           "tool",
						"args":              []string{"verify"},
						"targetRef":         "manifest.lock",
						"verificationScope": "manifest.lock",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want declared state-changing shell graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsUndeclaredStateChangingShellGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-undeclared-shell-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-undeclared-shell-graph/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"shell/command"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "run-command",
				"kind":  "shell/command",
				"input": map[string]any{"command": "tool", "args": []string{"sync"}},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "auditable state-changing operation") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want undeclared shell graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingNodeDuringEvidenceAcquisition(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-acquisition-state-changing-forbidden",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-acquisition-state-changing-forbidden/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "mutate-while-acquiring-evidence",
				"kind": "shell/command",
				"input": map[string]any{
					"command":             "tool",
					"args":                []string{"sync"},
					"stateChanging":       true,
					"stateChangingReason": "mutate workspace state while acquiring evidence",
					"targetRef":           "workspace-state",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "concrete failure evidence acquisition must return bounded") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want evidence acquisition state-changing rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingShellGraphWithoutBoundedVerification(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-shell-no-verification",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-shell-no-verification/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"shell/command"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "prepare-state",
				"kind": "shell/command",
				"input": map[string]any{
					"command":       "tool",
					"args":          []string{"sync"},
					"stateChanging": true,
					"targetRef":     "manifest.lock",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "auditable state-changing operation") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want missing bounded verification rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairPatchGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-patch-graph/zz-llm-repair-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "write_config",
					"kind": "patch/apply",
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
				{
					"id":        "inspect_after_patch",
					"kind":      "file/read",
					"dependsOn": []string{"write_config"},
					"input":     map[string]any{"path": "config.yaml"},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing patch graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairUnrelatedPatchTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-unrelated-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-unrelated-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "main_test.go", "exists": true, "contentHash": "hash-main-test"},
			},
			"missingDoDItems": []map[string]any{
				{"item": "gateway request path is covered", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "write-marker",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "fix-marker.txt",
					"content":   "fixed\n",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "not related to failure target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want unrelated repair target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairRelevantPatchTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-relevant-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-relevant-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "main_test.go", "exists": true, "contentHash": "hash-main-test"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main-test",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":       "unified_diff",
					"targetRef":       "main_test.go",
					"coverageTargets": []string{"gateway request path is covered"},
					"diff":            "--- a/main_test.go\n+++ b/main_test.go\n@@ -1 +1 @@\n-old\n+new\n",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want relevant repair patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsDeclaredRelatedFailureTargetPatch(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-related-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-related-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=config_contract_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=config_contract_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "config_contract_test.go", "exists": true, "contentHash": "hash-config-contract-test"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":             "file_write",
					"targetRef":             "config.yaml",
					"content":               "providers: []\n",
					"relatedFailureTargets": []string{"config_contract_test.go"},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want declared related failure target patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsMismatchedDeclaredRelatedFailureTargetPatch(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-mismatched-related-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-mismatched-related-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=config_contract_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=config_contract_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "config_contract_test.go", "exists": true, "contentHash": "hash-config-contract-test"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":             "file_write",
					"targetRef":             "config.yaml",
					"content":               "providers: []\n",
					"relatedFailureTargets": []string{"unrelated_test.go"},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "not related to failure target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want mismatched related failure target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairDependencyMetadataTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-dependency-metadata-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-dependency-metadata-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1",
			"failedStepOutput": map[string]any{
				"stderr": "missing checksum entry for module providing package example.org/feature",
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-dependency-metadata",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "deps.sum",
					"content":   "example.org/feature v1.2.3 h1:abc\n",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want related dependency metadata target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairUnrelatedDependencyMetadataTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-unrelated-dependency-metadata-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-unrelated-dependency-metadata-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=src/main.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=src/main.go|exitCode=1",
			"failedStepOutput": map[string]any{
				"stderr": "missing checksum entry for module providing package example.org/feature",
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-unrelated-dependency-metadata",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "deps.sum",
					"content":   "example.org/feature v1.2.3 h1:abc\n",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "not related to failure target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want unrelated dependency metadata target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsCoverageGapPatchWithoutCoverageTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-coverage-targets",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-coverage-targets/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			"missingDoDItems": []map[string]any{
				{"item": "gateway request path is covered", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main-test",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "main_test.go",
					"content":   "package main\n",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "coverageTargets") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsCoverageGapPatchWithoutStateChangingFlag(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-coverage-gap-targets-without-state-changing-flag",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-coverage-gap-targets-without-state-changing-flag/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			stepInputKeySoulStage:             soulStageValidation,
			"missingDoDItems": []map[string]any{
				{"item": "target alpha requirement", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "apply-unbound-change",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "artifact.txt",
					"content":   "updated\n",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "coverageTargets") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingPatchWithoutCoverageGapTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-without-coverage-gap-targets",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-without-coverage-gap-targets/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			stepInputKeyExecutionContextPackage: map[string]any{
				"dod": map[string]any{
					"items": []string{"streaming responses must be delivered incrementally"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "create-simple-test",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "dod_simple_test.go",
					"content":   "package main\n",
					"summary":   "create a smoke test",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want ordinary state-changing patch allowed without coverageTargets", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsInitialStateChangingPatchWithCoverageTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-initial-state-changing-coverage-targets-valid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-initial-state-changing-coverage-targets-valid/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "streaming responses must be delivered incrementally",
			}},
			stepInputKeyExecutionContextPackage: map[string]any{
				"dod": map[string]any{
					"items": []string{"streaming responses must be delivered incrementally"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-streaming",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "main.go",
					"content":   "package main\n",
					"coverageTargets": []map[string]any{{
						"kind": "dod",
						"item": "streaming responses must be delivered incrementally",
					}},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want initial coverage target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsCoverageGapPatchWithUnmatchedCoverageTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-coverage-target-mismatch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-coverage-target-mismatch/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"missingGoalCoverage": []map[string]any{
				{"goalRequirementId": "goal-req-001-neutral", "item": "support transparent gateway forwarding", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":       "file_write",
					"targetRef":       "main.go",
					"content":         "package main\n",
					"coverageTargets": []string{"unrelated target"},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "coverageTargets") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage target mismatch rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCoverageGapPatchWithStructuredCoverageTargets(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-structured-coverage-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-structured-coverage-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"missingDoDItems": []map[string]any{
				{"item": "streaming responses must be delivered incrementally", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "main.go",
					"content":   "package main\n",
					"coverageTargets": []map[string]any{{
						"kind": "dod",
						"item": "streaming responses must be delivered incrementally",
					}},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want structured coverage target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsGoalCoverageObjectByTextWhenIdExists(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-goal-coverage-object-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-goal-coverage-object-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"missingGoalCoverage": []map[string]any{
				{"goalRequirementId": "goal-req-001-neutral", "item": "support transparent gateway forwarding", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "main.go",
					"content":   "package main\n",
					"coverageTargets": []map[string]any{{
						"kind":       "goal",
						"targetText": "support transparent gateway forwarding",
					}},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want goal coverage text target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCoverageTargetBySourceRequirementID(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-source-requirement-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-source-requirement-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "goal",
				"source":              "goal",
				"sourceRequirementId": "goal-req-001-neutral",
				"targetText":          "**Goal**: support transparent gateway forwarding",
				"coverageTargetToken": "**Goal**: support transparent gateway forwarding",
				"state":               "evidence_ready",
			}},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "main.go",
					"content":   "package main\n",
					"coverageTargets": []map[string]any{{
						"source":              "goal",
						"sourceRequirementId": "goal-req-001-neutral",
						"targetText":          "Goal: support transparent gateway forwarding",
					}},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want sourceRequirementId coverage target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsGoalCoverageRawTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-coverage-raw-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-coverage-raw-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"missingGoalCoverage": []map[string]any{
				{"raw": "streaming token accounting is directly covered", "state": "failed"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":       "file_write",
					"targetRef":       "main.go",
					"content":         "package main\n",
					"coverageTargets": []string{"streaming token accounting is directly covered"},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want raw goal coverage target allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCoverageTargetPatchWhenFailureScopeIsDiagnosticOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-coverage-diagnostic-scope",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-coverage-diagnostic-scope/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"repairOfFailureSignature":        "class=shell_command_failed|failureTargets=metadata.lock,dependency.lock|exitCode=1",
			"coverageTargetSet": []map[string]any{{
				"kind":       "deliverable",
				"targetText": "coverage_map",
			}},
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "metadata.lock"},
				{"path": "dependency.lock"},
				{"path": "coverage_map.json"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "write-coverage-map",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "coverage_map.json",
					"content":   "{\"items\":[]}\n",
					"coverageTargets": []map[string]any{{
						"kind":       "deliverable",
						"targetText": "coverage_map",
					}},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage-targeted patch allowed despite diagnostic failure scope", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairPatchWithoutExplicitTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-no-explicit-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-no-explicit-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "write-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "config.yaml",
					"content":   "enabled: true\n",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want repair patch allowed without explicit target scope", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsImplementationPatchGraphGatedByLLMNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-gated-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-gated-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "analysis",
					"kind": llmWorkspaceExecutionStepKind,
				},
				{
					"id":        "write_main",
					"kind":      "patch/apply",
					"dependsOn": []string{"analysis"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "patch/apply depends on llm/workspace_execution") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want gated patch graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsImplementationPatchGraphAfterSameGraphRead(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-read-then-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-read-then-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read_go_mod",
					"kind":  "file/read",
					"input": map[string]any{"path": "go.mod"},
				},
				{
					"id":        "write_main",
					"kind":      "patch/apply",
					"dependsOn": []string{"read_go_mod"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "same-graph file/read") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want same-graph read/patch rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairPatchGraphGatedByLLMNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-gated-patch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-gated-patch/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "analysis",
					"kind": llmWorkspaceExecutionStepKind,
				},
				{
					"id":        "write_config",
					"kind":      "patch/apply",
					"dependsOn": []string{"analysis"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "patch/apply depends on llm/workspace_execution") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want gated repair patch graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsInvalidPatchGraphNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-invalid-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-invalid-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeySoulStage: soulStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "write_config",
				"kind":  "patch/apply",
				"input": map[string]any{"patchType": "file_write"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph patch/apply input invalid") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want patch graph input rejection", err)
	}
	if !strings.Contains(err.Error(), "patch targetRef is required") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want targetRef-specific contract reason", err)
	}
}

func TestRuntimeOrchestrator_InvalidPatchGraphTargetRefRoutesToRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-invalid-patch-targetref-repair/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				stepOutputKeyExecutionGraph: map[string]any{
					"nodes": []map[string]any{{
						"id":    "write_config",
						"kind":  "patch/apply",
						"input": map[string]any{"patchType": "file_write", "content": "enabled: true\n"},
					}},
				},
			}, nil
		case boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) || boolValue(request.Input[stepInputKeyProviderRetryFollowUp]):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "contract repair selected a concrete targetRef",
					"items": []map[string]any{{
						"item":         "targetRef repaired",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-invalid-patch-targetref-repair")
	runID := common.RunID("run-invalid-patch-targetref-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-invalid-patch-targetref-repair",
		DoDID:           "dod-invalid-patch-targetref-repair",
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s failureReason=%q, want completed", runItem.State, runItem.FailureReason)
	}
	if repairInput == nil ||
		repairInput[stepInputKeyRuntimeRepairFollowUp] != true ||
		repairInput[stepInputKeyProviderRetryFollowUp] == true ||
		repairInput["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		repairInput["stateChangingOutputRequired"] != nil ||
		repairInput["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("repairInput = %+v, want runtime repair acquiring concrete target evidence", repairInput)
	}
	if !strings.Contains(fmt.Sprint(repairInput["failureReason"]), "patch targetRef is required") {
		t.Fatalf("repair failureReason = %v, want targetRef-specific reason", repairInput["failureReason"])
	}
}

func TestValidateLLMExecutionOutputContractRejectsFollowUpReadOnlyGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-follow-up-read-only-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-follow-up-read-only-graph/zz-graph-follow-up-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":         true,
			stepInputKeySoulStage:   soulStageValidation,
			stepInputKeyGraphNodeID: "validate_implementation",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "concrete validation evidence") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want follow-up read-only graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractUsesRuntimeStageForVerificationFollowUp(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-runtime-stage-follow-up-read-only-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-runtime-stage-follow-up-read-only-graph/follow-up-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeyGraphNodeID:  "validate_implementation",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "concrete validation evidence") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want runtimeStage-based verification rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsVerificationEvidenceGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-runtime-stage-follow-up-evidence-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-runtime-stage-follow-up-evidence-graph/follow-up-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":            true,
			stepInputKeyRuntimeStage:   runtimeStageVerification,
			stepInputKeyGraphNodeID:    "validate_implementation",
			stepInputKeySoulStage:      soulStageValidation,
			"validationFollowUpReason": "rerun concrete verification",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "run_tests",
				"kind":  shellcommon.StepKindCommand,
				"input": map[string]any{"command": "test-runner", "args": []string{"./..."}},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want verification evidence graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsVerificationGraphWithReadOnlyEvidenceSupport(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-runtime-stage-follow-up-evidence-support-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-runtime-stage-follow-up-evidence-support-graph/follow-up-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":          true,
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeyGraphNodeID:  "validate_implementation",
			stepInputKeySoulStage:    soulStageValidation,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "run_tests",
					"kind":  shellcommon.StepKindCommand,
					"input": map[string]any{"command": "test-runner", "args": []string{"./..."}},
				},
				{
					"id":        "read_snapshot",
					"kind":      gitcommon.StepKindRead,
					"dependsOn": []string{"run_tests"},
					"input":     map[string]any{"operation": gitcommon.OperationLog},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want verification graph with read-only support allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsVerificationFollowUpStateChangingEvidenceGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-runtime-stage-follow-up-state-changing-evidence-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-runtime-stage-follow-up-state-changing-evidence-graph/follow-up-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":          true,
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeyGraphNodeID:  "validate_implementation",
			stepInputKeySoulStage:    soulStageValidation,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "commit_changes",
				"kind":  gitcommon.StepKindWrite,
				"input": map[string]any{"operation": gitcommon.OperationCommit, "message": "record snapshot"},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want verification follow-up state-changing evidence graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsReadOnlyPlanningGraphWithStateChangeContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-planning-read-only-with-state-change-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-planning-read-only-with-state-change-contract/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_config",
				"kind":  "file/read",
				"input": map[string]any{"path": "config.yaml"},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want planning read-only graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsReadOnlyPlanningGraphWithoutStateChangeContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-planning-read-only-allowed",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "allowed_but_not_required",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-planning-read-only-allowed/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_config",
				"kind":  "file/read",
				"input": map[string]any{"path": "config.yaml"},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want allowed without state-change contract", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsEvidenceRepairShellGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-shell-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-shell-graph/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageRepair,
			"repairObjective":        repairObjectiveConcreteValidationEvidence,
			"evidenceRepairRequired": true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "verify",
					"kind":  "shell/command",
					"input": map[string]any{"command": "verify", "args": []string{"--bounded"}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want evidence repair shell graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsConcreteValidationRepairAssessmentOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-assessment-only",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-assessment-only/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "verification evidence is still missing",
			"items": []map[string]any{{
				"item":  "verify accepted behavior",
				"state": "failed",
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "concrete validation evidence repair must return bounded") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want concrete validation evidence contract rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsConcreteValidationGraphWithoutEvidenceMode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-static-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-static-graph/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":            true,
					"mustDeclareValidationEvidenceMode":                true,
					"mustDeclareValidationEvidenceObservation":         true,
					"staticInspectionOnlyMayNotSatisfyRuntimeBehavior": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "inspect",
				"kind":  shellcommon.StepKindCommand,
				"input": map[string]any{"command": "verify-static"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "validationEvidence contract") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want neutral evidence-mode rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsConcreteValidationGraphWithEvidenceMode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-mode-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-mode-graph/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "printf ok && printf done"},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "unavailable runtime prerequisite",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want declared concrete evidence mode allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsConcreteValidationGraphWithProcessTermination(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-process-termination",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-process-termination/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify-service",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "timeout 3 ./service & sleep 1; curl -fsS http://127.0.0.1:8080/readyz; kill %1"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "process termination") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want process termination rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsConcreteValidationGraphWithDetachedProcess(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-detached-process",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-detached-process/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify-service",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "nohup ./service >/tmp/service.log 2>&1 & curl -fsS http://127.0.0.1:8080/readyz"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "detached or background processes") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want detached process rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsFileMutatingConcreteValidationShell(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-file-mutating-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-file-mutating-validation/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "set -e; sed -i '33d' target.txt; ./verify-target"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "must not mutate workspace files") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want file-mutating validation shell rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingGraphWithDetachedProcess(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-detached-process",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-detached-process/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "start-service",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command":               "sh",
					"args":                  []string{"-c", "./service >/tmp/service.log 2>&1 & echo $!"},
					"stateChanging":         true,
					"targetRef":             "service-process",
					"stateChangingReason":   "start a long-running process",
					"relatedFailureTargets": []string{"service-process"},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "state-changing execution graph") ||
		!strings.Contains(err.Error(), "detached or background processes") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing detached process rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsObservationModeAlias(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-observation-mode",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-observation-mode/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds": []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "verify-contract",
					"validationEvidence": map[string]any{
						"mode":            "runtime_observation",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want runtime_observation mode alias allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsPatchRepairNoOpValidationGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-patch-repair-noop-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-patch-repair-noop-validation/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failedStepKind":              "patch/preview",
			"failedStepOutput": map[string]any{
				"stage":                   "patch_preview",
				"error":                   "invalid argument: unified diff hunk context mismatch",
				failureOutputKeySignature: "class=runtime_failed|kind=patch/preview|stage=patch_preview",
			},
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main"},
			},
		},
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_execution_graph_plan",
		"summary": "Current snapshot already satisfies the intended patch; run bounded verification.",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read-go-mod",
					"kind":  "file/read",
					"input": map[string]any{"path": "go.mod"},
				},
				{
					"id":        "build",
					"kind":      "shell/command",
					"dependsOn": []string{"read-go-mod"},
					"input":     map[string]any{"command": "go", "args": []string{"build"}},
				},
				{
					"id":        "test",
					"kind":      "shell/command",
					"dependsOn": []string{"build"},
					"input":     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want patch no-op validation graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsSafeRepairFileWriteForExistingContextTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-file-write-safe-existing-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-repair-file-write-safe-existing-target/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedStepKind":                  "patch/apply",
			"patchRepairContext": []map[string]any{{
				"targetRef":        "main.go",
				"exists":           true,
				"contentHash":      "hash-main",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"redacted":         false,
				"truncated":        false,
			}},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "rewrite-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main.go",
						"patchType": "file_write",
						"baseHash":  "hash-main",
						"content":   "package main\n\nfunc main() {}\n",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want safe existing target file_write allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsRepairFileWriteForExistingContextTargetWithoutBaseHash(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-file-write-existing-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-repair-file-write-existing-target/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedStepKind":                  "patch/apply",
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": true},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "rewrite-main",
					"kind":  "patch/apply",
					"input": map[string]any{"targetRef": "main.go", "patchType": "file_write", "content": "package main\n"},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "file_write cannot replace existing target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want existing target file_write rejection", err)
	}
}
