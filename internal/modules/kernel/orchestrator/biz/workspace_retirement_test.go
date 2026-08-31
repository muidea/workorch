package orchestrator

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type countingOrchestratorWorkspaceDeletionRepository struct {
	repository.OrchestratorWorkspaceDeletionRepository
	compareAndSwapCalls int
}

func (r *countingOrchestratorWorkspaceDeletionRepository) CompareAndSwap(ctx context.Context, item *orchestratormodel.WorkspaceDeletion, expectedRevision uint64) error {
	r.compareAndSwapCalls++
	return r.OrchestratorWorkspaceDeletionRepository.CompareAndSwap(ctx, item, expectedRevision)
}

type countingOrchestratorWorkspaceEvidencePurger struct {
	calls int
}

func (p *countingOrchestratorWorkspaceEvidencePurger) PurgeWorkspaceEvidence(_ context.Context, scope deletioncontract.Scope) (repository.OrchestratorWorkspaceEvidencePurgeReceipt, error) {
	p.calls++
	return repository.NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, repository.EmptyOrchestratorWorkspaceEvidencePurgeObservations())
}

func TestWorkspaceDeletionCommandAdapterCannotInvokePhysicalPurgeSink(t *testing.T) {
	adapter, err := os.ReadFile("workspace_deletion.go")
	if err != nil {
		t.Fatal(err)
	}
	const sink = "m.workspacePurger.PurgeWorkspaceEvidence"
	if strings.Contains(string(adapter), sink) {
		t.Fatal("Workspace deletion command adapter directly invokes the physical purge sink")
	}
	dispatcher, err := os.ReadFile("workspace_deletion_internal_event.go")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(dispatcher), sink); count != 1 {
		t.Fatalf("owner-private deletion dispatcher purge sink count=%d, want exactly one", count)
	}
}

func TestWorkspaceDeletionDispatcherRejectsForgedPurgeCompletionWithoutEffects(t *testing.T) {
	ctx := context.Background()
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: "ws-forged-orchestrator-purge-completion", DeletionID: "deletion-forged-completion", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent/forged-completion",
		RunDrainingRevision: 1,
	}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
		DataDispositionIntent: intent.DataDispositionIntent, WorkspaceIntentHash: intent.WorkspaceIntentHash,
		RunDrainingRevision: intent.RunDrainingRevision, WorkspaceRunGeneration: intent.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	scope := deletioncontract.Scope{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
		Mode: intent.DataDispositionIntent, QuiescenceID: token.QuiescenceID,
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	ownerSet := memoryrepo.NewOrchestratorOwnerRepositorySet()
	base := ownerSet.WorkspaceDeletions()
	quiescing, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventQuiescingStarted, Intent: intent,
		GateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent),
	}, "2026-08-26T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Create(ctx, quiescing); err != nil {
		t.Fatal(err)
	}
	quiesced, err := reduceWorkspaceDeletionInternalEvent(quiescing, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventQuiesced, Intent: intent,
		GateID: quiescing.QuiescingGateID, Token: &token,
	}, "2026-08-26T00:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := base.CompareAndSwap(ctx, quiesced, quiescing.Revision); err != nil {
		t.Fatal(err)
	}
	current, err := reduceWorkspaceDeletionInternalEvent(quiesced, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope,
	}, "2026-08-26T00:02:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := base.CompareAndSwap(ctx, current, quiesced.Revision); err != nil {
		t.Fatal(err)
	}
	deletions := &countingOrchestratorWorkspaceDeletionRepository{OrchestratorWorkspaceDeletionRepository: base}
	purger := &countingOrchestratorWorkspaceEvidencePurger{}
	owner := &Biz{workspaceDeletions: deletions, workspacePurger: purger}
	receipt, err := repository.NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, repository.EmptyOrchestratorWorkspaceEvidencePurgeObservations())
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.dispatchWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope, PurgeReceipt: &receipt,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged purge completion error = %v, want conflict", err)
	}
	if result.Deletion != nil || result.PurgeReceipt != nil || purger.calls != 0 || deletions.compareAndSwapCalls != 0 {
		t.Fatalf("forged purge completion effects: result=%+v purgeCalls=%d casCalls=%d", result, purger.calls, deletions.compareAndSwapCalls)
	}
	persisted, err := base.Get(ctx, scope.WorkspaceID)
	if err != nil || !reflect.DeepEqual(persisted, current) {
		t.Fatalf("forged purge completion changed tombstone: persisted=%+v err=%v", persisted, err)
	}
}

func TestWorkspaceRetirementGateSurvivesBusyConfirmationAndRestart(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	runs := memoryrepo.NewRunRepository()
	steps := memoryrepo.NewRunGuardedStepRepository(runs)
	checkpoints := memoryrepo.NewRunGuardedRuntimeCheckpointRepository(runs)
	workspaceID := common.WorkspaceID("ws-runtime-retirement")
	runID := common.RunID("run-runtime-retirement")
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}

	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, steps, checkpoints, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}

	evidenceOwner := memoryrepo.NewOrchestratorOwnerRepositorySet()
	if err := evidenceOwner.ToolCalls().Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &orchestratormodel.ToolCall{
		ID: "tool-runtime-retirement", WorkspaceID: workspaceID, RunID: runID,
	}); err != nil {
		t.Fatal(err)
	}
	newOrchestrator := func() *Biz {
		owner := New()
		if err := owner.Configure(Dependencies{Evidence: evidenceOwner, CommandHub: hub, BackgroundRoutine: &recordingBackgroundRoutine{}}); err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	orchestratorOwner := newOrchestrator()
	t.Cleanup(func() {
		_ = orchestratorOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})

	driveKey := runtimeDriveKey(workspaceID, runID)
	if !orchestratorOwner.beginRuntimeDrive(driveKey) {
		t.Fatal("failed to install pre-retirement Runtime activity")
	}
	prepare := runevents.PrepareWorkspaceRetirementCommand{
		WorkspaceID: workspaceID, DeletionID: "deletion-1", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent/deletion-1",
	}
	prepared := sendWorkspaceRetirementTestCommand[runevents.PrepareWorkspaceRetirementResult](t, hub,
		runevents.TopicRunCommandPrepareWorkspaceRetirement, runevents.CommandDestination, "blocks/workspace", prepare)
	if err := prepared.ValidateFor(prepare); err != nil {
		t.Fatal(err)
	}

	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: workspaceID, DeletionID: prepare.DeletionID, DeletionGeneration: prepare.DeletionGeneration,
		DataDispositionIntent: prepare.DataDispositionIntent, WorkspaceIntentHash: prepare.WorkspaceIntentHash,
		RunDrainingRevision: prepared.Fence.FenceRevision, WorkspaceRunGeneration: prepared.Fence.WorkspaceRunGeneration,
	}
	begin := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: intent}
	busyEvent := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
		workspaceRetirementOrchestratorCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, begin)
	busyResult := hub.Send(busyEvent)
	if busyResult == nil || busyResult.Error() == nil || !errors.Is(busyResult.Error().Cause, common.ErrConflict) {
		t.Fatalf("busy confirmation error = %v, want conflict", busyResult)
	}
	if _, err := orchestratorOwner.workspaceDeletions.Get(ctx, workspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("busy Begin persisted an Orchestrator gate: %v", err)
	}

	orchestratorOwner.endRuntimeDrive(driveKey)
	started := sendWorkspaceRetirementTestCommand[orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, begin)
	if err := started.ValidateFor(begin); err != nil {
		t.Fatal(err)
	}
	confirm := orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand{Intent: intent}
	confirmed := sendWorkspaceRetirementTestCommand[orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, confirm)
	if err := confirmed.ValidateFor(confirm); err != nil {
		t.Fatal(err)
	}
	replayed := sendWorkspaceRetirementTestCommand[orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, confirm)
	if !reflect.DeepEqual(replayed, confirmed) {
		t.Fatalf("confirmation replay = %+v, want %+v", replayed, confirmed)
	}
	finalize := runevents.FinalizeWorkspaceRetirementCommand{QuiescenceToken: confirmed.Token}
	finalized := sendWorkspaceRetirementTestCommand[runevents.FinalizeWorkspaceRetirementResult](t, hub,
		runevents.TopicRunCommandFinalizeWorkspaceRetirement, runevents.CommandDestination, "blocks/workspace", finalize)
	if err := finalized.ValidateFor(finalize); err != nil {
		t.Fatal(err)
	}

	scope := deletioncontract.Scope{WorkspaceID: workspaceID, DeletionID: intent.DeletionID, DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: confirmed.Token.QuiescenceID}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	seal := orchestratorevents.SealWorkspaceOrchestratorDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	sealed := sendWorkspaceRetirementTestCommand[orchestratorevents.SealWorkspaceOrchestratorDeletionResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, seal)
	if sealed.ValidateFor(seal) != nil {
		t.Fatalf("seal result = %+v", sealed)
	}
	purge := orchestratorevents.PurgeWorkspaceOrchestratorEvidenceCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	firstPurge := sendWorkspaceRetirementTestCommand[orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, purge)
	secondPurge := sendWorkspaceRetirementTestCommand[orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult](t, hub,
		orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence, orchestratorevents.CommandDestination,
		workspaceRetirementOrchestratorCommandSource, purge)
	if firstPurge != secondPurge || firstPurge.ValidateFor(purge) != nil {
		t.Fatalf("purge replay first=%+v second=%+v", firstPurge, secondPurge)
	}
	if firstPurge.PurgeProofHash == orchestratorevents.StableOrchestratorDeletionPurgeProofHash(scope) {
		t.Fatal("non-empty purge returned the canned empty-repository proof")
	}

	if err := orchestratorOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	orchestratorOwner = newOrchestrator()
	if orchestratorOwner.beginRuntimeDrive(runtimeDriveKey(workspaceID, "run-after-restart")) {
		t.Fatal("startup retirement inventory failed to restore the permanent gate")
	}
}

func TestWorkspaceRetirementCommandsRejectUntrustedSources(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	runs := memoryrepo.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-runtime-retirement-source")
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-1", WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	orchestratorOwner := New()
	if err := orchestratorOwner.Configure(workspaceRetirementTestDependencies(hub)); err != nil {
		t.Fatal(err)
	}
	if err := orchestratorOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = orchestratorOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})

	prepare := runevents.PrepareWorkspaceRetirementCommand{
		WorkspaceID: workspaceID, DeletionID: "deletion-source", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent/deletion-source",
	}
	for _, source := range []string{"kernel/orchestrator", "application/workorchd", "untrusted/module"} {
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandPrepareWorkspaceRetirement, source,
			runevents.CommandDestination, magicevent.NewHeader(), ctx, prepare)
		result := hub.Send(event)
		if result == nil || result.Error() == nil || !errors.Is(result.Error().Cause, common.ErrInvalidArgument) {
			t.Fatalf("Run retirement source %q error = %v", source, result)
		}
	}
	if _, err := runs.GetWorkspaceRetirement(ctx, workspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("untrusted Run retirement created a fence: %v", err)
	}

	begin := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: workspaceID, DeletionID: prepare.DeletionID, DeletionGeneration: 1,
		DataDispositionIntent: prepare.DataDispositionIntent, WorkspaceIntentHash: prepare.WorkspaceIntentHash,
		RunDrainingRevision: 1, WorkspaceRunGeneration: 1,
	}}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
		"application/workorchd", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, begin)
	result := hub.Send(event)
	if result == nil || result.Error() == nil || !errors.Is(result.Error().Cause, common.ErrInvalidArgument) {
		t.Fatalf("untrusted Orchestrator confirmation error = %v", result)
	}
	if _, retired := orchestratorOwner.workspaceRetirementGate(workspaceID); retired {
		t.Fatal("untrusted Orchestrator confirmation installed a gate")
	}
}

func TestRuntimeDriveFailsClosedWithoutRunRetirementOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := New()
	if err := owner.Configure(workspaceRetirementTestDependencies(hub)); err != nil {
		t.Fatal(err)
	}
	err := owner.scheduleRuntimeDrive(ctx, "ws-retirement-owner-missing", "run-retirement-owner-missing", "missing-run-owner")
	if err == nil {
		t.Fatal("Runtime drive was admitted without the Run retirement fact owner")
	}
	if _, active := owner.workspaceRetirementGate("ws-retirement-owner-missing"); active {
		t.Fatal("missing Run owner installed a synthetic retirement fact")
	}
}

func workspaceRetirementTestDependencies(hub magicevent.Hub) Dependencies {
	return Dependencies{
		Evidence:          memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub:        hub,
		BackgroundRoutine: &recordingBackgroundRoutine{},
	}
}

func sendWorkspaceRetirementTestCommand[Response any](t *testing.T, hub magicevent.Hub, topic, destination, source string, command any) Response {
	t.Helper()
	event := magicevent.NewEventWithContext(topic, source, destination, magicevent.NewHeader(), context.Background(), command)
	result := hub.Send(event)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s: %v", topic, result)
	}
	value, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := value.(Response)
	if !ok {
		t.Fatalf("%s response type = %T", topic, value)
	}
	return response
}
