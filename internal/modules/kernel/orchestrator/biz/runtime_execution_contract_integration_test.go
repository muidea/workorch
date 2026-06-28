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

func TestValidateLLMExecutionOutputContractRejectsAssessmentOnlyExecutableOutputs(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-assessment-only-output-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-assessment-only-output-contract/assess",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			stepInputKeyRuntimeStage:   runtimeStageVerification,
		},
	}
	validAssessment := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "gap remains open",
			"items": []map[string]any{{
				"item":  "coverage item",
				"state": "open",
			}},
		},
	}
	if err := validateLLMExecutionOutputContract(run, step, validAssessment); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract valid assessment err = %v", err)
	}

	cases := []struct {
		name      string
		output    map[string]any
		wantField string
	}{
		{
			name: "patch request",
			output: map[string]any{
				"patchRequests": []map[string]any{{"targetRef": "artifact://target", "patchType": "file_write"}},
			},
			wantField: "patchRequests",
		},
		{
			name: "memory write",
			output: map[string]any{
				stepOutputKeyMemoryWrites: []map[string]any{{"kind": "diagnostic", "summary": "not allowed in assessment"}},
			},
			wantField: stepOutputKeyMemoryWrites,
		},
		{
			name: "artifact plan",
			output: map[string]any{
				"artifactPlan": map[string]any{"artifacts": []map[string]any{{"artifactId": "artifact-1"}}},
			},
			wantField: "artifactPlan",
		},
		{
			name: "nested llm decision patch request",
			output: map[string]any{
				stepOutputKeyLLMDecision: map[string]any{
					"patchRequests": []map[string]any{{"targetRef": "artifact://target", "patchType": "file_write"}},
				},
			},
			wantField: stepOutputKeyLLMDecision + ".patchRequests",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLLMExecutionOutputContract(run, step, tc.output)
			if !errors.Is(err, common.ErrInvalidArgument) ||
				!strings.Contains(err.Error(), "assessment-only step returned executable or state-changing output fields") ||
				!strings.Contains(err.Error(), tc.wantField) {
				t.Fatalf("validateLLMExecutionOutputContract err = %v, want assessment-only executable output rejection for %s", err, tc.wantField)
			}
			var structured runtimeContractViolationError
			if !errors.As(err, &structured) || structured.violationType != contractViolationAssessmentOnlyExecutableOutput {
				t.Fatalf("structured violation = %#v, want %s", structured, contractViolationAssessmentOnlyExecutableOutput)
			}
		})
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

func TestValidateLLMExecutionOutputContractAllowsStructuredStateChangingBlockers(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-structured-blocker",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-structured-blocker/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}
	cases := []struct {
		name   string
		output map[string]any
	}{
		{
			name: "decision",
			output: map[string]any{
				"status":                           "blocked",
				stepOutputKeyBlockDecisionReason:   "external input is required",
				stepOutputKeyBlockDecisionQuestion: "Which valid target should be changed?",
			},
		},
		{
			name: "approval",
			output: map[string]any{
				"status":                         "blocked",
				stepOutputKeyBlockApprovalReason: "manual approval is required",
				stepOutputKeyBlockApprovalAction: "apply high-impact workspace change",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateLLMExecutionOutputContract(run, step, tc.output); err != nil {
				t.Fatalf("validateLLMExecutionOutputContract err = %v, want structured blocker allowed", err)
			}
		})
	}
}

func TestValidateLLMExecutionOutputContractRejectsIncompleteStateChangingBlockers(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-incomplete-blocker",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-incomplete-blocker/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}
	cases := []struct {
		name    string
		output  map[string]any
		wantErr string
	}{
		{
			name: "decision reason only",
			output: map[string]any{
				"status":                         "blocked",
				stepOutputKeyBlockDecisionReason: "external input is required",
			},
			wantErr: "incomplete decision blocker",
		},
		{
			name: "approval reason only",
			output: map[string]any{
				"status":                         "blocked",
				stepOutputKeyBlockApprovalReason: "manual approval is required",
			},
			wantErr: "incomplete approval blocker",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLLMExecutionOutputContract(run, step, tc.output)
			if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateLLMExecutionOutputContract err = %v, want %s", err, tc.wantErr)
			}
		})
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

func TestValidateLLMExecutionOutputContractAllowsAcceptedGitWriteStateChangingGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-git-write-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-git-write-graph/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{gitcommon.StepKindWrite},
					"acceptedOperations":           []string{gitcommon.OperationCommit},
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
					"id":   "check-status",
					"kind": gitcommon.StepKindRead,
					"input": map[string]any{
						"operation": gitcommon.OperationStatus,
					},
				},
				{
					"id":        "stage-changes",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"check-status"},
					"input": map[string]any{
						"operation": gitcommon.OperationAdd,
						"pathspecs": []string{"artifact.txt"},
					},
				},
				{
					"id":        "commit-snapshot",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"stage-changes"},
					"input": map[string]any{
						"operation": gitcommon.OperationCommit,
						"message":   "Record workspace snapshot",
					},
				},
				{
					"id":        "verify-snapshot",
					"kind":      gitcommon.StepKindRead,
					"dependsOn": []string{"commit-snapshot"},
					"input": map[string]any{
						"operation": gitcommon.OperationLog,
						"limit":     1,
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want accepted git/write graph allowed", err)
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

func TestValidateLLMExecutionOutputContractNormalizesManagedValidationSession(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/managed-validation"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-managed-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	requiredOutput := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{validationScratchDirKey: scratch},
		map[string]any{},
		nil,
	)
	if !stringSliceContains(stringSliceFromValue(requiredOutput["acceptedKinds"]), managedValidationSessionNodeKind) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want managed validation capability", requiredOutput["acceptedKinds"])
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-managed-validation/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": requiredOutput,
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "managed-verify",
				"kind": managedValidationSessionNodeKind,
				"input": map[string]any{
					"allocatePort": true,
					"observation": map[string]any{
						"command":       "sh",
						"args":          []string{"-c", "printf managed-validation-ok:{{managedValidation.port}}"},
						"timeoutMillis": int64(5000),
						"validationEvidence": map[string]any{
							"mode":            "runtime_behavior",
							"observationType": "runtime_observation",
							"targetRefs":      []string{"managed-runtime-behavior"},
						},
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want managed validation declaration allowed", err)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 1 {
		t.Fatalf("executionGraph = %+v, want normalized graph", output[stepOutputKeyExecutionGraph])
	}
	node := graph.Nodes[0]
	if node.Kind != shellcommon.StepKindCommand {
		t.Fatalf("normalized node kind = %q, want shell/command", node.Kind)
	}
	if stringFromMap(node.Input, "command") != "sh" ||
		!strings.Contains(strings.Join(stringSliceFromValue(node.Input["args"]), " "), "managed-validation-ok:") ||
		strings.Contains(strings.Join(stringSliceFromValue(node.Input["args"]), " "), "{{managedValidation.port}}") {
		t.Fatalf("normalized node input = %+v, want observation command", node.Input)
	}
	if stringFromMap(node.Input, validationScratchDirKey) != scratch {
		t.Fatalf("validationScratchDir = %q, want %q", stringFromMap(node.Input, validationScratchDirKey), scratch)
	}
	managed := mapFromValue(node.Input["managedValidation"])
	if managed["runtimeManaged"] != true ||
		managed["autoCleanup"] != true ||
		stringFromMap(managed, "cleanupPolicy") != "process_group_on_timeout_or_completion" {
		t.Fatalf("managedValidation = %+v, want runtime-managed cleanup metadata", managed)
	}
	if port, ok := managedValidationInt(managed["allocatedPort"]); !ok || port <= 0 || node.Input["managedValidationPort"] != port {
		t.Fatalf("managedValidation = %+v input=%+v, want allocated port mirrored to input", managed, node.Input)
	}
	if evidence := mapFromValue(node.Input["validationEvidence"]); stringFromMap(evidence, "mode") != "runtime_behavior" {
		t.Fatalf("validationEvidence = %+v, want copied observation evidence", evidence)
	} else if refs := stringSliceFromValue(evidence["targetRefs"]); len(refs) == 0 {
		t.Fatalf("validationEvidence = %+v, want copied targetRefs", evidence)
	}
}

func TestConcreteValidationRequiredOutputIncludesManagedSessionExample(t *testing.T) {
	requiredOutput := concreteValidationEvidenceRequiredOutput(nil)
	if !stringSliceContains(stringSliceFromValue(requiredOutput["acceptedKinds"]), managedValidationSessionNodeKind) {
		t.Fatalf("acceptedKinds = %+v, want managed session kind", requiredOutput["acceptedKinds"])
	}
	example := mapFromValue(requiredOutput["managedValidationNodeExample"])
	if stringFromMap(example, "kind") != managedValidationSessionNodeKind {
		t.Fatalf("managedValidationNodeExample = %+v, want managed session node", example)
	}
	input := mapFromValue(example["input"])
	if !boolValue(input["allocatePort"]) {
		t.Fatalf("managedValidationNodeExample.input = %+v, want runtime allocated port", input)
	}
	observation := mapFromValue(input["observation"])
	if stringFromMap(observation, "command") == "" ||
		intFromAny(observation["timeoutMillis"]) <= 0 {
		t.Fatalf("managedValidationNodeExample.observation = %+v, want bounded command and timeout", observation)
	}
	evidence := mapFromValue(observation["validationEvidence"])
	if stringFromMap(evidence, "mode") != "runtime_behavior" ||
		stringFromMap(evidence, "observationType") != "runtime_observation" {
		t.Fatalf("managedValidationNodeExample.validationEvidence = %+v, want runtime observation", evidence)
	}
	if refs := stringSliceFromValue(evidence["targetRefs"]); len(refs) == 0 {
		t.Fatalf("managedValidationNodeExample.validationEvidence = %+v, want targetRefs", evidence)
	}
}

func TestValidateLLMExecutionOutputContractRejectsManagedValidationWithoutTargetRefs(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/managed-validation-missing-targets"
	run := &runmodel.Run{
		ID:              "run-managed-validation-missing-targets",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	requiredOutput := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{validationScratchDirKey: scratch},
		map[string]any{},
		nil,
	)
	step := &runmodel.Step{
		ID:   "run-managed-validation-missing-targets/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": requiredOutput,
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "managed-verify",
				"kind": managedValidationSessionNodeKind,
				"input": map[string]any{
					"observation": map[string]any{
						"command":       "sh",
						"args":          []string{"-c", "printf managed-validation-ok"},
						"timeoutMillis": int64(5000),
						"validationEvidence": map[string]any{
							"mode":            "runtime_behavior",
							"observationType": "runtime_observation",
						},
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "targetRefs") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want targetRefs contract rejection", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) || violation.rule != contractViolationRuleValidationEvidenceMetadataMissing {
		t.Fatalf("contract violation = %+v, want rule %s", violation, contractViolationRuleValidationEvidenceMetadataMissing)
	}
}

func TestValidateLLMExecutionOutputContractManagedRouteRejectsPlainShellGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-managed-route-rejects-plain-shell",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	requiredOutput := concreteValidationEvidenceRequiredOutput(nil)
	requiredOutput["validationRoute"] = runtimemodel.ValidationRouteManagedHarness
	requiredOutput["mustUseManagedHarness"] = true
	step := &runmodel.Step{
		ID:   "run-managed-route-rejects-plain-shell/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": requiredOutput,
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "plain-shell",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "printf ok"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "managed validation route requires") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want managed route contract rejection", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) || violation.rule != contractViolationRuleManagedValidationRequired {
		t.Fatalf("contract violation = %+v, want rule %s", violation, contractViolationRuleManagedValidationRequired)
	}
}

func TestValidateLLMExecutionOutputContractRejectsMixedValidationGraphMissingEvidenceMetadata(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-validation-mixed-metadata",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-validation-mixed-metadata/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageRepair,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
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
				"id":   "declared-validation",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "printf ok"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}, {
				"id":   "missing-validation-metadata",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "printf also-ok"},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "validation node missing") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want metadata contract rejection", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) || violation.rule != contractViolationRuleValidationEvidenceMetadataMissing {
		t.Fatalf("contract violation = %+v, want rule %s", violation, contractViolationRuleValidationEvidenceMetadataMissing)
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
	scratch := "/tmp/workorch-validation-test/file-mutating-validation"
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
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
		!strings.Contains(err.Error(), "write outside validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want file-mutating validation shell rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsConcreteValidationShellFileRedirection(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/file-redirection-validation"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-file-redirection-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-file-redirection-validation/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args":    []string{"-c", "printf ok > validation.log"},
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
		!strings.Contains(err.Error(), "write outside validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want file-redirection validation shell rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractNormalizesConcreteValidationScratchSetup(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/normalized-scratch-setup"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-normalized-scratch-setup",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-normalized-scratch-setup/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "setup-validation-scratch",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "mkdir -p .workorch-data/validation-scratch"},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "validation scratch directory",
					},
				},
			}, {
				"id":        "verify",
				"kind":      shellcommon.StepKindCommand,
				"dependsOn": []string{"setup-validation-scratch"},
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "printf ok"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want normalized scratch setup allowed", err)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 2 {
		t.Fatalf("normalized graph = %+v, ok=%v", graph, ok)
	}
	args := stringSliceFromValue(graph.Nodes[0].Input["args"])
	if len(args) != 2 || args[0] != "-c" || !strings.Contains(args[1], scratch) {
		t.Fatalf("normalized setup args = %+v, want runtime validationScratchDir", args)
	}
	if graph.Nodes[0].Input[validationScratchDirKey] != scratch || graph.Nodes[0].Input["validationScratchDirNormalized"] != true {
		t.Fatalf("normalized setup input = %+v, want scratch metadata", graph.Nodes[0].Input)
	}
}

func TestValidateLLMExecutionOutputContractAllowsConcreteValidationShellDevNullRedirection(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-dev-null-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-dev-null-validation/repair-001-of-verify",
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
					"args":    []string{"-c", "if verify >/dev/null 2>&1; then printf ok; fi"},
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want dev-null validation shell redirection allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsControlledSubstituteScratchFileRedirection(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-file"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-mktemp-file",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-mktemp-file/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args": []string{"-c", strings.Join([]string{
						`scratch="/tmp/workorch-validation-test/controlled-substitute-file"`,
						`mkdir -p "$scratch"`,
						`LOG="$scratch/validation.log"`,
						`if ! verify-command >"$LOG" 2>&1; then`,
						`  cat "$LOG"`,
						`  exit 1`,
						`fi`,
						`cat "$LOG"`,
					}, "\n")},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "validation command output capture",
						"substitute":      "temporary log file created under validationScratchDir",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scratch log redirection allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsControlledSubstituteScratchSetupAndHeredoc(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-heredoc"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-heredoc",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-heredoc/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"id":   "setup-validation-scratch",
					"kind": shellcommon.StepKindCommand,
					"input": map[string]any{
						"command": "sh",
						"args":    []string{"-c", `mkdir -p /tmp/workorch-validation-test/controlled-substitute-heredoc`},
						"validationEvidence": map[string]any{
							"mode":            "controlled_substitute",
							"observationType": "controlled_substitute_observation",
							"substituteFor":   "validation scratch setup",
						},
					},
				},
				{
					"id":        "capture-readonly-output",
					"kind":      shellcommon.StepKindCommand,
					"dependsOn": []string{"setup-validation-scratch"},
					"input": map[string]any{
						"command": "sh",
						"args": []string{"-c", strings.Join([]string{
							`printf 'first\n' > /tmp/workorch-validation-test/controlled-substitute-heredoc/validation.log`,
							`printf 'second\n' >> /tmp/workorch-validation-test/controlled-substitute-heredoc/validation.log`,
							`cat > /tmp/workorch-validation-test/controlled-substitute-heredoc/probe.txt <<'EOF'`,
							`temporary validation fixture`,
							`if observed >= expected { keep }`,
							`EOF`,
							`grep -n temporary /tmp/workorch-validation-test/controlled-substitute-heredoc/probe.txt >> /tmp/workorch-validation-test/controlled-substitute-heredoc/validation.log 2>&1`,
							`cat /tmp/workorch-validation-test/controlled-substitute-heredoc/validation.log`,
						}, "\n")},
						"validationEvidence": map[string]any{
							"mode":            "controlled_substitute",
							"observationType": "controlled_substitute_observation",
							"substituteFor":   "temporary validation fixture under validationScratchDir",
						},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scratch setup and heredoc writes allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsScratchBoundHereDocBodyOperators(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/heredoc-body-operators"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-heredoc-body-operators",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-heredoc-body-operators/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "capture-probe",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args": []string{"-c", strings.Join([]string{
						`set -eu`,
						`scratch="/tmp/workorch-validation-test/heredoc-body-operators"`,
						`mkdir -p "$scratch"`,
						`probe="$scratch/probe.txt"`,
						`cat > "$probe" <<'EOF'`,
						`if idx > 0 {`,
						`  value := "mkdir outside and kill are text inside the here-doc body"`,
						`}`,
						`EOF`,
						`cat "$probe" >/dev/null`,
					}, "\n")},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "temporary validation probe under validationScratchDir",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scratch-bound here-doc body operators ignored", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsHereDocTargetOutsideScratch(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/heredoc-target-outside"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-heredoc-target-outside",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-heredoc-target-outside/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "capture-probe",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args": []string{"-c", strings.Join([]string{
						`cat > validation.log <<'EOF'`,
						`idx > 0`,
						`EOF`,
					}, "\n")},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "temporary validation probe under validationScratchDir",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "write outside validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want here-doc target outside scratch rejected", err)
	}
	var violation runtimeContractViolationError
	if !errors.As(err, &violation) || violation.rule != contractViolationRuleValidationScratchEscape {
		t.Fatalf("contract violation = %+v, want rule %s", violation, contractViolationRuleValidationScratchEscape)
	}
}

func TestValidateLLMExecutionOutputContractRejectsControlledSubstituteWriteWithoutScratchContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-missing-scratch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-missing-scratch/repair-001-of-verify",
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
					"args":    []string{"-c", `printf ok > /tmp/workorch-validation-test/missing-scratch/validation.log`},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "validation command output capture",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "requires runtime validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want missing validationScratchDir rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsRelativeControlledSubstituteScratchContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-relative-scratch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-relative-scratch/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     "relative-validation-scratch",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    "relative-validation-scratch",
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args":    []string{"-c", `mkdir -p relative-validation-scratch; printf ok > relative-validation-scratch/validation.log`},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "validation command output capture",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "requires runtime validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want relative validationScratchDir rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsControlledSubstituteScratchPrefixCollision(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/prefix"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-prefix-collision",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-prefix-collision/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args":    []string{"-c", `mkdir -p /tmp/workorch-validation-test/prefix-other; printf ok > /tmp/workorch-validation-test/prefix-other/validation.log`},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "validation command output capture",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "write outside validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scratch prefix collision rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsDynamicExecutionTempCacheWrites(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/dynamic-temp-cache"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-dynamic-temp-cache",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-dynamic-temp-cache/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args": []string{"-c", strings.Join([]string{
						`set -eu`,
						`tmpdir="/tmp/workorch-validation-test/dynamic-temp-cache"`,
						`cleanup() {`,
						`  chmod -R u+w "$tmpdir" 2>/dev/null || true`,
						`}`,
						`trap cleanup EXIT INT TERM`,
						`mkdir -p "$tmpdir"`,
						`export GOCACHE="$tmpdir/gocache"`,
						`export GOMODCACHE="$tmpdir/gomodcache"`,
						`mkdir -p "$GOCACHE" "$GOMODCACHE"`,
						`printf ok > "$tmpdir/validation.out"`,
						`cat "$tmpdir/validation.out"`,
					}, "\n")},
					"validationEvidence": map[string]any{
						"mode":            "dynamic_execution",
						"observationType": "dynamic_execution_observation",
						"substitute":      "tool caches and captured output are isolated under validationScratchDir",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want dynamic execution temp writes allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsControlledSubstituteTempWorkspaceWrites(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-temp-writes"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-temp-writes",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-temp-writes/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args": []string{"-c", strings.Join([]string{
						`TMPDIR_ROOT="/tmp/workorch-validation-test/controlled-substitute-temp-writes"`,
						`mkdir -p "$TMPDIR_ROOT"`,
						`TMP_REPO="$TMPDIR_ROOT/repo"`,
						`XDG_CACHE_HOME="$TMPDIR_ROOT/home/.cache"`,
						`mkdir -p "$TMP_REPO"`,
						`mkdir -p "$XDG_CACHE_HOME"`,
						`cp -R . "$TMP_REPO"`,
						`cat > "$TMP_REPO/validation.txt" <<'EOF'`,
						`ok`,
						`EOF`,
						`sh -c 'test -f "$1"' _ "$TMP_REPO/validation.txt"`,
					}, "\n")},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "workspace mutation is isolated in a temporary copy",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want isolated controlled substitute writes allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsControlledSubstituteCopiedWorkspaceHarness(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-copied-workspace"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-copied-workspace",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-copied-workspace/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	command := strings.Join([]string{
		`set -eu`,
		`tmp="/tmp/workorch-validation-test/controlled-substitute-copied-workspace"`,
		`mkdir -p "$tmp"`,
		`mkdir -p "$tmp/home" "$tmp/gocache" "$tmp/gopath" "$tmp/copy"`,
		`XDG_CACHE_HOME="$tmp/home/.cache"`,
		`mkdir -p "$XDG_CACHE_HOME"`,
		`cp -R . "$tmp/copy/repo"`,
		`cd "$tmp/copy/repo"`,
		`set +e`,
		`HOME="$tmp/home" GOCACHE="$tmp/gocache" GOPATH="$tmp/gopath" TMPDIR="$tmp" sh scripts/verify_usage_traceability.sh >"$tmp/validator.stdout" 2>"$tmp/validator.stderr"`,
		`validator_rc=$?`,
		`set -e`,
		`printf 'validator_exit=%s\n' "$validator_rc"`,
		`cat "$tmp/validator.stdout"`,
		`cat "$tmp/validator.stderr"`,
		`usage_path="$(find "$tmp" -type f -name 'usage.csv' | head -n 1 || true)"`,
		`metadata_path="$(find "$tmp" -type f -path '*/interactions/*/metadata.json' | head -n 1 || true)"`,
		`[ -n "$usage_path" ] || { echo 'usage.csv missing after controlled substitute' >&2; exit 1; }`,
		`[ -n "$metadata_path" ] || { echo 'metadata.json missing after controlled substitute' >&2; exit 1; }`,
		`round_dir="$(dirname "$metadata_path")"`,
		`request_path="$(find "$round_dir" -maxdepth 1 -type f -name '*request*' ! -name '*upstream*' | head -n 1 || true)"`,
		`upstream_request_path="$(find "$round_dir" -maxdepth 1 -type f -name '*upstream*request*' | head -n 1 || true)"`,
		`response_path="$(find "$round_dir" -maxdepth 1 -type f -name '*response*' ! -name '*upstream*' | head -n 1 || true)"`,
		`upstream_response_path="$(find "$round_dir" -maxdepth 1 -type f -name '*upstream*response*' | head -n 1 || true)"`,
		`[ -n "$request_path" ] || { echo 'archived client request file missing' >&2; exit 1; }`,
		`[ -n "$upstream_request_path" ] || { echo 'archived upstream request file missing' >&2; exit 1; }`,
		`[ -n "$response_path" ] || { echo 'archived final response file missing' >&2; exit 1; }`,
		`[ -n "$upstream_response_path" ] || { echo 'archived upstream response file missing' >&2; exit 1; }`,
		`client_model="$(grep -Eo '"model"[[:space:]]*:[[:space:]]*"[^"]+"' "$request_path" | head -n 1 | sed 's/.*"model"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"`,
		`upstream_model="$(grep -Eo '"model"[[:space:]]*:[[:space:]]*"[^"]+"' "$upstream_request_path" | head -n 1 | sed 's/.*"model"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"`,
		`[ -n "$client_model" ] || { echo 'client model field missing from archived request' >&2; exit 1; }`,
		`[ -n "$upstream_model" ] || { echo 'upstream model field missing from archived upstream request' >&2; exit 1; }`,
		`printf 'usage_path=%s\n' "$usage_path"`,
		`printf 'metadata_path=%s\n' "$metadata_path"`,
	}, "\n")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", command},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "workspace validation in an isolated temporary copy",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want copied-workspace controlled substitute allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsControlledSubstituteTarCopiedWorkspaceWrite(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-tar-copy-write"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-tar-copy-write",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-tar-copy-write/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	command := strings.Join([]string{
		`set -eu`,
		`TMPDIR_ROOT="/tmp/workorch-validation-test/controlled-substitute-tar-copy-write"`,
		`cleanup() {`,
		`  chmod -R u+w "$TMPDIR_ROOT" 2>/dev/null || true`,
		`}`,
		`trap cleanup EXIT HUP INT TERM`,
		`mkdir -p "$TMPDIR_ROOT/src" "$TMPDIR_ROOT/gomodcache" "$TMPDIR_ROOT/gocache"`,
		`tar cf - . | (cd "$TMPDIR_ROOT/src" && tar xf -)`,
		`cd "$TMPDIR_ROOT/src"`,
		`touch generated-validation-artifact`,
		`printf 'artifact=%s\n' "$TMPDIR_ROOT/src/generated-validation-artifact"`,
	}, "\n")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "verify",
				"kind": shellcommon.StepKindCommand,
				"input": map[string]any{
					"command": "sh",
					"args":    []string{"-c", command},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "workspace validation in an isolated temporary copy",
					},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want tar-copied workspace controlled substitute write allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsControlledSubstituteWorkspaceWrite(t *testing.T) {
	scratch := "/tmp/workorch-validation-test/controlled-substitute-workspace-write"
	run := &runmodel.Run{
		ID:              "run-evidence-repair-controlled-substitute-workspace-write",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-controlled-substitute-workspace-write/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			validationScratchDirKey:     scratch,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					validationScratchDirKey:                    scratch,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
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
					"args":    []string{"-c", `scratch="/tmp/workorch-validation-test/controlled-substitute-workspace-write"; mkdir -p "$scratch"; printf ok > validation.log`},
					"validationEvidence": map[string]any{
						"mode":            "controlled_substitute",
						"observationType": "controlled_substitute_observation",
						"substituteFor":   "workspace mutation is not isolated",
					},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "write outside validationScratchDir") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scratch boundary write rejection", err)
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

func TestValidateLLMExecutionOutputContractRejectsUnsafeRepairFileWriteForExistingContextTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-file-write-unsafe-existing-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	cases := []struct {
		name string
		item map[string]any
		hash string
	}{
		{
			name: "base hash mismatch",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": true},
			hash: "hash-stale",
		},
		{
			name: "incomplete snapshot",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": false, "safeForFileWrite": true},
			hash: "hash-main",
		},
		{
			name: "unsafe snapshot",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": false},
			hash: "hash-main",
		},
		{
			name: "redacted snapshot",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": true, "redacted": true},
			hash: "hash-main",
		},
		{
			name: "truncated snapshot",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": true, "truncated": true},
			hash: "hash-main",
		},
		{
			name: "binary snapshot",
			item: map[string]any{"targetRef": "main.go", "exists": true, "contentHash": "hash-main", "contentComplete": true, "safeForFileWrite": true, "isBinary": true},
			hash: "hash-main",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := &runmodel.Step{
				ID:   "run-repair-file-write-unsafe-existing-target/repair-001-of-graph-001-003-patch",
				Kind: llmWorkspaceExecutionStepKind,
				Input: map[string]any{
					stepInputKeyRuntimeRepairFollowUp: true,
					stepInputKeyRuntimeStage:          runtimeStageRepair,
					"repairObjective":                 repairObjectiveStateChangingOutput,
					"stateChangingOutputRequired":     true,
					"failedStepKind":                  "patch/apply",
					"patchRepairContext":              []map[string]any{tc.item},
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
								"baseHash":  tc.hash,
								"content":   "package main\n",
							},
						},
					},
				},
			}

			err := validateLLMExecutionOutputContract(run, step, output)
			if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "file_write cannot replace existing target") {
				t.Fatalf("validateLLMExecutionOutputContract err = %v, want unsafe existing target file_write rejection", err)
			}
		})
	}
}

func TestValidateLLMExecutionOutputContractRejectsNonSubstantiveRepairUnifiedDiff(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-non-substantive-unified-diff",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-repair-non-substantive-unified-diff/repair-001-of-graph-001-003-patch",
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
			}},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "annotate-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":      "main.go",
						"patchType":      "unified_diff",
						"summary":        "add a neutral marker only to satisfy repair state change",
						"diff":           "--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n package main\n+\n+// state-change anchor for verification\n",
						"nonSubstantive": true,
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "non-substantive state-change placeholder") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want non-substantive unified diff rejection", err)
	}
}
