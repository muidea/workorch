package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeMemoryItemCommand(ctx context.Context, ev magicevent.Event) (memoryevents.MemoryCommandResult, error) {
	switch ev.ID() {
	case memoryevents.TopicMemoryCommandCreate:
		cmd, ok := ev.Data().(memoryevents.CreateMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.service.CreateMemoryItem(ctx, cmd); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		item, err := m.service.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryCreated, item)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryCommand)
		if !ok || cmd.MemoryID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.service.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Item: item}, nil
	case memoryevents.TopicMemoryCommandListByWorkspace:
		cmd, ok := ev.Data().(memoryevents.ListMemoryByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.service.ListMemoryByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Items: items}, nil
	case memoryevents.TopicMemoryCommandSearch:
		cmd, ok := ev.Data().(memoryevents.SearchMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.Query.WorkspaceID)
		items, err := m.service.SearchMemory(ctx, cmd.Query)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Items: items}, nil
	case memoryevents.TopicMemoryCommandUpdate:
		cmd, ok := ev.Data().(memoryevents.UpdateMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.service.UpdateMemoryItem(ctx, memoryevents.CreateMemoryCommand(cmd)); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		item, err := m.service.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryUpdated, item)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.MemoryID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.service.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		if item.WorkspaceID != cmd.WorkspaceID {
			return memoryevents.MemoryCommandResult{}, common.ErrConflict
		}
		if err := m.service.DeleteMemoryItem(ctx, cmd.MemoryID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryDeleted, item)
		return memoryevents.MemoryCommandResult{}, nil
	default:
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
}
