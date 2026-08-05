package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (d *CLICommandDispatcher) handleTypedCLIMemory(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIMemoryCreateCommand:
		item, err := CreateMemoryItemWithSession(ctx, d.borrow(), string(command.WorkspaceID), command.Memory)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryCreateResult{Memory: item}, nil)
	case workorchdevents.CLIMemoryGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadMemoryItemWithSession(ctx, d.borrow(), string(command.MemoryID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryGetResult{Memory: item}, nil)
	case workorchdevents.CLIMemoryUpdateCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := UpdateMemoryItemWithSession(ctx, d.borrow(), string(command.MemoryID), command.Memory)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryUpdateResult{Memory: item}, nil)
	case workorchdevents.CLIMemoryDeleteCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		err := DeleteMemoryItemWithSession(ctx, d.borrow(), string(command.MemoryID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLIMemoryListCommand:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), workorchmanagementcontract.MemorySearchQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryListResult{Memories: items}, nil)
	case workorchdevents.CLIMemorySearchCommand:
		query := command.Request
		query.WorkspaceID = string(command.WorkspaceID)
		items, err := SearchMemoryWithSession(ctx, d.borrow(), query)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemorySearchResult{Memories: items}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
