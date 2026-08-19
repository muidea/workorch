package events

import (
	"math"
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
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
	ModelID                string
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
	TypeQuota              []workorchmemorycontract.TypeQuotaEntry
	SelectionScores        []workorchmemorycontract.RecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []workorchmemorycontract.RecallHit
	ContextBudget          workorchmemorycontract.RecallContextBudget
	SemanticRecall         *workorchmemorycontract.RecallSelectionEvidence
	FallbackFrom           string
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}

func (c UpsertMemoryRecallTraceCommand) Validate() error {
	if c.TraceID == "" || c.WorkspaceID == "" || c.RunID == "" {
		return common.ErrInvalidArgument
	}
	if len(c.CandidateMemoryIDs) > workorchmemorycontract.RecallSelectionMaxCandidateCount ||
		len(c.SelectedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxSelectedCount ||
		len(c.ExcludedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxCandidateCount ||
		len(c.SelectionScores) > workorchmemorycontract.RecallSelectionMaxCandidateCount ||
		len(c.RetrievedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxSelectedCount ||
		len(c.RetrievedMemoryHits) > workorchmemorycontract.RecallSelectionMaxSelectedCount ||
		math.IsNaN(c.MinScore) || math.IsInf(c.MinScore, 0) || c.Limit < 0 || c.VectorDimension < 0 {
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
	for _, score := range c.SelectionScores {
		if score.ID == "" || strings.TrimSpace(score.Reason) == "" {
			return common.ErrInvalidArgument
		}
	}
	for _, hit := range c.RetrievedMemoryHits {
		if hit.ID == "" || hit.VectorDimension < 0 || math.IsNaN(hit.VectorScore) || math.IsInf(hit.VectorScore, 0) {
			return common.ErrInvalidArgument
		}
	}
	if strings.TrimSpace(c.Source) == "activation" {
		evidence := workorchmemorycontract.RecallSelectionEvidence{
			Policy: c.Policy, Status: c.Status, Reason: c.Reason, ProviderID: c.ProviderID, ModelID: c.ModelID,
			Operation: c.Operation, QueryTextPreview: c.QueryTextPreview, QueryTextSHA256: c.QueryTextSha256,
			MinScore: c.MinScore, Limit: c.Limit, EmbeddingModel: c.EmbeddingModel, VectorDimension: c.VectorDimension,
			RefreshSource: c.RefreshSource, SelectionPolicyVersion: c.SelectionPolicyVersion,
			CandidateMemoryIDs:  append([]common.MemoryID(nil), c.CandidateMemoryIDs...),
			SelectedMemoryIDs:   append([]common.MemoryID(nil), c.SelectedMemoryIDs...),
			ExcludedMemoryIDs:   append([]common.MemoryID(nil), c.ExcludedMemoryIDs...),
			TypeQuota:           append([]workorchmemorycontract.TypeQuotaEntry(nil), c.TypeQuota...),
			SelectionScores:     append([]workorchmemorycontract.RecallSelectionScore(nil), c.SelectionScores...),
			RetrievedMemoryIDs:  append([]common.MemoryID(nil), c.RetrievedMemoryIDs...),
			RetrievedMemoryHits: append([]workorchmemorycontract.RecallHit(nil), c.RetrievedMemoryHits...),
			ContextBudget:       c.ContextBudget, SemanticRecall: cloneRecallSelectionEvidence(c.SemanticRecall), FallbackFrom: c.FallbackFrom,
		}
		if err := evidence.Validate(); err != nil {
			return err
		}
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
	WorkspaceID   common.WorkspaceID
	CursorAfterID common.MemoryID
	Limit         int
}

type SearchMemoryCommand struct {
	Query workorchquerycontract.MemorySearchQuery
}

type GetMemoryRecallTraceCommand struct {
	WorkspaceID common.WorkspaceID
	TraceID     common.MemoryRecallTraceID
}

type ListMemoryRecallTracesCommand struct {
	Query workorchquerycontract.MemoryRecallTraceQuery
}

type CreateMemoryResult struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}

type GetMemoryResult struct {
	Item *memorymodel.MemoryItem
}

func (r GetMemoryResult) Clone() GetMemoryResult {
	return GetMemoryResult{Item: memorymodel.CloneMemoryItem(r.Item)}
}

type ListMemoryByWorkspaceResult struct {
	Items      []*memorymodel.MemoryItem
	NextCursor common.MemoryID
}

func (r ListMemoryByWorkspaceResult) Clone() ListMemoryByWorkspaceResult {
	return ListMemoryByWorkspaceResult{Items: memorymodel.CloneMemoryItems(r.Items), NextCursor: r.NextCursor}
}

type SearchMemoryResult struct {
	Items []*memorymodel.MemoryItem
}

func (r SearchMemoryResult) Clone() SearchMemoryResult {
	return SearchMemoryResult{Items: memorymodel.CloneMemoryItems(r.Items)}
}

type UpdateMemoryResult struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}

type DeleteMemoryResult struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}

type MemoryRecallTraceWriteDisposition string

const (
	MemoryRecallTraceRecorded  MemoryRecallTraceWriteDisposition = "recorded"
	MemoryRecallTraceDuplicate MemoryRecallTraceWriteDisposition = "duplicate"
)

type UpsertMemoryRecallTraceResult struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	TraceID     common.MemoryRecallTraceID
	Disposition MemoryRecallTraceWriteDisposition
}

func (r UpsertMemoryRecallTraceResult) Validate(expectedWorkspaceID common.WorkspaceID, expectedRunID common.RunID, expectedTraceID common.MemoryRecallTraceID) error {
	if r.WorkspaceID != expectedWorkspaceID || r.RunID != expectedRunID || r.TraceID != expectedTraceID {
		return common.ErrConflict
	}
	switch r.Disposition {
	case MemoryRecallTraceRecorded, MemoryRecallTraceDuplicate:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

type GetMemoryRecallTraceResult struct {
	Trace *MemoryRecallTrace
}

func (r GetMemoryRecallTraceResult) Clone() GetMemoryRecallTraceResult {
	return GetMemoryRecallTraceResult{Trace: cloneMemoryRecallTrace(r.Trace)}
}

type ListMemoryRecallTracesResult struct {
	Traces     []*MemoryRecallTrace
	NextCursor common.MemoryRecallTraceID
}

func (r ListMemoryRecallTracesResult) Clone() ListMemoryRecallTracesResult {
	return ListMemoryRecallTracesResult{Traces: cloneMemoryRecallTraces(r.Traces), NextCursor: r.NextCursor}
}

type DeleteMemoryRecallTraceResult struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	TraceID     common.MemoryRecallTraceID
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
	SelectionEvidence  *workorchmemorycontract.RecallSelectionEvidence
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
		Status: command.Status, ProviderID: command.ProviderID, ModelID: command.ModelID, Operation: command.Operation,
		QueryTextPreview: command.QueryTextPreview, QueryTextSha256: command.QueryTextSha256, MinScore: command.MinScore,
		Limit: command.Limit, EmbeddingModel: command.EmbeddingModel, VectorDimension: command.VectorDimension,
		RefreshSource: command.RefreshSource, SelectionPolicyVersion: command.SelectionPolicyVersion,
		CandidateMemoryIDs: append([]common.MemoryID(nil), command.CandidateMemoryIDs...),
		SelectedMemoryIDs:  append([]common.MemoryID(nil), command.SelectedMemoryIDs...),
		ExcludedMemoryIDs:  append([]common.MemoryID(nil), command.ExcludedMemoryIDs...),
		TypeQuota:          memoryTypeQuotaMap(command.TypeQuota), SelectionScores: append([]workorchmemorycontract.RecallSelectionScore(nil), command.SelectionScores...),
		RetrievedMemoryIDs:  append([]common.MemoryID(nil), command.RetrievedMemoryIDs...),
		RetrievedMemoryHits: append([]workorchmemorycontract.RecallHit(nil), command.RetrievedMemoryHits...), ContextBudget: command.ContextBudget,
		SemanticRecall: cloneRecallSelectionEvidence(command.SemanticRecall), FallbackFrom: command.FallbackFrom, Reason: command.Reason,
		OccurredAt: command.OccurredAt, EvidenceRef: command.EvidenceRef,
	}, nil
}

func cloneMemoryRecallTrace(item *MemoryRecallTrace) *MemoryRecallTrace {
	if item == nil {
		return nil
	}
	model, err := item.Model()
	if err != nil {
		return nil
	}
	return MemoryRecallTraceFromModel(model)
}

func cloneMemoryRecallTraces(items []*MemoryRecallTrace) []*MemoryRecallTrace {
	if items == nil {
		return nil
	}
	ret := make([]*MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneMemoryRecallTrace(item))
	}
	return ret
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
		ModelID:                trace.ModelID,
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
		SelectionScores:        append([]workorchmemorycontract.RecallSelectionScore(nil), trace.SelectionScores...),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    append([]workorchmemorycontract.RecallHit(nil), trace.RetrievedMemoryHits...),
		ContextBudget:          trace.ContextBudget,
		SemanticRecall:         cloneRecallSelectionEvidence(trace.SemanticRecall),
		FallbackFrom:           trace.FallbackFrom,
		Reason:                 trace.Reason,
		OccurredAt:             trace.OccurredAt,
		EvidenceRef:            trace.EvidenceRef,
	}
}

func cloneRecallSelectionEvidence(input *workorchmemorycontract.RecallSelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	if input == nil {
		return nil
	}
	value := input.Normalized()
	return &value
}

func memoryTypeQuotaEntries(input map[string]int) []workorchmemorycontract.TypeQuotaEntry {
	if input == nil {
		return nil
	}
	ret := make([]workorchmemorycontract.TypeQuotaEntry, 0, len(input))
	for key, value := range input {
		ret = append(ret, workorchmemorycontract.TypeQuotaEntry{Type: key, Quota: value})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Type < ret[j].Type })
	return ret
}

func memoryTypeQuotaMap(input []workorchmemorycontract.TypeQuotaEntry) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for _, item := range input {
		ret[item.Type] = item.Quota
	}
	return ret
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateMemoryCommand, CreateMemoryResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetMemoryCommand, GetMemoryResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListMemoryByWorkspaceCommand, ListMemoryByWorkspaceResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[SearchMemoryCommand, SearchMemoryResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryCommandSearch).MessageContract(),
		eventcontract.CommandTopic[UpdateMemoryCommand, UpdateMemoryResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteMemoryCommand, DeleteMemoryResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryCommandDelete).MessageContract(),
		eventcontract.CommandTopic[UpsertMemoryRecallTraceCommand, UpsertMemoryRecallTraceResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryRecallTraceCommandUpsert).MessageContract(),
		eventcontract.CommandTopic[GetMemoryRecallTraceCommand, GetMemoryRecallTraceResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryRecallTraceCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListMemoryRecallTracesCommand, ListMemoryRecallTracesResult]("memory", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMemoryRecallTraceCommandList).MessageContract(),
		eventcontract.CommandTopic[DeleteMemoryRecallTraceCommand, DeleteMemoryRecallTraceResult]("memory", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMemoryRecallTraceCommandDelete).MessageContract(),
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
