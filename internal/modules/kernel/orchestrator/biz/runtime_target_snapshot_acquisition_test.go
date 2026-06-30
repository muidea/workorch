package orchestrator

import (
	"context"
	"os"
	"testing"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeManagedTargetSnapshotAcquisitionBuildsReadOnlyGraph(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"target.txt", "cwi-ignored", "deliverable:source_change"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
				"acceptedKinds":                      []string{"file/list", "file/read", "git/read"},
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want true")
	}
	if output["runtimeManagedTargetSnapshotAcquisition"] != true {
		t.Fatalf("output = %+v, want runtime managed marker", output)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 1 {
		t.Fatalf("graph = %+v/%v, want one read target node", graph, ok)
	}
	node := graph.Nodes[0]
	if node.Kind != "file/read" || node.Input["path"] != "target.txt" || node.Input["targetRef"] != "target.txt" {
		t.Fatalf("node = %+v, want read-only target.txt file/read", node)
	}
	if node.Input["runtimeManagedAcquisition"] != true || node.Input["acquisitionKind"] != "target_snapshot" {
		t.Fatalf("node input = %+v, want target snapshot acquisition metadata", node.Input)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionFailsFastForUnreadableTargetRefs(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"cwi-001", "deliverable:source_change", "../escape.txt"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want deterministic fail-fast takeover")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput = %+v, want deterministic fail-fast output", output)
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no read graph for unreadable target refs", graph)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionFailsFastForCurrentActionAbstractFocusRefs(t *testing.T) {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
	}
	focus := map[string]any{
		"id":                  "focus-cwi-039",
		"coverageWorkItemIds": []string{"cwi-039"},
		"targetRefs":          []string{"cwi-039", "version_control_snapshot"},
		"structuredTargetRefs": []map[string]any{
			{"targetRef": "deliverable:source_change"},
			{"targetRef": "source_change"},
		},
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		currentActionContractKey: map[string]any{
			"currentFocus":   focus,
			"requiredOutput": required,
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want deterministic fail-fast for abstract focus refs")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput = %+v, want deterministic fail-fast output", output)
	}
	refs := stringSliceFromValue(output["abstractTargetRefs"])
	for _, want := range []string{"cwi-039", "deliverable:source_change", "source_change", "version_control_snapshot"} {
		if !stringSliceContains(refs, want) {
			t.Fatalf("abstractTargetRefs = %+v, want %q", refs, want)
		}
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no read graph for abstract focus refs", graph)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionDispatchesReadOnlyGraphWithoutRootLLM(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	hub := events.NewMemoryHub()
	deliverableDir := t.TempDir()
	if err := os.WriteFile(deliverableDir+"/target.txt", []byte("current target content\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}

	runtime := NewRuntimeAdapterRegistry()
	llmRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":   true,
			"summary":     "follow-up completed after target snapshot acquisition",
			"goalOutcome": "follow-up completed after target snapshot acquisition",
			"items": []map[string]any{{
				"item":         "target snapshot acquisition follow-up",
				"state":        "satisfied",
				"evidenceRefs": []string{"target snapshot acquired"},
			}},
		},
	}}
	if err := runtime.Register(llmWorkspaceExecutionStepKind, llmRuntime); err != nil {
		t.Fatalf("register llm adapter: %v", err)
	}
	if err := runtime.Register(filecommon.StepKindRead, filebiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register file/read adapter: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-runtime-managed-target-snapshot")
	runID := common.RunID("run-runtime-managed-target-snapshot")
	rootStepID := common.StepID(runID + "/step-1")
	workspace := &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "runtime managed target snapshot",
		DeliverableDirectory: deliverableDir,
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	input := runtimeManagedTargetSnapshotDispatchInput(workspaceID, runID)
	err := orch.dispatchStepWithWorkspace(ctx, run, rootStepID, llmWorkspaceExecutionStepKind, input, workspace)
	if err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	rootStep, err := stepRepo.Get(ctx, rootStepID)
	if err != nil {
		t.Fatalf("get root step: %v", err)
	}
	if rootStep.State != common.StepStateCompleted || rootStep.Output["runtimeManagedTargetSnapshotAcquisition"] != true {
		t.Fatalf("root step = %+v, want runtime-managed completed root", rootStep)
	}
	for _, request := range llmRuntime.requests {
		if request.StepID == rootStepID {
			t.Fatalf("root LLM runtime was called: %+v", request)
		}
	}

	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var readStep *runmodel.Step
	for _, item := range steps {
		if item.Kind == filecommon.StepKindRead {
			readStep = item
			break
		}
	}
	if readStep == nil {
		t.Fatalf("steps = %+v, want dispatched file/read graph step", steps)
	}
	if readStep.Input["path"] != "target.txt" || readStep.Input["runtimeManagedAcquisition"] != true {
		t.Fatalf("read step input = %+v, want runtime-managed target.txt read", readStep.Input)
	}
	if readStep.State != common.StepStateCompleted {
		t.Fatalf("read step state = %s, want completed", readStep.State)
	}
	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	var snapshotRecord bool
	for _, record := range records {
		if record.Kind == runtimemodel.EvidenceKindTargetSnapshot && stringFromMap(record.Metadata, "targetRef") == "target.txt" {
			snapshotRecord = true
			break
		}
	}
	if !snapshotRecord {
		t.Fatalf("evidence records = %+v, want accepted target snapshot record for target.txt", records)
	}
	if !publishedStepState(hub.Published(), rootStepID, common.StepStateCompleted) {
		t.Fatalf("events = %+v, want completed root step event", hub.Published())
	}
}

func runtimeManagedTargetSnapshotDispatchInput(workspaceID common.WorkspaceID, runID common.RunID) map[string]any {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
		"acceptedKinds":                      []string{"file/read", "file/list", "git/read"},
		"repairTargetSet":                    []string{"target.txt"},
	}
	return map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"workspaceId":   string(workspaceID),
			"runId":         string(runID),
			"currentFocus": map[string]any{
				"id":                   "focus-target-snapshot",
				"coverageWorkItemIds":  []string{"cwi-target-snapshot"},
				"structuredTargetRefs": []map[string]any{{"targetRef": "target.txt"}},
			},
			"nextAction": map[string]any{
				"kind":         runtimemodel.NextActionTypeStateChange,
				"prerequisite": targetSnapshotAcquisitionPrerequisite(),
			},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"runtimeStageMode":            "target_snapshot_acquisition",
			"stateChangingOutputRequired": true,
			"prerequisite":                targetSnapshotAcquisitionPrerequisite(),
		},
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"repairTargetSet": []string{"target.txt"},
			"requiredOutput":  cloneStepPayload(required),
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": cloneStepPayload(required),
		},
		"requiredOutput": cloneStepPayload(required),
	}
}
