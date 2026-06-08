package ownerreadports

import (
	"context"
	"errors"
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestPublishCommandWithResponseRejectsWriteTopic(t *testing.T) {
	_, err := PublishCommandWithResponse(context.Background(), events.NewMemoryHub(), runevents.TopicRunCommandUpdate, "run", "run-001", "", "run-001", map[string]any{
		events.PayloadKeyRunID: "run-001",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("PublishCommandWithResponse err = %v, want invalid argument for write topic", err)
	}
}

func TestPublishCommandWithResponsePublishesReadTopic(t *testing.T) {
	hub := events.NewMemoryHub()
	hub.Subscribe(runevents.TopicRunCommandGet, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/response",
			SourceModule: "test",
			Payload: map[string]any{
				"ok": true,
			},
		})
	})

	response, err := PublishCommandWithResponse(context.Background(), hub, runevents.TopicRunCommandGet, "run", "run-001", "", "run-001", map[string]any{
		events.PayloadKeyRunID: "run-001",
	})
	if err != nil {
		t.Fatalf("PublishCommandWithResponse err = %v", err)
	}
	if response.Payload["ok"] != true {
		t.Fatalf("response payload = %+v, want ok", response.Payload)
	}
}
