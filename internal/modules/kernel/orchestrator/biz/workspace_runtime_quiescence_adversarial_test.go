package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	workorchrepo "github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceRuntimeQuiescencePendingTerminalAckDrainsBeforeFreeze(t *testing.T) {
	fixture := newAdversarialRuntimeQuiescenceFixture(t, false)
	fence := fixture.prepare(t, deletioncontract.ModeRetain)
	command := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: adversarialQuiescenceIntent(*fence)}

	if _, err := fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("begin with pending terminal acknowledgement error = %v, want conflict", err)
	}
	if _, err := fixture.deletions.Get(fixture.ctx, fixture.workspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("pending acknowledgement installed durable gate: %v", err)
	}
	if adversarialWorkspaceIsQuiescing(fixture.owner, fixture.workspaceID) {
		t.Fatal("pending acknowledgement installed process-local gate")
	}
	for _, event := range []runtime.RuntimeInternalEvent{
		{Kind: runtime.RuntimeInternalEventFailureObserved, Scope: adversarialRuntimeScope(fixture)},
		{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, Scope: adversarialRuntimeScope(fixture)},
		{Kind: runtime.RuntimeInternalEventDriveLeaseScheduled, Scope: adversarialRuntimeScope(fixture), Delivery: &runtime.RuntimeInternalDeliveryPayload{DriveLease: &runtimecontract.RuntimeDriveLease{
			WorkspaceID: fixture.workspaceID, RunID: fixture.runID, Reason: runtimecontract.RuntimeDriveLeaseReasonProviderRetry,
			FailureSignature: "ordinary-provider-retry", DueAt: "2026-08-26T00:00:00Z",
		}}},
	} {
		if _, err := fixture.owner.submitRuntimeInternalEvent(fixture.ctx, event); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("Run draining admitted ordinary Runtime event %q: %v", event.Kind, err)
		}
	}
	preGateEvidenceCalled := false
	if err := fixture.owner.withOrchestratorEvidenceMutation(fixture.ctx, fixture.workspaceID, func(context.Context) error {
		preGateEvidenceCalled = true
		return nil
	}); !errors.Is(err, common.ErrConflict) || preGateEvidenceCalled {
		t.Fatalf("Run draining evidence fence err=%v callback=%v", err, preGateEvidenceCalled)
	}

	runAck := adversarialTerminalAcknowledgement(fixture.request, runtimecontract.RuntimeDeliveryOwnerRun)
	if err := fixture.owner.recordRuntimeDeliveryAcknowledgement(fixture.ctx, &runAck); err != nil {
		t.Fatalf("Run draining rejected exact terminal acknowledgement CAS: %v", err)
	}
	if _, err := fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command); err != nil {
		t.Fatalf("begin after exact terminal acknowledgement drain: %v", err)
	}

	before, err := fixture.owner.runtimeController.Load(fixture.ctx, fixture.workspaceID, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	blockedEvents := []runtime.RuntimeInternalEvent{
		{Kind: runtime.RuntimeInternalEventFailureObserved, Scope: adversarialRuntimeScope(fixture)},
		{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, Scope: adversarialRuntimeScope(fixture)},
		{Kind: runtime.RuntimeInternalEventTerminalDeliveryAcknowledged, Scope: adversarialRuntimeScope(fixture), Delivery: &runtime.RuntimeInternalDeliveryPayload{DeliveryAcknowledgement: &runAck}},
		{Kind: runtime.RuntimeInternalEventDriveLeaseScheduled, Scope: adversarialRuntimeScope(fixture), Delivery: &runtime.RuntimeInternalDeliveryPayload{DriveLease: &runtimecontract.RuntimeDriveLease{WorkspaceID: fixture.workspaceID, RunID: fixture.runID}}},
	}
	for _, event := range blockedEvents {
		if _, err := fixture.owner.submitRuntimeInternalEvent(fixture.ctx, event); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("post-freeze event %q error = %v, want conflict", event.Kind, err)
		}
	}
	after, err := fixture.owner.runtimeController.Load(fixture.ctx, fixture.workspaceID, fixture.runID)
	if err != nil || after.StateRevision != before.StateRevision {
		t.Fatalf("post-freeze aggregate revision = %d err=%v, want %d", after.StateRevision, err, before.StateRevision)
	}
	if fixture.owner.beginRuntimeDrive(runtimeDriveKey(fixture.workspaceID, "run-late")) {
		t.Fatal("post-freeze process-local drive was admitted")
	}
	called := false
	if err := fixture.deletions.WithWorkspaceActiveFence(fixture.ctx, fixture.workspaceID, func(context.Context) error {
		called = true
		return nil
	}); !errors.Is(err, common.ErrNotFound) || called {
		t.Fatalf("post-freeze evidence write err=%v callback=%v, want fenced without callback", err, called)
	}
	readCalled := false
	if err := fixture.owner.withOrchestratorWorkspaceResourceRead(fixture.ctx, fixture.workspaceID, func(context.Context) error {
		readCalled = true
		return nil
	}); !errors.Is(err, common.ErrNotFound) || readCalled {
		t.Fatalf("post-freeze resource read err=%v callback=%v, want hidden without callback", err, readCalled)
	}

	confirmed, err := fixture.owner.confirmWorkspaceRuntimeQuiescence(fixture.ctx, orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand{Intent: command.Intent})
	if err != nil {
		t.Fatalf("confirm quiescence: %v", err)
	}
	currentRun, err := fixture.runs.GetByWorkspaceID(fixture.ctx, fixture.workspaceID, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed.Token.Runs) != 1 || confirmed.Token.Runs[0].RuntimeStateHash != runtimecontract.StableRuntimeStateRecordHash(*currentRun.RuntimeState) {
		t.Fatalf("confirmed token did not carry Run's canonical Runtime snapshot: %+v", confirmed.Token)
	}
	retired, err := fixture.runs.FinalizeWorkspaceRetirement(fixture.ctx, confirmed.Token)
	if err != nil {
		t.Fatalf("Run finalize with shared token: %v", err)
	}
	if !retired.IsRetired() || retired.RuntimeInventoryHash != confirmed.Token.RuntimeInventoryHash || retired.QuiescenceID != confirmed.Token.QuiescenceID {
		t.Fatalf("retired fence = %+v, token = %+v", retired, confirmed.Token)
	}
}

func TestWorkspaceRuntimeQuiescencePendingDeliveryLeaseMustDrainBeforeFreeze(t *testing.T) {
	fixture := newAdversarialRuntimeQuiescenceFixture(t, true)
	requested := runtimecontract.RuntimeDriveLease{
		WorkspaceID: fixture.workspaceID, RunID: fixture.runID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		DueAt: "2026-08-26T00:00:00Z",
	}
	scheduled, changed, err := fixture.owner.runtimeScheduleDriveLease(fixture.ctx, fixture.workspaceID, fixture.runID, requested, 0)
	if err != nil || !changed {
		t.Fatalf("schedule pre-draining terminal lease = %+v changed=%v err=%v", scheduled, changed, err)
	}
	fence := fixture.prepare(t, deletioncontract.ModeRetain)
	command := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: adversarialQuiescenceIntent(*fence)}
	if _, err := fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("begin with persisted terminal lease error = %v, want conflict", err)
	}
	if _, err := fixture.deletions.Get(fixture.ctx, fixture.workspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("pending lease installed durable gate: %v", err)
	}
	if err := fixture.owner.scheduleRuntimeDrive(fixture.ctx, fixture.workspaceID, fixture.runID, "ordinary-post-terminal-drive"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Run draining admitted ordinary fully-acknowledged drive: %v", err)
	}

	claimed, claimedLease, err := fixture.owner.runtimeClaimDriveLease(fixture.ctx, scheduled, "2026-08-26T00:00:01Z")
	if err != nil || !claimedLease {
		t.Fatalf("claim draining terminal lease = %+v claimed=%v err=%v", claimed, claimedLease, err)
	}
	if released, err := fixture.owner.runtimeReleaseDriveLease(fixture.ctx, fixture.workspaceID, fixture.runID, claimed.RecoveryGeneration); err != nil || !released {
		t.Fatalf("release draining terminal lease released=%v err=%v", released, err)
	}
	if _, err := fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command); err != nil {
		t.Fatalf("begin after exact terminal lease drain: %v", err)
	}
}

func TestWorkspaceRuntimeQuiescenceStartupSetupFactsClassifyDrainingOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	runs := memoryrepo.NewRunRepository()
	_, _, item := adversarialTerminalRun(t, false)
	if err := runs.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: item.WorkspaceID, DeletionID: "deletion-startup-draining", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent:startup-draining",
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	// Configure is the framework Setup boundary. Run.Start is deliberately not
	// called: Orchestrator startup must acquire retirement/recovery facts before
	// the regular Run command surface is live.
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	background := &startupReadyRecordingBackgroundRoutine{scheduled: make(chan struct{}, 1)}
	orchestratorOwner := New()
	if err := orchestratorOwner.Configure(Dependencies{
		Evidence: memoryrepo.NewOrchestratorOwnerRepositorySet(), CommandHub: hub, BackgroundRoutine: background,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = orchestratorOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})
	if err := orchestratorOwner.Start(); err != nil {
		t.Fatalf("Orchestrator Start with Setup-only Run owner: %v", err)
	}
	orchestratorOwner.startupRecoveryMu.Lock()
	actions := append([]runtimeStartupRecoveryAction(nil), orchestratorOwner.startupRecoveryActions...)
	ready := orchestratorOwner.runRuntimeCommandsReady
	orchestratorOwner.startupRecoveryMu.Unlock()
	if ready || len(actions) != 1 || actions[0].Kind != runtimeStartupRecoveryActionDrive || actions[0].Source != "startup_recovery:workspace_retirement_terminal_delivery" || actions[0].Item.WorkspaceID != item.WorkspaceID || actions[0].Item.RunID != item.ID {
		t.Fatalf("Setup-only draining recovery actions=%+v ready=%v", actions, ready)
	}
	select {
	case <-background.scheduled:
		t.Fatal("draining startup action ran before Run commands-ready fact")
	default:
	}
}

func TestWorkspaceRuntimeQuiescenceStartupSetupFactsClassifyPendingDeliveryLeaseOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	runs := memoryrepo.NewRunRepository()
	aggregate, _, item := adversarialTerminalRun(t, false)
	aggregate.StateRevision++
	aggregate.DriveLease = &runtimecontract.RuntimeDriveLease{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AggregateRevision: aggregate.StateRevision, RecoveryGeneration: 1,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		DueAt: "2030-01-01T00:00:00Z", State: runtimecontract.RuntimeDriveLeaseScheduled,
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("terminal aggregate with persisted delivery lease: %v", err)
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	item.RuntimeState = &record
	item.RuntimeLifecycleFence = mustRuntimeLifecycleFence(t, item)
	if err := runs.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: item.WorkspaceID, DeletionID: "deletion-startup-draining-lease", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent:startup-draining-lease",
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	// Keep Run at Setup: startup classification may queue only the exact
	// persisted terminal-delivery lease drain, never an ordinary Runtime drive.
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	background := &startupReadyRecordingBackgroundRoutine{scheduled: make(chan struct{}, 1)}
	orchestratorOwner := New()
	if err := orchestratorOwner.Configure(Dependencies{
		Evidence: memoryrepo.NewOrchestratorOwnerRepositorySet(), CommandHub: hub, BackgroundRoutine: background,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = orchestratorOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})
	if err := orchestratorOwner.Start(); err != nil {
		t.Fatalf("Orchestrator Start with Setup-only Run owner: %v", err)
	}
	orchestratorOwner.startupRecoveryMu.Lock()
	actions := append([]runtimeStartupRecoveryAction(nil), orchestratorOwner.startupRecoveryActions...)
	ready := orchestratorOwner.runRuntimeCommandsReady
	orchestratorOwner.startupRecoveryMu.Unlock()
	if ready || len(actions) != 1 || actions[0].Kind != runtimeStartupRecoveryActionRecoverLease || actions[0].Source != "startup_recovery:workspace_retirement_drain" || actions[0].Item.WorkspaceID != item.WorkspaceID || actions[0].Item.RunID != item.ID {
		t.Fatalf("Setup-only draining lease recovery actions=%+v ready=%v", actions, ready)
	}
	select {
	case <-background.scheduled:
		t.Fatal("draining terminal-delivery lease recovery ran before Run commands-ready fact")
	default:
	}
}

func TestWorkspaceRuntimeQuiescenceStartupSetupFactsSkipRetired(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	runs := memoryrepo.NewRunRepository()
	_, _, item := adversarialTerminalRun(t, true)
	if err := runs.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	fence, err := runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: item.WorkspaceID, DeletionID: "deletion-startup-retired", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent:startup-retired",
	})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, item.WorkspaceID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	token := adversarialQuiescenceToken(*fence, persisted)
	if _, err := runs.FinalizeWorkspaceRetirement(ctx, token); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	background := &startupReadyRecordingBackgroundRoutine{scheduled: make(chan struct{}, 1)}
	orchestratorOwner := New()
	if err := orchestratorOwner.Configure(Dependencies{
		Evidence: memoryrepo.NewOrchestratorOwnerRepositorySet(), CommandHub: hub, BackgroundRoutine: background,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = orchestratorOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})
	if err := orchestratorOwner.Start(); err != nil {
		t.Fatalf("Orchestrator Start with Setup-only retired Run owner: %v", err)
	}
	orchestratorOwner.startupRecoveryMu.Lock()
	actionCount := len(orchestratorOwner.startupRecoveryActions)
	orchestratorOwner.startupRecoveryMu.Unlock()
	if actionCount != 0 {
		t.Fatalf("retired Workspace produced %d startup recovery actions", actionCount)
	}
	select {
	case <-background.scheduled:
		t.Fatal("retired Workspace scheduled a startup recovery drive")
	default:
	}
}

func TestWorkspaceRuntimeQuiescenceBeginLinearizesWithNewDrive(t *testing.T) {
	for iteration := 0; iteration < 30; iteration++ {
		fixture := newAdversarialNoRuntimeQuiescenceFixture(t, fmt.Sprintf("linearize-%d", iteration), memoryrepo.NewOrchestratorOwnerRepositorySet())
		fence := fixture.prepare(t, deletioncontract.ModeRetain)
		command := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: adversarialQuiescenceIntent(*fence)}
		driveKey := runtimeDriveKey(fixture.workspaceID, fixture.runID)
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var beginErr error
		var driveAccepted bool
		go func() {
			defer wait.Done()
			<-start
			_, beginErr = fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command)
		}()
		go func() {
			defer wait.Done()
			<-start
			driveAccepted = fixture.owner.beginRuntimeDrive(driveKey)
		}()
		close(start)
		wait.Wait()

		if driveAccepted {
			if !errors.Is(beginErr, common.ErrConflict) {
				t.Fatalf("iteration %d drive won but Begin error = %v", iteration, beginErr)
			}
			if _, err := fixture.deletions.Get(fixture.ctx, fixture.workspaceID); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("iteration %d busy drive raced with durable gate: %v", iteration, err)
			}
			fixture.owner.endRuntimeDrive(driveKey)
			if _, err := fixture.owner.beginWorkspaceRuntimeQuiescence(fixture.ctx, command); err != nil {
				t.Fatalf("iteration %d retry Begin after drive drain: %v", iteration, err)
			}
		} else if beginErr != nil {
			t.Fatalf("iteration %d neither Begin nor drive won: %v", iteration, beginErr)
		}
		if !adversarialWorkspaceIsQuiescing(fixture.owner, fixture.workspaceID) {
			t.Fatalf("iteration %d successful Begin did not install local gate", iteration)
		}
		fixture.close(t)
	}
}

func TestWorkspaceRuntimeQuiescenceFileRestartRestoresTokenGateAndRunDraining(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-quiescence-file-restart")
	runID := common.RunID("run-001")
	firstRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	firstHub := magicevent.NewHub(32)
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runOwnerTestDependencies(firstRepositories.Runs, firstRepositories.Steps, firstRepositories.RuntimeCheckpoints, firstHub)); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := firstRepositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	intent := runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-file-restart", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent:file-restart",
	}
	fence, err := firstRepositories.Runs.PrepareWorkspaceRetirement(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	firstOrchestrator := New()
	if err := firstOrchestrator.Configure(Dependencies{Evidence: firstRepositories.OrchestratorEvidence, CommandHub: firstHub, BackgroundRoutine: &recordingBackgroundRoutine{}}); err != nil {
		t.Fatal(err)
	}
	quiescenceIntent := adversarialQuiescenceIntent(*fence)
	if _, err := firstOrchestrator.beginWorkspaceRuntimeQuiescence(ctx, orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: quiescenceIntent}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := firstOrchestrator.confirmWorkspaceRuntimeQuiescence(ctx, orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand{Intent: quiescenceIntent})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	secondRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	reopenedFence, err := secondRepositories.Runs.GetWorkspaceRetirement(ctx, workspaceID)
	if err != nil || !reopenedFence.IsDraining() || !runmodel.SameWorkspaceRetirementFence(*reopenedFence, *fence) {
		t.Fatalf("reopened Run draining fence = %+v err=%v", reopenedFence, err)
	}
	reopenedDeletion, err := secondRepositories.OrchestratorDeletions.Get(ctx, workspaceID)
	if err != nil || reopenedDeletion.QuiescenceToken == nil || reopenedDeletion.QuiescenceToken.QuiescenceID != confirmed.Token.QuiescenceID || reopenedDeletion.QuiescenceToken.RuntimeInventoryHash != confirmed.Token.RuntimeInventoryHash {
		t.Fatalf("reopened Orchestrator token = %+v err=%v", reopenedDeletion, err)
	}

	secondHub := magicevent.NewHub(32)
	secondRunOwner := runmodule.New()
	if err := secondRunOwner.Configure(runOwnerTestDependencies(secondRepositories.Runs, secondRepositories.Steps, secondRepositories.RuntimeCheckpoints, secondHub)); err != nil {
		t.Fatal(err)
	}
	if err := secondRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	secondOrchestrator := New()
	if err := secondOrchestrator.Configure(Dependencies{Evidence: secondRepositories.OrchestratorEvidence, CommandHub: secondHub, BackgroundRoutine: &recordingBackgroundRoutine{}}); err != nil {
		t.Fatal(err)
	}
	if err := secondOrchestrator.Start(); err != nil {
		t.Fatalf("restart Orchestrator: %v", err)
	}
	t.Cleanup(func() {
		_ = secondOrchestrator.Stop(ctx)
		_ = secondRunOwner.Stop(ctx)
		secondHub.Terminate(ctx)
	})
	if !adversarialWorkspaceIsQuiescing(secondOrchestrator, workspaceID) {
		t.Fatal("restart did not restore durable quiescence gate")
	}
	if secondOrchestrator.beginRuntimeDrive(runtimeDriveKey(workspaceID, runID)) {
		t.Fatal("restart admitted a drive before restoring durable gate")
	}
}

type adversarialRuntimeQuiescenceFixture struct {
	ctx         context.Context
	hub         magicevent.Hub
	runs        repository.RunRepository
	deletions   repository.OrchestratorWorkspaceDeletionRepository
	owner       *Biz
	workspaceID common.WorkspaceID
	runID       common.RunID
	request     orchestratorevents.RuntimeTerminalDeliveryRequestData
	runOwner    *runmodule.Module
}

func newAdversarialRuntimeQuiescenceFixture(t *testing.T, fullyAcknowledged bool) *adversarialRuntimeQuiescenceFixture {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	runs := memoryrepo.NewRunRepository()
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	evidence := memoryrepo.NewOrchestratorOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{Evidence: evidence, CommandHub: hub, BackgroundRoutine: &recordingBackgroundRoutine{}}); err != nil {
		t.Fatal(err)
	}
	aggregate, request, item := adversarialTerminalRun(t, fullyAcknowledged)
	if err := runs.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	fixture := &adversarialRuntimeQuiescenceFixture{
		ctx: ctx, hub: hub, runs: runs, deletions: evidence.WorkspaceDeletions(), owner: owner,
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, request: request, runOwner: runOwner,
	}
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func adversarialTerminalRun(t *testing.T, fullyAcknowledged bool) (runtime.RuntimeAggregate, orchestratorevents.RuntimeTerminalDeliveryRequestData, *runmodel.Run) {
	t.Helper()
	aggregate := runtimeTerminalAggregate(t)
	request := mustTerminalDeliveryRequest(t, aggregate)
	owners := []runtimecontract.RuntimeDeliveryOwner{runtimecontract.RuntimeDeliveryOwnerResult, runtimecontract.RuntimeDeliveryOwnerWorkspace}
	if fullyAcknowledged {
		owners = append(owners, runtimecontract.RuntimeDeliveryOwnerRun)
	}
	for _, deliveryOwner := range owners {
		aggregate.DeliveryAcknowledgements = append(aggregate.DeliveryAcknowledgements, adversarialTerminalAcknowledgement(request, deliveryOwner))
		aggregate.StateRevision++
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("terminal aggregate fixture: %v", err)
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	item := runtimeRunForTest(common.RunStateFailed, aggregate.Definition)
	item.RuntimeState = &record
	item.RuntimeLifecycleFence = mustRuntimeLifecycleFence(t, item)
	identity := request.RuntimeTerminalDeliveryIdentity
	item.TerminalDelivery = &identity
	item.TerminalResultID = common.ResultID(string(request.WorkspaceID) + "/" + string(request.RunID) + "/result/dod-assessment")
	return aggregate, request, item
}

func newAdversarialNoRuntimeQuiescenceFixture(t *testing.T, suffix string, evidence repository.OrchestratorEvidenceOwnerRepositories) *adversarialRuntimeQuiescenceFixture {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	runs := memoryrepo.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-quiescence-" + suffix)
	runID := common.RunID("run-001")
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	owner := New()
	if err := owner.Configure(Dependencies{Evidence: evidence, CommandHub: hub, BackgroundRoutine: &recordingBackgroundRoutine{}}); err != nil {
		t.Fatal(err)
	}
	return &adversarialRuntimeQuiescenceFixture{
		ctx: ctx, hub: hub, runs: runs, deletions: evidence.WorkspaceDeletions(), owner: owner,
		workspaceID: workspaceID, runID: runID, runOwner: runOwner,
	}
}

func (f *adversarialRuntimeQuiescenceFixture) prepare(t *testing.T, mode deletioncontract.Mode) *runmodel.WorkspaceRetirementFence {
	t.Helper()
	fence, err := f.runs.PrepareWorkspaceRetirement(f.ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: f.workspaceID, DeletionID: "deletion-" + string(f.workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: mode, WorkspaceIntentHash: "workspace-intent:" + string(f.workspaceID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fence
}

func (f *adversarialRuntimeQuiescenceFixture) close(t *testing.T) {
	t.Helper()
	if f == nil || f.hub == nil {
		return
	}
	if f.runOwner != nil {
		_ = f.runOwner.Stop(f.ctx)
	}
	f.hub.Terminate(f.ctx)
	f.hub = nil
}

func adversarialQuiescenceIntent(fence runmodel.WorkspaceRetirementFence) deletioncontract.WorkspaceRuntimeQuiescenceIntent {
	return deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
	}
}

func adversarialTerminalAcknowledgement(request orchestratorevents.RuntimeTerminalDeliveryRequestData, owner runtimecontract.RuntimeDeliveryOwner) runtimecontract.RuntimeDeliveryAcknowledgement {
	state := runtimecontract.RuntimeDeliveryAcknowledgedNoop
	var resultID common.ResultID
	switch owner {
	case runtimecontract.RuntimeDeliveryOwnerResult:
		state = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
		resultID = common.ResultID(string(request.WorkspaceID) + "/" + string(request.RunID) + "/result/dod-assessment")
	case runtimecontract.RuntimeDeliveryOwnerWorkspace:
		state = runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased
	case runtimecontract.RuntimeDeliveryOwnerRun:
		state = runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal
	}
	return runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, ResultID: resultID, TerminalDeliveryID: request.TerminalDeliveryID,
		Owner: owner, AcknowledgedState: state, AuthoritySnapshotID: request.AuthoritySnapshotID,
		CausationID: request.CausationID, Outcome: request.Outcome,
	}
}

func adversarialQuiescenceToken(fence runmodel.WorkspaceRetirementFence, item *runmodel.Run) deletioncontract.WorkspaceRuntimeQuiescenceToken {
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
		Runs: []deletioncontract.WorkspaceRuntimeQuiescenceRun{{
			RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
			RuntimeStatePresent: true, RuntimeStateRevision: item.RuntimeState.StateRevision,
			RuntimeStateRef:    fmt.Sprintf("runtime-state/%s/%s/%d", item.WorkspaceID, item.ID, item.RuntimeState.StateRevision),
			RuntimeStateHash:   runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState),
			TerminalDeliveryID: item.TerminalDelivery.TerminalDeliveryID, TerminalDeliveryAcked: true,
		}},
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token
}

func adversarialRuntimeScope(fixture *adversarialRuntimeQuiescenceFixture) runtime.RuntimeInternalEventScope {
	return runtime.RuntimeInternalEventScope{WorkspaceID: fixture.workspaceID, RunID: fixture.runID}
}

func adversarialWorkspaceIsQuiescing(owner *Biz, workspaceID common.WorkspaceID) bool {
	owner.runtimeMutationFence.RLock()
	defer owner.runtimeMutationFence.RUnlock()
	_, exists := owner.quiescingWorkspaces[workspaceID]
	return exists
}
