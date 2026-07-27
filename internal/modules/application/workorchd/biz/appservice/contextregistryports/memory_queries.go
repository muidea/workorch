package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadMemoryItem(ctx context.Context, hub magicevent.Hub, memoryID string) (*memorymodel.MemoryItem, error) {
	return ownerreadports.ReadMemoryItemByOwnerCommand(ctx, hub, common.MemoryID(memoryID))
}

func ListMemoryByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID string) ([]*memorymodel.MemoryItem, error) {
	return ownerreadports.ListMemoryItemsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
}

func SearchMemory(ctx context.Context, hub magicevent.Hub, query memoryevents.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	return ownerreadports.SearchMemoryByOwnerCommand(ctx, hub, query)
}
