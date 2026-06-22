package runports

import (
	"context"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func recoverableRuntimePolicyForTest() map[string]any {
	return runtimemodel.RuntimeRecoveryPolicy{
		SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
		IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
		WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
	}.ToMap()
}

func nonIdempotentRuntimePolicyForTest() map[string]any {
	return runtimemodel.RuntimeRecoveryPolicy{
		SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalWrite,
		IdempotencyPolicy: runtimemodel.IdempotencyPolicyNonIdempotent,
		WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
	}.ToMap()
}

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
		Data:           map[string]any{"nextAction": "observe"},
		OccurredAt:     "2026-06-04T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed progress checkpoint: %v", err)
	}
	run := &runmodel.Run{ID: runID}
	if HasContinuationCheckpointForRun(ctx, exports.Infrastructure.EventHub, run) {
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
	if !HasContinuationCheckpointForRun(ctx, exports.Infrastructure.EventHub, run) {
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
	if HasContinuationCheckpointForRun(ctx, exports.Infrastructure.EventHub, run) {
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
	if err := exports.Repositories.Runs.Create(ctx, candidate); err != nil {
		t.Fatalf("seed candidate run: %v", err)
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

	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, candidate.WorkspaceID)
	if err := exports.Repositories.RuntimeCheckpoints.Create(scopedCtx, &runmodel.RuntimeCheckpoint{
		ID:             "run-stuck-candidate/checkpoint-001",
		WorkspaceID:    candidate.WorkspaceID,
		RunID:          candidate.ID,
		CheckpointType: common.CheckpointTypeRecovery,
		Data:           map[string]any{"nextAction": "resume_run"},
		OccurredAt:     "2026-06-04T00:03:00Z",
	}); err != nil {
		t.Fatalf("seed continuation checkpoint: %v", err)
	}
	if IsStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, candidate) {
		t.Fatal("running run with continuation checkpoint should not be a stuck-running candidate")
	}
}

func TestRecoveryCandidateReadsRunScopedItemsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-recovery-candidate-scope")
	runID := common.RunID("run-recovery-candidate-scope")
	hub.Subscribe(runevents.TopicRunCommandListCheckpointsByRun, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("checkpoint list envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.RunID != string(runID) {
			t.Fatalf("checkpoint list envelope run = %q, want %s", envelope.RunID, runID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("checkpoint list payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		if envelope.Payload[events.PayloadKeyRunID] != string(runID) {
			t.Fatalf("checkpoint list payload run = %v, want %s", envelope.Payload[events.PayloadKeyRunID], runID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyCheckpoints: []*runmodel.RuntimeCheckpoint{},
			},
		})
	})
	stepListHandler := func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("step list envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("step list payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeySteps: []*runmodel.Step{{ID: "run-recovery-candidate-scope/root", RunID: runID, State: common.StepStateCompleted}},
			},
		})
	}
	hub.Subscribe(runevents.TopicRunCommandListStepsByRun, stepListHandler)
	hub.Subscribe(runevents.TopicRunCommandListStepSummariesByRun, stepListHandler)
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}

	if !IsStuckRunningRunCandidate(ctx, hub, run) {
		t.Fatal("running run without continuation checkpoint should be candidate")
	}
	terminal, err := TerminalStepForRunningRun(ctx, hub, run)
	if err != nil {
		t.Fatalf("TerminalStepForRunningRun: %v", err)
	}
	if terminal == nil || terminal.ID != "run-recovery-candidate-scope/root" {
		t.Fatalf("terminal = %+v, want scoped root step", terminal)
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

func TestRecoverStaleOrphanDispatchedStepSkipsRequestNotAcceptedGap(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-orphan-recover-runports")
	workspaceID := common.WorkspaceID("ws-stale-orphan-recover-runports")
	stepID := common.StepID("run-stale-orphan-recover-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-orphan-recover-runports/checkpoint/recent",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:09:00Z",
	}); err != nil {
		t.Fatalf("seed recent checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:10:00Z")
	resumeCalls := 0
	recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep recent: %v", err)
	}
	if changed || recovered != nil || resumeCalls != 0 {
		t.Fatalf("recent checkpoint recovered=%+v changed=%v resumeCalls=%d, want no recovery", recovered, changed, resumeCalls)
	}

	now, _ = time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	recovered, changed, err = RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep old: %v", err)
	}
	if changed || recovered != nil || resumeCalls != 0 {
		t.Fatalf("request-not-accepted recovered=%+v changed=%v resumeCalls=%d, want diagnostic-only no recovery", recovered, changed, resumeCalls)
	}
}

func TestRecoverStaleOrphanDispatchedStepResumesRuntimeTerminalResultAvailable(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-terminal-gap-runports")
	workspaceID := common.WorkspaceID("ws-stale-terminal-gap-runports")
	stepID := common.StepID("run-stale-terminal-gap-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-terminal-gap-runports/checkpoint/accepted",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:00:00Z",
		Data: map[string]any{
			"event":     "runtime_execute_accepted",
			"requestId": "request-terminal-gap",
		},
	}); err != nil {
		t.Fatalf("seed accepted checkpoint: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-terminal-gap-runports/checkpoint/result",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:01:00Z",
		Data: map[string]any{
			"event":       "runtime_step_result_available",
			"requestId":   "request-terminal-gap",
			"completedAt": "2026-06-09T08:00:59Z",
			"output":      map[string]any{"status": "ok"},
		},
	}); err != nil {
		t.Fatalf("seed result checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	resumeCalls := 0
	recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep terminal gap: %v", err)
	}
	if !changed || recovered == nil || recovered.ID != stepID || resumeCalls != 1 {
		t.Fatalf("terminal gap recovered=%+v changed=%v resumeCalls=%d, want finalize recovery resume", recovered, changed, resumeCalls)
	}
}

func TestRecoverStaleRuntimeGapRunViewsUsesGapSpecificRecovery(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-gap-specific-runports")
	workspaceID := common.WorkspaceID("ws-stale-gap-specific-runports")
	stepID := common.StepID("run-stale-gap-specific-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-gap-specific-runports/checkpoint/accepted",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:00:00Z",
		Data: map[string]any{
			"event":     "runtime_execute_accepted",
			"requestId": "request-gap-specific",
			"queueKey":  "queue-gap-specific",
		},
	}); err != nil {
		t.Fatalf("seed accepted checkpoint: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-gap-specific-runports/checkpoint/result",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:01:00Z",
		Data: map[string]any{
			"event":       "runtime_step_result_available",
			"requestId":   "request-gap-specific",
			"queueKey":    "queue-gap-specific",
			"completedAt": "2026-06-09T08:00:59Z",
			"output":      map[string]any{"status": "ok"},
		},
	}); err != nil {
		t.Fatalf("seed result checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	var got RuntimeGapRecovery
	items, err := RecoverStaleRuntimeGapRunViews(ctx, exports.Infrastructure.EventHub, func(_ context.Context, current *runmodel.Run, recovery RuntimeGapRecovery) error {
		if current.ID != runID || current.WorkspaceID != workspaceID {
			t.Fatalf("current run = %+v, want scoped run", current)
		}
		got = recovery
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleRuntimeGapRunViews: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want recovered run view", len(items))
	}
	if got.StepID != stepID ||
		got.Gap != runtimeExecutionGapTerminalNotFinalized ||
		got.RequestID != "request-gap-specific" ||
		got.QueueKey != "queue-gap-specific" ||
		got.LatestStepTime != "2026-06-09T08:01:00Z" {
		t.Fatalf("gap recovery = %+v, want terminal-not-finalized metadata", got)
	}
	if got.RecoverySafety != "terminal_result_evidence" {
		t.Fatalf("gap recovery safety = %q, want terminal_result_evidence", got.RecoverySafety)
	}
}

func TestRecoverStaleRuntimeGapRunViewsNoEligibleGapUsesBoundedReads(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-stale-gap-bounded-reads")
	runID := common.RunID("run-stale-gap-bounded-reads")
	stepID := common.StepID("run-stale-gap-bounded-reads/root")
	listRunsCalls := 0
	stepSummaryCalls := 0
	checkpointCalls := 0
	readRunCalls := 0
	recoverCalls := 0

	hub.Subscribe(runevents.TopicRunCommandList, func(_ context.Context, envelope events.Envelope) error {
		listRunsCalls++
		if compact, _ := envelope.Payload[events.PayloadKeyCompact].(bool); !compact {
			t.Fatalf("run list compact = %v, want true for watchdog candidate scan", envelope.Payload[events.PayloadKeyCompact])
		}
		states, _ := envelope.Payload[events.PayloadKeyStates].([]string)
		if len(states) != 1 || states[0] != string(common.RunStateRunning) {
			t.Fatalf("run list states = %+v, want running-only watchdog candidate scan", envelope.Payload[events.PayloadKeyStates])
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyRuns: []*runmodel.Run{{
					ID:          runID,
					WorkspaceID: workspaceID,
					State:       common.RunStateRunning,
				}},
			},
		})
	})
	hub.Subscribe(runevents.TopicRunCommandListStepSummariesByRun, func(_ context.Context, envelope events.Envelope) error {
		stepSummaryCalls++
		if envelope.WorkspaceID != string(workspaceID) || envelope.RunID != string(runID) {
			t.Fatalf("step summary envelope scope = workspace %q run %q, want %s/%s", envelope.WorkspaceID, envelope.RunID, workspaceID, runID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeySteps: []*runmodel.Step{{
					ID:    stepID,
					RunID: runID,
					State: common.StepStateCompleted,
				}},
			},
		})
	})
	hub.Subscribe(runevents.TopicRunCommandListCheckpointsByRun, func(_ context.Context, envelope events.Envelope) error {
		checkpointCalls++
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{events.PayloadKeyCheckpoints: []*runmodel.RuntimeCheckpoint{}}})
	})
	hub.Subscribe(runevents.TopicRunCommandGet, func(_ context.Context, envelope events.Envelope) error {
		readRunCalls++
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{events.PayloadKeyRun: &runmodel.Run{ID: runID, WorkspaceID: workspaceID}}})
	})

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	items, err := RecoverStaleRuntimeGapRunViews(ctx, hub, func(context.Context, *runmodel.Run, RuntimeGapRecovery) error {
		recoverCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleRuntimeGapRunViews: %v", err)
	}
	if len(items) != 0 || recoverCalls != 0 {
		t.Fatalf("items=%+v recoverCalls=%d, want no recovery for non-dispatched terminal step", items, recoverCalls)
	}
	if listRunsCalls != 1 || stepSummaryCalls != 1 || checkpointCalls != 0 || readRunCalls != 0 {
		t.Fatalf("read calls list=%d summaries=%d checkpoints=%d readRun=%d, want bounded no-gap reads 1/1/0/0", listRunsCalls, stepSummaryCalls, checkpointCalls, readRunCalls)
	}
}

func TestRecoverStaleOrphanDispatchedStepSkipsRuntimeDoneWithoutTerminalResult(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-done-gap-runports")
	workspaceID := common.WorkspaceID("ws-stale-done-gap-runports")
	stepID := common.StepID("run-stale-done-gap-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-done-gap-runports/checkpoint/done",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:01:00Z",
		Data: map[string]any{
			"event":     "runtime_step_done",
			"requestId": "request-done-gap",
		},
	}); err != nil {
		t.Fatalf("seed done checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	resumeCalls := 0
	recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep done gap: %v", err)
	}
	if changed || recovered != nil || resumeCalls != 0 {
		t.Fatalf("done gap recovered=%+v changed=%v resumeCalls=%d, want no recovery without durable result", recovered, changed, resumeCalls)
	}
}

func TestRecoverStaleOrphanDispatchedStepSkipsAcceptedWithoutWorkerLostEvidence(t *testing.T) {
	testCases := []struct {
		name       string
		event      string
		extraData  map[string]any
		checkpoint string
	}{
		{
			name:       "accepted-no-progress",
			event:      "runtime_execute_accepted",
			checkpoint: "run-stale-accepted-no-progress/checkpoint/accepted",
		},
		{
			name:       "accepted-in-progress",
			event:      "runtime_step_progress",
			checkpoint: "run-stale-accepted-in-progress/checkpoint/progress",
			extraData:  map[string]any{"stage": "streaming"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			runtime := bootstrap.NewWithBackend("memory", "")
			t.Cleanup(func() {
				if err := runtime.Shutdown(context.Background()); err != nil {
					t.Errorf("shutdown runtime: %v", err)
				}
			})
			exports := runtime.Exports()
			runID := common.RunID("run-stale-" + tc.name)
			workspaceID := common.WorkspaceID("ws-stale-" + tc.name)
			stepID := common.StepID(string(runID) + "/provider-retry-001")
			run := &runmodel.Run{
				ID:          runID,
				WorkspaceID: workspaceID,
				State:       common.RunStateRunning,
			}
			if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
				ID:    stepID,
				RunID: runID,
				Kind:  "llm/workspace_execution",
				State: common.StepStateDispatched,
			}); err != nil {
				t.Fatalf("seed dispatched step: %v", err)
			}
			data := map[string]any{
				"event":     tc.event,
				"requestId": "request-" + tc.name,
			}
			for key, value := range tc.extraData {
				data[key] = value
			}
			if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
				ID:            common.CheckpointID(tc.checkpoint),
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-09T08:00:00Z",
				Data:          data,
			}); err != nil {
				t.Fatalf("seed checkpoint: %v", err)
			}

			now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
			resumeCalls := 0
			recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
				resumeCalls++
				return nil
			}, 10*time.Minute, now)
			if err != nil {
				t.Fatalf("RecoverStaleOrphanDispatchedStep: %v", err)
			}
			if changed || recovered != nil || resumeCalls != 0 {
				t.Fatalf("accepted gap recovered=%+v changed=%v resumeCalls=%d, want no recovery without worker-lost evidence", recovered, changed, resumeCalls)
			}
		})
	}
}

func TestRecoverStaleOrphanDispatchedStepSkipsNonIdempotentRuntimeGap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		event     string
		extraData map[string]any
	}{
		{
			name:      "worker-lost",
			event:     "runtime_worker_lost",
			extraData: map[string]any{"workerId": "runtimeevent|request-non-idempotent", "lostAt": "2026-06-09T08:03:00Z"},
		},
		{
			name:      "accepted-in-progress",
			event:     "runtime_worker_heartbeat",
			extraData: map[string]any{"workerId": "runtimeevent|request-non-idempotent", "heartbeatAt": "2026-06-09T08:03:00Z"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			runtime := bootstrap.NewWithBackend("memory", "")
			t.Cleanup(func() {
				if err := runtime.Shutdown(context.Background()); err != nil {
					t.Errorf("shutdown runtime: %v", err)
				}
			})
			exports := runtime.Exports()
			runID := common.RunID("run-stale-non-idempotent-" + tc.name)
			workspaceID := common.WorkspaceID("ws-stale-non-idempotent-" + tc.name)
			stepID := common.StepID(string(runID) + "/provider-retry-001")
			run := &runmodel.Run{
				ID:          runID,
				WorkspaceID: workspaceID,
				State:       common.RunStateRunning,
			}
			if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
				ID:    stepID,
				RunID: runID,
				Kind:  "llm/workspace_execution",
				State: common.StepStateDispatched,
			}); err != nil {
				t.Fatalf("seed dispatched step: %v", err)
			}
			if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
				ID:            common.CheckpointID(string(runID) + "/checkpoint/accepted"),
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-09T08:00:00Z",
				Data: map[string]any{
					"event":                 "runtime_execute_accepted",
					"requestId":             "request-non-idempotent",
					"acceptedAt":            "2026-06-09T08:00:00Z",
					"runtimeRecoveryPolicy": nonIdempotentRuntimePolicyForTest(),
				},
			}); err != nil {
				t.Fatalf("seed accepted checkpoint: %v", err)
			}
			data := map[string]any{
				"event":                 tc.event,
				"requestId":             "request-non-idempotent",
				"runtimeRecoveryPolicy": nonIdempotentRuntimePolicyForTest(),
			}
			for key, value := range tc.extraData {
				data[key] = value
			}
			if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
				ID:            common.CheckpointID(string(runID) + "/checkpoint/gap"),
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-09T08:03:00Z",
				Data:          data,
			}); err != nil {
				t.Fatalf("seed gap checkpoint: %v", err)
			}

			now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
			resumeCalls := 0
			recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
				resumeCalls++
				return nil
			}, 10*time.Minute, now)
			if err != nil {
				t.Fatalf("RecoverStaleOrphanDispatchedStep: %v", err)
			}
			if changed || recovered != nil || resumeCalls != 0 {
				t.Fatalf("non-idempotent gap recovered=%+v changed=%v resumeCalls=%d, want no automatic recovery", recovered, changed, resumeCalls)
			}
		})
	}
}

func TestRecoverStaleOrphanDispatchedStepResumesWorkerLostGap(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-worker-lost-runports")
	workspaceID := common.WorkspaceID("ws-stale-worker-lost-runports")
	stepID := common.StepID("run-stale-worker-lost-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-worker-lost-runports/checkpoint/accepted",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:00:00Z",
		Data: map[string]any{
			"event":                 "runtime_execute_accepted",
			"requestId":             "request-worker-lost-gap",
			"acceptedAt":            "2026-06-09T08:00:00Z",
			"runtimeRecoveryPolicy": recoverableRuntimePolicyForTest(),
		},
	}); err != nil {
		t.Fatalf("seed accepted checkpoint: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-worker-lost-runports/checkpoint/worker-lost",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:03:00Z",
		Data: map[string]any{
			"event":                 "runtime_worker_lost",
			"requestId":             "request-worker-lost-gap",
			"workerId":              "runtimeevent|request-worker-lost-gap",
			"lostAt":                "2026-06-09T08:03:00Z",
			"runtimeRecoveryPolicy": recoverableRuntimePolicyForTest(),
		},
	}); err != nil {
		t.Fatalf("seed worker lost checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	resumeCalls := 0
	recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep worker lost: %v", err)
	}
	if !changed || recovered == nil || recovered.ID != stepID || resumeCalls != 1 {
		t.Fatalf("worker lost recovered=%+v changed=%v resumeCalls=%d, want resume recovery", recovered, changed, resumeCalls)
	}
}

func TestRecoverStaleOrphanDispatchedStepResumesStaleWorkerHeartbeatGap(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	runID := common.RunID("run-stale-worker-heartbeat-runports")
	workspaceID := common.WorkspaceID("ws-stale-worker-heartbeat-runports")
	stepID := common.StepID("run-stale-worker-heartbeat-runports/provider-retry-001")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("seed dispatched step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-worker-heartbeat-runports/checkpoint/accepted",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:00:00Z",
		Data: map[string]any{
			"event":                 "runtime_execute_accepted",
			"requestId":             "request-worker-heartbeat-gap",
			"acceptedAt":            "2026-06-09T08:00:00Z",
			"runtimeRecoveryPolicy": recoverableRuntimePolicyForTest(),
		},
	}); err != nil {
		t.Fatalf("seed accepted checkpoint: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:            "run-stale-worker-heartbeat-runports/checkpoint/heartbeat",
		WorkspaceID:   workspaceID,
		RunID:         runID,
		CurrentStepID: stepID,
		OccurredAt:    "2026-06-09T08:03:00Z",
		Data: map[string]any{
			"event":                 "runtime_worker_heartbeat",
			"requestId":             "request-worker-heartbeat-gap",
			"workerId":              "runtimeevent|request-worker-heartbeat-gap",
			"heartbeatAt":           "2026-06-09T08:03:00Z",
			"runtimeRecoveryPolicy": recoverableRuntimePolicyForTest(),
		},
	}); err != nil {
		t.Fatalf("seed heartbeat checkpoint: %v", err)
	}

	now, _ := time.Parse(time.RFC3339Nano, "2026-06-09T08:30:00Z")
	resumeCalls := 0
	recovered, changed, err := RecoverStaleOrphanDispatchedStep(ctx, exports.Infrastructure.EventHub, run, 4, func(context.Context, *runmodel.Run) error {
		resumeCalls++
		return nil
	}, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("RecoverStaleOrphanDispatchedStep heartbeat gap: %v", err)
	}
	if !changed || recovered == nil || recovered.ID != stepID || resumeCalls != 1 {
		t.Fatalf("heartbeat gap recovered=%+v changed=%v resumeCalls=%d, want resume recovery", recovered, changed, resumeCalls)
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
	if recovery["blockingTicketId"] != string(run.BlockingTicketID) || recovery["nextAction"] != "resume_run" {
		t.Fatalf("recovery = %+v, want resolved decision continuation", recovery)
	}
}

func TestBlockedRunResumeCompressedContextUsesResolvedReplanDecision(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:               "run-blocked-decision-replan",
		WorkspaceID:      "ws-blocked-decision-replan",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-blocked-decision-replan",
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
	if compressed == nil || compressed["runId"] != string(run.ID) {
		t.Fatalf("compressed = %+v, want run identity for resolved replan", compressed)
	}
	recovery, _ := compressed["recovery"].(map[string]any)
	if recovery["blockingTicketId"] != string(run.BlockingTicketID) || recovery["nextAction"] != "resume_run" {
		t.Fatalf("recovery = %+v, want resolved replan continuation", recovery)
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
	if recovery["blockingApprovalId"] != string(run.BlockingApprovalID) || recovery["nextAction"] != "resume_run" {
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

func TestRecoverStuckRunningRunViewsResumesRunningRunWithCompletedTerminalStep(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-recover-completed-terminal")
	runID := common.RunID("run-recover-completed-terminal")
	stepID := common.StepID("run-recover-completed-terminal/graph-001/001-done")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "recover completed terminal",
		DeliverableDirectory: "workspaces/ws-recover-completed-terminal",
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
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}

	resumeCalls := 0
	got, err := RecoverStuckRunningRunViews(ctx, exports.Infrastructure.EventHub, func(resumeCtx context.Context, current *runmodel.Run) error {
		resumeCalls++
		latest, err := exports.Repositories.Runs.Get(resumeCtx, current.ID)
		if err != nil {
			return err
		}
		latest.State = common.RunStateCompleted
		return exports.Repositories.Runs.Update(resumeCtx, latest)
	})
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunViews: %v", err)
	}
	if resumeCalls != 1 {
		t.Fatalf("resumeCalls = %d, want 1", resumeCalls)
	}
	if len(got) != 1 || got[0].ID != string(runID) || got[0].State != string(common.RunStateCompleted) {
		t.Fatalf("recovered = %+v, want completed run view", got)
	}
}
