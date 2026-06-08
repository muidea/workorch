package ownercommandports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const sourceModule = "application/workorchd/appservice"

func Publish(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) error {
	_, err := PublishWithResponse(ctx, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
	return err
}

func PublishWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	return PublishWithResponseFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
}

func PublishFrom(ctx context.Context, source string, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) error {
	_, err := PublishWithResponseFrom(ctx, source, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
	return err
}

func PublishWithResponseFrom(ctx context.Context, source string, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	if err := validateOwnerWriteTopic(topic); err != nil {
		return events.Envelope{}, err
	}
	if source == "" {
		source = sourceModule
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: source,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, common.ErrInvalidArgument
}

func validateOwnerWriteTopic(topic string) error {
	contract, ok := events.ResolveEventHubContract(topic)
	if !ok || contract.CommandClass != events.EventCommandClassWrite {
		return common.ErrInvalidArgument
	}
	return nil
}
