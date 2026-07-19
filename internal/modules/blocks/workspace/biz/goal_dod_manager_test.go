package workspace

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type recordedWorkspaceNotification struct {
	topic string
	data  any
}

type workspaceNotificationRecorder struct {
	mu    sync.Mutex
	items []recordedWorkspaceNotification
}

func newWorkspaceNotificationRecorder(t *testing.T) (magicevent.Hub, *workspaceNotificationRecorder) {
	t.Helper()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	recorder := &workspaceNotificationRecorder{}
	const observerID = "/workspace/consumers/blocks/workspace/biz-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	topics := []string{
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceStateChanged,
		workspaceevents.TopicWorkspaceDeleted,
		workspaceevents.TopicGoalDefined,
		workspaceevents.TopicDoDDefined,
	}
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			recorder.mu.Lock()
			recorder.items = append(recorder.items, recordedWorkspaceNotification{topic: topic, data: event.Data()})
			recorder.mu.Unlock()
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
	return hub, recorder
}

func newGoalDoDManagerWithHub(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository, hub magicevent.Hub) *GoalDoDManager {
	return NewGoalDoDManagerWithPublisher(workspaces, goals, dods, hub.Post)
}

func (r *workspaceNotificationRecorder) wait(t *testing.T, count int) []recordedWorkspaceNotification {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		items := append([]recordedWorkspaceNotification(nil), r.items...)
		r.mu.Unlock()
		if len(items) >= count {
			return items
		}
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("notification count = %d, want at least %d: %+v", len(r.items), count, r.items)
	return nil
}

func TestGoalDoDManager_CreateWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository())

	cmd := workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-1"),
		Title:                "workorch workspace",
		Description:          "workspace description",
		DeliverableDirectory: "/tmp/workorch/ws-1",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	ws, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.Title != cmd.Title {
		t.Fatalf("title = %q, want %q", ws.Title, cmd.Title)
	}
	if ws.Description != cmd.Description {
		t.Fatalf("description = %q, want %q", ws.Description, cmd.Description)
	}
	if ws.DeliverableDirectory != cmd.DeliverableDirectory {
		t.Fatalf("deliverableDirectory = %q, want %q", ws.DeliverableDirectory, cmd.DeliverableDirectory)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
}

func TestGoalDoDManager_CreateWorkspace_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	manager := NewGoalDoDManager(memory.NewWorkspaceRepository(), memory.NewGoalRepository(), memory.NewDoDRepository())

	tests := []workspacecontracts.CreateWorkspaceCommand{
		{WorkspaceID: "", Title: "ok", DeliverableDirectory: "/tmp/workorch/ws-1"},
		{WorkspaceID: common.WorkspaceID("ws-1"), Title: "", DeliverableDirectory: "/tmp/workorch/ws-1"},
		{WorkspaceID: common.WorkspaceID("ws-1"), Title: "   ", DeliverableDirectory: "/tmp/workorch/ws-1"},
		{WorkspaceID: common.WorkspaceID("ws-1"), Title: "ok", DeliverableDirectory: ""},
		{WorkspaceID: common.WorkspaceID("ws-1"), Title: "ok", DeliverableDirectory: "   "},
	}
	for _, tt := range tests {
		if err := manager.CreateWorkspace(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}
}

func TestGoalDoDManager_DefineGoalAndDoDRejectBlankText(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(workspaceRepo, memory.NewGoalRepository(), memory.NewDoDRepository())
	workspaceID := common.WorkspaceID("ws-blank-definition")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: "/tmp/ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{
		GoalID:      "goal-blank",
		WorkspaceID: workspaceID,
		Text:        "   ",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("define blank goal err = %v, want invalid argument", err)
	}
	if err := manager.UpdateGoal(ctx, workspacecontracts.UpdateGoalCommand{
		GoalID: "goal-blank",
		Text:   "   ",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update blank goal err = %v, want invalid argument", err)
	}
	if err := manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{
		DoDID:       "dod-blank",
		WorkspaceID: workspaceID,
		Items:       []string{"valid item", "   "},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("define blank dod item err = %v, want invalid argument", err)
	}
	if err := manager.UpdateDoD(ctx, workspacecontracts.UpdateDoDCommand{
		DoDID: "dod-blank",
		Items: []string{"   "},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update blank dod item err = %v, want invalid argument", err)
	}
}

func TestGoalDoDManager_CreateWorkspace_RejectsDuplicateWorkspaceID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository())

	cmd := workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-duplicate"),
		Title:                "workorch workspace",
		DeliverableDirectory: "/tmp/workorch/ws-duplicate",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("first create workspace: %v", err)
	}

	second := workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          cmd.WorkspaceID,
		Title:                "workorch workspace updated",
		DeliverableDirectory: "/tmp/workorch/ws-duplicate-updated",
	}
	if err := manager.CreateWorkspace(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace after duplicate create: %v", err)
	}
	if got.Title != cmd.Title {
		t.Fatalf("workspace title = %q, want original %q", got.Title, cmd.Title)
	}
}

func TestGoalDoDManager_DeleteWorkspacePurgesGoalAndDoD(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	manager := NewGoalDoDManager(workspaceRepo, goalRepo, dodRepo)
	workspaceID := common.WorkspaceID("ws-delete-cascade")
	if err := manager.CreateWorkspace(ctx, workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                "delete cascade",
		DeliverableDirectory: "/tmp/workorch/ws-delete-cascade",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-delete-cascade", Text: "goal"}); err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-delete-cascade", Items: []string{"dod"}}); err != nil {
		t.Fatalf("define dod: %v", err)
	}

	if err := manager.DeleteWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if _, err := workspaceRepo.Get(ctx, workspaceID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("workspace get err = %v, want not found", err)
	}
	if _, err := goalRepo.Get(ctx, "goal-delete-cascade"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("goal get err = %v, want not found", err)
	}
	if _, err := dodRepo.Get(ctx, "dod-delete-cascade"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("dod get err = %v, want not found", err)
	}
}

func TestGoalDoDManager_CreateWorkspace_PublishesWorkspaceCreatedEvent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub, recorder := newWorkspaceNotificationRecorder(t)
	manager := newGoalDoDManagerWithHub(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	cmd := workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-event"),
		Title:                "workorch workspace",
		Description:          "workspace event description",
		DeliverableDirectory: "/tmp/workorch/ws-event",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	published := recorder.wait(t, 1)
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("topic = %s, want %s", published[0].topic, workspaceevents.TopicWorkspaceCreated)
	}
	data, ok := published[0].data.(workspaceevents.WorkspaceChangedData)
	if !ok || data.Title != cmd.Title || data.Description != cmd.Description || data.DeliverableDirectory != cmd.DeliverableDirectory || data.State != common.WorkspaceStateActive {
		t.Fatalf("workspace notification = %+v", published[0])
	}
}

func TestGoalDoDManager_PublishesWorkspaceNotificationsThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub, recorder := newWorkspaceNotificationRecorder(t)
	manager := newGoalDoDManagerWithHub(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	if err := manager.CreateWorkspace(ctx, workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-async"),
		Title:                "async workspace",
		DeliverableDirectory: "/tmp/workorch/ws-async",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	published := recorder.wait(t, 1)
	if published[0].topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("published = %+v, want workspace created notification", published)
	}
}

func TestGoalDoDManager_PublishesDefinitionNotificationsThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub, recorder := newWorkspaceNotificationRecorder(t)
	manager := newGoalDoDManagerWithHub(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	workspaceID := common.WorkspaceID("ws-definition-async")
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "definition workspace",
		DeliverableDirectory: "/tmp/workorch/ws-definition-async",
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{
		GoalID:      common.GoalID("goal-async"),
		WorkspaceID: workspaceID,
		Text:        "goal",
	}); err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{
		DoDID:       common.DoDID("dod-async"),
		WorkspaceID: workspaceID,
		Items:       []string{"dod"},
	}); err != nil {
		t.Fatalf("define dod: %v", err)
	}

	published := recorder.wait(t, 2)
	var sawGoal, sawDoD bool
	for _, item := range published {
		sawGoal = sawGoal || item.topic == workspaceevents.TopicGoalDefined
		sawDoD = sawDoD || item.topic == workspaceevents.TopicDoDDefined
	}
	if !sawGoal || !sawDoD {
		t.Fatalf("published = %+v, want goal and DoD notifications", published)
	}
}

func TestGoalDoDManager_EnsureWorkspaceCreatesMissingWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository())

	cmd := workspacecontracts.EnsureWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-ensure-create"),
		Title:                "workorch ensure workspace",
		Description:          "ensure description",
		DeliverableDirectory: "/tmp/workorch/ws-ensure-create",
	}
	if err := manager.EnsureWorkspace(ctx, cmd); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	ws, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.Title != cmd.Title {
		t.Fatalf("title = %q, want %q", ws.Title, cmd.Title)
	}
	if ws.Description != cmd.Description {
		t.Fatalf("description = %q, want %q", ws.Description, cmd.Description)
	}
	if ws.DeliverableDirectory != cmd.DeliverableDirectory {
		t.Fatalf("deliverableDirectory = %q, want %q", ws.DeliverableDirectory, cmd.DeliverableDirectory)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", ws.ActiveRunID)
	}
}

func TestGoalDoDManager_DefinitionLineageFields(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	hub, recorder := newWorkspaceNotificationRecorder(t)
	manager := newGoalDoDManagerWithHub(workspaceRepo, goalRepo, dodRepo, hub)
	workspaceID := common.WorkspaceID("ws-definition-lineage")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: "/tmp/ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-1", Text: "first goal"}); err != nil {
		t.Fatalf("define first goal: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-2", Text: "second goal"}); err != nil {
		t.Fatalf("define second goal: %v", err)
	}
	secondGoal, err := goalRepo.Get(ctx, "goal-2")
	if err != nil {
		t.Fatalf("get second goal: %v", err)
	}
	if secondGoal.DefinitionRevision == "" || secondGoal.PreviousDefinitionHash == "" || secondGoal.PreviousGoalID != "goal-1" || secondGoal.LineageState != "active" {
		t.Fatalf("second goal lineage = %+v, want revision, previous hash/id and active lineage", secondGoal)
	}

	if err := manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-1", Items: []string{"first dod"}}); err != nil {
		t.Fatalf("define first dod: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-2", Items: []string{"second dod"}}); err != nil {
		t.Fatalf("define second dod: %v", err)
	}
	secondDoD, err := dodRepo.Get(ctx, "dod-2")
	if err != nil {
		t.Fatalf("get second dod: %v", err)
	}
	if secondDoD.DefinitionRevision == "" || secondDoD.PreviousDefinitionHash == "" || secondDoD.PreviousDoDID != "dod-1" || secondDoD.LineageState != "active" {
		t.Fatalf("second dod lineage = %+v, want revision, previous hash/id and active lineage", secondDoD)
	}
	published := recorder.wait(t, 4)
	if len(published) == 0 {
		t.Fatalf("published events empty, want definition events")
	}
	var lineageDoD workspaceevents.DoDDefinedData
	for _, item := range published {
		if data, ok := item.data.(workspaceevents.DoDDefinedData); ok && data.PreviousDoDID == "dod-1" {
			lineageDoD = data
		}
	}
	if lineageDoD.DefinitionRevision == "" {
		t.Fatalf("definition events = %+v, want dod lineage fields", published)
	}
}

func TestGoalDoDManager_EnsureWorkspaceResetsWorkspaceAndPublishesStateChanged(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub, recorder := newWorkspaceNotificationRecorder(t)
	manager := newGoalDoDManagerWithHub(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	workspaceID := common.WorkspaceID("ws-ensure-reset")
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "stale title",
		Description:          "stale description",
		DeliverableDirectory: "/tmp/workorch/stale",
		State:                common.WorkspaceStateSuperseded,
		ActiveRunID:          common.RunID("run-stale"),
	}); err != nil {
		t.Fatalf("create stale workspace: %v", err)
	}

	if err := manager.EnsureWorkspace(ctx, workspacecontracts.EnsureWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                "workorch reset workspace",
		Description:          "reset description",
		DeliverableDirectory: "/tmp/workorch/ws-ensure-reset",
	}); err != nil {
		t.Fatalf("ensure workspace reset: %v", err)
	}

	ws, err := repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get reset workspace: %v", err)
	}
	if ws.Title != "workorch reset workspace" {
		t.Fatalf("title = %q, want workorch reset workspace", ws.Title)
	}
	if ws.Description != "reset description" {
		t.Fatalf("description = %q, want reset description", ws.Description)
	}
	if ws.DeliverableDirectory != "/tmp/workorch/ws-ensure-reset" {
		t.Fatalf("deliverableDirectory = %q, want /tmp/workorch/ws-ensure-reset", ws.DeliverableDirectory)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", ws.ActiveRunID)
	}

	published := recorder.wait(t, 1)
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].topic, workspaceevents.TopicWorkspaceStateChanged)
	}
	data, ok := published[0].data.(workspaceevents.WorkspaceChangedData)
	if !ok || data.Title != "workorch reset workspace" || data.Description != "reset description" || data.DeliverableDirectory != "/tmp/workorch/ws-ensure-reset" || data.State != common.WorkspaceStateActive || data.ActiveRunID != "" {
		t.Fatalf("workspace notification = %+v", published[0])
	}
}

func TestGoalDoDManager_DefinitionChangeReopensTerminalWorkspace(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		act  func(*GoalDoDManager, common.WorkspaceID) error
	}{
		{
			name: "define goal",
			act: func(manager *GoalDoDManager, workspaceID common.WorkspaceID) error {
				return manager.DefineGoal(ctx, workspacecontracts.DefineGoalCommand{
					GoalID:      "goal-reopen",
					WorkspaceID: workspaceID,
					Text:        "new goal requirement",
				})
			},
		},
		{
			name: "define dod",
			act: func(manager *GoalDoDManager, workspaceID common.WorkspaceID) error {
				return manager.DefineDoD(ctx, workspacecontracts.DefineDoDCommand{
					DoDID:       "dod-reopen",
					WorkspaceID: workspaceID,
					Items:       []string{"new acceptance requirement"},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewWorkspaceRepository()
			hub, recorder := newWorkspaceNotificationRecorder(t)
			manager := newGoalDoDManagerWithHub(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)
			workspaceID := common.WorkspaceID("ws-" + strings.ReplaceAll(tc.name, " ", "-"))
			if err := repo.Create(ctx, &workspacemodel.Workspace{
				ID:          workspaceID,
				Title:       "terminal workspace",
				State:       common.WorkspaceStateActive,
				ActiveRunID: "run-terminal",
			}); err != nil {
				t.Fatalf("create workspace: %v", err)
			}

			if err := tc.act(manager, workspaceID); err != nil {
				t.Fatalf("definition change: %v", err)
			}
			ws, err := repo.Get(ctx, workspaceID)
			if err != nil {
				t.Fatalf("get workspace: %v", err)
			}
			if ws.State != common.WorkspaceStateActive || ws.ActiveRunID != "" {
				t.Fatalf("workspace = %+v, want active without active run after definition change", ws)
			}
			published := recorder.wait(t, 2)
			if len(published) < 2 {
				t.Fatalf("published = %+v, want state change and definition event", published)
			}
			var stateData workspaceevents.WorkspaceChangedData
			for _, item := range published {
				if item.topic == workspaceevents.TopicWorkspaceStateChanged {
					stateData, _ = item.data.(workspaceevents.WorkspaceChangedData)
				}
			}
			if stateData.WorkspaceID == "" || stateData.State != common.WorkspaceStateActive || stateData.ActiveRunID != "" {
				t.Fatalf("notifications = %+v, want active state without activeRunId", published)
			}
		})
	}
}
