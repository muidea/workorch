package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateMemoryCommand = memorycontracts.CreateMemoryItemCommand
type UpdateMemoryCommand = memorycontracts.UpdateMemoryItemCommand
type DeleteMemoryCommand = memorycontracts.DeleteMemoryItemCommand
type UpsertMemoryRecallTraceCommand = memorycontracts.UpsertMemoryRecallTraceCommand
type DeleteMemoryRecallTraceCommand = memorycontracts.DeleteMemoryRecallTraceCommand

type GetMemoryCommand struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}

type ListMemoryByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type SearchMemoryCommand struct {
	Query repository.MemorySearchQuery
}

type GetMemoryRecallTraceCommand struct {
	WorkspaceID common.WorkspaceID
	TraceID     common.MemoryRecallTraceID
}

type ListMemoryRecallTracesCommand struct {
	Query repository.MemoryRecallTraceQuery
}

type MemoryData struct {
	Item        *memorymodel.MemoryItem
	RecallTrace *memorymodel.MemoryRecallTrace
}

type MemoryCommandResult struct {
	Item         *memorymodel.MemoryItem
	Items        []*memorymodel.MemoryItem
	RecallTrace  *memorymodel.MemoryRecallTrace
	RecallTraces []*memorymodel.MemoryRecallTrace
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateMemoryCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetMemoryCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListMemoryByWorkspaceCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[SearchMemoryCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandSearch).MessageContract(),
		eventcontract.CommandTopic[UpdateMemoryCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteMemoryCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandDelete).MessageContract(),
		eventcontract.CommandTopic[UpsertMemoryRecallTraceCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryRecallTraceCommandUpsert).MessageContract(),
		eventcontract.CommandTopic[GetMemoryRecallTraceCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryRecallTraceCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListMemoryRecallTracesCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryRecallTraceCommandList).MessageContract(),
		eventcontract.CommandTopic[DeleteMemoryRecallTraceCommand, MemoryCommandResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryRecallTraceCommandDelete).MessageContract(),
	}
	for _, topic := range []string{
		TopicMemoryCreated,
		TopicMemoryUpdated,
		TopicMemoryDeleted,
		TopicMemoryRecallRecorded,
	} {
		ret = append(ret, eventcontract.DataTopic[MemoryData]("memory", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
