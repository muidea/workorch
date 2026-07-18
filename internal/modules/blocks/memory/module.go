package memory

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorycontracts "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/contracts"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateMemoryItemCommand = memorycontracts.CreateMemoryItemCommand
type UpdateMemoryItemCommand = memorycontracts.UpdateMemoryItemCommand

type Dependencies struct {
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
	EventHub           magicevent.Hub
}

type Module struct {
	bizPtr   *memorybiz.Service
	eventHub magicevent.Hub
	observer magicevent.SimpleObserver
	topics   []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.MemoryItems == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = memorybiz.NewServiceWithRecallTraces(deps.MemoryItems, deps.MemoryRecallTraces)
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		memoryevents.TopicMemoryCommandCreate,
		memoryevents.TopicMemoryCommandGet,
		memoryevents.TopicMemoryCommandListByWorkspace,
		memoryevents.TopicMemoryCommandSearch,
		memoryevents.TopicMemoryCommandUpdate,
		memoryevents.TopicMemoryCommandDelete,
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		memoryevents.TopicMemoryRecallTraceCommandGet,
		memoryevents.TopicMemoryRecallTraceCommandList,
		memoryevents.TopicMemoryRecallTraceCommandDelete,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setMemoryMagicResult(result, response, err)
}

func (m *Module) executeCommand(ev magicevent.Event) (memoryevents.MemoryCommandResult, error) {
	if m == nil || m.bizPtr == nil {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case memoryevents.TopicMemoryCommandCreate:
		cmd, ok := ev.Data().(memoryevents.CreateMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.bizPtr.CreateMemoryItem(ctx, cmd); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		item, err := m.bizPtr.GetMemoryItem(ctx, cmd.MemoryID)
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
		item, err := m.bizPtr.GetMemoryItem(ctx, cmd.MemoryID)
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
		items, err := m.bizPtr.ListMemoryByWorkspace(ctx, cmd.WorkspaceID)
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
		items, err := m.bizPtr.SearchMemory(ctx, memorySearchQueryFromEvent(cmd.Query))
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
		if err := m.bizPtr.UpdateMemoryItem(ctx, cmd); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		item, err := m.bizPtr.GetMemoryItem(ctx, cmd.MemoryID)
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
		item, err := m.bizPtr.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		if item.WorkspaceID != cmd.WorkspaceID {
			return memoryevents.MemoryCommandResult{}, common.ErrConflict
		}
		if err := m.bizPtr.DeleteMemoryItem(ctx, cmd.MemoryID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		m.publishMemoryChanged(ctx, memoryevents.TopicMemoryDeleted, item)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandUpsert:
		cmd, ok := ev.Data().(memoryevents.UpsertMemoryRecallTraceCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		trace, err := memoryRecallTraceFromCommand(cmd)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		if err := m.bizPtr.UpsertMemoryRecallTrace(ctx, trace); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		event := magicevent.NewEventWithContext(
			memoryevents.TopicMemoryRecallRecorded,
			ModuleID,
			memoryevents.NotificationDestination,
			magicevent.NewHeader(),
			ctx,
			memoryevents.MemoryRecallRecordedData{
				TraceID:            trace.ID,
				WorkspaceID:        trace.WorkspaceID,
				RunID:              trace.RunID,
				StepID:             trace.StepID,
				Stage:              trace.Stage,
				Source:             trace.Source,
				Policy:             trace.Policy,
				Status:             trace.Status,
				ProviderID:         trace.ProviderID,
				Operation:          trace.Operation,
				RetrievedMemoryIDs: append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
				Reason:             trace.Reason,
				OccurredAt:         trace.OccurredAt,
			},
		)
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{
			SubjectType: "memory_recall",
			SubjectID:   string(trace.ID),
			WorkspaceID: string(trace.WorkspaceID),
			RunID:       string(trace.RunID),
			StepID:      string(trace.StepID),
		}))
		m.eventHub.Post(event)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.bizPtr.GetMemoryRecallTrace(ctx, cmd.TraceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTrace: item}, nil
	case memoryevents.TopicMemoryRecallTraceCommandList:
		cmd, ok := ev.Data().(memoryevents.ListMemoryRecallTracesCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.Query.WorkspaceID)
		items, err := m.bizPtr.ListMemoryRecallTraces(ctx, memoryRecallTraceQueryFromEvent(cmd.Query))
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTraces: items}, nil
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.bizPtr.DeleteMemoryRecallTrace(ctx, cmd.TraceID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	default:
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
}

func memorySearchQueryFromEvent(query memoryevents.MemorySearchQuery) repository.MemorySearchQuery {
	return repository.MemorySearchQuery{
		WorkspaceID: query.WorkspaceID, Query: query.Query, Tags: append([]string(nil), query.Tags...),
		Types: append([]string(nil), query.Types...), Vector: append([]float64(nil), query.Vector...),
		MinScore: query.MinScore, Limit: query.Limit,
	}
}

func memoryRecallTraceQueryFromEvent(query memoryevents.MemoryRecallTraceQuery) repository.MemoryRecallTraceQuery {
	return repository.MemoryRecallTraceQuery{
		WorkspaceID: query.WorkspaceID, RunID: query.RunID, StepID: query.StepID, Stage: query.Stage,
		Source: query.Source, Status: query.Status, Limit: query.Limit,
	}
}

func (m *Module) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) {
	if m == nil || m.eventHub == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, ModuleID, memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, memoryevents.MemoryChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(topic, workorchevents.EventHubLaneScope{
		SubjectType: "memory", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.eventHub.Post(event)
}

func memoryCommandContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	if workspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
}

func setMemoryMagicResult(result magicevent.Result, data memoryevents.MemoryCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func memoryRecallTraceFromCommand(cmd memorycontracts.UpsertMemoryRecallTraceCommand) (*memorymodel.MemoryRecallTrace, error) {
	if cmd.TraceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return &memorymodel.MemoryRecallTrace{
		ID:                     cmd.TraceID,
		WorkspaceID:            cmd.WorkspaceID,
		RunID:                  cmd.RunID,
		StepID:                 cmd.StepID,
		GraphNodeID:            cmd.GraphNodeID,
		Stage:                  cmd.Stage,
		Source:                 cmd.Source,
		Policy:                 cmd.Policy,
		Status:                 cmd.Status,
		ProviderID:             cmd.ProviderID,
		Operation:              cmd.Operation,
		QueryTextPreview:       cmd.QueryTextPreview,
		QueryTextSha256:        cmd.QueryTextSha256,
		MinScore:               cmd.MinScore,
		Limit:                  cmd.Limit,
		EmbeddingModel:         cmd.EmbeddingModel,
		VectorDimension:        cmd.VectorDimension,
		RefreshSource:          cmd.RefreshSource,
		SelectionPolicyVersion: cmd.SelectionPolicyVersion,
		CandidateMemoryIDs:     append([]common.MemoryID(nil), cmd.CandidateMemoryIDs...),
		SelectedMemoryIDs:      append([]common.MemoryID(nil), cmd.SelectedMemoryIDs...),
		ExcludedMemoryIDs:      append([]common.MemoryID(nil), cmd.ExcludedMemoryIDs...),
		TypeQuota:              cloneStringIntMap(cmd.TypeQuota),
		SelectionScores:        memoryRecallSelectionScoresFromCommand(cmd.SelectionScores),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), cmd.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    memoryRecallHitsFromCommand(cmd.RetrievedMemoryHits),
		Reason:                 cmd.Reason,
		OccurredAt:             cmd.OccurredAt,
		EvidenceRef:            cmd.EvidenceRef,
	}, nil
}

func memoryRecallSelectionScoresFromCommand(items []memorycontracts.MemoryRecallSelectionScoreCommand) []memorymodel.MemoryRecallSelectionScore {
	if items == nil {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorymodel.MemoryRecallSelectionScore{
			ID:     item.ID,
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func memoryRecallHitsFromCommand(items []memorycontracts.MemoryRecallHitCommand) []memorymodel.MemoryRecallHit {
	if items == nil {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, memorymodel.MemoryRecallHit{
			ID:              item.ID,
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func cloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
