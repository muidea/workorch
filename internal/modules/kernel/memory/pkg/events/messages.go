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

func UpsertMemoryRecallTraceCommandFromModel(trace *memorymodel.MemoryRecallTrace) UpsertMemoryRecallTraceCommand {
	if trace == nil {
		return UpsertMemoryRecallTraceCommand{}
	}
	return UpsertMemoryRecallTraceCommand{
		TraceID:                trace.ID,
		WorkspaceID:            trace.WorkspaceID,
		RunID:                  trace.RunID,
		StepID:                 trace.StepID,
		GraphNodeID:            trace.GraphNodeID,
		Stage:                  trace.Stage,
		Source:                 trace.Source,
		Policy:                 trace.Policy,
		Status:                 trace.Status,
		ProviderID:             trace.ProviderID,
		Operation:              trace.Operation,
		QueryTextPreview:       trace.QueryTextPreview,
		QueryTextSha256:        trace.QueryTextSha256,
		MinScore:               trace.MinScore,
		Limit:                  trace.Limit,
		EmbeddingModel:         trace.EmbeddingModel,
		VectorDimension:        trace.VectorDimension,
		RefreshSource:          trace.RefreshSource,
		SelectionPolicyVersion: trace.SelectionPolicyVersion,
		CandidateMemoryIDs:     append([]common.MemoryID(nil), trace.CandidateMemoryIDs...),
		SelectedMemoryIDs:      append([]common.MemoryID(nil), trace.SelectedMemoryIDs...),
		ExcludedMemoryIDs:      append([]common.MemoryID(nil), trace.ExcludedMemoryIDs...),
		TypeQuota:              cloneMemoryRecallTypeQuota(trace.TypeQuota),
		SelectionScores:        memoryRecallSelectionScoreCommands(trace.SelectionScores),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    memoryRecallHitCommands(trace.RetrievedMemoryHits),
		Reason:                 trace.Reason,
		OccurredAt:             trace.OccurredAt,
		EvidenceRef:            trace.EvidenceRef,
	}
}

func cloneMemoryRecallTypeQuota(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func memoryRecallSelectionScoreCommands(items []memorymodel.MemoryRecallSelectionScore) []memorycontracts.MemoryRecallSelectionScoreCommand {
	if items == nil {
		return nil
	}
	ret := make([]memorycontracts.MemoryRecallSelectionScoreCommand, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorycontracts.MemoryRecallSelectionScoreCommand{
			ID:     item.ID,
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func memoryRecallHitCommands(items []memorymodel.MemoryRecallHit) []memorycontracts.MemoryRecallHitCommand {
	if items == nil {
		return nil
	}
	ret := make([]memorycontracts.MemoryRecallHitCommand, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorycontracts.MemoryRecallHitCommand{
			ID:              item.ID,
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
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
