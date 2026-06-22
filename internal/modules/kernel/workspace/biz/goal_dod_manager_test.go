package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestGoalDoDManager_CreateWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), nil)

	cmd := workspaceservice.CreateWorkspaceCommand{
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
	manager := NewGoalDoDManager(memory.NewWorkspaceRepository(), memory.NewGoalRepository(), memory.NewDoDRepository(), nil)

	tests := []workspaceservice.CreateWorkspaceCommand{
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
	manager := NewGoalDoDManager(workspaceRepo, memory.NewGoalRepository(), memory.NewDoDRepository(), nil)
	workspaceID := common.WorkspaceID("ws-blank-definition")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: "/tmp/ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{
		GoalID:      "goal-blank",
		WorkspaceID: workspaceID,
		Text:        "   ",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("define blank goal err = %v, want invalid argument", err)
	}
	if err := manager.UpdateGoal(ctx, workspaceservice.UpdateGoalCommand{
		GoalID: "goal-blank",
		Text:   "   ",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update blank goal err = %v, want invalid argument", err)
	}
	if err := manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{
		DoDID:       "dod-blank",
		WorkspaceID: workspaceID,
		Items:       []string{"valid item", "   "},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("define blank dod item err = %v, want invalid argument", err)
	}
	if err := manager.UpdateDoD(ctx, workspaceservice.UpdateDoDCommand{
		DoDID: "dod-blank",
		Items: []string{"   "},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update blank dod item err = %v, want invalid argument", err)
	}
}

func TestGoalDoDManager_CreateWorkspace_RejectsDuplicateWorkspaceID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), nil)

	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-duplicate"),
		Title:                "workorch workspace",
		DeliverableDirectory: "/tmp/workorch/ws-duplicate",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("first create workspace: %v", err)
	}

	second := workspaceservice.CreateWorkspaceCommand{
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
	manager := NewGoalDoDManager(workspaceRepo, goalRepo, dodRepo, nil)
	workspaceID := common.WorkspaceID("ws-delete-cascade")
	if err := manager.CreateWorkspace(ctx, workspaceservice.CreateWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                "delete cascade",
		DeliverableDirectory: "/tmp/workorch/ws-delete-cascade",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-delete-cascade", Text: "goal"}); err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-delete-cascade", Items: []string{"dod"}}); err != nil {
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
	hub := events.NewMemoryHub()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-event"),
		Title:                "workorch workspace",
		Description:          "workspace event description",
		DeliverableDirectory: "/tmp/workorch/ws-event",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("topic = %s, want %s", published[0].Topic, workspaceevents.TopicWorkspaceCreated)
	}
	if published[0].Version != events.WorkorchEventVersion {
		t.Fatalf("version = %s, want %s", published[0].Version, events.WorkorchEventVersion)
	}
	if published[0].Payload[events.PayloadKeyTitle] != cmd.Title {
		t.Fatalf("title payload = %v, want %s", published[0].Payload[events.PayloadKeyTitle], cmd.Title)
	}
	if published[0].Payload[events.PayloadKeyDescription] != cmd.Description {
		t.Fatalf("description payload = %v, want %s", published[0].Payload[events.PayloadKeyDescription], cmd.Description)
	}
	if published[0].Payload[events.PayloadKeyDeliverableDirectory] != cmd.DeliverableDirectory {
		t.Fatalf("deliverableDirectory payload = %v, want %s", published[0].Payload[events.PayloadKeyDeliverableDirectory], cmd.DeliverableDirectory)
	}
	if published[0].Payload[events.PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state payload = %v, want %s", published[0].Payload[events.PayloadKeyState], common.WorkspaceStateActive)
	}
}

func TestGoalDoDManager_PublishesWorkspaceNotificationsThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub := &recordingWorkspaceAsyncHub{}
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	if err := manager.CreateWorkspace(ctx, workspaceservice.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-async"),
		Title:                "async workspace",
		DeliverableDirectory: "/tmp/workorch/ws-async",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if hub.syncPublishes != 0 || hub.asyncPublishes != 1 {
		t.Fatalf("publish counts = sync:%d async:%d, want async workspace notification", hub.syncPublishes, hub.asyncPublishes)
	}
	if len(hub.published) != 1 || hub.published[0].Topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("published = %+v, want workspace created notification", hub.published)
	}
}

func TestGoalDoDManager_PublishesDefinitionNotificationsThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub := &recordingWorkspaceAsyncHub{}
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	workspaceID := common.WorkspaceID("ws-definition-async")
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "definition workspace",
		DeliverableDirectory: "/tmp/workorch/ws-definition-async",
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{
		GoalID:      common.GoalID("goal-async"),
		WorkspaceID: workspaceID,
		Text:        "goal",
	}); err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{
		DoDID:       common.DoDID("dod-async"),
		WorkspaceID: workspaceID,
		Items:       []string{"dod"},
	}); err != nil {
		t.Fatalf("define dod: %v", err)
	}

	if hub.syncPublishes != 0 || hub.asyncPublishes != 2 {
		t.Fatalf("publish counts = sync:%d async:%d, want async definition notifications", hub.syncPublishes, hub.asyncPublishes)
	}
	if len(hub.published) != 2 ||
		hub.published[0].Topic != workspaceevents.TopicGoalDefined ||
		hub.published[1].Topic != workspaceevents.TopicDoDDefined {
		t.Fatalf("published = %+v, want goal and DoD notifications", hub.published)
	}
}

func TestGoalDoDManager_EnsureWorkspaceCreatesMissingWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), nil)

	cmd := workspaceservice.EnsureWorkspaceCommand{
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
	hub := events.NewMemoryHub()
	manager := NewGoalDoDManager(workspaceRepo, goalRepo, dodRepo, hub)
	workspaceID := common.WorkspaceID("ws-definition-lineage")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: "/tmp/ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-1", Text: "first goal"}); err != nil {
		t.Fatalf("define first goal: %v", err)
	}
	if err := manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-2", Text: "second goal"}); err != nil {
		t.Fatalf("define second goal: %v", err)
	}
	secondGoal, err := goalRepo.Get(ctx, "goal-2")
	if err != nil {
		t.Fatalf("get second goal: %v", err)
	}
	if secondGoal.DefinitionRevision == "" || secondGoal.PreviousDefinitionHash == "" || secondGoal.PreviousGoalID != "goal-1" || secondGoal.LineageState != "active" {
		t.Fatalf("second goal lineage = %+v, want revision, previous hash/id and active lineage", secondGoal)
	}

	if err := manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-1", Items: []string{"first dod"}}); err != nil {
		t.Fatalf("define first dod: %v", err)
	}
	if err := manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-2", Items: []string{"second dod"}}); err != nil {
		t.Fatalf("define second dod: %v", err)
	}
	secondDoD, err := dodRepo.Get(ctx, "dod-2")
	if err != nil {
		t.Fatalf("get second dod: %v", err)
	}
	if secondDoD.DefinitionRevision == "" || secondDoD.PreviousDefinitionHash == "" || secondDoD.PreviousDoDID != "dod-1" || secondDoD.LineageState != "active" {
		t.Fatalf("second dod lineage = %+v, want revision, previous hash/id and active lineage", secondDoD)
	}
	published := hub.Published()
	if len(published) == 0 {
		t.Fatalf("published events empty, want definition events")
	}
	last := published[len(published)-1]
	if last.Payload[events.PayloadKeyDefinitionRevision] == "" || last.Payload[events.PayloadKeyPreviousDoDID] != "dod-1" {
		t.Fatalf("last definition event payload = %+v, want dod lineage fields", last.Payload)
	}
}

func TestGoalDoDManager_EnsureWorkspaceResetsWorkspaceAndPublishesStateChanged(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)

	workspaceID := common.WorkspaceID("ws-ensure-reset")
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "stale title",
		Description:          "stale description",
		DeliverableDirectory: "/tmp/workorch/stale",
		State:                common.WorkspaceStateArchived,
		ActiveRunID:          common.RunID("run-stale"),
	}); err != nil {
		t.Fatalf("create stale workspace: %v", err)
	}

	if err := manager.EnsureWorkspace(ctx, workspaceservice.EnsureWorkspaceCommand{
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

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic, workspaceevents.TopicWorkspaceStateChanged)
	}
	if published[0].Payload[events.PayloadKeyTitle] != "workorch reset workspace" {
		t.Fatalf("title payload = %v, want workorch reset workspace", published[0].Payload[events.PayloadKeyTitle])
	}
	if published[0].Payload[events.PayloadKeyDescription] != "reset description" {
		t.Fatalf("description payload = %v, want reset description", published[0].Payload[events.PayloadKeyDescription])
	}
	if published[0].Payload[events.PayloadKeyDeliverableDirectory] != "/tmp/workorch/ws-ensure-reset" {
		t.Fatalf("deliverableDirectory payload = %v, want /tmp/workorch/ws-ensure-reset", published[0].Payload[events.PayloadKeyDeliverableDirectory])
	}
	if published[0].Payload[events.PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state payload = %v, want %s", published[0].Payload[events.PayloadKeyState], common.WorkspaceStateActive)
	}
	if published[0].Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("activeRunId payload = %v, want empty", published[0].Payload[events.PayloadKeyActiveRunID])
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
				return manager.DefineGoal(ctx, workspaceservice.DefineGoalCommand{
					GoalID:      "goal-reopen",
					WorkspaceID: workspaceID,
					Text:        "new goal requirement",
				})
			},
		},
		{
			name: "define dod",
			act: func(manager *GoalDoDManager, workspaceID common.WorkspaceID) error {
				return manager.DefineDoD(ctx, workspaceservice.DefineDoDCommand{
					DoDID:       "dod-reopen",
					WorkspaceID: workspaceID,
					Items:       []string{"new acceptance requirement"},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewWorkspaceRepository()
			hub := events.NewMemoryHub()
			manager := NewGoalDoDManager(repo, memory.NewGoalRepository(), memory.NewDoDRepository(), hub)
			workspaceID := common.WorkspaceID("ws-" + strings.ReplaceAll(tc.name, " ", "-"))
			if err := repo.Create(ctx, &workspacemodel.Workspace{
				ID:          workspaceID,
				Title:       "terminal workspace",
				State:       common.WorkspaceStateCompleted,
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
			published := hub.Published()
			if len(published) < 2 {
				t.Fatalf("published = %+v, want state change and definition event", published)
			}
			if published[0].Topic != workspaceevents.TopicWorkspaceStateChanged {
				t.Fatalf("first topic = %s, want workspace state changed", published[0].Topic)
			}
			if published[0].Payload[events.PayloadKeyState] != string(common.WorkspaceStateActive) ||
				published[0].Payload[events.PayloadKeyActiveRunID] != "" {
				t.Fatalf("state payload = %+v, want active without activeRunId", published[0].Payload)
			}
		})
	}
}

type recordingWorkspaceAsyncHub struct {
	syncPublishes  int
	asyncPublishes int
	published      []events.Envelope
}

func (h *recordingWorkspaceAsyncHub) Publish(_ context.Context, envelope events.Envelope) error {
	h.syncPublishes++
	h.published = append(h.published, envelope)
	return nil
}

func (h *recordingWorkspaceAsyncHub) PublishAsync(_ context.Context, envelope events.Envelope) error {
	h.asyncPublishes++
	h.published = append(h.published, envelope)
	return nil
}

func (h *recordingWorkspaceAsyncHub) Subscribe(string, events.Handler) func() {
	return func() {}
}
