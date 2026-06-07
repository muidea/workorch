package ownerreadports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PublishCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	return ownercommandports.PublishWithResponse(ctx, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
}
