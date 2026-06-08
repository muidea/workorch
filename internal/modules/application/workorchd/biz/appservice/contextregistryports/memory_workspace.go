package contextregistryports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/events"
)

func EnsureWorkspaceExists(ctx context.Context, hub events.Hub, workspaceID string) error {
	_, err := PublishCommandWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandGet, "workspace", workspaceID, map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
	})
	return err
}
