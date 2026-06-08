package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const sourceModule = "application/workorchd/context-registry"

func PublishCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) error {
	_, err := PublishCommandWithResponse(ctx, hub, topic, subjectType, subjectID, payload)
	return err
}

func PublishCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) (events.Envelope, error) {
	contract, ok := events.ResolveEventHubContract(topic)
	if !ok {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	switch contract.CommandClass {
	case events.EventCommandClassRead:
		return ownerreadports.PublishCommandWithResponseFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, "", "", payload)
	case events.EventCommandClassWrite:
		return ownercommandports.PublishWithResponseFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, "", "", payload)
	default:
		return events.Envelope{}, common.ErrInvalidArgument
	}
}
