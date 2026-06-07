package runtimeports

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type EventHubActionPort struct {
	hub events.Hub
}

func NewEventHubActionPort(hub events.Hub) *EventHubActionPort {
	return &EventHubActionPort{hub: hub}
}

func (p *EventHubActionPort) publish(ctx context.Context, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload map[string]any) error {
	if p == nil || p.hub == nil || topic == "" || subjectID == "" {
		return common.ErrInvalidArgument
	}
	err := p.hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: "workorchd/appservice/runtime-actions",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	})
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("runtime action command %s returned no response", topic)
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
