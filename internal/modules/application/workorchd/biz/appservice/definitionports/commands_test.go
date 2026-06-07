package definitionports

import (
	"context"
	"errors"
	"testing"

	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestConsolidateWorkspaceDefinitionRejectsMissingCanonicalBeforePublishing(t *testing.T) {
	hub := events.NewMemoryHub()
	if _, err := ConsolidateWorkspaceGoal(context.Background(), hub, "ws-definition-port", nil, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("goal error = %v, want ErrInvalidArgument", err)
	}
	if published := hub.Published(); len(published) != 0 {
		t.Fatalf("published goal envelopes = %d, want 0", len(published))
	}

	if _, err := ConsolidateWorkspaceDoD(context.Background(), hub, "ws-definition-port", nil, nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("dod error = %v, want ErrInvalidArgument", err)
	}
	if published := hub.Published(); len(published) != 0 {
		t.Fatalf("published envelopes = %d, want 0", len(published))
	}
}

func TestDefineOrConsolidateWorkspaceGoalCreatesWhenMissing(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-definition-port")
	hub.Subscribe(events.TopicWorkspaceCommandListGoalsByWS, func(_ context.Context, envelope events.Envelope) error {
		if got := envelope.Payload[events.PayloadKeyWorkspaceID]; got != string(workspaceID) {
			t.Fatalf("workspace payload = %v, want %s", got, workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyGoals: []*workspacemodel.Goal{},
			},
		})
	})
	hub.Subscribe(events.TopicWorkspaceCommandDefineGoal, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.DefineGoalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DefineGoalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		goalID := cmd.GoalID
		if goalID == "" {
			t.Fatalf("missing generated goal id")
		}
		text := cmd.Text
		if text != "- ship the feature" {
			t.Fatalf("goal text = %q, want formatted bullet", text)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyGoal: &workspacemodel.Goal{
					ID:          goalID,
					WorkspaceID: workspaceID,
					Text:        text,
				},
			},
		})
	})

	item, err := DefineOrConsolidateWorkspaceGoal(ctx, hub, nil, workspaceID, "ship the feature")
	if err != nil {
		t.Fatalf("DefineOrConsolidateWorkspaceGoal err = %v", err)
	}
	if item == nil || item.WorkspaceID != workspaceID || item.Text != "- ship the feature" {
		t.Fatalf("goal item = %#v", item)
	}
}

func TestListConsolidatedWorkspaceDoDsUpdatesCanonicalAndDeletesObsolete(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-definition-port")
	deleteCount := 0
	hub.Subscribe(events.TopicWorkspaceCommandListDoDsByWS, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyDoDs: []*workspacemodel.DoD{
					{ID: "dod-001", WorkspaceID: workspaceID, Items: []string{"first"}},
					{ID: "dod-002", WorkspaceID: workspaceID, Items: []string{"second"}},
				},
			},
		})
	})
	hub.Subscribe(events.TopicWorkspaceCommandUpdateDoD, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.UpdateDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want UpdateDoDCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		dodID := cmd.DoDID
		if dodID != "dod-002" {
			t.Fatalf("updated dod id = %q, want dod-002", dodID)
		}
		items := cmd.Items
		if len(items) != 2 || items[0] != "first" || items[1] != "second" {
			t.Fatalf("merged items = %#v, want first/second", items)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyDoD: &workspacemodel.DoD{
					ID:          dodID,
					WorkspaceID: workspaceID,
					Items:       items,
				},
			},
		})
	})
	hub.Subscribe(events.TopicWorkspaceCommandDeleteDoD, func(_ context.Context, envelope events.Envelope) error {
		deleteCount++
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.DeleteDoDCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want DeleteDoDCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if got := cmd.DoDID; got != "dod-001" {
			t.Fatalf("deleted dod id = %q, want dod-001", got)
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{}})
	})

	items, err := ListConsolidatedWorkspaceDoDs(ctx, hub, nil, workspaceID)
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

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
