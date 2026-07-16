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
	if module.servicePtr != nil {
		t.Fatal("new module service pointer is non-nil before Configure")
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
	if module.servicePtr == nil {
		t.Fatal("service pointer is nil after Configure")
	}
	if module.eventHub == nil {
		t.Fatal("event hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe workspace commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.servicePtr != nil ||
		module.bizPtr != nil ||
		module.eventHub != nil ||
		module.workspaces != nil ||
		module.goals != nil ||
		module.dods != nil ||
		module.templates != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestWorkspaceChangedNotificationClonesDefaultLLMTarget(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := New()
	if err := module.Configure(Dependencies{Workspaces: memory.NewWorkspaceRepository(), Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
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
	module.publishWorkspaceUpdated(context.Background(), &workspacemodel.Workspace{ID: "ws-notify", DefaultLLMTarget: &target})
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
	module := New()
	if err := module.Configure(Dependencies{Workspaces: repo, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: magicevent.NewHub(16)}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "custom-provider", ModelID: "model-a"}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: &target})
	result, err := module.executeCommand(event)
	if err != nil {
		t.Fatalf("update target: %v", err)
	}
	if result.Workspace == nil || result.Workspace.DefaultLLMTarget == nil || !result.Workspace.DefaultLLMTarget.Same(target) {
		t.Fatalf("updated target = %+v", result.Workspace)
	}
	if result.Workspace.ActiveRunID != "run-current" {
		t.Fatalf("active run changed = %q", result.Workspace.ActiveRunID)
	}
	if _, err := module.executeCommand(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: result.Workspace.DefaultLLMTarget})); err != nil {
		t.Fatalf("duplicate target update: %v", err)
	}
	target.ModelID = "mutated"
	stored, err := repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("Get workspace: %v", err)
	}
	if stored.DefaultLLMTarget == nil || stored.DefaultLLMTarget.ModelID != "model-a" {
		t.Fatalf("stored target = %+v", stored.DefaultLLMTarget)
	}
	clearEvent := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID})
	if _, err := module.executeCommand(clearEvent); err != nil {
		t.Fatalf("clear target: %v", err)
	}
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
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-after-restart"}
	update := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceLLMTargetCommand{WorkspaceID: workspaceID, Target: &target})
	if _, err := first.executeCommand(update); err != nil {
		t.Fatalf("persist target: %v", err)
	}

	secondHub := magicevent.NewHub(16)
	t.Cleanup(func() { secondHub.Terminate(context.Background()) })
	second := New()
	if err := second.Configure(Dependencies{Workspaces: filerepository.NewWorkspaceRepository(path), Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: secondHub}); err != nil {
		t.Fatalf("configure restarted owner: %v", err)
	}
	get := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	result, err := second.executeCommand(get)
	if err != nil {
		t.Fatalf("read target after restart: %v", err)
	}
	if result.Workspace == nil || result.Workspace.DefaultLLMTarget == nil || !result.Workspace.DefaultLLMTarget.Same(target) {
		t.Fatalf("target after restart = %+v", result.Workspace)
	}
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
