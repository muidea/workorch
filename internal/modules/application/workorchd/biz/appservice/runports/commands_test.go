package runports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestListRunsByWorkspaceFiltersOwnerReadResults(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	for _, item := range []*runmodel.Run{
		{ID: "run-list-a", WorkspaceID: "ws-list-a", State: common.RunStatePending},
		{ID: "run-list-b", WorkspaceID: "ws-list-b", State: common.RunStatePending},
	} {
		if err := exports.Repositories.Runs.Create(ctx, item); err != nil {
			t.Fatalf("seed run %s: %v", item.ID, err)
		}
	}

	all, err := ListRunsByWorkspace(ctx, exports.Infrastructure.CommandHub, "")
	if err != nil {
		t.Fatalf("ListRunsByWorkspace all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %+v, want two runs", all)
	}
	filtered, err := ListRunsByWorkspace(ctx, exports.Infrastructure.CommandHub, "ws-list-b")
	if err != nil {
		t.Fatalf("ListRunsByWorkspace filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-list-b" {
		t.Fatalf("filtered = %+v, want run-list-b", filtered)
	}
}

func TestListRunsByWorkspaceUsesWorkspaceScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	defer rawHub.Terminate(ctx)
	workspaceID := common.WorkspaceID("ws-list-command-scope")
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	observer.Subscribe(runevents.TopicRunCommandListByWorkspace, func(event magicevent.Event, result magicevent.Result) {
		cmd, ok := event.Data().(runevents.ListRunsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID != workspaceID || common.RuntimeWorkspaceIDFromContext(event.Context()) != workspaceID {
			t.Fatalf("command = %+v context workspace=%q", cmd, common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(runevents.RunCommandResult{Runs: []*runmodel.Run{{ID: "run-list-command-scope", WorkspaceID: workspaceID}}}, nil)
	})
	defer observer.Unsubscribe(runevents.TopicRunCommandListByWorkspace)

	items, err := ListRunsByWorkspace(ctx, rawHub, workspaceID)
	if err != nil {
		t.Fatalf("ListRunsByWorkspace err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "run-list-command-scope" {
		t.Fatalf("items = %+v, want scoped owner command result", items)
	}
}

func TestResumeRunPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	testHub := newRunportsMagicTestHub(t)
	hub := testHub.rawHub
	var captured orchestratorevents.ResumeRunCommand
	testHub.subscribeOrchestrator(orchestratorevents.TopicOrchestratorCommandResumeRun, func(data any) orchestratorevents.OrchestratorCommandResult {
		cmd, ok := data.(orchestratorevents.ResumeRunCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResumeRunCommand", data)
		}
		captured = cmd
		return orchestratorevents.OrchestratorCommandResult{}
	})
	run := &runmodel.Run{ID: "run-resume-command", WorkspaceID: "ws-resume-command"}

	if err := ResumeRun(ctx, hub, run); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}

	if captured.RunID != run.ID || captured.WorkspaceID != run.WorkspaceID {
		t.Fatalf("command = %+v, want populated resume command", captured)
	}
}

func TestValidateRunDefinitionsReadsDefinitionsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	defer runtime.Shutdown(ctx)
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-definition-validation-scope")
	goalID := common.GoalID("goal-definition-validation")
	dodID := common.DoDID("dod-definition-validation")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "- scoped goal"}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"scoped dod"}, State: common.DoDStateReady}); err != nil {
		t.Fatal(err)
	}

	if err := ValidateRunDefinitions(ctx, exports.Infrastructure.CommandHub, string(workspaceID), string(goalID), string(dodID)); err != nil {
		t.Fatalf("ValidateRunDefinitions err = %v", err)
	}
}

type runportsMagicTestHub struct {
	rawHub               magicevent.Hub
	orchestratorObserver magicevent.SimpleObserver
}

func newRunportsMagicTestHub(t *testing.T) *runportsMagicTestHub {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	ret := &runportsMagicTestHub{
		rawHub:               rawHub,
		orchestratorObserver: magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub),
	}
	t.Cleanup(func() {
		rawHub.Terminate(context.Background())
	})
	return ret
}

func (h *runportsMagicTestHub) subscribeOrchestrator(topic string, handler func(any) orchestratorevents.OrchestratorCommandResult) {
	h.orchestratorObserver.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(handler(ev.Data()), nil)
	})
}
