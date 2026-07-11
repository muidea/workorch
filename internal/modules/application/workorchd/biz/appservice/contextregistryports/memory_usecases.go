package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadMemoryItemView(ctx context.Context, hub magicevent.Hub, memoryID string) (contextregistrycore.MemoryItem, error) {
	if hub == nil {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	item, err := ReadMemoryItem(ctx, hub, memoryID)
	if err != nil {
		return contextregistrycore.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func ListMemoryByWorkspaceView(ctx context.Context, hub magicevent.Hub, workspaceID string, query string) ([]contextregistrycore.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	var err error
	var items []*memorymodel.MemoryItem
	if query != "" {
		repositoryQuery, queryErr := contextregistrycore.MemorySearchRepositoryQuery(contextregistrycore.MemoryQuery{
			WorkspaceID: workspaceID,
			Query:       query,
		})
		if queryErr != nil {
			return nil, queryErr
		}
		items, err = SearchMemory(ctx, hub, repositoryQuery)
	} else {
		items, err = ListMemoryByWorkspace(ctx, hub, workspaceID)
	}
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MapMemoryItems(items), nil
}

func SearchMemoryView(ctx context.Context, hub magicevent.Hub, query contextregistrycore.MemoryQuery) ([]contextregistrycore.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	repositoryQuery, err := contextregistrycore.MemorySearchRepositoryQuery(query)
	if err != nil {
		return nil, err
	}
	items, err := SearchMemory(ctx, hub, repositoryQuery)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MapMemoryItems(items), nil
}
