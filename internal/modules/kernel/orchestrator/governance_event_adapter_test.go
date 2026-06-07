package orchestrator

import (
	"context"
	"testing"

	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestDecisionResolutionEventRepositoryPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicDecisionCommandCreateResolution, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	repo := decisionEventResolutionRepository{hub: hub}

	if err := repo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-command-payload",
		TicketID: "ticket-command-payload",
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "ok"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(decisioncontracts.CreateResolutionCommand)
	if !ok {
		t.Fatalf("payload command = %T, want CreateResolutionCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.ResolutionID != "resolution-command-payload" || cmd.TicketID != "ticket-command-payload" || cmd.Payload["answer"] != "ok" {
		t.Fatalf("command = %+v, want populated resolution command", cmd)
	}
	if _, ok := published[0].Payload[events.PayloadKeyResolution]; ok {
		t.Fatalf("payload contains legacy resolution field")
	}
}
