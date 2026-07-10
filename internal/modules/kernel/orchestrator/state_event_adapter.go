package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runEventRepository struct {
	hub magicevent.Hub
}

func (r runEventRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID, runevents.CreateRunCommand(createRunCommandFromModel(item)))
	return err
}

func (r runEventRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID != "" {
		return r.GetByWorkspaceID(ctx, workspaceID, id)
	}
	response, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r runEventRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r runEventRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	response, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandList, "run", "all", "", "", runevents.ListRunsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (r runEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	response, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (r runEventRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, runevents.UpdateRunCommand(updateRunCommandFromModel(item)))
	return err
}

func (r runEventRepository) Delete(ctx context.Context, id common.RunID) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	_, err := sendRunEventCommand(ctx, r.hub, runevents.TopicRunCommandDelete, "run", string(id), workspaceID, id, runevents.DeleteRunCommand{WorkspaceID: workspaceID, RunID: id})
	return err
}

type workspaceEventRepository struct {
	hub magicevent.Hub
}

func (r workspaceEventRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceEventCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(item.ID), item.ID, "", workspaceevents.CreateWorkspaceCommand(createWorkspaceCommandFromModel(item)))
	return err
}

func (r workspaceEventRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	response, err := sendWorkspaceEventCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandGet, "workspace", string(id), id, "", workspaceevents.GetWorkspaceCommand{WorkspaceID: id})
	if err != nil {
		return nil, err
	}
	if response.Workspace == nil {
		return nil, common.ErrNotFound
	}
	return response.Workspace, nil
}

func (r workspaceEventRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	response, err := sendWorkspaceEventCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandList, "workspace", "all", "", "", workspaceevents.ListWorkspacesCommand{})
	if err != nil {
		return nil, err
	}
	return response.Workspaces, nil
}

func (r workspaceEventRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendWorkspaceEventCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, workspaceevents.UpdateWorkspaceCommand(updateWorkspaceCommandFromModel(item)))
	return err
}

func (r workspaceEventRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	_, err := sendWorkspaceEventCommand(ctx, r.hub, workspaceevents.TopicWorkspaceCommandDelete, "workspace", string(id), id, "", workspaceevents.DeleteWorkspaceCommand{WorkspaceID: id})
	return err
}

type memoryEventRepository struct {
	hub magicevent.Hub
}

func (r memoryEventRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandCreate, "memory", string(item.ID), item.WorkspaceID, item.RunID, memoryevents.CreateMemoryCommand(memoryCommandFromModel(item)))
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	response, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandGet, "memory", string(id), "", "", memoryevents.GetMemoryCommand{MemoryID: id})
	if err != nil {
		return nil, err
	}
	if response.Item == nil {
		return nil, common.ErrNotFound
	}
	return response.Item, nil
}

func (r memoryEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	response, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r memoryEventRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	response, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "", memoryevents.SearchMemoryCommand{Query: query})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r memoryEventRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandUpdate, "memory", string(item.ID), item.WorkspaceID, item.RunID, memoryevents.UpdateMemoryCommand(memoryCommandFromModel(item)))
	return err
}

func (r memoryEventRepository) Delete(ctx context.Context, id common.MemoryID) error {
	_, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryCommandDelete, "memory", string(id), "", "", memoryevents.DeleteMemoryCommand{MemoryID: id})
	return err
}

type memoryRecallTraceEventRepository struct {
	hub magicevent.Hub
}

func (r memoryRecallTraceEventRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	return r.Upsert(ctx, item)
}

func (r memoryRecallTraceEventRepository) Upsert(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, "memory_recall_trace", string(item.ID), item.WorkspaceID, item.RunID, memoryevents.UpsertMemoryRecallTraceCommand(memoryRecallTraceCommandFromModel(item)))
	return err
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	response, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), "", "", memoryevents.GetMemoryRecallTraceCommand{TraceID: id})
	if err != nil {
		return nil, err
	}
	if response.RecallTrace == nil {
		return nil, common.ErrNotFound
	}
	return response.RecallTrace, nil
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	response, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, memoryevents.ListMemoryRecallTracesCommand{Query: query})
	if err != nil {
		return nil, err
	}
	return response.RecallTraces, nil
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	_, err := sendMemoryEventCommand(ctx, r.hub, memoryevents.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), "", "", memoryevents.DeleteMemoryRecallTraceCommand{TraceID: id})
	return err
}

func sendRunEventCommand[Command any](ctx context.Context, hub magicevent.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) (runevents.RunCommandResult, error) {
	result, err := sendStateEventCommand[Command, runevents.RunCommandResult](ctx, hub, topic, runevents.CommandDestination, subjectType, subjectID, workspaceID, runID, command)
	if err != nil {
		return runevents.RunCommandResult{}, err
	}
	return result, nil
}

func sendWorkspaceEventCommand[Command any](ctx context.Context, hub magicevent.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) (workspaceevents.WorkspaceCommandResult, error) {
	result, err := sendStateEventCommand[Command, workspaceevents.WorkspaceCommandResult](ctx, hub, topic, workspaceevents.CommandDestination, subjectType, subjectID, workspaceID, runID, command)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	return result, nil
}

func sendStateEventCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) (Result, error) {
	var zero Result
	if hub == nil || topic == "" {
		return zero, common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	response := hub.Send(ev)
	if response == nil {
		return zero, fmt.Errorf("state event command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, err := response.Get()
	if err != nil {
		return zero, err
	}
	result, ok := data.(Result)
	if !ok {
		return zero, common.ErrInvalidArgument
	}
	return result, nil
}

func sendMemoryEventCommand[Command any](ctx context.Context, hub magicevent.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) (memoryevents.MemoryCommandResult, error) {
	if hub == nil || topic == "" {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	response := hub.Send(ev)
	if response == nil {
		return memoryevents.MemoryCommandResult{}, fmt.Errorf("memory event command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return memoryevents.MemoryCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return memoryevents.MemoryCommandResult{}, err
	}
	result, ok := data.(memoryevents.MemoryCommandResult)
	if !ok {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
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
