package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeMemoryItemCommand(ctx context.Context, ev magicevent.Event, result magicevent.Result) error {
	switch ev.ID() {
	case memoryevents.TopicObservePreparationInventory:
		c, ok := ev.Data().(memoryevents.ObservePreparationInventoryCommand)
		if !ok || ev.Source() != "application/workorchd" || c.Validate() != nil {
			setMemoryMagicResult(result, memoryevents.ObservePreparationInventoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		value, err := m.observePreparationInventory(ctx, c)
		setMemoryMagicResult(result, value, err)
		return err
	case memoryevents.TopicMemoryCommandCreate:
		cmd, ok := ev.Data().(memoryevents.CreateMemoryCommand)
		if !ok {
			setMemoryMagicResult(result, memoryevents.CreateMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		if err := m.service.createMemoryItem(ctx, cmd); err != nil {
			setMemoryMagicResult(result, memoryevents.CreateMemoryResult{}, err)
			return err
		}
		item, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.CreateMemoryResult{}, err)
			return err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryCreated, item)
		setMemoryMagicResult(result, memoryevents.CreateMemoryResult{WorkspaceID: cmd.WorkspaceID, MemoryID: cmd.MemoryID}, nil)
		return nil
	case memoryevents.TopicMemoryCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.MemoryID == "" {
			setMemoryMagicResult(result, memoryevents.GetMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		item, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.GetMemoryResult{}, err)
			return err
		}
		setMemoryMagicResult(result, (memoryevents.GetMemoryResult{Item: item}).Clone(), nil)
		return nil
	case memoryevents.TopicMemoryCommandListByWorkspace:
		cmd, ok := ev.Data().(memoryevents.ListMemoryByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			setMemoryMagicResult(result, memoryevents.ListMemoryByWorkspaceResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		items, nextCursor, err := m.service.listMemoryByWorkspace(ctx, cmd.WorkspaceID, cmd.CursorAfterID, cmd.Limit)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.ListMemoryByWorkspaceResult{}, err)
			return err
		}
		setMemoryMagicResult(result, (memoryevents.ListMemoryByWorkspaceResult{Items: items, NextCursor: nextCursor}).Clone(), nil)
		return nil
	case memoryevents.TopicMemoryCommandSearch:
		cmd, ok := ev.Data().(memoryevents.SearchMemoryCommand)
		if !ok || cmd.Query.WorkspaceID == "" {
			setMemoryMagicResult(result, memoryevents.SearchMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		items, err := m.service.searchMemory(ctx, cmd.Query)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.SearchMemoryResult{}, err)
			return err
		}
		setMemoryMagicResult(result, (memoryevents.SearchMemoryResult{Items: items}).Clone(), nil)
		return nil
	case memoryevents.TopicMemoryCommandUpdate:
		cmd, ok := ev.Data().(memoryevents.UpdateMemoryCommand)
		if !ok {
			setMemoryMagicResult(result, memoryevents.UpdateMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		if _, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID); err != nil {
			setMemoryMagicResult(result, memoryevents.UpdateMemoryResult{}, err)
			return err
		}
		if err := m.service.updateMemoryItem(ctx, memoryevents.CreateMemoryCommand(cmd)); err != nil {
			setMemoryMagicResult(result, memoryevents.UpdateMemoryResult{}, err)
			return err
		}
		item, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.UpdateMemoryResult{}, err)
			return err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryUpdated, item)
		setMemoryMagicResult(result, memoryevents.UpdateMemoryResult{WorkspaceID: cmd.WorkspaceID, MemoryID: cmd.MemoryID}, nil)
		return nil
	case memoryevents.TopicMemoryCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.MemoryID == "" {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		item, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryResult{}, err)
			return err
		}
		if err := m.service.deleteMemoryItem(ctx, cmd.MemoryID); err != nil {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryResult{}, err)
			return err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryDeleted, item)
		setMemoryMagicResult(result, memoryevents.DeleteMemoryResult{WorkspaceID: cmd.WorkspaceID, MemoryID: cmd.MemoryID}, nil)
		return nil
	default:
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
}
