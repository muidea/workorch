package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/pkg/events"
)

const sourceModule = "application/workorchd/context-registry"

func PublishCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) error {
	return ownercommandports.PublishFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, "", "", payload)
}

func PublishCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) (events.Envelope, error) {
	return ownercommandports.PublishWithResponseFrom(ctx, sourceModule, hub, topic, subjectType, subjectID, "", "", payload)
}
