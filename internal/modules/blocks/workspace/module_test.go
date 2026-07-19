package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/workspace" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.bizPtr != nil {
		t.Fatal("new module biz pointer is non-nil before Configure")
	}

	hub := magicevent.NewHub(16)
	if err := module.Configure(Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
		Templates:  memory.NewWorkspaceTemplateRepository(),
		EventHub:   hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil {
		t.Fatal("biz pointer is nil after Configure")
	}
	if module.bizPtr.EventHub() == nil {
		t.Fatal("biz event hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestWorkspaceChangedNotificationClonesDefaultLLMTarget(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-notify"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	const observerID = "/workspace/consumers/blocks/workspace/model-target-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	received := make(chan workspaceevents.WorkspaceChangedData, 1)
	observer.Subscribe(workspaceevents.TopicWorkspaceStateChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.WorkspaceChangedData); ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceStateChanged) })
	target := runtimecontract.ProviderModelTarget{ProviderID: "custom", ModelID: "model-a"}
	sendWorkspaceTestCommand(t, context.Background(), hub, workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: "ws-notify", Target: &target})
	target.ModelID = "mutated"
	select {
	case data := <-received:
		if data.DefaultLLMTarget == nil || data.DefaultLLMTarget.ModelID != "model-a" {
			t.Fatalf("notification target = %+v", data.DefaultLLMTarget)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workspace notification")
	}
}

func TestUpdateWorkspaceLLMTargetCommandPersistsCompleteTargetAndClears(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-model-target")
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "workspace", State: common.WorkspaceStateActive, ActiveRunID: "run-current"}); err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(Dependencies{Workspaces: repo, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "custom-provider", ModelID: "model-a"}
	result := sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: &target})
	if result.Workspace == nil || result.Workspace.DefaultLLMTarget == nil || !result.Workspace.DefaultLLMTarget.Same(target) {
		t.Fatalf("updated target = %+v", result.Workspace)
	}
	if result.Workspace.ActiveRunID != "run-current" {
		t.Fatalf("active run changed = %q", result.Workspace.ActiveRunID)
	}
	sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: result.Workspace.DefaultLLMTarget})
	target.ModelID = "mutated"
	stored, err := repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("Get workspace: %v", err)
	}
	if stored.DefaultLLMTarget == nil || stored.DefaultLLMTarget.ModelID != "model-a" {
		t.Fatalf("stored target = %+v", stored.DefaultLLMTarget)
	}
	sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID})
	stored, err = repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("Get cleared workspace: %v", err)
	}
	if stored.DefaultLLMTarget != nil {
		t.Fatalf("cleared target = %+v, want nil", stored.DefaultLLMTarget)
	}
}

func TestWorkspaceLLMTargetSurvivesFileBackedOwnerRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspaces.json")
	workspaceID := common.WorkspaceID("ws-model-restart")
	firstRepo := filerepository.NewWorkspaceRepository(path)
	if err := firstRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "workspace", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	firstHub := magicevent.NewHub(16)
	t.Cleanup(func() { firstHub.Terminate(context.Background()) })
	first := New()
	if err := first.Configure(Dependencies{Workspaces: firstRepo, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: firstHub}); err != nil {
		t.Fatalf("configure first owner: %v", err)
	}
	if err := first.Start(); err != nil {
		t.Fatalf("start first owner: %v", err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-after-restart"}
	sendWorkspaceTestCommand(t, ctx, firstHub, workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: &target})

	secondHub := magicevent.NewHub(16)
	t.Cleanup(func() { secondHub.Terminate(context.Background()) })
	second := New()
	if err := second.Configure(Dependencies{Workspaces: filerepository.NewWorkspaceRepository(path), Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: secondHub}); err != nil {
		t.Fatalf("configure restarted owner: %v", err)
	}
	if err := second.Start(); err != nil {
		t.Fatalf("start restarted owner: %v", err)
	}
	result := sendWorkspaceTestCommand(t, ctx, secondHub, workspaceevents.TopicWorkspaceCommandGet, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	if result.Workspace == nil || result.Workspace.DefaultLLMTarget == nil || !result.Workspace.DefaultLLMTarget.Same(target) {
		t.Fatalf("target after restart = %+v", result.Workspace)
	}
}

func sendWorkspaceTestCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) workspaceevents.WorkspaceCommandResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(topic, "workspace/module-test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(workspaceevents.WorkspaceCommandResult)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}

func TestModuleConfigureRequiresAllRepositories(t *testing.T) {
	valid := Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
		Templates:  memory.NewWorkspaceTemplateRepository(),
		EventHub:   magicevent.NewHub(16),
	}
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing workspaces", deps: Dependencies{Goals: valid.Goals, DoDs: valid.DoDs, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing goals", deps: Dependencies{Workspaces: valid.Workspaces, DoDs: valid.DoDs, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing dods", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing templates", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, DoDs: valid.DoDs, EventHub: valid.EventHub}},
		{name: "missing event hub", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, DoDs: valid.DoDs, Templates: valid.Templates}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}
