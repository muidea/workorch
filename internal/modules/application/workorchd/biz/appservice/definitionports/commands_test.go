package definitionports

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestConsolidateWorkspaceDefinitionRejectsMissingCanonicalBeforePublishing(t *testing.T) {
	hub := magicevent.NewHub(8)
	if _, err := ConsolidateWorkspaceGoal(context.Background(), hub, "ws-definition-port", nil, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("goal error = %v, want ErrInvalidArgument", err)
	}

	if _, err := ConsolidateWorkspaceDoD(context.Background(), hub, "ws-definition-port", nil, nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("dod error = %v, want ErrInvalidArgument", err)
	}
}

func TestDefineOrConsolidateWorkspaceGoalCreatesWhenMissing(t *testing.T) {
	ctx := context.Background()
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-port")
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandListGoalsByWS, func(data any) any {
		cmd, ok := data.(workspaceevents.ListGoalsByWorkspaceCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListGoalsByWorkspaceCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return workspaceevents.GoalListResult{Goals: []*workspacemodel.Goal{}}
	})
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandDefineGoal, func(data any) any {
		cmd, ok := data.(workspaceevents.DefineGoalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DefineGoalCommand", data)
		}
		goalID := cmd.GoalID
		if goalID == "" {
			t.Fatalf("missing generated goal id")
		}
		text := cmd.Text
		if text != "- ship the feature" {
			t.Fatalf("goal text = %q, want formatted bullet", text)
		}
		return workspaceevents.GoalResult{Goal: &workspacemodel.Goal{
			ID:          goalID,
			WorkspaceID: workspaceID,
			Text:        text,
		}}
	})

	item, err := DefineOrConsolidateWorkspaceGoal(ctx, hub, workspaceID, "ship the feature")
	if err != nil {
		t.Fatalf("DefineOrConsolidateWorkspaceGoal err = %v", err)
	}
	if item == nil || item.WorkspaceID != workspaceID || item.Text != "- ship the feature" {
		t.Fatalf("goal item = %#v", item)
	}
}

func TestUpdateGoalViewPublishesExplicitWorkspaceScope(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-definition-update-scope")
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-update-scope")
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandUpdateGoal, func(data any) any {
		cmd, ok := data.(workspaceevents.UpdateGoalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want UpdateGoalCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		if cmd.GoalID != "goal-definition-update-scope" || cmd.Text != "- updated goal" {
			t.Fatalf("command = %+v, want updated scoped goal", cmd)
		}
		return workspaceevents.GoalResult{Goal: &workspacemodel.Goal{
			ID:          cmd.GoalID,
			WorkspaceID: workspaceID,
			Text:        cmd.Text,
		}}
	})

	item, err := UpdateGoalView(ctx, hub, workspaceID, "goal-definition-update-scope", "updated goal")
	if err != nil {
		t.Fatalf("UpdateGoalView err = %v", err)
	}
	if item.WorkspaceID != string(workspaceID) || item.Text != "- updated goal" {
		t.Fatalf("goal = %+v, want scoped updated goal", item)
	}
}

func TestListConsolidatedWorkspaceDoDsUpdatesCanonicalAndDeletesObsolete(t *testing.T) {
	ctx := context.Background()
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-port")
	deleteCount := 0
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandListDoDsByWS, func(data any) any {
		cmd, ok := data.(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListDoDsByWorkspaceCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return workspaceevents.DoDListResult{DoDs: []*workspacemodel.DoD{
			{ID: "dod-001", WorkspaceID: workspaceID, DefinitionRevision: "dod:00000000000000000001:first", Items: []string{"first"}},
			{ID: "dod-002", WorkspaceID: workspaceID, DefinitionRevision: "dod:00000000000000000002:second", Items: []string{"second"}},
		}}
	})
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandUpdateDoD, func(data any) any {
		cmd, ok := data.(workspaceevents.UpdateDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want UpdateDoDCommand", data)
		}
		dodID := cmd.DoDID
		if dodID != "dod-002" {
			t.Fatalf("updated dod id = %q, want dod-002", dodID)
		}
		items := cmd.Items
		if len(items) != 2 || items[0] != "first" || items[1] != "second" {
			t.Fatalf("merged items = %#v, want first/second", items)
		}
		return workspaceevents.DoDResult{DoD: &workspacemodel.DoD{
			ID:                 dodID,
			WorkspaceID:        workspaceID,
			DefinitionRevision: "dod:00000000000000000003:merged",
			Items:              items,
		}}
	})
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandDeleteDoD, func(data any) any {
		deleteCount++
		cmd, ok := data.(workspaceevents.DeleteDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DeleteDoDCommand", data)
		}
		if got := cmd.DoDID; got != "dod-001" {
			t.Fatalf("deleted dod id = %q, want dod-001", got)
		}
		return workspaceevents.DeleteDoDResult{WorkspaceID: cmd.WorkspaceID, DoDID: cmd.DoDID}
	})

	items, err := ListConsolidatedWorkspaceDoDs(ctx, hub, workspaceID)
	if err != nil {
		t.Fatalf("ListConsolidatedWorkspaceDoDs err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "dod-002" {
		t.Fatalf("items = %#v, want canonical dod-002", items)
	}
	if deleteCount != 1 {
		t.Fatalf("delete count = %d, want 1", deleteCount)
	}
}

func TestUpdateDoDViewPublishesWorkspaceScopeFromContext(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-definition-update-scope")
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-update-scope")
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandUpdateDoD, func(data any) any {
		cmd, ok := data.(workspaceevents.UpdateDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want UpdateDoDCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		if cmd.DoDID != "dod-definition-update-scope" || len(cmd.Items) != 1 || cmd.Items[0] != "updated dod" {
			t.Fatalf("command = %+v, want updated scoped dod", cmd)
		}
		return workspaceevents.DoDResult{DoD: &workspacemodel.DoD{
			ID:          cmd.DoDID,
			WorkspaceID: workspaceID,
			Items:       cmd.Items,
		}}
	})

	item, err := UpdateDoDView(ctx, hub, workspaceID, "dod-definition-update-scope", []string{"updated dod"})
	if err != nil {
		t.Fatalf("UpdateDoDView err = %v", err)
	}
	if item.WorkspaceID != string(workspaceID) || len(item.Items) != 1 || item.Items[0] != "updated dod" {
		t.Fatalf("dod = %+v, want scoped updated dod", item)
	}
}

func TestDeleteGoalPublishesWorkspaceScopeFromContext(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-definition-delete-scope")
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-delete-scope")
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandDeleteGoal, func(data any) any {
		cmd, ok := data.(workspaceevents.DeleteGoalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DeleteGoalCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		if cmd.GoalID != "goal-definition-delete-scope" {
			t.Fatalf("command = %+v, want scoped goal delete", cmd)
		}
		return workspaceevents.DeleteGoalResult{WorkspaceID: cmd.WorkspaceID, GoalID: cmd.GoalID}
	})

	if err := DeleteGoal(ctx, hub, "", "goal-definition-delete-scope"); err != nil {
		t.Fatalf("DeleteGoal err = %v", err)
	}
}

func TestDeleteDoDPublishesWorkspaceScopeFromContext(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-definition-delete-scope")
	defHub := newDefinitionMagicTestHub(t)
	hub := defHub.rawHub
	workspaceID := common.WorkspaceID("ws-definition-delete-scope")
	defHub.subscribe(workspaceevents.TopicWorkspaceCommandDeleteDoD, func(data any) any {
		cmd, ok := data.(workspaceevents.DeleteDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DeleteDoDCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		if cmd.DoDID != "dod-definition-delete-scope" {
			t.Fatalf("command = %+v, want scoped dod delete", cmd)
		}
		return workspaceevents.DeleteDoDResult{WorkspaceID: cmd.WorkspaceID, DoDID: cmd.DoDID}
	})

	if err := DeleteDoD(ctx, hub, "", "dod-definition-delete-scope"); err != nil {
		t.Fatalf("DeleteDoD err = %v", err)
	}
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

type definitionMagicTestHub struct {
	rawHub   magicevent.Hub
	observer magicevent.SimpleObserver
}

func newDefinitionMagicTestHub(t *testing.T) *definitionMagicTestHub {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	ret := &definitionMagicTestHub{
		rawHub:   rawHub,
		observer: magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub),
	}
	t.Cleanup(func() {
		rawHub.Terminate(context.Background())
	})
	return ret
}

func (h *definitionMagicTestHub) subscribe(topic string, handler func(any) any) {
	h.observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(handler(ev.Data()), nil)
	})
}
