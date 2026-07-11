package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGraphLLMNodeInputRequiresStateChangeWhenRunContractRequiresIt(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-llm-state-change",
		WorkspaceID:     "ws-graph-llm-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "plan-implementation", Kind: llmWorkspaceExecutionStepKind, Input: map[string]any{"task": "produce implementation"}},
	}}
	last := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != "produce_state_changing_output" ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("graph llm input = %+v, want state-changing contract injected", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageExecution || input[stepInputKeyRuntimeStageMode] != "strict_convergence" {
		t.Fatalf("graph llm runtime stage = %v/%v, want execution/strict", input[stepInputKeyRuntimeStage], input[stepInputKeyRuntimeStageMode])
	}
}

func TestGraphReadNodeInputDoesNotRequireStateChangeWhenRunContractRequiresIt(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-read-state-change",
		WorkspaceID:     "ws-graph-read-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "read-main", Kind: "file/read", Input: map[string]any{"path": "main.go", "optional": true}},
	}}
	last := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input["stateChangingOutputRequired"] != nil || input["repairObjective"] != nil {
		t.Fatalf("graph read input = %+v, want read node to stay observation-only", input)
	}
}

func TestNextGraphStepInputPropagatesRepairBudgetToLLMGraphNode(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-repair-budget")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-graph-repair-budget",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "repair_target", Kind: llmWorkspaceExecutionStepKind},
	}}
	last := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  3,
			stepInputKeyRepairOfStepID:        string(runID) + "/graph-001/001-verify",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/graph-001/001-verify",
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=target.txt|exitCode=1",
			"failureTargetScope":              "failureTargets=target.txt",
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input[stepInputKeyRuntimeStage] != runtimeStageRepair || input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("graph repair input stage/followUp = %v/%v, want repair budget contract", input[stepInputKeyRuntimeStage], input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input[stepInputKeyRuntimeRepairAttempt] != 3 || input["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("graph repair input attempt/scope = %v/%v, want inherited repair attempt and target scope", input[stepInputKeyRuntimeRepairAttempt], input["failureTargetScope"])
	}

	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     common.StepID(string(runID) + "/graph-002/001-repair-target"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateFailed,
		Input:  input,
		Output: map[string]any{"status": "failed"},
	}); err != nil {
		t.Fatalf("seed graph repair node: %v", err)
	}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/002-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=target.txt|exitCode=1",
		},
	}
	orch := &RuntimeOrchestrator{steps: stepRepo}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failed); got != 1 {
		t.Fatalf("effective graph repair attempts = %d, want LLM graph repair node counted", got)
	}
}

func TestNextGraphStepInputPreservesRepairLineageForNonLLMGraphNodeWithoutBudget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-repair-block-lineage",
		WorkspaceID:     "ws-graph-repair-block-lineage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify_target", Kind: shellcommon.StepKindCommand},
	}}
	last := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/repair-001"),
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  2,
			stepInputKeyRepairOfRootStepID:    string(run.ID) + "/graph-001/001-verify",
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=target.txt|exitCode=1",
			"failureTargetScope":              "failureTargets=target.txt",
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input[stepInputKeyRuntimeStage] != runtimeStageRepair || input["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("graph block input stage/scope = %v/%v, want inherited repair lineage", input[stepInputKeyRuntimeStage], input["failureTargetScope"])
	}
	if input[stepInputKeyRuntimeRepairFollowUp] != nil || input[stepInputKeyRuntimeRepairAttempt] != nil {
		t.Fatalf("graph block input repair budget = %v/%v, want non-LLM block excluded from repair attempt budget", input[stepInputKeyRuntimeRepairFollowUp], input[stepInputKeyRuntimeRepairAttempt])
	}
}

func TestNextGraphStepInputPreservesFullRepairContractForPatchNode(t *testing.T) {
	runID := common.RunID("run-graph-patch-full-repair-contract")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-graph-patch-full-repair-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{
		ID:   "fix-primary-diagnostic",
		Kind: "patch/apply",
		Input: map[string]any{
			"patchType": "text_replace",
			"targetRef": "main.go",
			"oldText":   "\t\"strings\"\n",
			"newText":   "",
		},
	}}}
	primary := map[string]any{
		"required":   true,
		"targetRefs": []string{"main.go"},
		"lineRefs": []map[string]any{{
			"targetRef": "main.go",
			"line":      11,
		}},
		"messages": []string{`./main.go:11:2: "strings" imported and not used`},
	}
	last := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(runID) + "/graph-001/001-verify",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/graph-001/001-verify",
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1",
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"repairActionPackage": map[string]any{
				"primaryDiagnosticRepair": primary,
				"targetSnapshots": []map[string]any{{
					"targetRef": "main.go",
					"content": strings.Join([]string{
						"package main",
						"",
						"import (",
						"\t\"strings\"",
						")",
					}, "\n"),
				}},
			},
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)
	pkg := mapFromValue(input["repairActionPackage"])
	inheritedPrimary := mapFromValue(pkg["primaryDiagnosticRepair"])
	if refs := mapSliceFromValue(inheritedPrimary["lineRefs"]); len(refs) != 1 || intFromAny(refs[0]["line"]) != 11 {
		t.Fatalf("primaryDiagnosticRepair.lineRefs = %+v, want full line refs inherited for patch/apply node", inheritedPrimary["lineRefs"])
	}
	if snapshots := mapSliceFromValue(pkg["targetSnapshots"]); len(snapshots) != 1 || stringFromMap(snapshots[0], "content") == "" {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want full target snapshot inherited for patch/apply node", pkg["targetSnapshots"])
	}
}

func TestGraphLLMNodeInputIncludesDependencyOutput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-dependency-output",
		WorkspaceID:     "ws-graph-dependency-output",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "scan_workspace", Kind: "file/list", Input: map[string]any{"path": ".", "recursive": true}},
		{ID: "refine_plan_from_manifest", Kind: llmWorkspaceExecutionStepKind, DependsOn: []string{"scan_workspace"}},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	scan := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-001/001-scan-workspace"),
		RunID: run.ID,
		Kind:  "file/list",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "scan_workspace",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepOutputKeyExecutionGraph: graph,
		},
		Output: map[string]any{
			"status":    "completed",
			"stage":     "file_list",
			"path":      ".",
			"entries":   []map[string]any{},
			"truncated": false,
		},
	}
	completed := map[string]bool{"scan_workspace": true}

	input := nextGraphStepInput(run, scan, graph, graph.Nodes[1], completed, "", []*runmodel.Step{root, scan}, root.ID)

	if input["previousStepKind"] != "file/list" {
		t.Fatalf("previousStepKind = %v, want file/list", input["previousStepKind"])
	}
	previousOutput := mapFromValue(input["previousStepOutput"])
	if previousOutput["status"] != "completed" || previousOutput["truncated"] != false {
		t.Fatalf("previousStepOutput = %+v, want compact scan output", input["previousStepOutput"])
	}
	if _, ok := previousOutput["entries"]; !ok {
		t.Fatalf("previousStepOutput = %+v, want explicit empty entries from scan output", previousOutput)
	}
	evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || evidence[0]["nodeId"] != "scan_workspace" || evidence[0]["kind"] != "file/list" {
		t.Fatalf("graphEvidence = %+v, want dependency evidence for scan_workspace", input[stepInputKeyGraphEvidence])
	}
	evidenceOutput := mapFromValue(evidence[0]["output"])
	if evidenceOutput["truncated"] != false {
		t.Fatalf("graphEvidence output = %+v, want scan output", evidenceOutput)
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterExecutionGraphCompletes(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "inspect", Kind: "graph/mock"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-graph-follow-up/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case request.StepID == "run-graph-follow-up/graph-001/001-inspect":
				return map[string]any{
					"status": "completed",
					"stdout": "workspace inspected",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && request.Input["graphPreviousNodeId"] == "inspect":
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "graph completed",
						"items": []map[string]any{
							{
								"item":         "graph completed",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-graph-follow-up/graph-001/001-inspect"},
							},
						},
					},
				}, nil
			default:
				return nil, errors.New("unexpected graph follow-up step")
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-follow-up")
	runID := common.RunID("run-graph-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-follow-up"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-graph-follow-up/step-1" ||
		runtime.calls[1] != "run-graph-follow-up/graph-001/001-inspect" ||
		!strings.HasPrefix(string(runtime.calls[2]), "run-graph-follow-up/follow-up-graph-") ||
		strings.Contains(string(runtime.calls[2]), "run-graph-follow-up-step-1-inspect") {
		t.Fatalf("runtime calls = %#v, want graph then llm follow-up", runtime.calls)
	}
	if followUpInput["graphFollowUp"] != true ||
		followUpInput["graphPreviousNodeId"] != "inspect" ||
		followUpInput["previousStepId"] != "run-graph-follow-up/graph-001/001-inspect" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want graph follow-up context", followUpInput)
	}
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("graph follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
	}
	if _, ok := followUpInput["previousStepOutput"]; ok {
		t.Fatalf("previousStepOutput = %+v, want graph follow-up to use graphEvidence only", followUpInput["previousStepOutput"])
	}
	evidence := mapSliceFromValue(followUpInput[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || evidence[0]["nodeId"] != "inspect" {
		t.Fatalf("graphEvidence = %+v, want graph node evidence", followUpInput[stepInputKeyGraphEvidence])
	}
	output, _ := evidence[0]["output"].(map[string]any)
	if output["stdout"] != "workspace inspected" {
		t.Fatalf("graphEvidence output = %+v, want graph node output", output)
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterLLMPlannedShellLeaf(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-shell-follow-up/step-1":
				return map[string]any{
					"status":   "completed",
					"nextStep": "run-shell-follow-up/shell-1",
					"nextKind": "shell/command",
					"nextInput": map[string]any{
						"command": "cat",
						"args":    []string{"result.txt"},
					},
				}, nil
			case request.StepID == "run-shell-follow-up/shell-1":
				return map[string]any{
					"status": "completed",
					"stdout": "result ready",
				}, nil
			case boolValue(request.Input["completionFollowUp"]):
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "shell evidence assessed",
						"items": []map[string]any{
							{
								"item":         "result ready",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-shell-follow-up/shell-1"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected shell follow-up step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-shell-follow-up")
	runID := common.RunID("run-shell-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-shell-follow-up"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if followUpInput["completionFollowUp"] != true ||
		followUpInput["previousStepKind"] != "shell/command" ||
		followUpInput["previousStepId"] != "run-shell-follow-up/shell-1" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want completion follow-up context", followUpInput)
	}
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("completion follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
	}
	if followUpInput[stepInputKeyAssessmentOnly] != true || followUpInput["stateChangingOutputRequired"] != nil {
		t.Fatalf("completion follow-up assessment contract = %+v, want assessment-only without state-changing output", followUpInput)
	}
	previousOutput, ok := followUpInput["previousStepOutput"].(map[string]any)
	if !ok || previousOutput["stdout"] != "result ready" {
		t.Fatalf("previousStepOutput = %+v, want shell output", followUpInput["previousStepOutput"])
	}
	runEvidence := mapSliceFromValue(followUpInput[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) == 0 {
		t.Fatalf("runEvidenceSummary = %+v, want compact run evidence", followUpInput[stepInputKeyRunEvidenceSummary])
	}
	foundShellEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != "run-shell-follow-up/shell-1" {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundShellEvidence = output["stdout"] == "result ready"
	}
	if !foundShellEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want shell output evidence", runEvidence)
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterLLMPatchSideEffect(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.Kind == "patch/apply":
				result, err := filebiz.NewRuntimeAdapter().ExecuteRuntimeStep(ctx, request)
				return result.Output, err
			case request.StepID == "run-patch-follow-up/step-1":
				return map[string]any{
					"status": "completed",
					"patches": []map[string]any{
						{
							"patchType": "file_write",
							"targetRef": "result.txt",
							"content":   "result ready",
						},
					},
				}, nil
			case boolValue(request.Input["completionFollowUp"]):
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "patch evidence assessed",
						"items": []map[string]any{
							{
								"item":         "result ready",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-patch-follow-up/step-1/patch/1"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected patch follow-up step %s", request.StepID)
			}
		},
	}
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
			MemoryItems:        memory.NewMemoryItemRepository(),
		})

	workspaceID := common.WorkspaceID("ws-patch-follow-up")
	runID := common.RunID("run-patch-follow-up")
	deliverableDirectory := t.TempDir()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-follow-up"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if followUpInput["completionFollowUp"] != true ||
		followUpInput["previousStepKind"] != "llm/workspace_execution" ||
		followUpInput["previousStepId"] != "run-patch-follow-up/step-1" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want completion follow-up context", followUpInput)
	}
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("patch follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
	}
}

func TestRuntimeOrchestrator_GraphFollowUpIncludesAllCompletedParallelEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "cpu", Kind: "graph/mock"},
		{ID: "memory", Kind: "graph/mock"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-graph-parallel-follow-up/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case request.StepID == "run-graph-parallel-follow-up/graph-001/001-cpu":
				return map[string]any{
					"status": "completed",
					"stdout": "cpu: 4 cores",
				}, nil
			case request.StepID == "run-graph-parallel-follow-up/graph-001/002-memory":
				return map[string]any{
					"status": "completed",
					"stdout": "memory: 15Gi",
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-parallel-follow-up/follow-up-graph-"):
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "parallel graph evidence assessed",
						"items": []map[string]any{
							{
								"item":         "parallel graph completed",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-graph-parallel-follow-up/graph-001/001-cpu", "run-graph-parallel-follow-up/graph-001/002-memory"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected graph parallel follow-up step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-parallel-follow-up")
	runID := common.RunID("run-graph-parallel-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-parallel-follow-up"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	evidence, ok := followUpInput["graphEvidence"].([]map[string]any)
	if !ok || len(evidence) != 2 {
		t.Fatalf("graphEvidence = %#v, want two completed graph node evidence entries", followUpInput["graphEvidence"])
	}
	if evidence[0]["nodeId"] != "cpu" || evidence[1]["nodeId"] != "memory" {
		t.Fatalf("graphEvidence = %#v, want graph node order", evidence)
	}
	output0, _ := evidence[0]["output"].(map[string]any)
	output1, _ := evidence[1]["output"].(map[string]any)
	if output0["stdout"] != "cpu: 4 cores" || output1["stdout"] != "memory: 15Gi" {
		t.Fatalf("graphEvidence outputs = %#v, want both parallel outputs", evidence)
	}
	if !reflect.DeepEqual(followUpInput["graphCompletedNodes"], []string{"cpu", "memory"}) {
		t.Fatalf("graphCompletedNodes = %#v, want both completed nodes", followUpInput["graphCompletedNodes"])
	}
}

func TestRuntimeOrchestrator_GraphFollowUpReusesLoadedStepsForEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-graph-follow-up-efficiency")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-follow-up-efficiency"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "cpu", Kind: "graph/mock"},
		{ID: "memory", Kind: "graph/mock"},
	}}
	anchor := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateSucceeded,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	for _, step := range []*runmodel.Step{
		anchor,
		{
			ID:     common.StepID(string(runID) + "/block/cpu"),
			RunID:  runID,
			Kind:   "graph/mock",
			State:  common.StepStateSucceeded,
			Input:  map[string]any{stepInputKeyGraphNodeID: "cpu", stepInputKeyGraphRootStepID: string(anchor.ID)},
			Output: map[string]any{"stdout": "cpu ok"},
		},
		{
			ID:     common.StepID(string(runID) + "/block/memory"),
			RunID:  runID,
			Kind:   "graph/mock",
			State:  common.StepStateSucceeded,
			Input:  map[string]any{stepInputKeyGraphNodeID: "memory", stepInputKeyGraphRootStepID: string(anchor.ID)},
			Output: map[string]any{"stdout": "memory ok"},
		},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, steps := orch.graphManager().readyGraphStepSpecsWithSteps(ctx, run, anchor)
	if len(specs) != 0 {
		t.Fatalf("ready specs = %+v, want no remaining graph node", specs)
	}
	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, anchor, steps)

	if got := stepRepo.listCount.Load(); got != 1 {
		t.Fatalf("step ListByRun count = %d, want 1 reused read", got)
	}
	if evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence]); len(evidence) != 2 {
		t.Fatalf("graphEvidence = %+v, want two evidence items", input[stepInputKeyGraphEvidence])
	}
	if !reflect.DeepEqual(input[stepInputKeyGraphCompletedNodes], []string{"cpu", "memory"}) {
		t.Fatalf("graphCompletedNodes = %#v, want cpu and memory", input[stepInputKeyGraphCompletedNodes])
	}
	runEvidence := mapSliceFromValue(input[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) != 3 {
		t.Fatalf("runEvidenceSummary = %+v, want root and graph node evidence", input[stepInputKeyRunEvidenceSummary])
	}
	foundCPUEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != string(runID)+"/block/cpu" {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundCPUEvidence = item["graphNodeId"] == "cpu" && output["stdout"] == "cpu ok"
	}
	if !foundCPUEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want graph node evidence", runEvidence)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpRestoresRepairContractAfterConcreteEvidenceAcquisition(t *testing.T) {
	runID := common.RunID("run-evidence-acquisition-restores-repair")
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-evidence-acquisition-restores-repair", InitialStepKind: llmWorkspaceExecutionStepKind}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "read-shape", Kind: "file/read"},
	}}
	anchor := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			failureOutputKeyClass:             failureClassShellCommand,
			failureOutputKeySignature:         "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"repairActionPackage": map[string]any{
				"requiredOutput":          evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
				failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
				"structuredFailureSignals": map[string]any{
					"targetRefs": []string{"main_test.go"},
				},
			},
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	readStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-read-shape"),
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "read-shape",
			stepInputKeyGraphRootStepID: string(anchor.ID),
		},
		Output: map[string]any{
			"status":      "completed",
			"stage":       "file_read",
			"path":        "main.go",
			"exists":      true,
			"content":     "package main\n\ntype Provider struct { BaseURL string }\n",
			"contentHash": "hash-main",
			"truncated":   false,
			"isBinary":    false,
		},
	}

	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, anchor, []*runmodel.Step{anchor, readStep})

	if input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input[stepInputKeyRuntimeStageMode] != "strict_convergence" ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["concreteFailureEvidenceRequired"] != nil {
		t.Fatalf("follow-up input = %+v, want restored state-changing repair contract", input)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want patch/apply contract", required)
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main" {
		t.Fatalf("targetSnapshots = %+v, want acquired current target snapshot", targets)
	}
	if !stringSliceContains(stringSliceFromValue(actionPackage["repairTargetSet"]), "main_test.go") ||
		!stringSliceContains(stringSliceFromValue(actionPackage["repairTargetSet"]), "main.go") {
		t.Fatalf("repairTargetSet = %+v, want original failure and acquired evidence targets", actionPackage["repairTargetSet"])
	}
}

func TestRuntimeOrchestrator_GraphFollowUpRestoresRepairContractFromRootAfterListAndShellEvidence(t *testing.T) {
	runID := common.RunID("run-evidence-acquisition-root-list-shell")
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-evidence-acquisition-root-list-shell", InitialStepKind: llmWorkspaceExecutionStepKind}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "list-targets", Kind: "file/list"},
		{ID: "check-target", Kind: "shell/command", DependsOn: []string{"list-targets"}},
	}}
	root := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			failureOutputKeyClass:             failureClassRuntime,
			failureOutputKeySignature:         "class=runtime_failed|kind=verification|failureTargets=resources/contract.txt",
			"repairActionPackage": map[string]any{
				"requiredOutput":          evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
				failureOutputKeySignature: "class=runtime_failed|kind=verification|failureTargets=resources/contract.txt",
			},
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	listStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/001-list-targets"),
		RunID: runID,
		Kind:  "file/list",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "list-targets",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepOutputKeyExecutionGraph: graph,
		},
		Output: map[string]any{
			"stage": "file_list",
			"entries": []map[string]any{
				{"path": "resources/contract.txt", "kind": "file", "contentHash": "hash-contract", "isBinary": false},
				{"path": "resources/archive", "kind": "directory"},
			},
		},
	}
	shellStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/002-check-target"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "check-target",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepOutputKeyExecutionGraph: graph,
		},
		Output: map[string]any{
			"stdout": "resources/contract.txt:12: missing required field",
		},
	}

	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, shellStep, []*runmodel.Step{root, listStep, shellStep})

	if input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input[stepInputKeyRuntimeStageMode] != "strict_convergence" ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["concreteFailureEvidenceRequired"] != nil {
		t.Fatalf("follow-up input = %+v, want restored state-changing repair contract from graph root", input)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want patch/apply contract", required)
	}
	if !stringSliceContains(stringSliceFromValue(actionPackage["repairTargetSet"]), "resources/contract.txt") {
		t.Fatalf("repairTargetSet = %+v, want evidence-acquired target", actionPackage["repairTargetSet"])
	}
	signals := mapFromValue(input["structuredFailureSignals"])
	if !stringSliceContains(stringSliceFromValue(signals["targetRefs"]), "resources/contract.txt") || len(mapSliceFromValue(signals["lineRefs"])) == 0 {
		t.Fatalf("structuredFailureSignals = %+v, want acquired target and line refs", signals)
	}
}

func TestRuntimeRepairFollowUpInheritsGraphSiblingEvidenceFromRoot(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-inherits-sibling-evidence")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:      stepRepo,
		workspaces: memory.NewWorkspaceRepository(),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-repair-inherits-sibling-evidence",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "read-target", Kind: "file/read"},
		{ID: "check-target", Kind: "shell/command", DependsOn: []string{"read-target"}},
	}}
	root := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
			},
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	readStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-read-target"),
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "read-target",
			stepInputKeyGraphRootStepID: string(root.ID),
		},
		Output: map[string]any{
			"status":           "completed",
			"stage":            "file_read",
			"path":             "main.go",
			"exists":           true,
			"content":          "package main\n\nfunc main() {}\n",
			"contentHash":      "hash-main",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"isBinary":         false,
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/002-check-target"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "check-target",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepInputKeyRuntimeStage:    runtimeStageExecution,
		},
		Output: map[string]any{
			"status":                  "failed",
			"stdout":                  "MISSING_REQUIRED_SYMBOL:Handle\n",
			"error":                   "exit status 1",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=check-target|exitCode=1",
		},
	}
	for _, step := range []*runmodel.Step{root, readStep, failedStep} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("exit status 1"))

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["concreteFailureEvidenceRequired"] != nil {
		t.Fatalf("repair input = %+v, want graph sibling evidence to restore state-changing repair contract", input)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	snapshots := mapSliceFromValue(pkg["targetSnapshots"])
	if len(snapshots) != 1 || snapshots[0]["targetRef"] != "main.go" || snapshots[0]["contentHash"] != "hash-main" {
		t.Fatalf("targetSnapshots = %+v, want target snapshot from sibling file/read evidence", snapshots)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpDoesNotRestoreRepairContractWithoutConcreteEvidence(t *testing.T) {
	runID := common.RunID("run-evidence-acquisition-no-target")
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-evidence-acquisition-no-target", InitialStepKind: llmWorkspaceExecutionStepKind}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "probe", Kind: "shell/command"},
	}}
	root := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
			},
		},
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	probeStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-probe"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "probe",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepOutputKeyExecutionGraph: graph,
		},
		Output: map[string]any{"stdout": "probe completed without target reference"},
	}

	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, probeStep, []*runmodel.Step{root, probeStep})

	if input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] == repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] == true ||
		input["repairActionPackage"] != nil {
		t.Fatalf("follow-up input = %+v, want no forced state-changing repair without concrete evidence", input)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpRunEvidenceIncludesPriorCompletedSteps(t *testing.T) {
	runID := common.RunID("run-graph-follow-up-run-evidence")
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-follow-up-run-evidence"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify", Kind: "shell/command"},
	}}
	priorStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/previous-validation"),
		RunID:  runID,
		Kind:   "shell/command",
		State:  common.StepStateSucceeded,
		Output: map[string]any{"stdout": "previous validation passed"},
	}
	anchor := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateSucceeded,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	graphStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/verify"),
		RunID:  runID,
		Kind:   "shell/command",
		State:  common.StepStateSucceeded,
		Input:  map[string]any{stepInputKeyGraphNodeID: "verify", stepInputKeyGraphRootStepID: string(anchor.ID)},
		Output: map[string]any{"stdout": "current graph validation passed"},
	}

	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, anchor, []*runmodel.Step{priorStep, anchor, graphStep})

	graphEvidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(graphEvidence) != 1 || graphEvidence[0]["nodeId"] != "verify" {
		t.Fatalf("graphEvidence = %+v, want current graph evidence only", input[stepInputKeyGraphEvidence])
	}
	runEvidence := mapSliceFromValue(input[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) != 3 {
		t.Fatalf("runEvidenceSummary = %+v, want prior, root, and current graph evidence", input[stepInputKeyRunEvidenceSummary])
	}
	foundPriorEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != string(priorStep.ID) {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundPriorEvidence = output["stdout"] == "previous validation passed"
	}
	if !foundPriorEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want prior completed evidence", runEvidence)
	}
}

func TestGraphExpansionLimitAssessmentInputIncludesLatestClosureEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-limit-assessment-closure")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-limit-assessment-closure", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	failedStepID := common.StepID(runID + "/graph-006/002-targeted-test")
	latestStepID := common.StepID(runID + "/graph-007/002-targeted-test")
	anchor := &runmodel.Step{
		ID:     common.StepID(runID + "/follow-up-graph-previous"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed"},
	}
	for _, step := range []*runmodel.Step{
		{ID: failedStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateFailed, Output: map[string]any{"stderr": "old targeted failure"}},
		{ID: latestStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateSucceeded, Output: map[string]any{"stdout": "latest targeted validation passed"}},
		anchor,
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID(runID + "/graph-007/001-apply-patch/patch/main-go"),
		WorkspaceID: run.WorkspaceID,
		RunID:       runID,
		StepID:      latestStepID,
		PatchType:   "unified_diff",
		TargetRef:   "main.go",
		Summary:     "applied target change",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(latestStepID) + "/validation/runtime-step"),
		WorkspaceID:     run.WorkspaceID,
		RunID:           runID,
		StepID:          latestStepID,
		ValidationKind:  "runtime_step",
		TargetRef:       "main.go",
		State:           common.ValidationResultStatePassed,
		Summary:         "targeted validation passed",
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Patches:           patchRepo,
			ValidationResults: validationRepo,
		},
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}

	input := orch.graphManager().graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx, run, anchor, steps, maxGraphFollowUpExpansionAttempts+1)

	if input[stepInputKeyAssessmentOnly] != true || input[stepInputKeyGraphLimitFollowUp] != true {
		t.Fatalf("input = %+v, want graph limit assessment-only markers", input)
	}
	closure := mapFromValue(input["latestClosureEvidence"])
	if len(closure) == 0 {
		t.Fatalf("latestClosureEvidence missing from graph limit assessment input: %+v", input)
	}
	latest := mapSliceFromValue(closure["latestEvidence"])
	if len(latest) == 0 || stringFromMap(latest[0], "evidenceRef") != string(latestStepID) {
		t.Fatalf("latestClosureEvidence = %+v, want latest successful step", closure)
	}
	if patches := mapSliceFromValue(closure["latestPatchEvidence"]); len(patches) == 0 || patches[0]["targetRef"] != "main.go" {
		t.Fatalf("latestPatchEvidence = %+v, want patch evidence", closure["latestPatchEvidence"])
	}
	if validations := mapSliceFromValue(closure["latestValidationEvidence"]); len(validations) == 0 || validations[0]["targetRef"] != "main.go" {
		t.Fatalf("latestValidationEvidence = %+v, want validation evidence", closure["latestValidationEvidence"])
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["assessmentEvidenceGuidance"]), "\n"), "do not cite only the run ID") {
		t.Fatalf("assessmentEvidenceGuidance = %+v, want concrete evidence ref guidance", input["assessmentEvidenceGuidance"])
	}
}

func TestCompletionFollowUpRestoresCoverageStateChangeConvergenceContract(t *testing.T) {
	runID := common.RunID("run-completion-cwi-state-change")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-completion-cwi-state-change",
		DoDID:       "dod",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-completion-cwi-state-change",
				"text": "deliver observable behavior",
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{{
							"id":           "version_control_snapshot",
							"required":     true,
							"evidenceRole": "primary_state_change",
						}},
					},
				},
			},
		},
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:     common.StepID(runID + "/graph-005/001-previous"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed"},
	}

	input := llmCompletionFollowUpStepInputFromSteps(run, step, []*runmodel.Step{step})

	if input[stepInputKeyAssessmentOnly] == true {
		t.Fatalf("input = %+v, want coverage convergence contract to clear assessment-only", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["concreteFailureEvidenceRequired"] != true ||
		input["stateChangingOutputRequired"] != nil {
		t.Fatalf("input = %+v, want concrete target evidence acquisition before state-changing repair", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract = %+v, want need_evidence", contract)
	}
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) == 0 || targets[0]["sourceRequirementId"] != "version_control_snapshot" {
		t.Fatalf("coverageTargetSet = %+v, want focused coverage work item target", input["coverageTargetSet"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteFailureEvidence"] != true ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want bounded evidence acquisition contract", required)
	}
}

func TestCompletionFollowUpPreservesAssessmentOnlyAfterAcceptedStateChangeEvidence(t *testing.T) {
	runID := common.RunID("run-completion-cwi-assessment-after-state-change")
	run := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      "ws-completion-cwi-assessment-after-state-change",
		DoDID:            "dod",
		ExecutionContext: map[string]any{},
		InitialStepKind:  llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:     common.StepID(runID + "/artifact-apply-001"),
		RunID:  runID,
		Kind:   "artifact/apply",
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed"},
	}

	input := llmCompletionFollowUpStepInput(run, step)
	input[coverageWorkItemContextKey] = map[string]any{
		"currentFocus": map[string]any{
			"id":                    "cwi-artifact-output",
			"status":                "open",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "artifact-output",
			"targetText":            "artifact output exists and needs final assessment",
			"requiredEvidenceKinds": []string{"state_change", "validation"},
			"missingEvidenceKinds":  []string{"validation"},
			"acceptedEvidenceRefs":  []string{"run-completion-cwi-assessment-after-state-change/artifact-apply"},
			"acceptedEvidenceByKind": map[string]any{
				"state_change": []string{"run-completion-cwi-assessment-after-state-change/artifact-apply"},
			},
			"acceptedEvidenceBindingByKind": map[string]any{
				"state_change": []string{"run-completion-cwi-assessment-after-state-change/artifact-apply"},
			},
		},
	}
	input = restoreCoverageWorkItemContinuationConvergenceContract(run, input)

	if input["completionFollowUp"] != true || input[stepInputKeyAssessmentOnly] != true ||
		input["assessmentOnlyReason"] != "completion_follow_up" {
		t.Fatalf("input = %+v, want completion assessment-only markers preserved", input)
	}
	if input["repairObjective"] != nil || input["boundedValidationRequired"] != nil ||
		input["stateChangingOutputRequired"] != nil {
		t.Fatalf("input = %+v, want no executable validation or state-changing contract", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment ||
		stringFromMap(contract, "runtimeStage") != runtimeStageVerification {
		t.Fatalf("convergenceContract = %+v, want need_assessment verification contract", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if len(required) != 0 {
		t.Fatalf("requiredOutput = %+v, want no executable required output on completion assessment", required)
	}
}

func TestGraphExpansionLimitFollowUpRestoresCoverageValidationConvergenceContract(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-limit-cwi-validation")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-graph-limit-cwi-validation",
		DoDID:       "dod",
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"id":    "dod-validation",
				"items": []string{"focused behavior is validated"},
			},
		},
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	stepRepo := memory.NewStepRepository()
	anchor := &runmodel.Step{
		ID:     common.StepID(runID + "/follow-up-graph-previous"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed"},
	}
	if err := stepRepo.Create(ctx, anchor); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	orch := &RuntimeOrchestrator{steps: stepRepo}

	input := orch.graphManager().graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx, run, anchor, steps, maxGraphFollowUpExpansionAttempts+1)

	if input[stepInputKeyAssessmentOnly] != true ||
		input["assessmentOnlyReason"] != "graph_follow_up_expansion_limit_exceeded" {
		t.Fatalf("input = %+v, want graph-limit assessment-only contract", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != nil ||
		input["boundedValidationRequired"] != nil {
		t.Fatalf("input = %+v, want no executable validation contract", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment {
		t.Fatalf("convergenceContract = %+v, want need_assessment", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if len(required) != 0 {
		t.Fatalf("requiredOutput = %+v, want no executable required output", required)
	}
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) != 1 || targets[0]["sourceRequirementId"] != "dod-validation-dod-item-001" {
		t.Fatalf("coverageTargetSet = %+v, want focused coverage work item target", input["coverageTargetSet"])
	}
}

func TestGraphFollowUpRestoresCoverageValidationConvergenceContract(t *testing.T) {
	runID := common.RunID("run-graph-follow-up-cwi-validation")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-graph-follow-up-cwi-validation",
		DoDID:       "dod",
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"id":    "dod-graph-validation",
				"items": []string{"focused graph follow-up behavior is validated"},
			},
		},
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	anchor := &runmodel.Step{
		ID:     common.StepID(runID + "/graph-001/002-verify"),
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed", "stdout": "bounded validation candidate"},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.graphManager().graphFollowUpStepInputFromSteps(run, anchor, []*runmodel.Step{anchor})

	if input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true {
		t.Fatalf("input = %+v, want concrete validation convergence contract", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		required["mustDeclareCoverageTargets"] != true ||
		required["mustReferenceCoverageWorkItem"] != true {
		t.Fatalf("requiredOutput = %+v, want coverage-bound validation contract", required)
	}
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) != 1 || targets[0]["sourceRequirementId"] != "dod-graph-validation-dod-item-001" {
		t.Fatalf("coverageTargetSet = %+v, want focused coverage work item target", input["coverageTargetSet"])
	}
}

func TestMergeRunExecutionContextPatchRefreshesEffectiveDefinitionIDs(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-definition-patch",
		WorkspaceID:      "ws-definition-patch",
		GoalID:           "goal-old",
		DoDID:            "dod-old",
		ExecutionContext: map[string]any{"existing": "kept"},
	}

	mergeRunExecutionContextPatch(run, map[string]any{
		"goalId": "goal-new",
		"doDId":  "dod-new",
		"goal":   map[string]any{"id": "goal-new"},
		"dod":    map[string]any{"id": "dod-new"},
	})

	if run.GoalID != "goal-new" || run.DoDID != "dod-new" {
		t.Fatalf("run definitions = %s/%s, want refreshed IDs", run.GoalID, run.DoDID)
	}
	if run.ExecutionContext["existing"] != "kept" || run.ExecutionContext["goalId"] != "goal-new" || run.ExecutionContext["doDId"] != "dod-new" {
		t.Fatalf("executionContext = %+v, want merged definition patch", run.ExecutionContext)
	}
}

func TestRuntimeOrchestrator_DispatchStepGraphFollowUpReusesLoadedStepsForEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	workspaceID := common.WorkspaceID("ws-graph-follow-up-dispatch-efficiency")
	runID := common.RunID("run-graph-follow-up-dispatch-efficiency")
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "loadtest-tool", Kind: "mcp/tool_call"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == common.StepID(string(runID)+"/block/loadtest-tool"):
			return map[string]any{
				"status": "completed",
				"stdout": "loadtest evidence",
			}, nil
		case strings.HasPrefix(string(request.StepID), string(runID)+"/follow-up-graph-"):
			followUpInput = request.Input
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "graph follow-up accepted",
					"items": []map[string]any{
						{
							"item":         "graph evidence reused",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := &RuntimeOrchestrator{
		runtime:    runtime,
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			Results:            resultRepo,
			Artifacts:          artifactRepo,
		},
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-follow-up-dispatch-efficiency"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/llm-after-approval"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateSucceeded,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	graphStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/loadtest-tool"),
		RunID:  runID,
		Kind:   "mcp/tool_call",
		State:  common.StepStateExecuting,
		Input:  map[string]any{stepInputKeyGraphNodeID: "loadtest-tool", stepInputKeyGraphRootStepID: string(root.ID), stepOutputKeyExecutionGraph: graph, "previousStepId": string(root.ID)},
		Output: map[string]any{"stdout": "loadtest evidence"},
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, root); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, graphStep); err != nil {
		t.Fatalf("seed graph step: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, graphStep.ID, graphStep.Kind, graphStep.Input, workspace); err != nil {
		t.Fatalf("dispatch graph step: %v", err)
	}

	if got := stepRepo.graphEvidenceListCount.Load(); got != 0 {
		t.Fatalf("graph evidence ListByRun count = %d, want reused loaded steps without an extra evidence scan", got)
	}
	if evidence := mapSliceFromValue(followUpInput[stepInputKeyGraphEvidence]); len(evidence) != 1 || evidence[0]["nodeId"] != "loadtest-tool" {
		t.Fatalf("graphEvidence = %+v, want loadtest-tool evidence", followUpInput[stepInputKeyGraphEvidence])
	}
	if _, ok := followUpInput["previousStepOutput"]; ok {
		t.Fatalf("previousStepOutput = %+v, want graph follow-up payload to avoid duplicate evidence", followUpInput["previousStepOutput"])
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_DispatchesRepeatedGraphNodeIDInNewGraphScope(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	firstGraph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	secondGraph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{
			ID:   "verify-file",
			Kind: "patch/apply",
			Input: map[string]any{
				"patchType": "file_write",
				"targetRef": "main.go",
				"content":   "package main\n",
			},
		},
	}}
	var secondGraphStepID common.StepID
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-graph-repeat/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": firstGraph,
				}, nil
			case request.StepID == "run-graph-repeat/graph-001/001-verify-file":
				return map[string]any{
					"status": "completed",
					"stdout": "first verify",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && request.Input["previousStepId"] == "run-graph-repeat/graph-001/001-verify-file":
				return map[string]any{
					"status":         "completed",
					"executionGraph": secondGraph,
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-repeat/graph-002/") && strings.HasSuffix(string(request.StepID), "-verify-file"):
				secondGraphStepID = request.StepID
				return map[string]any{
					"status": "completed",
					"stdout": "second verify",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && secondGraphStepID != "" && request.Input["previousStepId"] == string(secondGraphStepID):
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "second graph evidence assessed",
						"items": []map[string]any{
							{
								"item":         "repeated graph node completed",
								"state":        "satisfied",
								"evidenceRefs": []string{string(secondGraphStepID)},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected repeated graph step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-repeat")
	runID := common.RunID("run-graph-repeat")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-repeat"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run state = %s failureReason=%q calls=%#v steps=%+v, want completed", runItem.State, runItem.FailureReason, runtime.calls, steps)
	}
	if secondGraphStepID == "" || secondGraphStepID == "run-graph-repeat/graph-001/001-verify-file" {
		t.Fatalf("secondGraphStepID = %q, want graph-root-scoped unique step id", secondGraphStepID)
	}
	if strings.Contains(string(secondGraphStepID), "zz-llm-after-graph") || strings.Contains(string(secondGraphStepID), "run-graph-repeat-step-1") {
		t.Fatalf("secondGraphStepID = %q, want readable scoped id without embedded ancestor lineage", secondGraphStepID)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitRoutesToAssessmentOnly(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var graphExpansions int
	var assessmentOnlySeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
					t.Fatalf("assessment-only input unexpectedly uses runtime repair follow-up: %+v", request.Input)
				}
				if _, ok := request.Input[stepOutputKeyExecutionGraph]; ok {
					t.Fatalf("assessment-only input retained executionGraph: %+v", request.Input[stepOutputKeyExecutionGraph])
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "graph expansion bounded into final assessment",
						"goalOutcome": "The run completed after graph expansion assessment.",
						"items": []map[string]any{
							{
								"item":         "graph expansion bounded",
								"state":        "satisfied",
								"evidenceRefs": []string{string(request.StepID)},
							},
						},
					},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				graphExpansions++
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   fmt.Sprintf("verify_%d", graphExpansions),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": fmt.Sprintf("graph-%d.txt", graphExpansions),
								"content":   fmt.Sprintf("graph expansion %d\n", graphExpansions),
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-expansion-limit")
	runID := common.RunID("run-graph-expansion-limit")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-expansion-limit"),
		State:           common.RunStatePending,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not dispatched after graph expansion limit")
	}
	if graphExpansions != maxGraphFollowUpExpansionAttempts+1 {
		t.Fatalf("graphExpansions = %d, want one bounded overflow beyond limit %d", graphExpansions, maxGraphFollowUpExpansionAttempts)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after assessment-only follow-up", runItem, steps)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAllowsConcreteValidationEvidenceGraph(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	runID := common.RunID("run-graph-limit-validation-evidence")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-graph-limit-validation-evidence",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateActive,
	}
	for i := 0; i < maxGraphFollowUpExpansionAttempts; i++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/follow-up-%d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateSucceeded,
			Input: map[string]any{"graphFollowUp": true},
			Output: map[string]any{
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: fmt.Sprintf("patch-%d", i), Kind: "patch/apply", Input: map[string]any{"patchType": "file_write", "targetRef": fmt.Sprintf("file-%d.txt", i), "content": "ok"}},
				}},
			},
		}
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed expansion step: %v", err)
		}
	}
	current := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-validation"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"graphFollowUp":              true,
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
		},
		Output: map[string]any{
			stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
				{ID: "verify", Kind: "shell/command", Input: map[string]any{"command": "true"}},
			}},
		},
	}
	if err := stepRepo.Create(ctx, current); err != nil {
		t.Fatalf("seed current step: %v", err)
	}

	handled, err := orch.validateGraphExpansionBoundary(ctx, run, current, nil, current.Output, nil)
	if err != nil {
		t.Fatalf("validate graph expansion boundary: %v", err)
	}
	if handled {
		t.Fatalf("handled = true, want concrete validation evidence graph to bypass graph expansion assessment")
	}
	if boolValue(current.Output["ignoredExecutionGraph"]) {
		t.Fatalf("concrete validation graph was ignored: %+v", current.Output)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAppliesToRepeatedConcreteValidationEvidenceGraph(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				return map[string]any{
					"completionState": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "bounded concrete validation graph expansion assessed",
						"goalOutcome": "bounded",
						"items": []map[string]any{{
							"item":         "bounded",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			return map[string]any{"status": "completed"}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-limit-repeated-validation-evidence")
	runID := common.RunID("run-graph-limit-repeated-validation-evidence")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateActive,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for i := 0; i < maxGraphFollowUpExpansionAttempts; i++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/follow-up-validation-%d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateSucceeded,
			Input: map[string]any{"graphFollowUp": true},
			Output: map[string]any{
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: fmt.Sprintf("verify-%d", i), Kind: "shell/command", Input: map[string]any{"command": "true"}},
				}},
			},
		}
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed expansion step: %v", err)
		}
	}
	current := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-current"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateExecuting,
		Input: map[string]any{
			"graphFollowUp":              true,
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
		},
		Output: map[string]any{
			stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
				{ID: "verify-current", Kind: "shell/command", Input: map[string]any{"command": "true"}},
			}},
		},
	}
	if err := stepRepo.Create(ctx, current); err != nil {
		t.Fatalf("seed current step: %v", err)
	}

	handled, err := orch.validateGraphExpansionBoundary(ctx, run, current, nil, current.Output, workspace)
	if err != nil {
		t.Fatalf("validate graph expansion boundary: %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want repeated concrete validation evidence graph to obey graph expansion limit")
	}
	stored, err := stepRepo.Get(ctx, current.ID)
	if err != nil {
		t.Fatalf("get current step: %v", err)
	}
	if !boolValue(stored.Output["ignoredExecutionGraph"]) ||
		stored.Output["graphFollowUpExpansionKind"] != "concrete_validation_evidence" {
		t.Fatalf("current output = %+v, want concrete validation expansion limit marker", stored.Output)
	}
	if stored.State != common.StepStateSucceeded ||
		stringFromMap(stored.Output, failureOutputKeyClass) != "" ||
		stringFromMap(stored.Output, "contractViolationType") != "" {
		t.Fatalf("stored step state=%s output=%+v, want completed transition without failure diagnostics", stored.State, stored.Output)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAppliesToConcreteFailureEvidenceAcquisition(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				return map[string]any{
					"completionState": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":             true,
						"summary":               "graph expansion limit assessment completed",
						"goalOutcome":           "bounded",
						"implementationSummary": "bounded",
						"verificationSummary":   "bounded",
						"items": []map[string]any{{
							"item":         "bounded",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			return map[string]any{"status": "completed"}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-graph-limit-concrete-failure-evidence")
	runID := common.RunID("run-graph-limit-concrete-failure-evidence")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateActive,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for i := 0; i <= maxGraphFollowUpExpansionAttempts; i++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/follow-up-%d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				"graphFollowUp":                   true,
				"concreteFailureEvidenceRequired": true,
				"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			},
			Output: map[string]any{
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: fmt.Sprintf("verify-%d", i), Kind: "shell/command", Input: map[string]any{"command": "true"}},
				}},
			},
		}
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed expansion step: %v", err)
		}
	}
	current := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-current"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateExecuting,
		Input: map[string]any{
			"graphFollowUp":                   true,
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		},
		Output: map[string]any{
			stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
				{ID: "verify-current", Kind: "shell/command", Input: map[string]any{"command": "true"}},
			}},
		},
	}
	if err := stepRepo.Create(ctx, current); err != nil {
		t.Fatalf("seed current step: %v", err)
	}

	handled, err := orch.validateGraphExpansionBoundary(ctx, run, current, nil, current.Output, workspace)
	if err != nil {
		t.Fatalf("validate graph expansion boundary: %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want concrete failure evidence graph to obey graph expansion limit")
	}
	stored, err := stepRepo.Get(ctx, current.ID)
	if err != nil {
		t.Fatalf("get current step: %v", err)
	}
	if !boolValue(stored.Output["ignoredExecutionGraph"]) {
		t.Fatalf("ignoredExecutionGraph = %v, want true", stored.Output["ignoredExecutionGraph"])
	}
	if stored.State != common.StepStateSucceeded ||
		stringFromMap(stored.Output, failureOutputKeyClass) != "" ||
		stringFromMap(stored.Output, "contractViolationType") != "" {
		t.Fatalf("stored step state=%s output=%+v, want completed transition without failure diagnostics", stored.State, stored.Output)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAllowsContractedStateChangingGraph(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	runID := common.RunID("run-graph-limit-state-change")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-graph-limit-state-change",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateActive,
	}
	for i := 0; i < maxGraphFollowUpExpansionAttempts; i++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/follow-up-%d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateSucceeded,
			Input: map[string]any{"graphFollowUp": true},
			Output: map[string]any{
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: fmt.Sprintf("patch-%d", i), Kind: "patch/apply", Input: map[string]any{"patchType": "file_write", "targetRef": fmt.Sprintf("file-%d.txt", i), "content": "ok"}},
				}},
			},
		}
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed expansion step: %v", err)
		}
	}
	current := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-state-change"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"graphFollowUp":               true,
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
				{ID: "patch", Kind: "patch/apply", Input: map[string]any{"patchType": "file_write", "targetRef": "target.txt", "content": "changed"}},
				{ID: "verify", Kind: "shell/command", DependsOn: []string{"patch"}, Input: map[string]any{"command": "true"}},
			}},
		},
	}
	if err := stepRepo.Create(ctx, current); err != nil {
		t.Fatalf("seed current step: %v", err)
	}

	handled, err := orch.validateGraphExpansionBoundary(ctx, run, current, nil, current.Output, nil)
	if err != nil {
		t.Fatalf("validate graph expansion boundary: %v", err)
	}
	if handled {
		t.Fatalf("handled = true, want contracted state-changing graph to bypass graph expansion assessment")
	}
	if boolValue(current.Output["ignoredExecutionGraph"]) {
		t.Fatalf("contracted state-changing graph was ignored: %+v", current.Output)
	}
}

func TestValidateLLMFollowUpGraphAllowsConcreteFailureEvidenceAcquisitionGraph(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-evidence/follow-up",
		RunID: "run-evidence",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeStageMode:      "strict_convergence",
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds": []string{
						"file/list",
						"file/read",
						"git/read",
						"shell/command",
						"mcp/tool_call",
					},
					"mustProduceConcreteFailureEvidence": true,
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
			{ID: "diff", Kind: "git/read", Input: map[string]any{"operation": "diff"}},
			{ID: "verify", Kind: "shell/command", DependsOn: []string{"diff"}, Input: map[string]any{"command": "true"}},
			{ID: "read", Kind: "file/read", DependsOn: []string{"verify"}, Input: map[string]any{"path": "target.txt"}},
		}},
	}
	if err := validateLLMFollowUpGraphStateChangingContract(&runmodel.Run{}, step, output); err != nil {
		t.Fatalf("validate evidence acquisition graph: %v", err)
	}
}

func TestValidateLLMFollowUpGraphAllowsBoundedReasonDuringConcreteFailureEvidenceAcquisition(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-evidence/follow-up-bounded-reason",
		RunID: "run-evidence",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeStageMode:      "strict_convergence",
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds": []string{
						"file/list",
						"file/read",
						"git/read",
						"shell/command",
						"mcp/tool_call",
					},
					"mustProduceConcreteFailureEvidence": true,
					"stateChangingForbidden":             true,
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
			{ID: "list", Kind: "file/list", Input: map[string]any{"path": "", "recursive": false}},
			{
				ID:        "verify",
				Kind:      "shell/command",
				DependsOn: []string{"list"},
				Input: map[string]any{
					"command":       "verify",
					"args":          []string{"--scope", "target.txt"},
					"boundedReason": "bounded read-only failure evidence acquisition",
					"relatedFailureTargets": []string{
						"target.txt",
					},
					"verificationScope": "failure-evidence",
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
					},
				},
			},
		}},
	}
	if err := validateLLMFollowUpGraphStateChangingContract(&runmodel.Run{}, step, output); err != nil {
		t.Fatalf("validate evidence acquisition graph with boundedReason: %v", err)
	}
}

func TestValidateLLMFollowUpGraphAllowsDeclaredToolStateChangingRepairGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-declared-tool-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    "run-declared-tool-state-change/follow-up",
		RunID: "run-declared-tool-state-change",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":               true,
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failedStepId":                "run-declared-tool-state-change/graph-001/001-verify",
			"lastFailureDiagnostics": map[string]any{
				"failureClass":   failureClassShellCommand,
				"failureTargets": []string{"target.txt"},
				"error":          "target state is stale",
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                  []string{"shell/command", "mcp/tool_call"},
					"mustIncludeStateChangingNode":   true,
					"mustProduceStateChangingOutput": true,
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
			{
				ID:   "materialize-target-state",
				Kind: "shell/command",
				Input: map[string]any{
					"command":               "sh",
					"args":                  []string{"-c", "printf '%s\\n' refreshed > target.txt"},
					"stateChanging":         true,
					"targetRef":             "target.txt",
					"stateChangingReason":   "refresh derived workspace state from current inputs",
					"relatedFailureTargets": []string{"target.txt"},
				},
			},
			{
				ID:        "verify-target-state",
				Kind:      "shell/command",
				DependsOn: []string{"materialize-target-state"},
				Input: map[string]any{
					"command":               "sh",
					"args":                  []string{"-c", "test -s target.txt"},
					"targetRef":             "target.txt",
					"verificationScope":     "target.txt",
					"boundedReason":         "verify the declared state-changing target only",
					"relatedFailureTargets": []string{"target.txt"},
				},
			},
		}},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validate declared tool state-changing repair graph: %v", err)
	}
}

func TestValidateLLMFollowUpGraphRejectsPatchDuringConcreteFailureEvidenceAcquisition(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-evidence/follow-up",
		RunID: "run-evidence",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
			{ID: "patch", Kind: "patch/apply", Input: map[string]any{"patchType": "file_write", "targetRef": "target.txt", "content": "changed"}},
		}},
	}
	err := validateLLMFollowUpGraphStateChangingContract(&runmodel.Run{}, step, output)
	if err == nil || !strings.Contains(err.Error(), "concrete failure evidence acquisition") {
		t.Fatalf("err = %v, want evidence acquisition contract rejection", err)
	}
}

func TestValidateLLMFollowUpGraphRejectsVerificationReadOnlyGraphAsValidationEvidenceGap(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-validation/follow-up",
		RunID: "run-validation",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":          true,
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
			{ID: "read", Kind: "file/read", Input: map[string]any{"path": "target.txt"}},
		}},
	}
	err := validateLLMFollowUpGraphStateChangingContract(&runmodel.Run{}, step, output)
	if err == nil {
		t.Fatalf("err = nil, want validation evidence contract rejection")
	}
	if !strings.Contains(err.Error(), "concrete validation evidence") {
		t.Fatalf("err = %v, want concrete validation evidence rejection", err)
	}
	if strings.Contains(err.Error(), "patch output") || strings.Contains(err.Error(), "patch/apply") {
		t.Fatalf("err = %v, should not require state-changing patch output for verification", err)
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyContractViolationDispatchesOneRetry(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var assessmentOnlySeen bool
	var retrySeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if recovery := mapFromValue(request.Input[stepInputKeyContractViolationRecovery]); len(recovery) > 0 {
				retrySeen = true
				if _, ok := request.Input[stepOutputKeyExecutionGraph]; ok {
					t.Fatalf("retry input retained executionGraph: %+v", request.Input)
				}
				if actual := mapFromValue(recovery["actualOutputSummary"]); len(actual) > 0 && actual[stepOutputKeyExecutionGraph] == nil {
					t.Fatalf("contractViolationRecovery.actualOutputSummary = %+v, want compact rejected graph summary", actual)
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "assessment-only retry completed",
						"goalOutcome": "assessment-only retry completed",
						"items": []map[string]any{{
							"item":         "contract corrected",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-follow-up.txt",
								"content":   "assessment follow-up\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-assessment-only-rejects-graph")
	runID := common.RunID("run-assessment-only-rejects-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-rejects-graph"),
		State:           common.RunStatePending,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not reached")
	}
	if !retrySeen {
		t.Fatalf("assessment-only contract violation retry was not reached")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after contract violation retry", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
	}
}

func TestAssessmentOnlyContractCorrectionInputStripsRecoveryMarkersAndNestedExecutableFields(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-correction/zz-llm-after-graph-step-provider-retry-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly:        true,
			"providerRetryAssessmentOnly":     true,
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: "run-correction/zz-llm-after-graph-step-provider-retry-1",
			stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
			stepInputKeyRecoveryRootStepID:    "run-correction/zz-llm-after-graph-step-provider-retry-1",
			stepInputKeyRecoveryLineageType:   "provider_retry",
			stepInputKeyRecoveryBudgetScope: map[string]any{
				"lineageType": "provider_retry",
				"rootStepId":  "run-correction/zz-llm-after-graph-step-provider-retry-1",
			},
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 "produce_state_changing_output",
			"governanceHistory": []map[string]any{{
				"resolutionPayload": map[string]any{"recoveryAction": "replan"},
			}},
			"executionContextPackage": map[string]any{
				"replanContinuation": map[string]any{
					"resolutionPayload": map[string]any{"recoveryAction": "replan"},
				},
			},
			"failedStepOutput": map[string]any{
				"convertedDecision": map[string]any{"question": "choose a continuation"},
			},
			"previousStepOutput": map[string]any{
				"llmDecision": map[string]any{
					stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
					"patchRequests":             []any{map[string]any{"targetRef": "x"}},
				},
				"items": []any{map[string]any{
					"shellCommands": []any{map[string]any{"command": "echo"}},
				}},
			},
		},
	}
	output := map[string]any{
		"llmDecision": map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
		},
		stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
	}

	input := assessmentOnlyContractCorrectionStepInput(step, output, errors.New("assessment-only step returned executionGraph"))
	for _, key := range []string{
		"providerRetryAssessmentOnly",
		stepInputKeyProviderRetryFollowUp,
		stepInputKeyProviderRetryAttempt,
		stepInputKeyProviderRetryRootStep,
		stepInputKeyRecoveryAction,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyRecoveryLineageType,
		stepInputKeyRecoveryBudgetScope,
		stepInputKeyRuntimeRepairFollowUp,
		"stateChangingOutputRequired",
		"repairObjective",
		"governanceHistory",
		"failedStepOutput",
		"previousStepOutput",
	} {
		if _, ok := input[key]; ok {
			t.Fatalf("correction input retained %s: %+v", key, input)
		}
	}
	if input[stepInputKeyAssessmentOnly] != true || input["assessmentOnlyContractCorrection"] != true {
		t.Fatalf("correction input = %+v, want assessment-only correction markers", input)
	}
	executionContextPackage := mapFromValue(input["executionContextPackage"])
	if executionContextPackage["replanContinuation"] != nil {
		t.Fatalf("correction input retained nested replanContinuation: %+v", executionContextPackage)
	}
	invalid := mapFromValue(input["previousInvalidAssessmentOutput"])
	if nested := mapFromValue(invalid["llmDecision"]); nested[stepOutputKeyExecutionGraph] != nil {
		t.Fatalf("previousInvalidAssessmentOutput retained nested graph: %+v", invalid)
	}
	if invalid["llmDecision"] != nil {
		t.Fatalf("previousInvalidAssessmentOutput retained llmDecision: %+v", invalid)
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyExecutableOutputRetriesWithoutSideEffects(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	var assessmentOnlySeen bool
	var contractRetrySeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if len(mapFromValue(request.Input[stepInputKeyContractViolationRecovery])) > 0 {
				contractRetrySeen = true
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "assessment-only corrected after rejected executable output",
						"goalOutcome": "assessment-only corrected after rejected executable output",
						"items": []map[string]any{{
							"item":         "contract corrected",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				return map[string]any{
					"status": "completed",
					stepOutputKeyMemoryWrites: []map[string]any{{
						"memoryType": "project_context",
						"summary":    "assessment-only rejected output must not persist",
						"content":    "This memory write belongs to an invalid assessment-only response.",
					}},
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "assessment-only completed despite ignored graph",
						"goalOutcome": "assessment-only completed despite ignored graph",
						"items": []map[string]any{{
							"item":         "contract corrected",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-repeat.txt",
								"content":   "assessment repeat\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{MemoryItems: memoryRepo})

	workspaceID := common.WorkspaceID("ws-assessment-only-ignores-graph-with-assessment")
	runID := common.RunID("run-assessment-only-ignores-graph-with-assessment")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-ignores-graph-with-assessment"),
		State:           common.RunStatePending,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not reached")
	}
	if !contractRetrySeen {
		t.Fatalf("contract violation retry was not reached")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after rejected assessment-only graph retry", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var violationMarked bool
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
		if stringFromMap(step.Output, outputKeyContractViolationType) == contractViolationAssessmentOnlyExecutableOutput ||
			len(mapFromValue(step.Output[stepOutputKeyContractViolation])) > 0 {
			violationMarked = true
			if refs := stringSliceFromValue(step.Output[stepOutputKeyMemoryRefs]); len(refs) > 0 {
				t.Fatalf("contract violation step memoryRefs = %+v, rejected output must not create memory refs", refs)
			}
		}
	}
	if !violationMarked {
		t.Fatalf("steps = %+v, want rejected diagnostic marker on assessment step", steps)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	for _, item := range memories {
		if strings.Contains(item.Summary, "assessment-only rejected output must not persist") ||
			strings.Contains(item.Content, "This memory write belongs to an invalid assessment-only response.") {
			t.Fatalf("memories = %+v, rejected assessment-only output must not create long-term memory", memories)
		}
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyContractViolationEscalatesRepeatedGraph(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var retrySeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if len(mapFromValue(request.Input[stepInputKeyContractViolationRecovery])) > 0 {
				retrySeen = true
			}
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-repeated-graph.txt",
								"content":   "assessment repeated graph\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-assessment-only-rejects-repeated-graph")
	runID := common.RunID("run-assessment-only-rejects-repeated-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-rejects-repeated-graph"),
		State:           common.RunStatePending,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if !retrySeen {
		t.Fatalf("contract violation retry was not reached")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want decision after repeated contract violation", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyDecisionForContinuableCandidateRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "repair produced implementation evidence",
					"goalOutcome": "implementation evidence is now available",
					"items": []map[string]any{{
						"item":         "produce implementation",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		}
		return map[string]any{
			"status":                           "completed",
			"stage":                            "llm_decision_required",
			stepOutputKeyBlockDecisionReason:   "empty workspace requires execution target confirmation",
			stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
			stepOutputKeyBlockDecisionQuestion: "Initialize the current workspace or provide existing project content?",
			stepOutputKeyBlockDecisionContext: map[string]any{
				"runtimeCanContinue": true,
				"candidateValues": map[string]any{
					"executionTargetMode": []any{"init_from_scratch", "use_existing_project"},
				},
				"missingFields":       []any{"executionTargetMode"},
				"requiresHumanFields": []any{"executionTargetMode"},
			},
		}, nil
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		})

	workspaceID := common.WorkspaceID("ws-assessment-only-decision-repair")
	runID := common.RunID("run-assessment-only-decision-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-assessment-only-decision-repair"),
		DoDID:           common.DoDID("dod-assessment-only-decision-repair"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"produce implementation"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, "run-assessment-only-decision-repair/provider-retry", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeySoulStage:         soulStageValidation,
		"providerRetryAssessmentOnly": true,
		"providerRetryMode":           "completion_assessment_only",
	}, nil); err != nil {
		t.Fatalf("dispatch assessment-only step: %v", err)
	}

	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		ids := make([]common.TicketID, 0, len(tickets))
		for _, ticket := range tickets {
			ids = append(ids, ticket.ID)
		}
		steps, _ := stepRepo.ListByRun(ctx, runID)
		stepSummaries := make([]map[string]any, 0, len(steps))
		for _, step := range steps {
			stepSummaries = append(stepSummaries, map[string]any{
				"id":    step.ID,
				"state": step.State,
				"input": step.Input,
				"stage": stringFromMap(step.Output, "stage"),
				"error": firstNonEmptyString(stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason")),
			})
		}
		t.Fatalf("tickets = %+v ids=%v steps=%+v, want no decision ticket for continuable assessment-only candidate", tickets, ids, stepSummaries)
	}
	failed, err := stepRepo.Get(ctx, "run-assessment-only-decision-repair/provider-retry")
	if err != nil {
		t.Fatalf("get assessment step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["assessmentOnlyDecisionConvertedToRepair"] != true {
		t.Fatalf("assessment step = %+v, want failed converted to repair", failed)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var repairSeen bool
	for _, step := range steps {
		if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairSeen = true
			break
		}
	}
	if !repairSeen {
		t.Fatalf("steps = %+v, want runtime repair follow-up", steps)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_FailedAssessmentDecisionWithActionableCoverageRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                           "completed",
				"stage":                            "llm_decision_required",
				stepOutputKeyBlockDecisionReason:   "current stage does not allow state-changing output",
				stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
				stepOutputKeyBlockDecisionQuestion: "Allow runtime to continue with targeted repair?",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": false,
					"summary":   "coverage gaps remain",
					"items": []map[string]any{{
						"item":        "observable behavior",
						"state":       "failed",
						"reason":      "missing direct validation evidence",
						"nextActions": []string{"add bounded validation evidence and rerun verification"},
					}},
				},
				"llmDecision": map[string]any{
					"completionState": "blocked_on_decision",
				},
			}, nil
		}
		if !boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			t.Fatalf("repair input = %+v, want runtime repair follow-up", request.Input)
		}
		return map[string]any{
			"status": "completed",
			"stage":  "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied":   true,
				"summary":     "coverage repaired",
				"goalOutcome": "direct evidence is now available",
				"items": []map[string]any{{
					"item":         "observable behavior",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
			},
		}, nil
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		})

	workspaceID := common.WorkspaceID("ws-failed-assessment-decision-repair")
	runID := common.RunID("run-failed-assessment-decision-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-failed-assessment-decision-repair"),
		DoDID:           common.DoDID("dod-failed-assessment-decision-repair"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"observable behavior"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, "run-failed-assessment-decision-repair/validation", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeySoulStage:        soulStageValidation,
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
	}, nil); err != nil {
		t.Fatalf("dispatch validation step: %v", err)
	}

	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no decision ticket for actionable failed assessment", tickets)
	}
	first, err := stepRepo.Get(ctx, "run-failed-assessment-decision-repair/validation")
	if err != nil {
		t.Fatalf("get validation step: %v", err)
	}
	if first.State != common.StepStateFailed || first.Output["actionableFailedAssessmentDecisionConvertedToRepair"] != true {
		t.Fatalf("validation step = %+v, want decision converted to repair", first)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_ContinuableDecisionRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "initialized project from scratch",
					"goalOutcome": "project bootstrap evidence is available",
					"items": []map[string]any{{
						"item":         "produce implementation",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		}
		return map[string]any{
			"status":                           "completed",
			"stage":                            "llm_decision_required",
			stepOutputKeyBlockDecisionReason:   "empty workspace requires execution target confirmation",
			stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
			stepOutputKeyBlockDecisionQuestion: "Sync existing code or initialize a new project?",
			stepOutputKeyBlockDecisionContext: map[string]any{
				"runtimeCanContinue": true,
				"candidateValues": map[string]any{
					"initializationPermission": []any{"允许在空目录从零初始化 Go 网关项目", "不允许初始化，仅等待现有代码同步"},
					"workspaceSource":          []any{"提供现有项目文件到当前 workspace"},
				},
				"missingFields":       []any{"workspaceSource", "initializationPermission"},
				"requiresHumanFields": []any{"workspaceSource", "initializationPermission"},
			},
		}, nil
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		})

	workspaceID := common.WorkspaceID("ws-continuable-decision-repair")
	runID := common.RunID("run-continuable-decision-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-continuable-decision-repair"),
		DoDID:           common.DoDID("dod-continuable-decision-repair"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"produce implementation"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, "run-continuable-decision-repair/step-1", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeySoulStage: soulStageExecution,
	}, nil); err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no decision ticket for continuable candidate", tickets)
	}
	failed, err := stepRepo.Get(ctx, "run-continuable-decision-repair/step-1")
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["continuableDecisionConvertedToRepair"] != true {
		t.Fatalf("step = %+v, want continuable decision converted to repair", failed)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_KnownActionableDecisionRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                           "completed",
				"stage":                            "llm_decision_required",
				stepOutputKeyBlockDecisionReason:   "current stage does not allow state-changing output",
				stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
				stepOutputKeyBlockDecisionQuestion: "Approve switching to execution stage to continue the known work?",
				stepOutputKeyBlockDecisionContext: map[string]any{
					"runtimeCanContinue":  false,
					"missingFields":       []any{},
					"requiresHumanFields": []any{},
					"knownValues": map[string]any{
						"currentGoal":     "new requirement",
						"targetFiles":     []any{"config.yaml", "config.go", "proxy.go", "proxy_test.go"},
						"proposedChanges": "update config, runtime selection, and tests",
					},
				},
				"nextStep": "run-known-actionable-decision/llm-after-decision",
				"nextKind": llmcommon.StepKindWorkspaceExecution,
			}, nil
		}
		if !boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			t.Fatalf("repair input = %+v, want runtime repair follow-up", request.Input)
		}
		return map[string]any{
			"status": "completed",
			"stage":  "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied":   true,
				"summary":     "known actionable work continued through repair",
				"goalOutcome": "state-changing continuation evidence is available",
				"items": []map[string]any{{
					"item":         "produce implementation",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
			},
		}, nil
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		})

	workspaceID := common.WorkspaceID("ws-known-actionable-decision")
	runID := common.RunID("run-known-actionable-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-known-actionable-decision"),
		DoDID:           common.DoDID("dod-known-actionable-decision"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"produce implementation"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, "run-known-actionable-decision/step-1", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		stepInputKeySoulStage:        soulStageValidation,
	}, nil); err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no decision ticket for known actionable stage switch", tickets)
	}
	failed, err := stepRepo.Get(ctx, "run-known-actionable-decision/step-1")
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["continuableDecisionConvertedToRepair"] != true {
		t.Fatalf("step = %+v, want continuable decision converted to repair", failed)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if call != 2 || runItem.State != common.RunStateCompleted {
		t.Fatalf("call=%d run=%+v, want completed after auto repair", call, runItem)
	}
}

func TestRuntimeOrchestrator_ReadableResourceDecisionRoutesToRepairWithHumanFields(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                           "completed",
				"stage":                            "llm_decision_required",
				stepOutputKeyBlockDecisionReason:   "need readable workspace source to continue",
				stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
				stepOutputKeyBlockDecisionQuestion: "Please provide pkg/lsp/server.go content.",
				stepOutputKeyBlockDecisionContext: map[string]any{
					"knownValues": map[string]any{
						"currentFailure": "validation needs source inspection",
					},
					"missingFields":       []any{"pkg/lsp/server.go content"},
					"requiresHumanFields": []any{"pkg/lsp/server.go content"},
				},
			}, nil
		}
		if !boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			t.Fatalf("repair input = %+v, want runtime repair follow-up", request.Input)
		}
		return map[string]any{
			"status": "completed",
			"stage":  "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied":   true,
				"summary":     "readable resource decision repaired",
				"goalOutcome": "resource was available to runtime",
				"items": []map[string]any{{
					"item":         "inspect source",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
			},
		}, nil
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		})

	deliverableDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDir, "pkg", "lsp"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDir, "pkg", "lsp", "server.go"), []byte("package lsp\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-readable-resource-decision")
	runID := common.RunID("run-readable-resource-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDir, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-readable-resource-decision"),
		DoDID:           common.DoDID("dod-readable-resource-decision"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"inspect source"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, "run-readable-resource-decision/step-1", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		stepInputKeySoulStage:        soulStageExecution,
	}, nil); err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no decision ticket for readable workspace resource", tickets)
	}
	failed, err := stepRepo.Get(ctx, "run-readable-resource-decision/step-1")
	if err != nil {
		t.Fatalf("get first step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["continuableDecisionConvertedToRepair"] != true {
		t.Fatalf("step = %+v, want readable resource decision converted to repair", failed)
	}
	converted := mapFromValue(failed.Output["convertedDecision"])
	contextMap := mapFromValue(converted["context"])
	if !stringSliceContains(stringListFromValue(contextMap["readableResources"]), "pkg/lsp/server.go") {
		t.Fatalf("converted context = %+v, want readable resource recorded", contextMap)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if call != 2 || runItem.State != common.RunStateCompleted {
		t.Fatalf("call=%d run=%+v, want completed after readable resource repair", call, runItem)
	}
}

func TestProviderRetryStepInputUsesAssessmentOnlyModeForCompletionGateFollowUp(t *testing.T) {
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify", Kind: "graph/mock"},
	}}
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-assessment/zz-llm-after-graph-verify",
		RunID: "run-provider-retry-assessment",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":             true,
			stepOutputKeyExecutionGraph: graph,
			"previousStepOutput":        map[string]any{"large": "payload"},
			"failedStepInput":           map[string]any{"large": strings.Repeat("x", 1024)},
			"failedStepOutput":          map[string]any{"large": strings.Repeat("x", 1024)},
			"executionContext": map[string]any{
				"dialogLanguage": "zh-CN",
				"skills":         []map[string]any{{"id": "skill-provider-retry", "instruction": strings.Repeat("x", 1024)}},
			},
			stepInputKeyExecutionContextPackage: map[string]any{"selectedContext": map[string]any{"skills": []any{map[string]any{"id": "skill-provider-retry"}}}},
		},
		Output: map[string]any{
			"stage":                     "llm_provider_timeout",
			failureOutputKeyClass:       failureClassProviderTimeout,
			failureOutputKeySignature:   "class=provider_timeout|stage=llm_provider_timeout",
			stepOutputKeyExecutionGraph: graph,
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))

	if input["providerRetryMode"] != "completion_assessment_only" || input["providerRetryAssessmentOnly"] != true || input["completionFollowUp"] != true {
		t.Fatalf("input = %+v, want assessment-only provider retry markers", input)
	}
	if _, ok := input[stepOutputKeyExecutionGraph]; ok {
		t.Fatalf("input retained executionGraph: %+v", input[stepOutputKeyExecutionGraph])
	}
	if _, ok := input["previousStepOutput"]; ok {
		t.Fatalf("input retained previousStepOutput: %+v", input["previousStepOutput"])
	}
	for _, key := range []string{"failedStepInput", stepInputKeyExecutionContextPackage} {
		if _, ok := input[key]; ok {
			t.Fatalf("input retained %s: %+v", key, input[key])
		}
	}
	executionContext := mapFromValue(input["executionContext"])
	if executionContext["dialogLanguage"] != "zh-CN" {
		t.Fatalf("executionContext = %+v, want slim dialog language", executionContext)
	}
	if _, ok := executionContext["skills"]; ok {
		t.Fatalf("executionContext retained raw skills: %+v", executionContext["skills"])
	}
	if input["failedStepOutput"] == nil || input[failureOutputKeyClass] != failureClassProviderTimeout {
		t.Fatalf("input = %+v, want compact failure context", input)
	}
}

func TestRuntimeOrchestrator_FailsLLMRunCompletionWithoutDoDSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-summary-missing/validation/runtime-step"},
				},
			},
		},
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-dod-summary-missing")
	runID := common.RunID("run-dod-summary-missing")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "dodAssessment.summary is required") {
		t.Fatalf("run = %+v, want failed DoD summary gate", runItem)
	}
}
