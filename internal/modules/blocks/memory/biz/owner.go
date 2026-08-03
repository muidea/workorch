package memory

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorycontracts "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/contracts"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
	EventHub           magicevent.Hub
	BackgroundRoutine  task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *Service
	topics  []string
	started bool
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.MemoryItems == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: NewServiceWithRecallTraces(deps.MemoryItems, deps.MemoryRecallTraces)}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
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
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.started = false
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setMemoryMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (memoryevents.MemoryCommandResult, error) {
	if m == nil || m.service == nil {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	if workspaceID := memoryCommandWorkspaceID(ev.Data()); workspaceID != "" {
		var err error
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
	}
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
		items, err := m.service.SearchMemory(ctx, memorySearchQueryFromEvent(cmd.Query))
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
		if err := m.service.UpdateMemoryItem(ctx, cmd); err != nil {
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
		if err := m.service.UpsertMemoryRecallTrace(ctx, trace); err != nil {
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
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), event.ID(), workorchevents.EventHubLaneScope{
			SubjectType: "memory_recall",
			SubjectID:   string(trace.ID),
			WorkspaceID: string(trace.WorkspaceID),
			RunID:       string(trace.RunID),
			StepID:      string(trace.StepID),
		}))
		m.PostEvent(event)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.service.GetMemoryRecallTrace(ctx, cmd.TraceID)
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
		items, err := m.service.ListMemoryRecallTraces(ctx, memoryRecallTraceQueryFromEvent(cmd.Query))
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTraces: items}, nil
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.DeleteMemoryRecallTrace(ctx, cmd.TraceID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	default:
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
}

// memoryCommandWorkspaceID extracts only the durable command scope. It does
// not inspect component state, so the shared owner can serve concurrent
// Workspace lanes without a mutable "current workspace" field.
func memoryCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case memoryevents.CreateMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.GetMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.ListMemoryByWorkspaceCommand:
		return cmd.WorkspaceID
	case memoryevents.SearchMemoryCommand:
		return cmd.Query.WorkspaceID
	case memoryevents.DeleteMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.UpsertMemoryRecallTraceCommand:
		return cmd.WorkspaceID
	case memoryevents.GetMemoryRecallTraceCommand:
		return cmd.WorkspaceID
	case memoryevents.ListMemoryRecallTracesCommand:
		return cmd.Query.WorkspaceID
	default:
		return ""
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

func (m *Biz) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, ModuleID, memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, memoryevents.MemoryChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), topic, workorchevents.EventHubLaneScope{
		SubjectType: "memory", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.PostEvent(event)
}

func memoryCommandContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	if workspaceID == "" {
		return ctx
	}
	// executeCommand checks command scope before this helper is reached.
	bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return ctx
	}
	return bound
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
