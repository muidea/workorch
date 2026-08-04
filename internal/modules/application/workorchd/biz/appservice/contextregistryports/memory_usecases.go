package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadMemoryItemView(ctx context.Context, hub magicevent.Hub, memoryID string) (workorchmanagementcontract.MemoryItem, error) {
	if hub == nil {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	item, err := ReadMemoryItem(ctx, hub, memoryID)
	if err != nil {
		return workorchmanagementcontract.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func ListMemoryByWorkspaceView(ctx context.Context, hub magicevent.Hub, workspaceID string, query string) ([]workorchmanagementcontract.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	var err error
	var items []*memorymodel.MemoryItem
	if query != "" {
		ownerQuery, queryErr := contextregistrycore.MemorySearchOwnerQuery(workorchmanagementcontract.MemorySearchQuery{
			WorkspaceID: workspaceID,
			Query:       query,
		})
		if queryErr != nil {
			return nil, queryErr
		}
		items, err = SearchMemory(ctx, hub, ownerQuery)
	} else {
		items, err = ListMemoryByWorkspace(ctx, hub, workspaceID)
	}
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MapMemoryItems(items), nil
}

func SearchMemoryView(ctx context.Context, hub magicevent.Hub, query workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ownerQuery, err := contextregistrycore.MemorySearchOwnerQuery(query)
	if err != nil {
		return nil, err
	}
	items, err := SearchMemory(ctx, hub, ownerQuery)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MapMemoryItems(items), nil
}
