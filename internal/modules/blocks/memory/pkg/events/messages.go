package events

import (
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateMemoryCommand struct {
	MemoryID            common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	Vector              []float64
}

type UpdateMemoryCommand CreateMemoryCommand

type DeleteMemoryCommand struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}

type MemoryRecallSelectionScore struct {
	ID     common.MemoryID
	Score  int
	Reason string
}

type MemoryRecallHit struct {
	ID              common.MemoryID
	Summary         string
	VectorScore     float64
	VectorDimension int
}

type MemoryTypeQuotaEntry struct {
	Type  string
	Quota int
}

type UpsertMemoryRecallTraceCommand struct {
	TraceID                common.MemoryRecallTraceID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	GraphNodeID            string
	Stage                  string
	Source                 string
	Policy                 string
	Status                 string
	ProviderID             string
	Operation              string
	QueryTextPreview       string
	QueryTextSha256        string
	MinScore               float64
	Limit                  int
	EmbeddingModel         string
	VectorDimension        int
	RefreshSource          string
	SelectionPolicyVersion string
	CandidateMemoryIDs     []common.MemoryID
	SelectedMemoryIDs      []common.MemoryID
	ExcludedMemoryIDs      []common.MemoryID
	TypeQuota              []MemoryTypeQuotaEntry
	SelectionScores        []MemoryRecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []MemoryRecallHit
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}

func (c UpsertMemoryRecallTraceCommand) Validate() error {
	if c.TraceID == "" || c.WorkspaceID == "" || c.RunID == "" {
		return common.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(c.TypeQuota))
	for _, entry := range c.TypeQuota {
		key := strings.TrimSpace(entry.Type)
		if key == "" || entry.Quota < 0 {
			return common.ErrInvalidArgument
		}
		if _, ok := seen[key]; ok {
			return common.ErrInvalidArgument
		}
		seen[key] = struct{}{}
	}
	return nil
}

type DeleteMemoryRecallTraceCommand struct {
	WorkspaceID common.WorkspaceID
	TraceID     common.MemoryRecallTraceID
}

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

func CreateMemoryCommandFromModel(item *memorymodel.MemoryItem) CreateMemoryCommand {
	if item == nil {
		return CreateMemoryCommand{}
	}
	return CreateMemoryCommand{
		MemoryID:            item.ID,
		WorkspaceID:         item.WorkspaceID,
		RunID:               item.RunID,
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                append([]string(nil), item.Tags...),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		Vector:              append([]float64(nil), item.Vector...),
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

// MemoryRecallTrace is the owner-local EventHub result DTO. Its shape follows
// the upsert contract, but it is a distinct result type and keeps TypeQuota as
// a stable entry slice instead of exposing the persistence model's map.
type MemoryRecallTrace UpsertMemoryRecallTraceCommand

func MemoryRecallTraceFromModel(trace *memorymodel.MemoryRecallTrace) *MemoryRecallTrace {
	if trace == nil {
		return nil
	}
	command := UpsertMemoryRecallTraceCommandFromModel(trace)
	result := MemoryRecallTrace(command)
	return &result
}

func MemoryRecallTracesFromModels(items []*memorymodel.MemoryRecallTrace) []*MemoryRecallTrace {
	ret := make([]*MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallTraceFromModel(item))
	}
	return ret
}

func (t *MemoryRecallTrace) Model() (*memorymodel.MemoryRecallTrace, error) {
	if t == nil {
		return nil, common.ErrInvalidArgument
	}
	command := UpsertMemoryRecallTraceCommand(*t)
	if err := command.Validate(); err != nil {
		return nil, err
	}
	return &memorymodel.MemoryRecallTrace{
		ID: command.TraceID, WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: command.StepID,
		GraphNodeID: command.GraphNodeID, Stage: command.Stage, Source: command.Source, Policy: command.Policy,
		Status: command.Status, ProviderID: command.ProviderID, Operation: command.Operation,
		QueryTextPreview: command.QueryTextPreview, QueryTextSha256: command.QueryTextSha256, MinScore: command.MinScore,
		Limit: command.Limit, EmbeddingModel: command.EmbeddingModel, VectorDimension: command.VectorDimension,
		RefreshSource: command.RefreshSource, SelectionPolicyVersion: command.SelectionPolicyVersion,
		CandidateMemoryIDs: append([]common.MemoryID(nil), command.CandidateMemoryIDs...),
		SelectedMemoryIDs:  append([]common.MemoryID(nil), command.SelectedMemoryIDs...),
		ExcludedMemoryIDs:  append([]common.MemoryID(nil), command.ExcludedMemoryIDs...),
		TypeQuota:          memoryTypeQuotaMap(command.TypeQuota), SelectionScores: memoryRecallSelectionScores(command.SelectionScores),
		RetrievedMemoryIDs:  append([]common.MemoryID(nil), command.RetrievedMemoryIDs...),
		RetrievedMemoryHits: memoryRecallHits(command.RetrievedMemoryHits), Reason: command.Reason,
		OccurredAt: command.OccurredAt, EvidenceRef: command.EvidenceRef,
	}, nil
}

type MemoryCommandResult struct {
	Item         *memorymodel.MemoryItem
	Items        []*memorymodel.MemoryItem
	RecallTrace  *MemoryRecallTrace
	RecallTraces []*MemoryRecallTrace
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
		TypeQuota:              memoryTypeQuotaEntries(trace.TypeQuota),
		SelectionScores:        memoryRecallSelectionScoreCommands(trace.SelectionScores),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    memoryRecallHitCommands(trace.RetrievedMemoryHits),
		Reason:                 trace.Reason,
		OccurredAt:             trace.OccurredAt,
		EvidenceRef:            trace.EvidenceRef,
	}
}

func memoryTypeQuotaEntries(input map[string]int) []MemoryTypeQuotaEntry {
	if input == nil {
		return nil
	}
	ret := make([]MemoryTypeQuotaEntry, 0, len(input))
	for key, value := range input {
		ret = append(ret, MemoryTypeQuotaEntry{Type: key, Quota: value})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Type < ret[j].Type })
	return ret
}

func memoryRecallSelectionScoreCommands(items []memorymodel.MemoryRecallSelectionScore) []MemoryRecallSelectionScore {
	if items == nil {
		return nil
	}
	ret := make([]MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallSelectionScore{
			ID:     item.ID,
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func memoryRecallHitCommands(items []memorymodel.MemoryRecallHit) []MemoryRecallHit {
	if items == nil {
		return nil
	}
	ret := make([]MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallHit{
			ID:              item.ID,
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func memoryTypeQuotaMap(input []MemoryTypeQuotaEntry) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for _, item := range input {
		ret[item.Type] = item.Quota
	}
	return ret
}

func memoryRecallSelectionScores(items []MemoryRecallSelectionScore) []memorymodel.MemoryRecallSelectionScore {
	if items == nil {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorymodel.MemoryRecallSelectionScore{ID: item.ID, Score: item.Score, Reason: item.Reason})
	}
	return ret
}

func memoryRecallHits(items []MemoryRecallHit) []memorymodel.MemoryRecallHit {
	if items == nil {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorymodel.MemoryRecallHit{ID: item.ID, Summary: item.Summary, VectorScore: item.VectorScore, VectorDimension: item.VectorDimension})
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
