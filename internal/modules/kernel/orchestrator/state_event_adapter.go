package orchestrator

import (
	"context"
	"fmt"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runEventRepository struct {
	hub events.Hub
}

func (r runEventRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: createRunCommandFromModel(item),
	})
	return err
}

func (r runEventRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandGet, "run", string(id), "", id, map[string]any{
		events.PayloadKeyRunID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r runEventRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r runEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r runEventRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: updateRunCommandFromModel(item),
	})
	return err
}

func (r runEventRepository) Delete(ctx context.Context, id common.RunID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandDelete, "run", string(id), "", id, map[string]any{
		events.PayloadKeyCommand: runcontracts.DeleteRunCommand{RunID: id},
	})
	return err
}

type workspaceEventRepository struct {
	hub events.Hub
}

func (r workspaceEventRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandCreate, "workspace", string(item.ID), item.ID, "", map[string]any{
		events.PayloadKeyCommand: createWorkspaceCommandFromModel(item),
	})
	return err
}

func (r workspaceEventRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandGet, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r workspaceEventRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandList, "workspace", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyWorkspaces].([]*workspacemodel.Workspace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r workspaceEventRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, map[string]any{
		events.PayloadKeyCommand: updateWorkspaceCommandFromModel(item),
	})
	return err
}

func (r workspaceEventRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandDelete, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.DeleteWorkspaceCommand{WorkspaceID: id},
	})
	return err
}

type memoryEventRepository struct {
	hub events.Hub
}

func (r memoryEventRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandCreate, "memory", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyCommand: memoryCommandFromModel(item),
	})
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandGet, "memory", string(id), "", "", map[string]any{
		events.PayloadKeyMemoryID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r memoryEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryEventRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "", map[string]any{
		events.PayloadKeyMemorySearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryEventRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandUpdate, "memory", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyCommand: memoryCommandFromModel(item),
	})
	return err
}

func (r memoryEventRepository) Delete(ctx context.Context, id common.MemoryID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandDelete, "memory", string(id), "", "", map[string]any{
		events.PayloadKeyCommand: memorycontracts.DeleteMemoryItemCommand{MemoryID: id},
	})
	return err
}

type memoryRecallTraceEventRepository struct {
	hub events.Hub
}

func (r memoryRecallTraceEventRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	return r.Upsert(ctx, item)
}

func (r memoryRecallTraceEventRepository) Upsert(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandUpsert, "memory_recall_trace", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyCommand: memoryRecallTraceCommandFromModel(item),
	})
	return err
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), "", "", map[string]any{
		events.PayloadKeyTraceID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryRecallTrace].(*memorymodel.MemoryRecallTrace)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTraceQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryRecallTraces].([]*memorymodel.MemoryRecallTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), "", "", map[string]any{
		events.PayloadKeyCommand: memorycontracts.DeleteMemoryRecallTraceCommand{TraceID: id},
	})
	return err
}

func publishStateEventCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: ModuleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, fmt.Errorf("state event command %s returned no response", topic)
}

func createRunCommandFromModel(item *runmodel.Run) runcontracts.CreateRunCommand {
	if item == nil {
		return runcontracts.CreateRunCommand{}
	}
	return runcontracts.CreateRunCommand{
		RunID:              item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   cloneMap(item.ExecutionContext),
		Priority:           item.Priority,
	}
}

func updateRunCommandFromModel(item *runmodel.Run) runcontracts.UpdateRunCommand {
	if item == nil {
		return runcontracts.UpdateRunCommand{}
	}
	return runcontracts.UpdateRunCommand{
		RunID:              item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   cloneMap(item.ExecutionContext),
		State:              item.State,
		Priority:           item.Priority,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		FailureReason:      item.FailureReason,
	}
}

func createWorkspaceCommandFromModel(item *workspacemodel.Workspace) workspacecontracts.CreateWorkspaceCommand {
	if item == nil {
		return workspacecontracts.CreateWorkspaceCommand{}
	}
	return workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
	}
}

func updateWorkspaceCommandFromModel(item *workspacemodel.Workspace) workspacecontracts.UpdateWorkspaceCommand {
	if item == nil {
		return workspacecontracts.UpdateWorkspaceCommand{}
	}
	return workspacecontracts.UpdateWorkspaceCommand{
		WorkspaceID:          item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         cloneSoulBindingMap(item.SoulBindings),
		ContextHints:         cloneMap(item.ContextHints),
		EvidencePolicies:     cloneMapSlice(item.EvidencePolicies),
	}
}

func memoryCommandFromModel(item *memorymodel.MemoryItem) memorycontracts.CreateMemoryItemCommand {
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

func memoryRecallTraceCommandFromModel(trace *memorymodel.MemoryRecallTrace) memorycontracts.UpsertMemoryRecallTraceCommand {
	if trace == nil {
		return memorycontracts.UpsertMemoryRecallTraceCommand{}
	}
	return memorycontracts.UpsertMemoryRecallTraceCommand{
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
		TypeQuota:              cloneStringIntMap(trace.TypeQuota),
		SelectionScores:        memoryRecallSelectionScoreCommands(trace.SelectionScores),
		RetrievedMemoryIDs:     append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
		RetrievedMemoryHits:    memoryRecallHitCommands(trace.RetrievedMemoryHits),
		Reason:                 trace.Reason,
		OccurredAt:             trace.OccurredAt,
		EvidenceRef:            trace.EvidenceRef,
	}
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

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
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

func cloneSoulBindingMap(input map[string]common.SoulID) map[string]common.SoulID {
	if input == nil {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneMap(item))
	}
	return ret
}
