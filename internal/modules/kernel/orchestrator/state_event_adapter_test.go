package orchestrator

import (
	"context"
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestRunEventRepositoryGetPropagatesRuntimeWorkspaceScope(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-run-event-scope")
	hub := events.NewMemoryHub()
	var captured events.Envelope
	hub.Subscribe(runevents.TopicRunCommandGet, func(_ context.Context, envelope events.Envelope) error {
		captured = envelope
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyRun: &runmodel.Run{
					ID:          "run-001",
					WorkspaceID: "ws-run-event-scope",
				},
			},
		})
	})

	item, err := (runEventRepository{hub: hub}).Get(ctx, "run-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.WorkspaceID != "ws-run-event-scope" {
		t.Fatalf("run workspace = %q", item.WorkspaceID)
	}
	if captured.WorkspaceID != "ws-run-event-scope" || captured.RunID != "run-001" {
		t.Fatalf("captured scope = workspace %q run %q", captured.WorkspaceID, captured.RunID)
	}
	if captured.Payload[events.PayloadKeyWorkspaceID] != "ws-run-event-scope" {
		t.Fatalf("payload workspace = %#v", captured.Payload[events.PayloadKeyWorkspaceID])
	}
}
