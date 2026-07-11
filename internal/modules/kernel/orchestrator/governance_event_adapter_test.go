package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func TestDecisionResolutionEventRepositorySendsTypedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	observer := magicevent.NewSimpleObserver(decisionevents.CommandDestination, rawHub)
	defer observer.Unsubscribe(decisionevents.TopicDecisionCommandCreateResolution)
	var got decisionevents.CreateResolutionCommand
	observer.Subscribe(decisionevents.TopicDecisionCommandCreateResolution, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(decisionevents.CreateResolutionCommand)
		if !ok {
			t.Fatalf("event data = %T, want CreateResolutionCommand", ev.Data())
		}
		got = cmd
		result.Set(decisionevents.DecisionCommandResult{}, nil)
	})
	repo := decisionEventResolutionRepository{hub: rawHub}

	if err := repo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-command-payload",
		TicketID: "ticket-command-payload",
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "ok"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.ResolutionID != "resolution-command-payload" || got.TicketID != "ticket-command-payload" || got.Payload["answer"] != "ok" {
		t.Fatalf("command = %+v, want populated resolution command", got)
	}
}
