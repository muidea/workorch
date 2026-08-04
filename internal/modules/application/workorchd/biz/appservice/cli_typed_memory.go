package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLIMemory(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIMemoryCreateCommand:
		item, err := CreateMemoryItemWithSession(ctx, d.borrow(), string(command.WorkspaceID), cliMemoryCommand(command.Memory))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryCreateResult{Memory: readmodelports.PortMemoryItemInfo(item)}, nil)
	case workorchdevents.CLIMemoryGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadMemoryItemWithSession(ctx, d.borrow(), string(command.MemoryID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryGetResult{Memory: readmodelports.PortMemoryItemInfo(item)}, nil)
	case workorchdevents.CLIMemoryUpdateCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := UpdateMemoryItemWithSession(ctx, d.borrow(), string(command.MemoryID), cliMemoryCommand(command.Memory))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemoryUpdateResult{Memory: readmodelports.PortMemoryItemInfo(item)}, nil)
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
		result.Set(workorchdevents.CLIMemoryListResult{Memories: readmodelports.PortMemoryItemInfos(items)}, nil)
	case workorchdevents.CLIMemorySearchCommand:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), workorchmanagementcontract.MemorySearchQuery{WorkspaceID: string(command.WorkspaceID), Query: command.Request.Query, Tags: append([]string(nil), command.Request.Tags...), Types: append([]string(nil), command.Request.Types...), Limit: command.Request.Limit})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMemorySearchResult{Memories: readmodelports.PortMemoryItemInfos(items)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}

func cliMemoryCommand(memory workorchport.MemoryItemInfo) workorchmanagementcontract.MemoryMutation {
	return workorchmanagementcontract.MemoryMutation{
		MemoryID: string(memory.ID), RunID: string(memory.RunID), SourceType: memory.SourceType,
		SourceRef: memory.SourceRef, MemoryType: memory.MemoryType, Content: memory.Content,
		Summary: memory.Summary, Tags: append([]string(nil), memory.Tags...), Importance: memory.Importance,
		Confidence: memory.Confidence, Visibility: memory.Visibility,
		EmbeddingProviderID: memory.EmbeddingProviderID, EmbeddingModel: memory.EmbeddingModel,
		EmbeddingText: memory.EmbeddingText, Vector: append([]float64(nil), memory.Vector...),
	}
}
