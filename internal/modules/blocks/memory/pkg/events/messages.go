package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorycontracts "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
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
	Query MemorySearchQuery
}

type MemorySearchQuery struct {
	WorkspaceID common.WorkspaceID
	Query       string
	Tags        []string
	Types       []string
	Vector      []float64
	MinScore    float64
	Limit       int
}

type GetMemoryRecallTraceCommand struct {
	WorkspaceID common.WorkspaceID
	TraceID     common.MemoryRecallTraceID
}

type ListMemoryRecallTracesCommand struct {
	Query MemoryRecallTraceQuery
}

type MemoryRecallTraceQuery struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Stage       string
	Source      string
	Status      string
	Limit       int
}

type MemoryChangedData struct {
	MemoryID    common.MemoryID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	MemoryType  string
	Summary     string
	Content     string
	SourceType  string
	SourceRef   string
	Tags        []string
	Importance  int
	Confidence  int
	Visibility  string
}

func MemoryChangedDataFromModel(item *memorymodel.MemoryItem) MemoryChangedData {
	if item == nil {
		return MemoryChangedData{}
	}
	return MemoryChangedData{
		MemoryID:    item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		MemoryType:  item.MemoryType,
		Summary:     item.Summary,
		Content:     item.Content,
		SourceType:  item.SourceType,
		SourceRef:   item.SourceRef,
		Tags:        append([]string(nil), item.Tags...),
		Importance:  item.Importance,
		Confidence:  item.Confidence,
		Visibility:  item.Visibility,
	}
}

type MemoryRecallRecordedData struct {
	TraceID            common.MemoryRecallTraceID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	Stage              string
	Source             string
	Policy             string
	Status             string
	ProviderID         string
	Operation          string
	RetrievedMemoryIDs []common.MemoryID
	Reason             string
	OccurredAt         string
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
	} {
		ret = append(ret, eventcontract.DataTopic[MemoryChangedData]("memory", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret, eventcontract.DataTopic[MemoryRecallRecordedData]("memory", definitionPackage, eventcontract.LanePolicyState, TopicMemoryRecallRecorded).MessageContract())
	return ret
}
