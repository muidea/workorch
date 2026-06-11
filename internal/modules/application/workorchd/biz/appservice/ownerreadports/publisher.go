package ownerreadports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const sourceModule = "application/workorchd/appservice"

func workspaceScopeFromContext(ctx context.Context) common.WorkspaceID {
	return common.RuntimeWorkspaceIDFromContext(ctx)
}

func payloadWithWorkspaceScope(payload map[string]any, workspaceID common.WorkspaceID) map[string]any {
	if workspaceID == "" {
		return payload
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	return payload
}

func PublishCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	return PublishCommandWithResponseFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
}

func PublishCommandWithResponseFrom(ctx context.Context, source string, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	if err := validateOwnerReadTopic(topic); err != nil {
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

func validateOwnerReadTopic(topic string) error {
	contract, ok := events.ResolveEventHubContract(topic)
	if !ok || contract.CommandClass != events.EventCommandClassRead {
		return common.ErrInvalidArgument
	}
	return nil
}
