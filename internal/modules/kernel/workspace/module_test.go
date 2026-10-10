package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceModuleTestBackend struct {
	binding    repository.OwnerRepositoryBinding
	workspaces repository.WorkspaceRepository
	cleaner    repository.WorkspaceDeletionMaterialCleaner
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	templates  repository.WorkspaceTemplateRepository
}

func (b workspaceModuleTestBackend) WorkspaceOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b workspaceModuleTestBackend) WorkspaceOwnerWorkspaces() repository.WorkspaceRepository {
	return b.workspaces
}
func (b workspaceModuleTestBackend) WorkspaceOwnerDeletionMaterialCleaner() repository.WorkspaceDeletionMaterialCleaner {
	return b.cleaner
}
func (b workspaceModuleTestBackend) WorkspaceOwnerGoals() repository.GoalRepository { return b.goals }
func (b workspaceModuleTestBackend) WorkspaceOwnerDoDs() repository.DoDRepository   { return b.dods }
func (b workspaceModuleTestBackend) WorkspaceOwnerTemplates() repository.WorkspaceTemplateRepository {
	return b.templates
}

type workspaceModuleTestWorkspaceRepository struct {
	repository.WorkspaceRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceModuleTestWorkspaceRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceModuleTestCleaner struct {
	repository.WorkspaceDeletionMaterialCleaner
	binding repository.OwnerRepositoryBinding
}

func (r workspaceModuleTestCleaner) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceModuleTestGoalRepository struct {
	repository.GoalRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceModuleTestGoalRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceModuleTestDoDRepository struct {
	repository.DoDRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceModuleTestDoDRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceModuleTestTemplateRepository struct {
	repository.WorkspaceTemplateRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceModuleTestTemplateRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

func newWorkspaceModuleTestRepositories(
	workspaces repository.WorkspaceRepository,
	cleaner repository.WorkspaceDeletionMaterialCleaner,
	goals repository.GoalRepository,
	dods repository.DoDRepository,
	templates repository.WorkspaceTemplateRepository,
) repository.WorkspaceOwnerRepositories {
	binding := repository.NewWorkspaceOwnerRepositoryBinding()
	owner, err := repository.NewWorkspaceOwnerRepositories(workspaceModuleTestBackend{
		binding:    binding,
		workspaces: workspaceModuleTestWorkspaceRepository{WorkspaceRepository: workspaces, binding: binding},
		cleaner:    workspaceModuleTestCleaner{WorkspaceDeletionMaterialCleaner: cleaner, binding: binding},
		goals:      workspaceModuleTestGoalRepository{GoalRepository: goals, binding: binding},
		dods:       workspaceModuleTestDoDRepository{DoDRepository: dods, binding: binding},
		templates:  workspaceModuleTestTemplateRepository{WorkspaceTemplateRepository: templates, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

func workspaceModuleTestDependencies(
	hub magicevent.Hub,
	workspaces repository.WorkspaceRepository,
	cleaner repository.WorkspaceDeletionMaterialCleaner,
	goals repository.GoalRepository,
	dods repository.DoDRepository,
	templates repository.WorkspaceTemplateRepository,
) workspacebiz.Dependencies {
	return workspacebiz.Dependencies{
		OwnerRepositories: newWorkspaceModuleTestRepositories(workspaces, cleaner, goals, dods, templates),
		EventHub:          hub,
	}
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/workspace" {
		t.Fatalf("ModuleID = %q，需保留 Workspace 稳定 owner ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.bizPtr != nil {
		t.Fatal("new module biz pointer is non-nil before Configure")
	}

	hub := magicevent.NewHub(16)
	workspaces := memory.NewWorkspaceRepository()
	if err := module.Configure(workspaceModuleTestDependencies(
		hub, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository(),
	)); err != nil {
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

func TestWorkspaceChangedNotificationClonesDefaultLLMSelection(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-notify"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	module := New()
	if err := module.Configure(workspaceModuleTestDependencies(hub, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
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
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}
	selection := runtimecontract.ProviderInferenceSelection{Target: target, ReasoningEffort: "high"}
	current, _ := workspaces.Get(context.Background(), "ws-notify")
	sendWorkspaceTestCommand(t, context.Background(), hub, workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: "ws-notify", Selection: &selection, ExpectedRevision: current.Revision})
	target.ModelID = "mutated"
	select {
	case data := <-received:
		if data.DefaultLLMSelection == nil || data.DefaultLLMSelection.Target.ModelID != "model-a" || data.DefaultLLMSelection.ReasoningEffort != "high" {
			t.Fatalf("notification target = %+v", data.DefaultLLMSelection)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workspace notification")
	}
}

func TestUpdateWorkspaceLLMSelectionCommandPersistsCompleteTargetAndClears(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-model-target")
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "workspace", State: common.WorkspaceStateActive, ActiveRunID: "run-current"}); err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(workspaceModuleTestDependencies(hub, repo, repo, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}
	selection := runtimecontract.ProviderInferenceSelection{Target: target, ReasoningEffort: "none"}
	current, _ := repo.Get(ctx, workspaceID)
	result := sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, Selection: &selection, ExpectedRevision: current.Revision})
	if !result.Changed {
		t.Fatal("first selection update was not reported as changed")
	}
	if result.Workspace == nil || result.Workspace.DefaultLLMSelection == nil || !result.Workspace.DefaultLLMSelection.Same(selection) {
		t.Fatalf("updated target = %+v", result.Workspace)
	}
	if result.Workspace.ActiveRunID != "run-current" {
		t.Fatalf("active run changed = %q", result.Workspace.ActiveRunID)
	}
	duplicate := sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, Selection: result.Workspace.DefaultLLMSelection, ExpectedRevision: result.Workspace.Revision})
	if duplicate.Changed || duplicate.Workspace == nil || duplicate.Workspace.Revision != result.Workspace.Revision {
		t.Fatalf("duplicate update = %+v, want unchanged revision %d", duplicate, result.Workspace.Revision)
	}
	staleResult := hub.Send(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection,
		"workspace/module-test",
		workspaceevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		workspaceevents.UpdateWorkspaceLLMSelectionCommand{
			WorkspaceID: workspaceID,
			Selection: &runtimecontract.ProviderInferenceSelection{
				Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "stale-model"},
			},
			ExpectedRevision: current.Revision,
		},
	))
	if staleResult == nil || !errors.Is(staleResult.Error(), common.ErrConflict) {
		t.Fatalf("stale update error = %v, want conflict", staleResult)
	}
	target.ModelID = "mutated"
	stored, err := repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("Get workspace: %v", err)
	}
	if stored.DefaultLLMSelection == nil || stored.DefaultLLMSelection.Target.ModelID != "model-a" || stored.DefaultLLMSelection.ReasoningEffort != "none" {
		t.Fatalf("stored target = %+v", stored.DefaultLLMSelection)
	}
	sendWorkspaceTestCommand(t, ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, ExpectedRevision: stored.Revision})
	stored, err = repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("Get cleared workspace: %v", err)
	}
	if stored.DefaultLLMSelection != nil {
		t.Fatalf("cleared target = %+v, want nil", stored.DefaultLLMSelection)
	}
}

func TestWorkspaceLLMSelectionSurvivesFileBackedOwnerRestart(t *testing.T) {
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
	if err := first.Configure(workspaceModuleTestDependencies(firstHub, firstRepo, firstRepo, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
		t.Fatalf("configure first owner: %v", err)
	}
	if err := first.Start(); err != nil {
		t.Fatalf("start first owner: %v", err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-after-restart"}
	selection := runtimecontract.ProviderInferenceSelection{Target: target, ReasoningEffort: "low"}
	current, _ := firstRepo.Get(ctx, workspaceID)
	sendWorkspaceTestCommand(t, ctx, firstHub, workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, Selection: &selection, ExpectedRevision: current.Revision})

	secondHub := magicevent.NewHub(16)
	t.Cleanup(func() { secondHub.Terminate(context.Background()) })
	second := New()
	secondRepo := filerepository.NewWorkspaceRepository(path)
	if err := second.Configure(workspaceModuleTestDependencies(secondHub, secondRepo, secondRepo, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
		t.Fatalf("configure restarted owner: %v", err)
	}
	if err := second.Start(); err != nil {
		t.Fatalf("start restarted owner: %v", err)
	}
	result := sendWorkspaceTestCommand(t, ctx, secondHub, workspaceevents.TopicWorkspaceCommandGet, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	if result.Workspace == nil || result.Workspace.DefaultLLMSelection == nil || !result.Workspace.DefaultLLMSelection.Same(selection) {
		t.Fatalf("target after restart = %+v", result.Workspace)
	}
}

type workspaceTestCommandResult struct {
	Workspace *workspaceevents.Workspace
	Changed   bool
}

func sendWorkspaceTestCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) workspaceTestCommandResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(topic, "workspace/module-test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	var typed workspaceTestCommandResult
	switch response := data.(type) {
	case workspaceevents.WorkspaceResult:
		typed.Workspace = response.Workspace
	case workspaceevents.WorkspaceLLMSelectionResult:
		typed.Workspace = response.Workspace
		typed.Changed = response.Changed
	default:
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}

func TestModuleConfigureRequiresBoundOwnerRepositoriesAndEventHub(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	ownerRepositories := memory.NewWorkspaceOwnerRepositorySet().OwnerRepositories()
	for _, tc := range []struct {
		name string
		deps workspacebiz.Dependencies
	}{
		{name: "missing owner repositories", deps: workspacebiz.Dependencies{EventHub: hub}},
		{name: "missing event hub", deps: workspacebiz.Dependencies{OwnerRepositories: ownerRepositories}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}
