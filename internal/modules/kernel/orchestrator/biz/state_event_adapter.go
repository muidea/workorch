package orchestrator

// Owner state adapters are internal to the Runtime Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
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
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandCreate, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, createRunCommandFromModel(item))
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID)
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(id), workspaceID, id)
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(id), workspaceID, id)
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandList, "run", "all", workspaceID, "")
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
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
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandStoreRuntimeStateRecord, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, storeRuntimeStateRecordCommandFromModel(item))
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandStoreRuntimeStateRecord, "run", string(item.ID), item.WorkspaceID, item.ID)
	_, err := runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandStoreRuntimeStateRecord)
	return err
}

func (r runEventRepository) Delete(ctx context.Context, id common.RunID) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandDelete, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.DeleteRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandDelete, "run", string(id), workspaceID, id)
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
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, createWorkspaceCommandFromModel(item))
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(item.ID), item.ID, "")
	_, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandCreate)
	return err
}

func (r workspaceEventRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: id})
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGet, "workspace", string(id), id, "")
	response, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Workspace == nil {
		return nil, common.ErrNotFound
	}
	return workspaceeventmapping.Workspace(response.Workspace), nil
}

func (r workspaceEventRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspacesCommand{})
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandList, "workspace", "all", workspaceID, "")
	response, err := workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandList)
	if err != nil {
		return nil, err
	}
	return workspaceeventmapping.Workspaces(response.Workspaces), nil
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
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID)
	_, err = workspaceCommandResponse(r.hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdate)
	return err
}

func (r workspaceEventRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteWorkspaceCommand{WorkspaceID: id})
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDelete, "workspace", string(id), id, "")
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
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandCreate, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.CreateMemoryCommandFromModel(item))
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandCreate, "memory", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandCreate)
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryCommand{MemoryID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandGet, "memory", string(id), workspaceID, "")
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
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r memoryEventRepository) Search(ctx context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandSearch, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.SearchMemoryCommand{Query: workorchquerycontract.MemorySearchQuery{WorkspaceID: query.WorkspaceID, Query: query.Query, Tags: append([]string(nil), query.Tags...), Types: append([]string(nil), query.Types...), Vector: append([]float64(nil), query.Vector...), MinScore: query.MinScore, Limit: query.Limit}})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "")
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
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandUpdate, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.UpdateMemoryCommand(memoryevents.CreateMemoryCommandFromModel(item)))
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandUpdate, "memory", string(item.ID), item.WorkspaceID, item.RunID)
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
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandDelete, "memory", string(id), workspaceID, "")
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
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandUpsert, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.UpsertMemoryRecallTraceCommandFromModel(item))
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandUpsert, "memory_recall_trace", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandUpsert)
	return err
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryRecallTraceCommand{TraceID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), workspaceID, "")
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.RecallTrace == nil {
		return nil, common.ErrNotFound
	}
	return response.RecallTrace.Model()
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: workorchquerycontract.MemoryRecallTraceQuery{WorkspaceID: query.WorkspaceID, RunID: query.RunID, StepID: query.StepID, Stage: query.Stage, Source: query.Source, Status: query.Status, Limit: query.Limit}})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID)
	response, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
	if err != nil {
		return nil, err
	}
	ret := make([]*memorymodel.MemoryRecallTrace, 0, len(response.RecallTraces))
	for _, item := range response.RecallTraces {
		trace, err := item.Model()
		if err != nil {
			return nil, err
		}
		ret = append(ret, trace)
	}
	return ret, nil
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandDelete, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: workspaceID, TraceID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), workspaceID, "")
	_, err := memoryCommandResponse(r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandDelete)
	return err
}

func createRunCommandFromModel(item *runmodel.Run) runevents.CreateRunCommand {
	if item == nil {
		return runevents.CreateRunCommand{}
	}
	return runevents.CreateRunCommand{
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

func storeRuntimeStateRecordCommandFromModel(item *runmodel.Run) runevents.StoreRuntimeStateRecordCommand {
	if item == nil {
		return runevents.StoreRuntimeStateRecordCommand{}
	}
	ret := runevents.StoreRuntimeStateRecordCommand{
		RunID:        item.ID,
		WorkspaceID:  item.WorkspaceID,
		RuntimeState: cloneRuntimeStateRecord(item.RuntimeState),
	}
	if item.RuntimeState != nil {
		revision := item.RuntimeState.StateRevision
		ret.ExpectedRuntimeStateRevision = &revision
		ret.LifecycleFence = runtimeLifecycleFenceFromModel(item)
	}
	return ret
}

func runtimeLifecycleFenceFromModel(item *runmodel.Run) *runtimecontract.RuntimeOpaqueLifecycleFence {
	if item == nil || item.RuntimeState == nil || item.RuntimeBootstrap == nil {
		return nil
	}
	state := item.RuntimeState
	fence := runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, RuntimeStateRevision: state.StateRevision,
		RuntimeStateRef:  runevents.RuntimeStateRef(item.WorkspaceID, item.ID, state.StateRevision),
		RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(*state),
		BootstrapHash:    runtimecontract.StableRuntimeBootstrapHash(*item.RuntimeBootstrap),
	}
	if count := len(state.GovernanceResolutions); count != 0 {
		value := state.GovernanceResolutions[count-1]
		fence.GovernanceFenceID = runtimeGovernanceLifecycleFenceID(value.AuthoritySnapshotID, value.DispatchID)
	} else if count := len(state.GovernanceRequests); count != 0 {
		value := state.GovernanceRequests[count-1]
		fence.GovernanceFenceID = runtimeGovernanceLifecycleFenceID(value.AuthoritySnapshotID, value.DispatchID)
	}
	if outcome, ok := runtimeTerminalOutcomeFromState(state); ok {
		fence.TerminalFenceID = runtimecontract.StableTerminalDeliveryID(item.WorkspaceID, item.ID, state.Decision.AuthoritySnapshot.ID, outcome)
	}
	if fence.GovernanceFenceID == "" && fence.TerminalFenceID == "" {
		return nil
	}
	return &fence
}

func runtimeGovernanceLifecycleFenceID(authoritySnapshotID, dispatchID string) string {
	return "runtime-governance/" + authoritySnapshotID + "/" + dispatchID
}

func runtimeTerminalOutcomeFromState(state *runtimecontract.RuntimeStateRecord) (runtimecontract.TerminalOutcome, bool) {
	if state == nil {
		return runtimecontract.TerminalOutcome{}, false
	}
	if state.Termination != nil {
		kind := runtimecontract.TerminalOutcomeKindCancelled
		if state.Termination.Kind == runtimecontract.RuntimeTerminationSuperseded {
			kind = runtimecontract.TerminalOutcomeKindSuperseded
		}
		return runtimecontract.TerminalOutcome{Kind: kind, Code: "administrative_" + string(state.Termination.Kind), Summary: state.Termination.Reason}.Normalized(), true
	}
	if state.Decision.Terminal == nil {
		return runtimecontract.TerminalOutcome{}, false
	}
	kind := runtimecontract.TerminalOutcomeKindFailed
	if state.Decision.State == runtimecontract.ConvergenceStateComplete {
		kind = runtimecontract.TerminalOutcomeKindCompleted
	} else if state.Decision.State != runtimecontract.ConvergenceStateFailed {
		return runtimecontract.TerminalOutcome{}, false
	}
	return runtimecontract.TerminalOutcome{Kind: kind, Code: state.Decision.Terminal.Code, Summary: state.Decision.Terminal.Summary}.Normalized(), true
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

func createWorkspaceCommandFromModel(item *workspacemodel.Workspace) workspaceevents.CreateWorkspaceCommand {
	if item == nil {
		return workspaceevents.CreateWorkspaceCommand{}
	}
	return workspaceevents.CreateWorkspaceCommand{
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
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMSelection:  runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection),
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         soulbinding.FromSoulIDMap(item.SoulBindings),
		ContextHints:         item.ContextHints.Clone(),
		EvidencePolicies:     workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}, nil
}
