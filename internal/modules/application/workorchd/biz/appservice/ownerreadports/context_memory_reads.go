package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadMemoryItemByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandGet, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), commandCtx, memoryevents.GetMemoryCommand{WorkspaceID: workspaceID, MemoryID: id})
	ownerevent.BindLane(event, memoryevents.TopicMemoryCommandGet, "memory", string(id), workspaceID, "", "")
	response, err := ownerevent.Memory(hub.Send(event), memoryevents.TopicMemoryCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Item == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Item, nil
}

func ListMemoryItemsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandListByWorkspace, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Memory(hub.Send(event), memoryevents.TopicMemoryCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func SearchMemoryByOwnerCommand(ctx context.Context, hub magicevent.Hub, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandSearch, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.SearchMemoryCommand{Query: memoryevents.MemorySearchQuery{WorkspaceID: query.WorkspaceID, Query: query.Query, Tags: append([]string(nil), query.Tags...), Types: append([]string(nil), query.Types...), Vector: append([]float64(nil), query.Vector...), MinScore: query.MinScore, Limit: query.Limit}})
	ownerevent.BindLane(event, memoryevents.TopicMemoryCommandSearch, "memory_search", string(query.WorkspaceID), query.WorkspaceID, "", "")
	response, err := ownerevent.Memory(hub.Send(event), memoryevents.TopicMemoryCommandSearch)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}
