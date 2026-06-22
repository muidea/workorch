package governance

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventWorkspaceReaderCarriesRuntimeWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-definition-scope")
	goalID := common.GoalID("goal-governance-definition-scope")
	dodID := common.DoDID("dod-governance-definition-scope")
	var goalEnvelope events.Envelope
	var dodEnvelope events.Envelope
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(_ context.Context, envelope events.Envelope) error {
		goalEnvelope = envelope
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyGoal: &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "goal"},
		}})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(_ context.Context, envelope events.Envelope) error {
		dodEnvelope = envelope
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyDoD: &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"dod"}, State: common.DoDStateReady},
		}})
	})
	reader := eventWorkspaceReader{hub: hub}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

	if _, err := reader.GetGoal(scopedCtx, goalID); err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if _, err := reader.GetDoD(scopedCtx, dodID); err != nil {
		t.Fatalf("GetDoD: %v", err)
	}
	if goalEnvelope.WorkspaceID != string(workspaceID) || goalEnvelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
		t.Fatalf("goal envelope scope = (%q, %v), want %s", goalEnvelope.WorkspaceID, goalEnvelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
	}
	if dodEnvelope.WorkspaceID != string(workspaceID) || dodEnvelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
		t.Fatalf("dod envelope scope = (%q, %v), want %s", dodEnvelope.WorkspaceID, dodEnvelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
	}
}
