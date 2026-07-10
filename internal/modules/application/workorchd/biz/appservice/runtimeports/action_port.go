package runtimeports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type EventHubActionPort struct {
	hub events.Hub
}

func NewEventHubActionPort(hub events.Hub) *EventHubActionPort {
	return &EventHubActionPort{hub: hub}
}

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func (p *EventHubActionPort) send(ctx context.Context, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, command any) error {
	if p == nil || p.hub == nil || topic == "" || subjectID == "" {
		return common.ErrInvalidArgument
	}
	provider, ok := p.hub.(rawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, "workorchd/appservice/runtime-actions", destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{
		Topic:       topic,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
	}))
	result := provider.RawHub().Send(ev)
	if result == nil {
		return fmt.Errorf("runtime action command %s returned no response", topic)
	}
	if err := result.Error(); err != nil {
		return err
	}
	if _, resultErr := result.Get(); resultErr != nil {
		return resultErr
	}
	return nil
}

func cloneActionPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
