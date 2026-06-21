package runports

import (
	"context"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestListRunsByWorkspaceFiltersOwnerReadResults(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	for _, item := range []*runmodel.Run{
		{ID: "run-list-a", WorkspaceID: "ws-list-a", State: common.RunStateCreated},
		{ID: "run-list-b", WorkspaceID: "ws-list-b", State: common.RunStateCreated},
	} {
		if err := exports.Repositories.Runs.Create(ctx, item); err != nil {
			t.Fatalf("seed run %s: %v", item.ID, err)
		}
	}

	all, err := ListRunsByWorkspace(ctx, exports.Infrastructure.EventHub, "")
	if err != nil {
		t.Fatalf("ListRunsByWorkspace all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %+v, want two runs", all)
	}
	filtered, err := ListRunsByWorkspace(ctx, exports.Infrastructure.EventHub, "ws-list-b")
	if err != nil {
		t.Fatalf("ListRunsByWorkspace filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-list-b" {
		t.Fatalf("filtered = %+v, want run-list-b", filtered)
	}
}

func TestListRunsByWorkspaceUsesWorkspaceScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-list-command-scope")
	hub.Subscribe(runevents.TopicRunCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyRuns: []*runmodel.Run{{ID: "run-list-command-scope", WorkspaceID: workspaceID}},
			},
		})
	})

	items, err := ListRunsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		t.Fatalf("ListRunsByWorkspace err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "run-list-command-scope" {
		t.Fatalf("items = %+v, want scoped owner command result", items)
	}
}

func TestResumeRunPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	run := &runmodel.Run{ID: "run-resume-command", WorkspaceID: "ws-resume-command"}
	patch := map[string]any{"mode": "resume"}

	if err := ResumeRun(ctx, hub, run, patch); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(runcontracts.ResumeRunCommand)
	if !ok {
		t.Fatalf("payload command = %T, want ResumeRunCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.RunID != run.ID || cmd.WorkspaceID != run.WorkspaceID || cmd.ExecutionContextPatch["mode"] != "resume" {
		t.Fatalf("command = %+v, want populated resume command", cmd)
	}
	if _, ok := published[0].Payload[events.PayloadKeyRunID]; ok {
		t.Fatalf("payload contains legacy runId field")
	}
	if _, ok := published[0].Payload[events.PayloadKeyExecutionContextPatch]; ok {
		t.Fatalf("payload contains legacy executionContextPatch field")
	}
}

func TestRecoverRuntimeGapPublishesGapSpecificCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	run := &runmodel.Run{ID: "run-runtime-gap-command", WorkspaceID: "ws-runtime-gap-command"}
	recovery := RuntimeGapRecovery{
		WorkspaceID:    run.WorkspaceID,
		RunID:          run.ID,
		StepID:         "run-runtime-gap-command/step-001",
		StepKind:       "llm/workspace_execution",
		Gap:            runtimeExecutionGapWorkerLost,
		RequestID:      "request-runtime-gap",
		QueueKey:       "queue-runtime-gap",
		LatestStepTime: "2026-06-17T14:00:00Z",
		RecoveryPolicy: runtimemodel.RuntimeRecoveryPolicy{
			SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
			IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
			WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
		},
		RecoverySafety: "recoverable_idempotent",
		Reason:         "stale dispatched runtime gap: worker_lost",
	}

	if err := RecoverRuntimeGap(ctx, hub, run, recovery, map[string]any{"mode": "recover"}); err != nil {
		t.Fatalf("RecoverRuntimeGap: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	if published[0].Topic != orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap {
		t.Fatalf("topic = %q, want recover runtime gap", published[0].Topic)
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(runcontracts.RecoverRuntimeGapCommand)
	if !ok {
		t.Fatalf("payload command = %T, want RecoverRuntimeGapCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.RunID != run.ID ||
		cmd.WorkspaceID != run.WorkspaceID ||
		cmd.StepID != recovery.StepID ||
		cmd.Gap != string(runtimeExecutionGapWorkerLost) ||
		cmd.RequestID != recovery.RequestID ||
		cmd.QueueKey != recovery.QueueKey ||
		cmd.RecoveryPolicy.WatchdogRecovery != runtimemodel.WatchdogRecoveryRetryExternal ||
		cmd.RecoverySafety != recovery.RecoverySafety ||
		cmd.ExecutionContextPatch["mode"] != "recover" {
		t.Fatalf("command = %+v, want populated runtime gap recovery command", cmd)
	}
}

func TestValidateRunDefinitionsReadsDefinitionsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-definition-validation-scope")
	goalID := common.GoalID("goal-definition-validation")
	dodID := common.DoDID("dod-definition-validation")
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("workspace get envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("workspace get payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyWorkspace: &workspacemodel.Workspace{ID: workspaceID},
			},
		})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("goal get envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("goal get payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyGoal: &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "- scoped goal"},
			},
		})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("dod get envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("dod get payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyDoD: &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"scoped dod"}, State: common.DoDStateReady},
			},
		})
	})

	if err := ValidateRunDefinitions(ctx, hub, string(workspaceID), string(goalID), string(dodID)); err != nil {
		t.Fatalf("ValidateRunDefinitions err = %v", err)
	}
}
