package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadMemoryItemByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[memoryevents.GetMemoryCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, "memory", string(id), memoryevents.GetMemoryCommand{WorkspaceID: workspaceID, MemoryID: id})
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
	response, err := sendMagicCommand[memoryevents.ListMemoryByWorkspaceCommand, memoryevents.MemoryCommandResult](ctx, hub, memoryevents.TopicMemoryCommandListByWorkspace, memoryevents.CommandDestination, "workspace", string(workspaceID), memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func SearchMemoryByOwnerCommand(ctx context.Context, hub magicevent.Hub, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[memoryevents.SearchMemoryCommand, memoryevents.MemoryCommandResult](ctx, hub, memoryevents.TopicMemoryCommandSearch, memoryevents.CommandDestination, "memory_search", string(query.WorkspaceID), memoryevents.SearchMemoryCommand{Query: query})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}
