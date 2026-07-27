package orchestrator

// Owner state adapters are internal to the Runtime V2 Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memorycontracts "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/contracts"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runcontracts "github.com/muidea/workorch/internal/modules/blocks/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runEventRepository struct {
	hub magicevent.Hub
}

func (r runEventRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandCreate, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.CreateRunCommand(createRunCommandFromModel(item)))
	bindOwnerCommandLane(event, runevents.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID)
	_, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandCreate)
	return err
}

func (r runEventRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID != "" {
		return r.GetByWorkspaceID(ctx, workspaceID, id)
	}
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerCommandLane(event, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id)
	response, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandGet)
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
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerCommandLane(event, runevents.TopicRunCommandGet, "run", string(id), workspaceID, id)
	response, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

func (r runEventRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandList, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsCommand{})
	bindOwnerCommandLane(event, runevents.TopicRunCommandList, "run", "all", workspaceID, "")
	response, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandList)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (r runEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	bindOwnerCommandLane(event, runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	response, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (r runEventRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandUpdate, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.UpdateRunCommand(updateRunCommandFromModel(item)))
	bindOwnerCommandLane(event, runevents.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID)
	_, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandUpdate)
	return err
}

func (r runEventRepository) Delete(ctx context.Context, id common.RunID) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandDelete, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.DeleteRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerCommandLane(event, runevents.TopicRunCommandDelete, "run", string(id), workspaceID, id)
	_, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandDelete)
	return err
}

type workspaceEventRepository struct {
	hub magicevent.Hub
}

func (r workspaceEventRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.CreateWorkspaceCommand(createWorkspaceCommandFromModel(item)))
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(item.ID), item.ID, "")
	_, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandCreate)
	return err
}

func (r workspaceEventRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: id})
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandGet, "workspace", string(id), id, "")
	response, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Workspace == nil {
		return nil, common.ErrNotFound
	}
	return response.Workspace, nil
}

func (r workspaceEventRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspacesCommand{})
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandList, "workspace", "all", workspaceID, "")
	response, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandList)
	if err != nil {
		return nil, err
	}
	return response.Workspaces, nil
}

func (r workspaceEventRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	command, err := updateWorkspaceCommandFromModel(item)
	if err != nil {
		return err
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdate, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID)
	_, err = workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdate)
	return err
}

func (r workspaceEventRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteWorkspaceCommand{WorkspaceID: id})
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandDelete, "workspace", string(id), id, "")
	_, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandDelete)
	return err
}

type memoryEventRepository struct {
	hub magicevent.Hub
}

func (r memoryEventRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandCreate, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.CreateMemoryCommand(memoryCommandFromModel(item)))
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandCreate, "memory", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandCreate)
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryCommand{MemoryID: id})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandGet, "memory", string(id), workspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Item == nil {
		return nil, common.ErrNotFound
	}
	return response.Item, nil
}

func (r memoryEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandListByWorkspace, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r memoryEventRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandSearch, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.SearchMemoryCommand{Query: memoryevents.MemorySearchQuery{WorkspaceID: query.WorkspaceID, Query: query.Query, Tags: append([]string(nil), query.Tags...), Types: append([]string(nil), query.Types...), Vector: append([]float64(nil), query.Vector...), MinScore: query.MinScore, Limit: query.Limit}})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandSearch)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r memoryEventRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandUpdate, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.UpdateMemoryCommand(memoryCommandFromModel(item)))
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandUpdate, "memory", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandUpdate)
	return err
}

func (r memoryEventRepository) Delete(ctx context.Context, id common.MemoryID) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandDelete, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.DeleteMemoryCommand{WorkspaceID: workspaceID, MemoryID: id})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryCommandDelete, "memory", string(id), workspaceID, "")
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandDelete)
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
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandUpsert, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.UpsertMemoryRecallTraceCommand(memoryRecallTraceCommandFromModel(item)))
	bindOwnerCommandLane(event, memoryevents.TopicMemoryRecallTraceCommandUpsert, "memory_recall_trace", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandUpsert)
	return err
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryRecallTraceCommand{TraceID: id})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), workspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.RecallTrace == nil {
		return nil, common.ErrNotFound
	}
	return response.RecallTrace, nil
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: memoryevents.MemoryRecallTraceQuery{WorkspaceID: query.WorkspaceID, RunID: query.RunID, StepID: query.StepID, Stage: query.Stage, Source: query.Source, Status: query.Status, Limit: query.Limit}})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID)
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
	if err != nil {
		return nil, err
	}
	return response.RecallTraces, nil
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandDelete, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.DeleteMemoryRecallTraceCommand{TraceID: id})
	bindOwnerCommandLane(event, memoryevents.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), workspaceID, "")
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandDelete)
	return err
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
		RuntimeBootstrap:   cloneRuntimeBootstrap(item.RuntimeBootstrap),
		Priority:           item.Priority,
	}
}

func updateRunCommandFromModel(item *runmodel.Run) runcontracts.UpdateRunCommand {
	if item == nil {
		return runcontracts.UpdateRunCommand{}
	}
	ret := runcontracts.UpdateRunCommand{
		RunID:                  item.ID,
		WorkspaceID:            item.WorkspaceID,
		GoalID:                 item.GoalID,
		DoDID:                  item.DoDID,
		Lineage:                item.Lineage,
		DefinitionRevision:     item.DefinitionRevision,
		InitialStepKind:        item.InitialStepKind,
		RuntimeBootstrap:       cloneRuntimeBootstrap(item.RuntimeBootstrap),
		RuntimeContractVersion: item.RuntimeContractVersion,
		RuntimeState:           cloneRuntimeStateRecord(item.RuntimeState),
		State:                  item.State,
		Priority:               item.Priority,
		BlockingTicketID:       item.BlockingTicketID,
		BlockingApprovalID:     item.BlockingApprovalID,
		FailureReason:          item.FailureReason,
		CreatedAt:              item.CreatedAt,
		StartedAt:              item.StartedAt,
		EndedAt:                item.EndedAt,
	}
	if item.RuntimeState != nil {
		revision := item.RuntimeState.StateRevision
		ret.ExpectedRuntimeStateRevision = &revision
	}
	return ret
}

func cloneRuntimeStateRecord(value *runtimecontract.RuntimeStateRecord) *runtimecontract.RuntimeStateRecord {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}

func cloneRuntimeBootstrap(value *runtimecontract.RuntimeBootstrap) *runtimecontract.RuntimeBootstrap {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
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

func updateWorkspaceCommandFromModel(item *workspacemodel.Workspace) (workspaceevents.UpdateWorkspaceCommand, error) {
	if item == nil {
		return workspaceevents.UpdateWorkspaceCommand{}, common.ErrInvalidArgument
	}
	return workspaceevents.UpdateWorkspaceCommand{
		WorkspaceID:          item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     cloneProviderModelTarget(item.DefaultLLMTarget),
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         cloneSoulBindingMap(item.SoulBindings),
		ContextHints:         item.ContextHints.Clone(),
		EvidencePolicies:     workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}, nil
}

func cloneProviderModelTarget(value *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
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
