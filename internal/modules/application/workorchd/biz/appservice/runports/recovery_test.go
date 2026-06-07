package runports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func TestHasContinuationCheckpointUsesLatestRecoveryCheckpoint(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-recovery-checkpoint")

	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             "run-recovery-checkpoint/checkpoint-001",
		RunID:          runID,
		CheckpointType: common.CheckpointTypeRunState,
		Data:           map[string]any{"nextAction": "continue"},
		OccurredAt:     "2026-06-04T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed progress checkpoint: %v", err)
	}
	if HasContinuationCheckpoint(ctx, exports.Infrastructure.EventHub, runID) {
		t.Fatal("progress checkpoint should not count as continuation checkpoint")
	}

	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             "run-recovery-checkpoint/checkpoint-002",
		RunID:          runID,
		CheckpointType: common.CheckpointTypeRecovery,
		Data:           map[string]any{"resumeAction": "resume_run"},
		OccurredAt:     "2026-06-04T00:01:00Z",
	}); err != nil {
		t.Fatalf("seed recovery checkpoint: %v", err)
	}
	if !HasContinuationCheckpoint(ctx, exports.Infrastructure.EventHub, runID) {
		t.Fatal("latest recovery checkpoint with resume action should count")
	}

	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             "run-recovery-checkpoint/checkpoint-003",
		RunID:          runID,
		CheckpointType: common.CheckpointTypeRecovery,
		Data:           map[string]any{},
		OccurredAt:     "2026-06-04T00:02:00Z",
	}); err != nil {
		t.Fatalf("seed empty recovery checkpoint: %v", err)
	}
	if HasContinuationCheckpoint(ctx, exports.Infrastructure.EventHub, runID) {
		t.Fatal("latest recovery checkpoint without continuation action should not count")
	}
}

func TestIsStuckRunningRunCandidateRequiresRunningUnblockedRunWithoutContinuation(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()

	candidate := &runmodel.Run{
		ID:          "run-stuck-candidate",
		WorkspaceID: "ws-stuck-candidate",
		State:       common.RunStateRunning,
	}
	if !IsStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, candidate) {
		t.Fatal("running unblocked run without continuation checkpoint should be a stuck-running candidate")
	}
	if IsStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, &runmodel.Run{
		ID:          "run-completed",
		WorkspaceID: "ws-stuck-candidate",
		State:       common.RunStateCompleted,
	}) {
		t.Fatal("completed run should not be a stuck-running candidate")
	}
	if IsStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, &runmodel.Run{
		ID:               "run-blocked",
		WorkspaceID:      "ws-stuck-candidate",
		State:            common.RunStateRunning,
		BlockingTicketID: "ticket-001",
	}) {
		t.Fatal("blocked run should not be a stuck-running candidate")
	}

	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             "run-stuck-candidate/checkpoint-001",
		RunID:          candidate.ID,
		CheckpointType: common.CheckpointTypeRecovery,
		Data:           map[string]any{"nextAction": "continue_run"},
		OccurredAt:     "2026-06-04T00:03:00Z",
	}); err != nil {
		t.Fatalf("seed continuation checkpoint: %v", err)
	}
	if IsStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, candidate) {
		t.Fatal("running run with continuation checkpoint should not be a stuck-running candidate")
	}
}

func TestOrphanDispatchedTerminalStepReturnsDispatchedTerminalStep(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-orphan-terminal-runports")
	rootStepID := common.StepID("run-orphan-terminal-runports/root")
	dispatchedStepID := common.StepID("run-orphan-terminal-runports/follow-up")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-orphan-terminal-runports",
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    dispatchedStepID,
		RunID: runID,
		State: common.StepStateDispatched,
		Input: map[string]any{"previousStepId": string(rootStepID)},
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}

	terminal, err := OrphanDispatchedTerminalStep(ctx, exports.Infrastructure.EventHub, run)
	if err != nil {
		t.Fatalf("OrphanDispatchedTerminalStep: %v", err)
	}
	if terminal == nil || terminal.ID != dispatchedStepID {
		t.Fatalf("terminal = %+v, want %s", terminal, dispatchedStepID)
	}

	run.State = common.RunStateCompleted
	terminal, err = OrphanDispatchedTerminalStep(ctx, exports.Infrastructure.EventHub, run)
	if err != nil {
		t.Fatalf("OrphanDispatchedTerminalStep completed run: %v", err)
	}
	if terminal != nil {
		t.Fatalf("terminal for completed run = %+v, want nil", terminal)
	}
}

func TestRecoverOrphanDispatchedStepUsesInjectedResume(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-orphan-recover-runports")
	workspaceID := common.WorkspaceID("ws-orphan-recover-runports")
	rootStepID := common.StepID("run-orphan-recover-runports/root")
	dispatchedStepID := common.StepID("run-orphan-recover-runports/follow-up")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    dispatchedStepID,
		RunID: runID,
		State: common.StepStateDispatched,
		Input: map[string]any{"previousStepId": string(rootStepID)},
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}

	resumeCalls := 0
	recovered, changed, err := RecoverOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(resumeCtx context.Context, current *runmodel.Run) error {
		resumeCalls++
		step, err := exports.Repositories.Steps.Get(resumeCtx, dispatchedStepID)
		if err != nil {
			return err
		}
		step.State = common.StepStateCompleted
		if err := exports.Repositories.Steps.Update(resumeCtx, step); err != nil {
			return err
		}
		latest, err := exports.Repositories.Runs.Get(resumeCtx, current.ID)
		if err != nil {
			return err
		}
		latest.State = common.RunStateCompleted
		return exports.Repositories.Runs.Update(resumeCtx, latest)
	})
	if err != nil {
		t.Fatalf("RecoverOrphanDispatchedStep: %v", err)
	}
	if !changed || recovered == nil || recovered.ID != dispatchedStepID {
		t.Fatalf("recovered=%+v changed=%v, want dispatched step recovery", recovered, changed)
	}
	if resumeCalls != 1 {
		t.Fatalf("resumeCalls = %d, want 1", resumeCalls)
	}
	runAfter, err := exports.Repositories.Runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run after recovery: %v", err)
	}
	if runAfter.State != common.RunStateCompleted {
		t.Fatalf("runAfter.State = %s, want completed", runAfter.State)
	}
}

func TestRecordRecoveryMemoryRecallTraceUsesEventHubOwnerCommand(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-recovery-memory-runports")
	runID := common.RunID("run-recovery-memory-runports")
	if err := exports.Repositories.MemoryRecallTraces.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 "trace-recovery-memory-runports-source",
		WorkspaceID:        workspaceID,
		RunID:              runID,
		Source:             "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-recovery-runports"},
		EvidenceRef:        "trace-recovery-memory-runports-source",
	}); err != nil {
		t.Fatalf("seed source trace: %v", err)
	}
	if err := exports.Repositories.MemoryRecallTraces.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 "trace-recovery-memory-runports-other",
		WorkspaceID:        "ws-other",
		RunID:              runID,
		Source:             "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-other"},
		EvidenceRef:        "trace-recovery-memory-runports-other",
	}); err != nil {
		t.Fatalf("seed other trace: %v", err)
	}

	err := RecordRecoveryMemoryRecallTrace(ctx, exports.Infrastructure.EventHub, readmodelports.MemoryRecallTraceReader{Hub: exports.Infrastructure.EventHub}, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}, map[string]any{
		"sourceSectionRefs": []string{"run", "memory_recall"},
	})
	if err != nil {
		t.Fatalf("RecordRecoveryMemoryRecallTrace: %v", err)
	}
	traces, err := exports.Repositories.MemoryRecallTraces.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: workspaceID, RunID: runID, Source: "recovery"})
	if err != nil {
		t.Fatalf("list recovery traces: %v", err)
	}
	if len(traces) != 1 ||
		traces[0].Policy != "inherit_compressed_context" ||
		len(traces[0].RetrievedMemoryIDs) != 1 ||
		traces[0].RetrievedMemoryIDs[0] != "memory-recovery-runports" {
		t.Fatalf("recovery traces = %+v, want inherited workspace-scoped memory recall trace", traces)
	}
}

func TestAttachCompressedContextToRunUpdatesCreatedRunThroughEventHub(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-attach-compressed-runports")
	workspaceID := common.WorkspaceID("ws-attach-compressed-runports")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	readers, err := readmodelports.NewOwnerReaders(exports.Infrastructure.EventHub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}

	if err := AttachCompressedContextToRun(ctx, CompressedContextDependencies{
		Runs:          readers.Runs,
		MemoryRecalls: readers.MemoryRecalls,
		EventHub:      exports.Infrastructure.EventHub,
		RecoverySnapshot: func(context.Context, common.RunID) (readmodelcore.RunSnapshot, error) {
			return readmodelcore.RunSnapshot{}, nil
		},
		DefaultLanguage: "en-US",
	}, runID); err != nil {
		t.Fatalf("AttachCompressedContextToRun: %v", err)
	}
	updated, err := exports.Repositories.Runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read updated run: %v", err)
	}
	compressed, ok := updated.ExecutionContext["compressedContext"].(map[string]any)
	if !ok || compressed["runId"] != string(runID) || compressed["workspaceId"] != string(workspaceID) {
		t.Fatalf("compressedContext = %#v, want created-run identity", updated.ExecutionContext["compressedContext"])
	}
}

func TestCompressedContextPatchForRunUsesRecoverySnapshotFallback(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-compressed-patch-runports")
	workspaceID := common.WorkspaceID("ws-compressed-patch-runports")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	readers, err := readmodelports.NewOwnerReaders(exports.Infrastructure.EventHub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}
	snapshotCalls := 0
	patch, loaded, err := CompressedContextPatchForRun(ctx, CompressedContextDependencies{
		Runs:          readers.Runs,
		MemoryRecalls: readers.MemoryRecalls,
		EventHub:      exports.Infrastructure.EventHub,
		RecoverySnapshot: func(context.Context, common.RunID) (readmodelcore.RunSnapshot, error) {
			snapshotCalls++
			return readmodelcore.RunSnapshot{
				Workspace: readmodelcore.Workspace{ID: string(workspaceID), State: "running"},
				Run:       readmodelcore.Run{ID: string(runID), WorkspaceID: string(workspaceID), State: "running"},
				TaskProgress: readmodelcore.TaskProgress{
					WorkspaceID: string(workspaceID),
					RunID:       string(runID),
					Summary:     "fallback recovery snapshot",
				},
			}, nil
		},
		DefaultLanguage: "en-US",
	}, runID)
	if err != nil {
		t.Fatalf("CompressedContextPatchForRun: %v", err)
	}
	if loaded == nil || loaded.ID != runID {
		t.Fatalf("loaded = %+v, want %s", loaded, runID)
	}
	if snapshotCalls != 1 {
		t.Fatalf("snapshotCalls = %d, want 1", snapshotCalls)
	}
	compressed, ok := patch["compressedContext"].(map[string]any)
	if !ok || compressed["runId"] != string(runID) || compressed["summary"] != "fallback recovery snapshot" {
		t.Fatalf("compressed patch = %#v", patch["compressedContext"])
	}
}

func TestBlockedRunResumeCompressedContextUsesResolvedDecision(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:               "run-blocked-decision-runports",
		WorkspaceID:      "ws-blocked-decision-runports",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-blocked-decision-runports",
	}
	compressed, err := BlockedRunResumeCompressedContext(
		context.Background(),
		fakeDecisionTicketReader{items: map[common.TicketID]*decisionmodel.Ticket{
			run.BlockingTicketID: {ID: run.BlockingTicketID, State: common.DecisionTicketStateResolved},
		}},
		fakeDecisionResolutionReader{items: map[common.TicketID]*decisionmodel.Resolution{
			run.BlockingTicketID: {TicketID: run.BlockingTicketID, Type: "provide_missing_info"},
		}},
		nil,
		run,
		"en-US",
	)
	if err != nil {
		t.Fatalf("BlockedRunResumeCompressedContext: %v", err)
	}
	if compressed == nil || compressed["runId"] != string(run.ID) {
		t.Fatalf("compressed = %+v, want run identity", compressed)
	}
	recovery, _ := compressed["recovery"].(map[string]any)
	if recovery["blockingTicketId"] != string(run.BlockingTicketID) || recovery["nextAction"] != "continue_run" {
		t.Fatalf("recovery = %+v, want resolved decision continuation", recovery)
	}
}

func TestBlockedRunResumeCompressedContextSkipsUnsupportedDecisionResolution(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:               "run-blocked-decision-skip",
		WorkspaceID:      "ws-blocked-decision-skip",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-blocked-decision-skip",
	}
	compressed, err := BlockedRunResumeCompressedContext(
		context.Background(),
		fakeDecisionTicketReader{items: map[common.TicketID]*decisionmodel.Ticket{
			run.BlockingTicketID: {ID: run.BlockingTicketID, State: common.DecisionTicketStateResolved},
		}},
		fakeDecisionResolutionReader{items: map[common.TicketID]*decisionmodel.Resolution{
			run.BlockingTicketID: {TicketID: run.BlockingTicketID, Type: "replan"},
		}},
		nil,
		run,
		"en-US",
	)
	if err != nil {
		t.Fatalf("BlockedRunResumeCompressedContext: %v", err)
	}
	if compressed != nil {
		t.Fatalf("compressed = %+v, want nil for unsupported resolution", compressed)
	}
}

func TestBlockedRunResumeCompressedContextUsesApprovedApproval(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:                 "run-blocked-approval-runports",
		WorkspaceID:        "ws-blocked-approval-runports",
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-blocked-runports",
	}
	compressed, err := BlockedRunResumeCompressedContext(
		context.Background(),
		nil,
		nil,
		fakeApprovalReader{items: map[common.ApprovalID]*approvalmodel.Approval{
			run.BlockingApprovalID: {ID: run.BlockingApprovalID, State: common.ApprovalStateApproved},
		}},
		run,
		"en-US",
	)
	if err != nil {
		t.Fatalf("BlockedRunResumeCompressedContext: %v", err)
	}
	if compressed == nil || compressed["runId"] != string(run.ID) {
		t.Fatalf("compressed = %+v, want run identity", compressed)
	}
	recovery, _ := compressed["recovery"].(map[string]any)
	if recovery["blockingApprovalId"] != string(run.BlockingApprovalID) || recovery["nextAction"] != "continue_run" {
		t.Fatalf("recovery = %+v, want approved approval continuation", recovery)
	}
}

type fakeDecisionTicketReader struct {
	items map[common.TicketID]*decisionmodel.Ticket
}

func (f fakeDecisionTicketReader) Get(_ context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	if item := f.items[id]; item != nil {
		return item, nil
	}
	return nil, common.ErrNotFound
}

func (f fakeDecisionTicketReader) List(context.Context) ([]*decisionmodel.Ticket, error) {
	return nil, nil
}

func (f fakeDecisionTicketReader) ListByRunID(context.Context, common.RunID) ([]*decisionmodel.Ticket, error) {
	return nil, nil
}

func (f fakeDecisionTicketReader) ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return nil, nil
}

type fakeDecisionResolutionReader struct {
	items map[common.TicketID]*decisionmodel.Resolution
}

func (f fakeDecisionResolutionReader) GetByTicketID(_ context.Context, id common.TicketID) (*decisionmodel.Resolution, error) {
	if item := f.items[id]; item != nil {
		return item, nil
	}
	return nil, common.ErrNotFound
}

type fakeApprovalReader struct {
	items map[common.ApprovalID]*approvalmodel.Approval
}

func (f fakeApprovalReader) Get(_ context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	if item := f.items[id]; item != nil {
		return item, nil
	}
	return nil, common.ErrNotFound
}

func (f fakeApprovalReader) List(context.Context) ([]*approvalmodel.Approval, error) {
	return nil, nil
}

func (f fakeApprovalReader) ListByRunID(context.Context, common.RunID) ([]*approvalmodel.Approval, error) {
	return nil, nil
}

func (f fakeApprovalReader) ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	return nil, nil
}

func TestRecoverTerminalActiveWorkspacePointersClearsTerminalRun(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-terminal-active-runports")
	runID := common.RunID("run-terminal-active-runports")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "terminal active runports",
		DeliverableDirectory: "workspaces/ws-terminal-active-runports",
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateSuperseded,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := RecoverTerminalActiveWorkspacePointers(ctx, exports.Infrastructure.EventHub); err != nil {
		t.Fatalf("RecoverTerminalActiveWorkspacePointers: %v", err)
	}
	workspace, err := exports.Repositories.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.ActiveRunID != "" || workspace.State != common.WorkspaceStateFailed {
		t.Fatalf("workspace = %+v, want failed without active run", workspace)
	}
}

func TestFailRunAndRecoverActiveWorkspacePointerClearsActiveRun(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-fail-active-runports")
	runID := common.RunID("run-fail-active-runports")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "fail active runports",
		DeliverableDirectory: "workspaces/ws-fail-active-runports",
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	failed, err := FailRunAndRecoverActiveWorkspacePointer(ctx, exports.Infrastructure.EventHub, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}, "runports recovery failure")
	if err != nil {
		t.Fatalf("FailRunAndRecoverActiveWorkspacePointer: %v", err)
	}
	if failed.State != common.RunStateFailed || failed.FailureReason != "runports recovery failure" {
		t.Fatalf("failed run = %+v, want failed with reason", failed)
	}
	workspace, err := exports.Repositories.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.ActiveRunID != "" || workspace.State != common.WorkspaceStateFailed {
		t.Fatalf("workspace = %+v, want failed without active run", workspace)
	}
}

func TestRecoverStuckRunningRunViewsFailsRunningRunWithoutContinuation(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-recover-run-view")
	runID := common.RunID("run-recover-run-view")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "recover run view",
		DeliverableDirectory: "workspaces/ws-recover-run-view",
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	got, err := RecoverStuckRunningRunViews(ctx, exports.Infrastructure.EventHub, func(context.Context, *runmodel.Run) error {
		return common.ErrConflict
	})
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunViews: %v", err)
	}
	if len(got) != 1 || got[0].ID != string(runID) || got[0].State != string(common.RunStateFailed) {
		t.Fatalf("recovered = %+v, want failed run view", got)
	}
	if got[0].FailureReason != StuckRunningRunFailureReason {
		t.Fatalf("failure reason = %q, want %q", got[0].FailureReason, StuckRunningRunFailureReason)
	}
}
