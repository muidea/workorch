package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/events"
)

func EnsureWorkspaceExists(ctx context.Context, hub events.Hub, workspaceID string) error {
	_, err := PublishCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGet, "workspace", workspaceID, map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
	})
	return err
}
