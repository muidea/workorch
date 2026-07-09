package memory

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	memorybiz "github.com/muidea/workorch/internal/modules/kernel/memory/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	memoryservice "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = memoryservice.Service
type CreateMemoryItemCommand = memorycontracts.CreateMemoryItemCommand
type UpdateMemoryItemCommand = memorycontracts.UpdateMemoryItemCommand

type Dependencies struct {
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
	EventHub           magicevent.Hub
}

type Module struct {
	bizPtr     *memorybiz.Service
	servicePtr *memoryservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.MemoryItems == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = memorybiz.NewServiceWithRecallTraces(deps.MemoryItems, deps.MemoryRecallTraces)
	m.servicePtr = memoryservice.New(memoryservice.Dependencies{
		CreateMemoryItem:      m.bizPtr.CreateMemoryItem,
		UpdateMemoryItem:      m.bizPtr.UpdateMemoryItem,
		GetMemoryItem:         m.bizPtr.GetMemoryItem,
		DeleteMemoryItem:      m.bizPtr.DeleteMemoryItem,
		ListMemoryByWorkspace: m.bizPtr.ListMemoryByWorkspace,
		SearchMemory:          m.bizPtr.SearchMemory,
		UpsertRecallTrace:     m.bizPtr.UpsertMemoryRecallTrace,
		GetRecallTrace:        m.bizPtr.GetMemoryRecallTrace,
		ListRecallTraces:      m.bizPtr.ListMemoryRecallTraces,
		DeleteRecallTrace:     m.bizPtr.DeleteMemoryRecallTrace,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
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
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setMemoryMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (memoryevents.MemoryCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case memoryevents.TopicMemoryCommandCreate:
		cmd, ok := data.(memoryevents.CreateMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.servicePtr.CreateMemoryItem(ctx, cmd); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryCommandGet:
		cmd, ok := data.(memoryevents.GetMemoryCommand)
		if !ok || cmd.MemoryID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.servicePtr.GetMemoryItem(ctx, cmd.MemoryID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Item: item}, nil
	case memoryevents.TopicMemoryCommandListByWorkspace:
		cmd, ok := data.(memoryevents.ListMemoryByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.servicePtr.ListMemoryByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Items: items}, nil
	case memoryevents.TopicMemoryCommandSearch:
		cmd, ok := data.(memoryevents.SearchMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.Query.WorkspaceID)
		items, err := m.servicePtr.SearchMemory(ctx, cmd.Query)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{Items: items}, nil
	case memoryevents.TopicMemoryCommandUpdate:
		cmd, ok := data.(memoryevents.UpdateMemoryCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.servicePtr.UpdateMemoryItem(ctx, cmd); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryCommandDelete:
		cmd, ok := data.(memoryevents.DeleteMemoryCommand)
		if !ok || cmd.MemoryID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteMemoryItem(ctx, cmd.MemoryID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandUpsert:
		cmd, ok := data.(memoryevents.UpsertMemoryRecallTraceCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		trace, err := memoryRecallTraceFromCommand(cmd)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		if err := m.servicePtr.UpsertMemoryRecallTrace(ctx, trace); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		cmd, ok := data.(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.servicePtr.GetMemoryRecallTrace(ctx, cmd.TraceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTrace: item}, nil
	case memoryevents.TopicMemoryRecallTraceCommandList:
		cmd, ok := data.(memoryevents.ListMemoryRecallTracesCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.Query.WorkspaceID)
		items, err := m.servicePtr.ListMemoryRecallTraces(ctx, cmd.Query)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTraces: items}, nil
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		cmd, ok := data.(memoryevents.DeleteMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteMemoryRecallTrace(ctx, cmd.TraceID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	default:
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
}

func memoryCommandContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	if workspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
}

func setMemoryMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Module) handleMemoryQueryCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if workspaceID := memoryScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case memoryevents.TopicMemoryCommandCreate:
		var cmd memorycontracts.CreateMemoryItemCommand
		if payloadCommand, ok := envelope.Payload[events.PayloadKeyCommand].(memorycontracts.CreateMemoryItemCommand); ok {
			cmd = payloadCommand
		} else {
			item, err := memoryItemFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = memoryCommandFromItem(item)
		}
		if err := m.servicePtr.CreateMemoryItem(ctx, cmd); err != nil {
			return err
		}
		return memoryCommandResponse(envelope, nil)
	case memoryevents.TopicMemoryCommandGet:
		item, err := m.servicePtr.GetMemoryItem(ctx, common.MemoryID(stringPayload(envelope.Payload, events.PayloadKeyMemoryID)))
		if err != nil {
			return err
		}
		return memoryCommandResponse(envelope, map[string]any{events.PayloadKeyMemoryItem: item})
	case memoryevents.TopicMemoryCommandListByWorkspace:
		items, err := m.servicePtr.ListMemoryByWorkspace(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return memoryCommandResponse(envelope, map[string]any{events.PayloadKeyMemoryItems: items})
	case memoryevents.TopicMemoryCommandSearch:
		query, ok := envelope.Payload[events.PayloadKeyMemorySearchQuery].(repository.MemorySearchQuery)
		if !ok {
			return common.ErrInvalidArgument
		}
		items, err := m.servicePtr.SearchMemory(ctx, query)
		if err != nil {
			return err
		}
		return memoryCommandResponse(envelope, map[string]any{events.PayloadKeyMemoryItems: items})
	case memoryevents.TopicMemoryCommandUpdate:
		var cmd memorycontracts.UpdateMemoryItemCommand
		if payloadCommand, ok := envelope.Payload[events.PayloadKeyCommand].(memorycontracts.UpdateMemoryItemCommand); ok {
			cmd = payloadCommand
		} else {
			item, err := memoryItemFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = memoryCommandFromItem(item)
		}
		if err := m.servicePtr.UpdateMemoryItem(ctx, cmd); err != nil {
			return err
		}
		return memoryCommandResponse(envelope, nil)
	case memoryevents.TopicMemoryCommandDelete:
		if err := m.servicePtr.DeleteMemoryItem(ctx, deleteMemoryIDFromPayload(envelope.Payload)); err != nil {
			return err
		}
		return memoryCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func (m *Module) handleMemoryRecallTraceCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if workspaceID := memoryScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case memoryevents.TopicMemoryRecallTraceCommandUpsert:
		trace, err := memoryRecallTraceFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.UpsertMemoryRecallTrace(ctx, trace); err != nil {
			return err
		}
		return memoryCommandResponse(envelope, nil)
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		item, err := m.servicePtr.GetMemoryRecallTrace(ctx, common.MemoryRecallTraceID(stringPayload(envelope.Payload, events.PayloadKeyTraceID)))
		if err != nil {
			return err
		}
		return memoryCommandResponse(envelope, map[string]any{events.PayloadKeyMemoryRecallTrace: item})
	case memoryevents.TopicMemoryRecallTraceCommandList:
		query, ok := envelope.Payload[events.PayloadKeyMemoryRecallTraceQuery].(repository.MemoryRecallTraceQuery)
		if !ok {
			return common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListMemoryRecallTraces(ctx, query)
		if err != nil {
			return err
		}
		return memoryCommandResponse(envelope, map[string]any{events.PayloadKeyMemoryRecallTraces: items})
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		if err := m.servicePtr.DeleteMemoryRecallTrace(ctx, deleteMemoryRecallTraceIDFromPayload(envelope.Payload)); err != nil {
			return err
		}
		return memoryCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func memoryItemFromPayload(payload map[string]any) (*memorymodel.MemoryItem, error) {
	item, ok := payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func memoryRecallTraceFromPayload(payload map[string]any) (*memorymodel.MemoryRecallTrace, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(memorycontracts.UpsertMemoryRecallTraceCommand); ok {
		return memoryRecallTraceFromCommand(cmd)
	}
	trace, ok := payload[events.PayloadKeyMemoryRecallTrace].(*memorymodel.MemoryRecallTrace)
	if !ok || trace == nil {
		return nil, common.ErrInvalidArgument
	}
	return trace, nil
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

func deleteMemoryRecallTraceIDFromPayload(payload map[string]any) common.MemoryRecallTraceID {
	if cmd, ok := payload[events.PayloadKeyCommand].(memorycontracts.DeleteMemoryRecallTraceCommand); ok && cmd.TraceID != "" {
		return cmd.TraceID
	}
	return common.MemoryRecallTraceID(stringPayload(payload, events.PayloadKeyTraceID))
}

func memoryCommandFromItem(item *memorymodel.MemoryItem) memorycontracts.CreateMemoryItemCommand {
	if item == nil {
		return memorycontracts.CreateMemoryItemCommand{}
	}
	return memorycontracts.CreateMemoryItemCommand{
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

func deleteMemoryIDFromPayload(payload map[string]any) common.MemoryID {
	if cmd, ok := payload[events.PayloadKeyCommand].(memorycontracts.DeleteMemoryItemCommand); ok && cmd.MemoryID != "" {
		return cmd.MemoryID
	}
	return common.MemoryID(stringPayload(payload, events.PayloadKeyMemoryID))
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func memoryScopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
}

func memoryCommandResponse(request events.Envelope, payload map[string]any) error {
	return events.Response(events.Envelope{
		Topic:        request.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  request.SubjectType,
		SubjectID:    request.SubjectID,
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		Payload:      payload,
	})
}
