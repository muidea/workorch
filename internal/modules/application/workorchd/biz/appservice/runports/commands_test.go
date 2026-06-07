package runports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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

func TestResumeRunPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	run := &runmodel.Run{ID: "run-resume-command", WorkspaceID: "ws-resume-command"}
	patch := map[string]any{"mode": "continue"}

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
	if cmd.RunID != run.ID || cmd.WorkspaceID != run.WorkspaceID || cmd.ExecutionContextPatch["mode"] != "continue" {
		t.Fatalf("command = %+v, want populated resume command", cmd)
	}
	if _, ok := published[0].Payload[events.PayloadKeyRunID]; ok {
		t.Fatalf("payload contains legacy runId field")
	}
	if _, ok := published[0].Payload[events.PayloadKeyExecutionContextPatch]; ok {
		t.Fatalf("payload contains legacy executionContextPatch field")
	}
}
