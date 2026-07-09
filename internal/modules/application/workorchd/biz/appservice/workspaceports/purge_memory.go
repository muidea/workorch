package workspaceports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceMemoryItems(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	items, err := ownerreadports.ListMemoryItemsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.PublishMemoryItemDelete(ctx, hub, workspaceID, item.RunID, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
