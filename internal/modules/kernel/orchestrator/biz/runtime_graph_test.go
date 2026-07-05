package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_EventDrivenGraphBatchSerializesLLMDispatchRequests(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-graph")
	workspaceID := common.WorkspaceID("ws-event-driven-graph")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	err := orch.dispatchGraphStepBatchWithWorkspace(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}, []stepDispatchSpec{
		{ID: common.StepID(runID + "/graph-001/001-a"), Kind: "llm/workspace_execution", Input: map[string]any{"node": "a"}},
		{ID: common.StepID(runID + "/graph-001/002-b"), Kind: "llm/workspace_execution", Input: map[string]any{"node": "b"}},
	}, &workspacemodel.Workspace{ID: workspaceID})
	if err != nil {
		t.Fatalf("dispatchGraphStepBatchWithWorkspace: %v", err)
	}
	published := hub.Published()
	var dispatches []events.Envelope
	for _, envelope := range published {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatches = append(dispatches, envelope)
		}
	}
	if len(dispatches) != 1 {
		t.Fatalf("dispatch events = %d in %+v, want 1 serialized LLM dispatch", len(dispatches), published)
	}
	if dispatches[0].RunID != string(runID) || dispatches[0].WorkspaceID != string(workspaceID) || dispatches[0].StepID == "" {
		t.Fatalf("dispatch envelope = %+v, want scoped workspace/run/step", dispatches[0])
	}
	if key, _ := dispatches[0].Payload[events.PayloadKeyIdempotencyKey].(string); key == "" {
		t.Fatalf("dispatch payload = %+v, want idempotency key", dispatches[0].Payload)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(runID+"/graph-001/001-a")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("graph step after dispatch request err = %v, want not found before dispatch handler runs", err)
	}
}

func TestNormalizeGraphStateChangingDispatchAuthorityRewritesStaleValidationAuthority(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-graph-state-change-authority",
		WorkspaceID: "ws-graph-state-change-authority",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
		},
	}
	input := map[string]any{
		"executionContext": cloneStepPayload(run.ExecutionContext),
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}
	node := runtimemodel.BlockExecutionNode{
		ID:   "go-mod-tidy",
		Kind: runtimemodel.StepKindShellCommand,
		Input: map[string]any{
			"command":             "sh",
			"args":                []string{"-c", "true"},
			"stateChanging":       true,
			"targetRef":           "workspace",
			"stateChangingReason": "update generated dependency metadata",
		},
	}
	for key, value := range node.Input {
		input[key] = value
	}

	got := normalizeGraphStateChangingDispatchAuthority(run, "run-graph-state-change-authority/graph-001/001-go-mod-tidy", input, node)

	if kind := stringFromMap(currentActionNextActionFromInput(got), "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("current nextAction = %+v, want state_change", currentActionNextActionFromInput(got))
	}
	if kind := stringFromMap(mapFromValue(mapFromValue(got[convergenceAuthorityKey])["nextAction"]), "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("convergenceAuthority.nextAction = %+v, want state_change", mapFromValue(got[convergenceAuthorityKey])["nextAction"])
	}
	if kind := stringFromMap(mapFromValue(mapFromValue(got[gapBaselineKey])["nextAction"]), "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("gapBaseline.nextAction = %+v, want state_change", mapFromValue(got[gapBaselineKey])["nextAction"])
	}
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %q", state, convergenceStateNeedStateChange)
	}
	if !requiredOutputRequiresStateChanging(activeRequiredOutputForCurrentAction(got)) {
		t.Fatalf("active requiredOutput = %+v, want state-changing", activeRequiredOutputForCurrentAction(got))
	}
	if nextAction := mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"]); stringFromMap(nextAction, "type") != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction = %+v, want state_change", nextAction)
	}
	if nextAction := mapFromValue(mapFromValue(got[currentActionContractKey])["nextAction"]); stringFromMap(nextAction, "kind") != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("currentActionContract.nextAction = %+v, want state_change", nextAction)
	}
	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, got); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func TestNormalizeGraphStateChangingDispatchAuthorityAcceptsGitWriteCommit(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-graph-git-write-authority",
		WorkspaceID: "ws-graph-git-write-authority",
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			"currentFocus": map[string]any{
				"id":                  "cwi-git",
				"coverageWorkItemIds": []string{"cwi-git"},
				"targetRefs":          []string{"deliverable:version_snapshot"},
				"structuredTargetRefs": []string{
					"cwi-git",
					"deliverable:version_snapshot",
				},
			},
		},
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"acceptedKinds":                []string{runtimemodel.StepKindGitWrite},
				"mustIncludeStateChangingNode": true,
				"coverageWorkItemIds":          []string{"cwi-git"},
				"structuredTargetRefs":         []string{"cwi-git", "deliverable:version_snapshot"},
			},
		},
	}
	node := runtimemodel.BlockExecutionNode{
		ID:   "git-commit",
		Kind: runtimemodel.StepKindGitWrite,
		Input: map[string]any{
			"operation": "commit",
			"message":   "record version snapshot",
		},
	}
	for key, value := range node.Input {
		input[key] = value
	}

	got := normalizeGraphStateChangingDispatchAuthority(run, "run-graph-git-write-authority/graph-001/001-git-commit", input, node)

	if kind := stringFromMap(currentActionNextActionFromInput(got), "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("current nextAction = %+v, want state_change", currentActionNextActionFromInput(got))
	}
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %q", state, convergenceStateNeedStateChange)
	}
	required := activeRequiredOutputForCurrentAction(got)
	if !requiredOutputAcceptsKind(required, runtimemodel.StepKindGitWrite) || !requiredOutputRequiresStateChanging(required) {
		t.Fatalf("active requiredOutput = %+v, want git/write state-changing contract", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-git" {
		t.Fatalf("requiredOutput.coverageWorkItemIds = %+v, want cwi-git", ids)
	}
}

func TestPrepareGapBaselineDispatchSpecsPreservesGraphLocalStateChangingAuthority(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-graph-local-authority",
		WorkspaceID: "ws-graph-local-authority",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                  "cwi-run",
					"coverageWorkItemIds": []string{"cwi-run"},
				},
			},
		},
	}
	input := normalizeGraphStateChangingDispatchAuthority(run, "run-graph-local-authority/graph-001/001-tidy", map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		stepInputKeyGraphNodeID:     "tidy",
		stepInputKeyGraphRootStepID: "run-graph-local-authority/step-1",
		"stateChanging":             true,
		"targetRef":                 "go.mod",
		"stateChangingReason":       "update dependency metadata",
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}, runtimemodel.BlockExecutionNode{ID: "tidy", Kind: runtimemodel.StepKindShellCommand})

	specs, err := (&RuntimeOrchestrator{}).prepareGapBaselineDispatchSpecs(context.Background(), run, []stepDispatchSpec{{
		ID:    "run-graph-local-authority/graph-001/001-tidy",
		Kind:  runtimemodel.StepKindShellCommand,
		Input: input,
	}})
	if err != nil {
		t.Fatalf("prepareGapBaselineDispatchSpecs: %v", err)
	}
	if got := stringFromMap(currentActionNextActionFromInput(specs[0].Input), "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("nextAction = %q, want graph-local state_change", got)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(specs[0].Input[convergenceAuthorityKey])["nextAction"]), "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("convergenceAuthority.nextAction = %q, want graph-local state_change", got)
	}
	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, specs[0].Input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func TestDispatchRequestedStepPreservesGraphLocalAuthority(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-graph-local-authority")
	workspaceID := common.WorkspaceID("ws-dispatch-graph-local-authority")
	stepID := common.StepID(runID + "/graph-001/001-state-change")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{"id": "cwi-run", "missingEvidenceKinds": []string{"validation"}},
			},
			coverageFocusKey: map[string]any{
				"id":                  "cwi-run",
				"coverageWorkItemIds": []string{"cwi-run"},
			},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  map[string]any{"id": "cwi-run", "missingEvidenceKinds": []string{"validation"}},
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  map[string]any{"id": "cwi-run", "missingEvidenceKinds": []string{"validation"}},
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	input := normalizeGraphStateChangingDispatchAuthority(run, stepID, map[string]any{
		stepInputKeyGraphNodeID:     "state-change",
		stepInputKeyGraphRootStepID: string(runID + "/step-1"),
		"stateChanging":             true,
		"targetRef":                 "workspace",
		"stateChangingReason":       "apply graph-local state change",
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}, runtimemodel.BlockExecutionNode{ID: "state-change", Kind: runtimemodel.StepKindShellCommand})
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"status": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	if err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        runtimemodel.StepKindShellCommand,
		Input:       input,
	}); err != nil {
		t.Fatalf("DispatchRequestedStep: %v", err)
	}
	created, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(created.Input[convergenceAuthorityKey])["nextAction"]), "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("created convergenceAuthority.nextAction = %q, want state_change; input=%+v", got, created.Input)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(created.Input[gapBaselineKey])["nextAction"]), "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("created gapBaseline.nextAction = %q, want state_change; input=%+v", got, created.Input)
	}
}

func TestNormalizeGraphValidationDispatchAuthorityBindsValidationEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-graph-validation-authority",
		WorkspaceID: "ws-graph-validation-authority",
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		"command":             "go",
		"args":                []string{"build", "./..."},
		"timeoutMillis":       int64(120000),
		"coverageWorkItemIds": []string{"cwi-001"},
		"coverageTargets":     []string{"goal-req-001"},
		"targetRef":           ".",
		"boundedReason":       "verify the declared coverage focus after state change",
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-001",
				"coverageWorkItemIds": []string{"cwi-001"},
				"targetRefs":          []string{"goal-req-001"},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
		},
		"validationEvidence": map[string]any{
			"mode":            "controlled_substitute",
			"observationType": "controlled_substitute_observation",
		},
	}
	got := normalizeGraphValidationDispatchAuthority(run, "run-graph-validation-authority/graph-001/002-verify", input, runtimemodel.BlockExecutionNode{
		ID:   "verify",
		Kind: runtimemodel.StepKindShellCommand,
	})

	if kind := stringFromMap(currentActionNextActionFromInput(got), "kind"); kind != runtimemodel.NextActionTypeValidate {
		t.Fatalf("nextAction = %q, want validate", kind)
	}
	evidence := mapFromValue(got["validationEvidence"])
	if ids := stringSliceFromValue(evidence["coverageWorkItemIds"]); !stringSliceContains(ids, "cwi-001") {
		t.Fatalf("validationEvidence.coverageWorkItemIds = %+v, want cwi-001", ids)
	}
	if refs := stringSliceFromValue(evidence["targetRefs"]); !stringSliceContains(refs, "goal-req-001") {
		t.Fatalf("validationEvidence.targetRefs = %+v, want goal-req-001", refs)
	}
	if len(stringSliceFromValue(evidence["observedClaims"])) == 0 {
		t.Fatalf("validationEvidence.observedClaims missing: %+v", evidence)
	}
	if stringFromMap(evidence, "substituteFor") == "" {
		t.Fatalf("validationEvidence.substituteFor missing for controlled substitute: %+v", evidence)
	}
	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, got); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func TestRuntimeOrchestrator_EventDrivenGraphBatchStepCompletionDispatchesNextNode(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-graph-continuation")
	workspaceID := common.WorkspaceID("ws-event-driven-graph-continuation")
	rootStepID := common.StepID(runID + "/step-1")
	childStepID := common.StepID(runID + "/graph-001/001-list-root")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Output: map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{
					{
						"id":    "list-root",
						"kind":  "file/list",
						"input": map[string]any{"path": ""},
					},
					{
						"id":        "read-config",
						"kind":      "file/read",
						"dependsOn": []string{"list-root"},
						"input":     map[string]any{"path": "config.yaml"},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    childStepID,
		RunID: runID,
		Kind:  "file/list",
		State: common.StepStateDispatched,
		Input: map[string]any{
			stepInputKeyGraphBatch:      true,
			stepInputKeyGraphRootStepID: string(rootStepID),
			stepInputKeyGraphNodeID:     "list-root",
			"graphSeq":                  1,
			"graphNodeSeq":              1,
		},
	}); err != nil {
		t.Fatalf("seed graph child step: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	if err := orch.FinalizeRuntimeStepResult(ctx, runID, childStepID, runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil); err != nil {
		t.Fatalf("FinalizeRuntimeStepResult: %v", err)
	}
	var dispatches []events.Envelope
	for _, envelope := range hub.Published() {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatches = append(dispatches, envelope)
		}
	}
	if len(dispatches) != 1 {
		t.Fatalf("dispatch events = %d in %+v, want next graph node dispatch", len(dispatches), hub.Published())
	}
	cmd, ok := dispatches[0].Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand)
	if !ok {
		t.Fatalf("dispatch command = %#v, want DispatchStepCommand", dispatches[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.StepID != common.StepID(runID+"/graph-001/002-read-config") || cmd.Kind != "file/read" {
		t.Fatalf("dispatch command = %+v, want read-config graph step", cmd)
	}
	if nodeID := stringFromMap(cmd.Input, stepInputKeyGraphNodeID); nodeID != "read-config" {
		t.Fatalf("dispatch input = %+v, want read-config node", cmd.Input)
	}
	if completed := stringSliceFromValue(cmd.Input[stepInputKeyGraphCompletedNodes]); len(completed) != 1 || completed[0] != "list-root" {
		t.Fatalf("dispatch input completed nodes = %+v, want list-root", cmd.Input)
	}
}

func TestRuntimeOrchestrator_CompletedGraphBatchTerminalReplayContinuesNextNode(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-graph-replay-continuation")
	workspaceID := common.WorkspaceID("ws-event-driven-graph-replay-continuation")
	rootStepID := common.StepID(runID + "/step-1")
	childStepID := common.StepID(runID + "/graph-001/001-list-root")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Output: map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{
					{"id": "list-root", "kind": "file/list", "input": map[string]any{"path": ""}},
					{"id": "read-config", "kind": "file/read", "dependsOn": []string{"list-root"}, "input": map[string]any{"path": "config.yaml"}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     childStepID,
		RunID:  runID,
		Kind:   "file/list",
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed"},
		Input: map[string]any{
			stepInputKeyGraphBatch:      true,
			stepInputKeyGraphRootStepID: string(rootStepID),
			stepInputKeyGraphNodeID:     "list-root",
			"graphSeq":                  1,
			"graphNodeSeq":              1,
		},
	}); err != nil {
		t.Fatalf("seed completed graph child step: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	if err := orch.FinalizeRuntimeStepResult(ctx, runID, childStepID, runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil); err != nil {
		t.Fatalf("FinalizeRuntimeStepResult: %v", err)
	}
	var dispatches []events.Envelope
	for _, envelope := range hub.Published() {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatches = append(dispatches, envelope)
		}
	}
	if len(dispatches) != 1 {
		t.Fatalf("dispatch events = %d in %+v, want replay to continue graph", len(dispatches), hub.Published())
	}
	cmd, ok := dispatches[0].Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand)
	if !ok {
		t.Fatalf("dispatch command = %#v, want DispatchStepCommand", dispatches[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.StepID != common.StepID(runID+"/graph-001/002-read-config") || cmd.Kind != "file/read" {
		t.Fatalf("dispatch command = %+v, want read-config graph step", cmd)
	}
}

func TestRuntimeOrchestrator_CompletedPatchGraphNodeContinuesDependentValidation(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-patch-graph-validation")
	workspaceID := common.WorkspaceID("ws-event-driven-patch-graph-validation")
	rootStepID := common.StepID(runID + "/contract-retry-001-of-graph-005-001-fix-dependency")
	patchStepID := common.StepID(runID + "/graph-006/001-fix-main")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                   "cwi-source",
					"coverageWorkItemIds":  []string{"cwi-source"},
					"missingEvidenceKinds": []string{"validation"},
				},
			},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  map[string]any{"id": "cwi-source", "missingEvidenceKinds": []string{"validation"}},
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  map[string]any{"id": "cwi-source", "missingEvidenceKinds": []string{"validation"}},
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	graph := map[string]any{
		"nodes": []map[string]any{
			{
				"id":   "fix-main",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":       "file_write",
					"targetRef":       "main.go",
					"content":         "package main\n",
					"coverageTargets": []string{"source_change"},
				},
			},
			{
				"id":        "verify-build",
				"kind":      "shell/command",
				"dependsOn": []string{"fix-main"},
				"input": map[string]any{
					"command": "go",
					"args":    []string{"build", "./..."},
					"validationEvidence": map[string]any{
						"observationType":     "runtime_observation",
						"coverageWorkItemIds": []string{"cwi-source"},
						"targetRefs":          []string{"cwi-source"},
						"observedClaims":      []string{"build verifies the current source tree"},
					},
				},
			},
		},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     rootStepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed", stepOutputKeyExecutionGraph: graph},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    patchStepID,
		RunID: runID,
		Kind:  runtimemodel.RuntimeBlockKindPatchApply,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphBatch:      true,
			stepInputKeyGraphRootStepID: string(rootStepID),
			stepInputKeyGraphNodeID:     "fix-main",
			"graphSeq":                  6,
			"graphNodeSeq":              1,
			stepOutputKeyExecutionGraph: graph,
			currentActionContractKey: map[string]any{
				"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
		},
		Output: map[string]any{"status": "completed", "stage": "patch_apply"},
	}); err != nil {
		t.Fatalf("seed completed patch step: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	if err := orch.FinalizeRuntimeStepResult(ctx, runID, patchStepID, runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil); err != nil {
		t.Fatalf("FinalizeRuntimeStepResult: %v", err)
	}
	var dispatches []events.Envelope
	for _, envelope := range hub.Published() {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatches = append(dispatches, envelope)
		}
	}
	if len(dispatches) != 1 {
		t.Fatalf("dispatch events = %d in %+v, want dependent validation dispatch", len(dispatches), hub.Published())
	}
	cmd, ok := dispatches[0].Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand)
	if !ok {
		t.Fatalf("dispatch command = %#v, want DispatchStepCommand", dispatches[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.StepID != common.StepID(runID+"/graph-001/002-verify-build") || cmd.Kind != runtimemodel.StepKindShellCommand {
		t.Fatalf("dispatch command = %+v, want verify-build shell step", cmd)
	}
	if nodeID := stringFromMap(cmd.Input, stepInputKeyGraphNodeID); nodeID != "verify-build" {
		t.Fatalf("dispatch input = %+v, want verify-build node", cmd.Input)
	}
}

func TestRuntimeOrchestrator_GraphBatchPatchApplyFailureDispatchesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-graph-patch-repair")
	workspaceID := common.WorkspaceID("ws-event-driven-graph-patch-repair")
	stepID := common.StepID(runID + "/graph-001/002-patch-target")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: common.DoDID("dod-graph-patch-repair"), State: common.RunStateRunning, InitialStepKind: llmWorkspaceExecutionStepKind}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.RuntimeBlockKindPatchApply,
		State: common.StepStateDispatched,
		Input: map[string]any{
			stepInputKeyGraphBatch:      true,
			stepInputKeyGraphRootStepID: string(runID + "/step-1"),
			stepInputKeyGraphNodeID:     "patch-target",
			"graphSeq":                  1,
			"graphNodeSeq":              2,
		},
		Output: map[string]any{
			"status": "failed",
			"error":  "conflict: patch baseHash mismatch",
		},
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "continue"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	if err := orch.handleRuntimeStepFailure(ctx, run, step, nil, step.Output, "runtime patch application failed", errors.New("conflict: patch baseHash mismatch")); err != nil {
		t.Fatalf("handleRuntimeStepFailure: %v", err)
	}

	var repairDispatch *orchestratorcontracts.DispatchStepCommand
	for _, envelope := range hub.Published() {
		if envelope.Topic != orchestratorevents.TopicOrchestratorDispatchStepRequested {
			continue
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand)
		if !ok {
			t.Fatalf("dispatch command = %#v, want DispatchStepCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		repairDispatch = &cmd
		break
	}
	if repairDispatch == nil {
		t.Fatalf("repair dispatch not published after graph batch patch failure; events=%+v", hub.Published())
	}
	if repairDispatch.Kind != llmWorkspaceExecutionStepKind || !boolValue(repairDispatch.Input[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("repair dispatch = %+v, want llm runtime repair follow-up", repairDispatch)
	}
	if got := stringFromMap(repairDispatch.Input, "failedStepId"); got != string(stepID) {
		t.Fatalf("repair failedStepId = %q, want %s", got, stepID)
	}
	stored, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get failed step: %v", err)
	}
	last := mapFromValue(stored.Output["lastFailureDiagnostics"])
	if stringFromMap(last, "stepId") != string(stepID) {
		t.Fatalf("lastFailureDiagnostics = %+v, want failed step context", last)
	}
}

func TestRuntimeOrchestrator_EventDrivenDispatchPublishesAuthorityAlignedInput(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-event-driven-authority")
	stepID := common.StepID(runID + "/step-1")
	workspaceID := common.WorkspaceID("ws-event-driven-authority")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.State = common.RunStateRunning
	run.ExecutionContext[convergenceAuthorityKey].(map[string]any)["workspaceId"] = string(workspaceID)
	run.ExecutionContext[convergenceAuthorityKey].(map[string]any)["runId"] = string(runID)
	run.ExecutionContext[gapBaselineKey].(map[string]any)["workspaceId"] = string(workspaceID)
	run.ExecutionContext[gapBaselineKey].(map[string]any)["runId"] = string(runID)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	orch.EnableEventDrivenDispatch()

	err := orch.RequestUniqueDispatchStep(ctx, run, stepID, llmWorkspaceExecutionStepKind, map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"workspaceId":   "stale-workspace",
			"runId":         "stale-run",
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"nextAction": map[string]any{"kind": "validate"},
		},
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}, workspaceID)
	if err != nil {
		t.Fatalf("RequestUniqueDispatchStep: %v", err)
	}
	var dispatch events.Envelope
	for _, envelope := range hub.Published() {
		if envelope.Topic == orchestratorevents.TopicOrchestratorDispatchStepRequested {
			dispatch = envelope
		}
	}
	if dispatch.Topic == "" {
		t.Fatalf("published events = %+v, want dispatch requested", hub.Published())
	}
	cmd, ok := dispatch.Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand)
	if !ok {
		t.Fatalf("dispatch command = %#v, want DispatchStepCommand", dispatch.Payload[events.PayloadKeyCommand])
	}
	input := cmd.Input
	authority := mapFromValue(input[convergenceAuthorityKey])
	if stringFromMap(authority, "workspaceId") != string(workspaceID) || stringFromMap(authority, "runId") != string(runID) {
		t.Fatalf("queued authority = %+v, want current workspace/run", authority)
	}
	assertFocusIDForTest(t, mapFromValue(authority["currentFocus"]), "cwi-current")
	assertFocusIDForTest(t, mapFromValue(input[coverageFocusKey]), "focus-cwi-current")
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if got := stringFromMap(required, "coverageFocusId"); got != "focus-cwi-current" {
		t.Fatalf("queued repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "cwi-current")
}

func TestRuntimeOrchestrator_NextDispatchReusesActiveLLMWorkspaceExecution(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-active-llm-lane")
	activeStepID := common.StepID(runID + "/follow-up-active")
	stepRepo := memoryrepo.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-active-llm-lane", State: common.RunStateRunning}
	completedRoot := &runmodel.Step{
		ID:     common.StepID(runID + "/step-1"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"completionState": "continue"},
	}
	active := &runmodel.Step{
		ID:    activeStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateDispatched,
		Input: map[string]any{"current": "active"},
	}
	for _, step := range []*runmodel.Step{completedRoot, active} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, err := orch.nextDispatchSpecs(ctx, run)
	if err != nil {
		t.Fatalf("nextDispatchSpecs: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != activeStepID || specs[0].Input["current"] != "active" {
		t.Fatalf("specs = %+v, want active LLM step reused", specs)
	}
}

func TestRuntimeOrchestrator_DispatchRequestedStepSkipsSecondActiveLLMWorkspaceExecution(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-active-llm-lane")
	workspaceID := common.WorkspaceID("ws-dispatch-active-llm-lane")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	activeStepID := common.StepID(runID + "/active-llm")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    activeStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed active step: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	secondStepID := common.StepID(runID + "/second-llm")
	err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      secondStepID,
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input:       map[string]any{"node": "second"},
	})
	if err != nil {
		t.Fatalf("DispatchRequestedStep: %v", err)
	}
	if _, err := stepRepo.Get(ctx, secondStepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("second LLM step err = %v, want not created while %s is active", err, activeStepID)
	}
}

func TestRuntimeOrchestrator_DispatchRequestedStepHydratesGapBaseline(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-gap-baseline")
	workspaceID := common.WorkspaceID("ws-dispatch-gap-baseline")
	stepID := common.StepID(runID + "/step-1")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "- complete the current requested outcome",
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	if err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input:       map[string]any{"operator": "requested"},
	}); err != nil {
		t.Fatalf("DispatchRequestedStep: %v", err)
	}
	created, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if baseline := mapFromValue(created.Input[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("step input = %+v, want hydrated gapBaseline", created.Input)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if baseline := mapFromValue(persisted.ExecutionContext[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("run executionContext = %+v, want persisted gapBaseline", persisted.ExecutionContext)
	}
}

func TestRuntimeOrchestratorDispatchRequestedStepUsesScopedRunLookup(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-scoped-lookup")
	workspaceID := common.WorkspaceID("ws-dispatch-scoped-lookup")
	stepID := common.StepID(runID + "/step-1")
	runRepo := newCountingRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	if err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input:       map[string]any{"operator": "requested"},
	}); err != nil {
		t.Fatalf("DispatchRequestedStep: %v", err)
	}
	if got := runRepo.getCount.Load(); got != 0 {
		t.Fatalf("run Get count = %d, want 0 for workspace-scoped dispatch command", got)
	}
	if got := runRepo.getByWorkspaceIDCount.Load(); got == 0 {
		t.Fatalf("run GetByWorkspaceID count = %d, want scoped dispatch reads", got)
	}
}

func TestRuntimeOrchestrator_NextDispatchSpecsHonorsGapBaselineWaitAction(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-gap-baseline-wait-decision")
	workspaceID := common.WorkspaceID("ws-gap-baseline-wait-decision")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	run := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-gap-baseline-wait",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-gap-baseline-wait",
				"text": "- close the current accepted gap",
			},
			"dod": map[string]any{
				"id":    "dod-gap-baseline-wait",
				"items": []string{"gap is closed with evidence"},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{runs: runRepo, steps: stepRepo}

	specs, err := orch.nextDispatchSpecs(ctx, run)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("nextDispatchSpecs err = %v, want blocked", err)
	}
	if len(specs) != 0 {
		t.Fatalf("specs = %+v, want no dispatch while waiting decision", specs)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	baseline := mapFromValue(persisted.ExecutionContext[gapBaselineKey])
	if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got != "request_decision" {
		t.Fatalf("gapBaseline.nextAction = %+v, want request_decision", baseline["nextAction"])
	}
}

func TestRuntimeOrchestrator_DispatchRequestedStepHonorsGapBaselineWaitAction(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-gap-baseline-wait-approval")
	workspaceID := common.WorkspaceID("ws-dispatch-gap-baseline-wait-approval")
	stepID := common.StepID(runID + "/step-1")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateBlocked, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-gap-baseline-wait",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-dispatch-gap-baseline-wait",
				"text": "- close the current accepted gap",
			},
			"dod": map[string]any{
				"id":    "dod-dispatch-gap-baseline-wait",
				"items": []string{"gap is closed with evidence"},
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input:       map[string]any{"operator": "requested"},
	})
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("DispatchRequestedStep err = %v, want blocked", err)
	}
	if _, err := stepRepo.Get(ctx, stepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step err = %v, want not found while waiting approval", err)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	baseline := mapFromValue(persisted.ExecutionContext[gapBaselineKey])
	if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got != "request_approval" {
		t.Fatalf("gapBaseline.nextAction = %+v, want request_approval", baseline["nextAction"])
	}
}

func TestRuntimeOrchestrator_NextDispatchSpecsHonorsGapBaselineTerminalActions(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		state   common.RunState
		wantErr error
	}{
		{name: "completed no action", state: common.RunStateCompleted},
		{name: "failed fail fast", state: common.RunStateFailed, wantErr: common.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := common.RunID("run-gap-baseline-terminal-" + strings.ReplaceAll(tc.name, " ", "-"))
			runRepo := memoryrepo.NewRunRepository()
			stepRepo := memoryrepo.NewStepRepository()
			run := &runmodel.Run{
				ID:          runID,
				WorkspaceID: common.WorkspaceID("ws-" + runID),
				State:       tc.state,
				ExecutionContext: map[string]any{
					"goal": map[string]any{
						"id":   "goal-gap-baseline-terminal",
						"text": "- close the current accepted gap",
					},
					"dod": map[string]any{
						"id":    "dod-gap-baseline-terminal",
						"items": []string{"gap is closed with evidence"},
					},
				},
			}
			if err := runRepo.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			orch := &RuntimeOrchestrator{runs: runRepo, steps: stepRepo}

			specs, err := orch.nextDispatchSpecs(ctx, run)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("nextDispatchSpecs err = %v, want %v", err, tc.wantErr)
			}
			if len(specs) != 0 {
				t.Fatalf("specs = %+v, want no dispatch for %s", specs, tc.state)
			}
		})
	}
}

func TestRuntimeOrchestrator_RunLLMDispatchLockSerializesSameRun(t *testing.T) {
	runID := common.RunID("run-llm-dispatch-lock")
	orch := &RuntimeOrchestrator{}
	unlockFirst := orch.lockRunLLMDispatch(runID)
	var secondEntered atomic.Bool
	secondDone := make(chan struct{})
	go func() {
		unlockSecond := orch.lockRunLLMDispatch(runID)
		secondEntered.Store(true)
		unlockSecond()
		close(secondDone)
	}()

	select {
	case <-secondDone:
		t.Fatal("second lock entered before first lock released")
	case <-time.After(20 * time.Millisecond):
	}
	if secondEntered.Load() {
		t.Fatal("second lock marked entered before first lock released")
	}
	unlockFirst()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second lock did not enter after first lock released")
	}
}

func TestRunEvidenceSummaryPrioritizesActionEvidenceWhenLarge(t *testing.T) {
	runID := common.RunID("run-evidence-priority")
	steps := make([]*runmodel.Step, 0, maxLLMContextSliceItems+3)
	for idx := 0; idx < maxLLMContextSliceItems+2; idx++ {
		steps = append(steps, &runmodel.Step{
			ID:     common.StepID(fmt.Sprintf("%s/llm-%03d", runID, idx)),
			RunID:  runID,
			Kind:   llmWorkspaceExecutionStepKind,
			State:  common.StepStateCompleted,
			Output: map[string]any{"status": "completed"},
		})
	}
	critical := &runmodel.Step{
		ID:     common.StepID(runID + "/graph-001/002-verify"),
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "critical validation passed"},
	}
	steps = append([]*runmodel.Step{critical}, steps...)

	evidence := runEvidenceSummaryFromSteps(steps)

	if len(evidence) != maxLLMContextSliceItems {
		t.Fatalf("runEvidenceSummary length = %d, want %d", len(evidence), maxLLMContextSliceItems)
	}
	foundCritical := false
	for _, item := range evidence {
		if item["stepId"] == string(critical.ID) {
			foundCritical = true
			break
		}
	}
	if !foundCritical {
		t.Fatalf("runEvidenceSummary = %+v, want critical shell evidence retained", evidence)
	}
}

func TestRunEvidenceSummaryIncludesOutputRefAndTargetedLargeTextExcerpt(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-evidence-ref",
		WorkspaceID: "ws-evidence-ref",
	}
	step := &runmodel.Step{
		ID:    "run-evidence-ref/graph-001/001-verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stderr": strings.Repeat("head-line\n", 200) + "final actionable failure",
		},
	}

	evidence := runEvidenceSummaryFromRunSteps(run, []*runmodel.Step{step})

	if len(evidence) != 1 {
		t.Fatalf("runEvidenceSummary length = %d, want 1", len(evidence))
	}
	if evidence[0]["outputRef"] != "workorch://workspaces/ws-evidence-ref/runs/run-evidence-ref/steps/run-evidence-ref/graph-001/001-verify/output.json" {
		t.Fatalf("outputRef = %#v", evidence[0]["outputRef"])
	}
	output := mapFromValue(evidence[0]["output"])
	stderr := mapFromValue(output["stderr"])
	if stderr["truncated"] != true ||
		!strings.Contains(fmt.Sprint(stderr["headPreview"]), "head-line") ||
		!strings.Contains(fmt.Sprint(stderr["tailPreview"]), "final actionable failure") {
		t.Fatalf("stderr = %+v, want targeted head/tail excerpt", stderr)
	}
}

func TestRuntimeOrchestrator_ReadyGraphStepSpecsReusesLoadedStepsForStepID(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-graph-step-id-efficiency")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-step-id-efficiency"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/zz-llm-after-graph-root"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	conflictingBaseStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/verify-file"),
		RunID:  runID,
		Kind:   "graph/mock",
		State:  common.StepStateCompleted,
		Input:  map[string]any{stepInputKeyGraphNodeID: "verify-file", stepInputKeyGraphRootStepID: string(runID) + "/previous-root"},
		Output: map[string]any{"stdout": "previous graph scope"},
	}
	for _, step := range []*runmodel.Step{root, conflictingBaseStep} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, _ := orch.graphManager().readyGraphStepSpecsWithSteps(ctx, run, root)

	if got := stepRepo.listCount.Load(); got != 1 {
		t.Fatalf("step ListByRun count = %d, want one graph step scan", got)
	}
	if got := stepRepo.getCount.Load(); got != 0 {
		t.Fatalf("step Get count = %d, want step ID resolution to reuse loaded steps", got)
	}
	if len(specs) != 1 {
		t.Fatalf("ready specs = %+v, want one graph node spec", specs)
	}
	wantID := graphScopedStepID(runID, 2, 1, "verify-file")
	if specs[0].ID != wantID {
		t.Fatalf("spec ID = %s, want scoped ID %s", specs[0].ID, wantID)
	}
	if strings.Contains(string(specs[0].ID), "zz-llm-after-graph") {
		t.Fatalf("spec ID = %s, want readable scoped ID without embedded root lineage", specs[0].ID)
	}
}

func TestGraphSourceContractViolationBlocksSideEffectSpecs(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-contract-gate")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-contract-gate"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "must-not-execute", Kind: "graph/mock"},
	}}
	tests := []struct {
		name      string
		state     common.StepState
		output    map[string]any
		wantSpecs int
	}{
		{
			name:  "failed source with graph",
			state: common.StepStateFailed,
			output: map[string]any{
				"status":                       "failed",
				stepOutputKeyExecutionGraph:    graph,
				outputKeyContractViolationType: contractViolationLLMSchemaInvalid,
			},
		},
		{
			name:  "completed source with contract violation marker",
			state: common.StepStateCompleted,
			output: map[string]any{
				"status":                    "completed",
				stepOutputKeyExecutionGraph: graph,
				stepOutputKeyContractViolation: map[string]any{
					"violatedRule": contractViolationRuleUnparseableOutput,
				},
			},
		},
		{
			name:  "completed admitted source",
			state: common.StepStateCompleted,
			output: map[string]any{
				"status":                    "completed",
				stepOutputKeyExecutionGraph: graph,
			},
			wantSpecs: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stepRepo := memoryrepo.NewStepRepository()
			source := &runmodel.Step{
				ID:     common.StepID(string(runID) + "/" + strings.ReplaceAll(tt.name, " ", "-")),
				RunID:  runID,
				Kind:   llmcommon.StepKindWorkspaceExecution,
				State:  tt.state,
				Input:  map[string]any{},
				Output: tt.output,
			}
			if err := stepRepo.Create(ctx, source); err != nil {
				t.Fatalf("seed source: %v", err)
			}

			specs, _ := newGraphSubsystem(stepRepo, RuntimeEvidenceRepositories{}).readyGraphStepSpecsWithSteps(ctx, run, source)

			if len(specs) != tt.wantSpecs {
				t.Fatalf("specs = %+v, want %d", specs, tt.wantSpecs)
			}
		})
	}
}

func TestConcreteEvidenceAcquisitionGateStopsGraphAndRestoresStateChange(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-evidence-acquisition-gate")
	workspaceID := common.WorkspaceID("ws-evidence-acquisition-gate")
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "list-root", Kind: "file/list"},
		{ID: "probe-after-list", Kind: "shell/command", DependsOn: []string{"list-root"}},
	}}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	root := &runmodel.Step{
		ID:     common.StepID(runID + "/repair-001"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"coverageWorkItemContext": map[string]any{
				"currentFocus": map[string]any{
					"id":                    "coverage-item-1",
					"status":                "missing",
					"requiredEvidenceKinds": []string{"state_change"},
					"missingEvidenceKinds":  []string{"state_change"},
					"sourceScope":           "deliverable",
				},
			},
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	listStep := &runmodel.Step{
		ID:    graphScopedStepID(runID, 1, 1, "list-root"),
		RunID: runID,
		Kind:  "file/list",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID:       string(root.ID),
			stepInputKeyGraphNodeID:           "list-root",
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"repairActionPackage":             root.Input["repairActionPackage"],
		},
		Output: map[string]any{
			"stage":     "file_list",
			"status":    "completed",
			"path":      ".",
			"entries":   []map[string]any{},
			"truncated": false,
		},
	}
	steps := []*runmodel.Step{root, listStep}
	graphManager := &graphSubsystem{}

	nextSpecs := graphManager.readyGraphStepSpecsFromSteps(ctx, run, listStep, graph, steps)
	if len(nextSpecs) != 1 || nextSpecs[0].ID != graphScopedStepID(runID, 1, 2, "probe-after-list") {
		t.Fatalf("ready graph specs = %+v, want downstream probe ready before gate", nextSpecs)
	}
	input, ok := graphManager.concreteEvidenceAcquisitionFollowUpInputFromSteps(run, listStep, steps)
	if !ok {
		t.Fatalf("concrete evidence acquisition gate did not produce follow-up input")
	}
	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		!boolValue(input["stateChangingOutputRequired"]) ||
		boolValue(input["concreteFailureEvidenceRequired"]) {
		t.Fatalf("follow-up input contract = %+v, want restored state-changing contract", input)
	}
	if !boolValue(input["emptyWorkspaceEvidenceEscalatedToImplementation"]) ||
		!boolValue(input["evidenceAcquisitionFollowUp"]) {
		t.Fatalf("follow-up input = %+v, want empty workspace evidence escalation", input)
	}
	evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || evidence[0]["nodeId"] != "list-root" {
		t.Fatalf("graph evidence = %+v, want completed list evidence only", evidence)
	}
}

func TestConcreteEvidenceAcquisitionGateDoesNotInterceptStateChangingGraph(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-evidence-acquisition-state-change-dispatch")
	workspaceID := common.WorkspaceID("ws-evidence-acquisition-state-change-dispatch")
	root := &runmodel.Step{
		ID:     common.StepID(runID + "/repair-001"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{ID: "list-root", Kind: "file/list"}}}},
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"coverageWorkItemContext": map[string]any{
				"currentFocus": map[string]any{
					"id":                    "coverage-item-1",
					"status":                "missing",
					"requiredEvidenceKinds": []string{"state_change"},
					"missingEvidenceKinds":  []string{"state_change"},
					"sourceScope":           "deliverable",
				},
			},
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	listStep := &runmodel.Step{
		ID:    graphScopedStepID(runID, 1, 1, "list-root"),
		RunID: runID,
		Kind:  "file/list",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID:       string(root.ID),
			stepInputKeyGraphNodeID:           "list-root",
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		},
		Output: map[string]any{
			"stage":     "file_list",
			"status":    "completed",
			"path":      ".",
			"entries":   []map[string]any{},
			"truncated": false,
		},
	}
	stateChangingGraph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{
			ID:   "write-deliverable",
			Kind: "patch/apply",
			Input: map[string]any{
				"patchType":       "file_write",
				"targetRef":       "deliverable.txt",
				"content":         "implemented\n",
				"coverageTargets": []string{"coverage-item-1"},
			},
		},
	}}
	followUp := &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-list-root"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyGraphRootStepID:       string(root.ID),
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
		Output: map[string]any{
			"status":                    "completed",
			stepOutputKeyExecutionGraph: stateChangingGraph,
		},
	}
	steps := []*runmodel.Step{root, listStep, followUp}
	graphManager := &graphSubsystem{}

	if input, ok := graphManager.concreteEvidenceAcquisitionFollowUpInputFromSteps(
		&runmodel.Run{ID: runID, WorkspaceID: workspaceID, InitialStepKind: llmWorkspaceExecutionStepKind},
		followUp,
		steps,
	); ok {
		t.Fatalf("gate returned follow-up input = %+v, want state-changing graph to dispatch", input)
	}
	specs := graphManager.readyGraphStepSpecsFromSteps(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, followUp, stateChangingGraph, steps)
	if len(specs) != 1 || specs[0].Kind != "patch/apply" || specs[0].ID != graphScopedStepID(runID, 2, 1, "write-deliverable") {
		t.Fatalf("ready graph specs = %+v, want patch/apply state-changing node", specs)
	}
}
