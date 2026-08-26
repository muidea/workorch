package run

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunOwnerRetainRetirementHidesOrdinaryReadsButKeepsOwnerFact(t *testing.T) {
	owner := memoryrepo.NewRunOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-run-retain-read-hide-memory")
	ctx, runID, stepID, checkpointID, fence := prepareRetiredReadFixture(t, owner, workspaceID, false)

	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	biz := New()
	if err := biz.Configure(Dependencies{OwnerRepositories: owner, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	// Draining is an internal mutation fence, not a read-hiding tombstone.
	if _, err := executeRetirementReadTestCommand(biz, ctx, runevents.TopicRunCommandGet, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: runID}); err != nil {
		t.Fatalf("draining Run read: %v", err)
	}
	assertRuntimeStateRecordVisibility(t, biz, ctx, workspaceID, runID, true)

	retireReadFixture(t, owner.Runs(), ctx, *fence)
	assertRetiredOrdinaryReadsHidden(t, biz, ctx, workspaceID, runID, stepID, checkpointID)
	assertRuntimeStateRecordVisibility(t, biz, ctx, workspaceID, runID, false)
	assertRetiredOwnerFactVisible(t, biz, ctx, workspaceID, fence.DeletionID)
}

func TestRunOwnerFileRetainRetirementHidesReadsAfterReopen(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-run-retain-read-hide-file")
	first := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	ctx, runID, stepID, checkpointID, fence := prepareRetiredReadFixture(t, first.RunOwner, workspaceID, true)
	retireReadFixture(t, first.RunOwner.Runs(), ctx, *fence)

	// A fresh repository/Biz instance must derive visibility from the durable
	// retired generation head rather than a process-local deletion cache.
	reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	biz := New()
	if err := biz.Configure(Dependencies{OwnerRepositories: reopened.RunOwner, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	assertRetiredOrdinaryReadsHidden(t, biz, ctx, workspaceID, runID, stepID, checkpointID)
	assertRuntimeStateRecordVisibility(t, biz, ctx, workspaceID, runID, false)
	assertRetiredOwnerFactVisible(t, biz, ctx, workspaceID, fence.DeletionID)
}

func prepareRetiredReadFixture(t *testing.T, owner repository.RunOwnerRepositories, workspaceID common.WorkspaceID, fileIDs bool) (context.Context, common.RunID, common.StepID, common.CheckpointID, *runmodel.WorkspaceRetirementFence) {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	item := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := owner.Runs().CreateNextWorkspaceRun(ctx, item); err != nil {
		t.Fatal(err)
	}
	current, err := owner.Runs().GetByWorkspaceID(ctx, workspaceID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimecontract.RuntimeStateRecord{StateRevision: 1}
	terminalDeliveryID := "terminal-delivery:" + string(workspaceID) + ":" + string(item.ID)
	current.RuntimeState = &state
	current.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: workspaceID, RunID: item.ID, RuntimeStateRevision: state.StateRevision,
		RuntimeStateRef:  runevents.RuntimeStateRef(workspaceID, item.ID, state.StateRevision),
		RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(state), BootstrapHash: "bootstrap-hash:read-visibility",
		TerminalFenceID: terminalDeliveryID,
	}
	current.TerminalDelivery = &runtimecontract.RuntimeTerminalDeliveryIdentity{TerminalDeliveryID: terminalDeliveryID}
	if err := owner.Runs().CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	stepID := common.StepID(string(item.ID) + "/step-1")
	if fileIDs {
		// Keep the path-backed fixture explicit and stable across reopen.
		stepID = common.StepID(filepath.ToSlash(string(stepID)))
	}
	if err := owner.Steps().Create(ctx, &runmodel.Step{ID: stepID, RunID: item.ID, State: common.StepStateSucceeded}); err != nil {
		t.Fatal(err)
	}
	checkpointID := common.CheckpointID("checkpoint-retain-read-hide")
	if err := owner.Checkpoints().Create(ctx, &runmodel.RuntimeCheckpoint{ID: checkpointID, WorkspaceID: workspaceID, RunID: item.ID, RunStateSnapshot: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	fence, err := owner.Runs().PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent:" + string(workspaceID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, item.ID, stepID, checkpointID, fence
}

func retireReadFixture(t *testing.T, runs repository.RunRepository, ctx context.Context, fence runmodel.WorkspaceRetirementFence) {
	t.Helper()
	items, err := runs.ListByWorkspaceID(ctx, fence.WorkspaceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("retirement inventory=%+v err=%v", items, err)
	}
	item := items[0]
	runFact := deletioncontract.WorkspaceRuntimeQuiescenceRun{
		RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
	}
	if item.RuntimeState != nil {
		runFact.RuntimeStatePresent = true
		runFact.RuntimeStateRevision = item.RuntimeState.StateRevision
		runFact.RuntimeStateRef = runevents.RuntimeStateRef(item.WorkspaceID, item.ID, item.RuntimeState.StateRevision)
		runFact.RuntimeStateHash = runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState)
		if item.TerminalDelivery != nil {
			runFact.TerminalDeliveryID = item.TerminalDelivery.TerminalDeliveryID
			runFact.TerminalDeliveryAcked = true
		}
	}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration, OrchestratorQuiescenceRevision: 2,
		Runs: []deletioncontract.WorkspaceRuntimeQuiescenceRun{runFact},
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	if _, err := runs.FinalizeWorkspaceRetirement(ctx, token); err != nil {
		t.Fatal(err)
	}
}

func assertRuntimeStateRecordVisibility(t *testing.T, biz *Biz, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, visible bool) {
	t.Helper()
	if err := biz.PrepareRuntimeRecoveryInventory(); err != nil {
		t.Fatalf("prepare Runtime state read: %v", err)
	}
	result := biz.EventHub().Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecord, workspaceRetirementFactQuerySource, runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordCommand{WorkspaceID: workspaceID, RunID: runID},
	))
	if result == nil {
		t.Fatal("Runtime state read returned nil result")
	}
	if visible {
		data, err := result.Get()
		response, ok := data.(runevents.QueryRuntimeStateRecordResult)
		if err != nil || !ok || !response.StatePresent || response.RuntimeState == nil {
			t.Fatalf("draining Runtime state read=%+v err=%v", data, err)
		}
		return
	}
	ownerErr := result.Error()
	if ownerErr == nil || !errors.Is(ownerErr.Cause, common.ErrNotFound) {
		t.Fatalf("retired Runtime state read error=%v, want not found", ownerErr)
	}
}

func assertRetiredOrdinaryReadsHidden(t *testing.T, biz *Biz, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, checkpointID common.CheckpointID) {
	t.Helper()
	for name, query := range map[string]struct {
		topic string
		data  any
	}{
		"run get":         {runevents.TopicRunCommandGet, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: runID}},
		"step get":        {runevents.TopicRunCommandGetStep, runevents.GetStepCommand{WorkspaceID: workspaceID, RunID: runID, StepID: stepID}},
		"step list":       {runevents.TopicRunCommandListStepsByRun, runevents.ListStepsByRunCommand{WorkspaceID: workspaceID, RunID: runID}},
		"checkpoint get":  {runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.GetRuntimeCheckpointCommand{WorkspaceID: workspaceID, RunID: runID, CheckpointID: checkpointID}},
		"checkpoint list": {runevents.TopicRunCommandListCheckpointsByRun, runevents.ListRuntimeCheckpointsByRunCommand{WorkspaceID: workspaceID, RunID: runID}},
	} {
		if _, err := executeRetirementReadTestCommand(biz, ctx, query.topic, query.data); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("%s error=%v, want not found", name, err)
		}
	}
	response, err := executeRetirementReadTestCommand(biz, ctx, runevents.TopicRunCommandListByWorkspace, runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil || len(response.(runevents.ListRunsByWorkspaceResult).Runs) != 0 {
		t.Fatalf("retired workspace list=%+v err=%v, want empty", response, err)
	}
	response, err = executeRetirementReadTestCommand(biz, ctx, runevents.TopicRunCommandList, runevents.ListRunsCommand{})
	if err != nil || len(response.(runevents.ListRunsResult).Runs) != 0 {
		t.Fatalf("global Run list=%+v err=%v, want retired workspace filtered", response, err)
	}
}

func assertRetiredOwnerFactVisible(t *testing.T, biz *Biz, ctx context.Context, workspaceID common.WorkspaceID, deletionID string) {
	t.Helper()
	response, err := executeRetirementReadTestCommandFrom(biz, ctx, runevents.TopicRunCommandWorkspaceRetirementFact, workspaceRetirementFactQuerySource, runevents.QueryWorkspaceRetirementFactCommand{WorkspaceID: workspaceID, DeletionID: deletionID})
	if err != nil {
		t.Fatalf("specialized retirement fact after retired: %v", err)
	}
	if fact := response.(runevents.QueryWorkspaceRetirementFactResult); fact.Fence.Phase != runmodel.WorkspaceRetirementPhaseRetired || len(fact.Runs) != 1 {
		t.Fatalf("retirement fact=%+v, want durable retired inventory", fact)
	}
}

func executeRetirementReadTestCommand(biz *Biz, ctx context.Context, topic string, data any) (any, error) {
	return executeRetirementReadTestCommandFrom(biz, ctx, topic, "test/read-visibility", data)
}

func executeRetirementReadTestCommandFrom(biz *Biz, ctx context.Context, topic, source string, data any) (any, error) {
	event := magicevent.NewEventWithContext(topic, source, runevents.CommandDestination, magicevent.NewHeader(), ctx, data)
	return biz.executeCommand(event)
}
