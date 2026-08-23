package orchestrator

// Owner state adapters are internal to the Runtime Biz.

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runSnapshotReader is the Orchestrator's only Run lifecycle read surface.
// Runtime aggregate persistence uses the separate opaque record query/CAS
// contract below; this reader intentionally cannot create, list, mutate, or
// delete Run owner state.
type runSnapshotReader struct {
	hub magicevent.Hub
}

func (r runSnapshotReader) Read(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: id})
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(id), workspaceID, id)
	response, err := runCommandResponse[runevents.GetRunResult](r.hub.Send(event), runevents.TopicRunCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrNotFound
	}
	return response.Run, nil
}

// workspaceSnapshotReader is deliberately read-only. Workspace lifecycle and
// default-selection mutation stay entirely inside the Workspace owner.
type workspaceSnapshotReader struct {
	hub magicevent.Hub
}

func (r workspaceSnapshotReader) Read(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
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
	_, err := memoryCommandResponse[memoryevents.CreateMemoryResult](r.hub.Send(event), memoryevents.TopicMemoryCommandCreate)
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryCommand{WorkspaceID: workspaceID, MemoryID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandGet, "memory", string(id), workspaceID, "")
	response, err := memoryCommandResponse[memoryevents.GetMemoryResult](r.hub.Send(event), memoryevents.TopicMemoryCommandGet)
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
	ret := make([]*memorymodel.MemoryItem, 0)
	var cursor common.MemoryID
	seen := map[common.MemoryID]struct{}{}
	for page := 0; page < 1000; page++ {
		event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandListByWorkspace, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryByWorkspaceCommand{WorkspaceID: workspaceID, CursorAfterID: cursor, Limit: 200})
		bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
		response, err := memoryCommandResponse[memoryevents.ListMemoryByWorkspaceResult](r.hub.Send(event), memoryevents.TopicMemoryCommandListByWorkspace)
		if err != nil {
			return nil, err
		}
		ret = append(ret, response.Items...)
		if response.NextCursor == "" {
			return memorymodel.CloneMemoryItems(ret), nil
		}
		if _, duplicate := seen[response.NextCursor]; duplicate {
			return nil, common.ErrConflict
		}
		seen[response.NextCursor] = struct{}{}
		cursor = response.NextCursor
	}
	return nil, common.ErrConflict
}

func (r memoryEventRepository) Search(ctx context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandSearch, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.SearchMemoryCommand{Query: workorchquerycontract.MemorySearchQuery{WorkspaceID: query.WorkspaceID, Query: query.Query, Tags: append([]string(nil), query.Tags...), Types: append([]string(nil), query.Types...), Vector: append([]float64(nil), query.Vector...), MinScore: query.MinScore, Limit: query.Limit}})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "")
	response, err := memoryCommandResponse[memoryevents.SearchMemoryResult](r.hub.Send(event), memoryevents.TopicMemoryCommandSearch)
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
	_, err := memoryCommandResponse[memoryevents.UpdateMemoryResult](r.hub.Send(event), memoryevents.TopicMemoryCommandUpdate)
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
	_, err := memoryCommandResponse[memoryevents.DeleteMemoryResult](r.hub.Send(event), memoryevents.TopicMemoryCommandDelete)
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
	response, err := memoryCommandResponse[memoryevents.UpsertMemoryRecallTraceResult](r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandUpsert)
	if err != nil {
		return err
	}
	return response.Validate(item.WorkspaceID, item.RunID, item.ID)
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandGet, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.GetMemoryRecallTraceCommand{WorkspaceID: workspaceID, TraceID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), workspaceID, "")
	response, err := memoryCommandResponse[memoryevents.GetMemoryRecallTraceResult](r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Trace == nil {
		return nil, common.ErrNotFound
	}
	return response.Trace.Model()
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := query.WorkspaceID
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	query.WorkspaceID = workspaceID
	requestedLimit := query.Limit
	if requestedLimit < 0 {
		requestedLimit = 0
	}
	ret := make([]*memorymodel.MemoryRecallTrace, 0)
	cursor := query.CursorAfterID
	seen := map[common.MemoryRecallTraceID]struct{}{}
	for page := 0; page < 1000; page++ {
		pageLimit := 200
		if requestedLimit > 0 && requestedLimit-len(ret) < pageLimit {
			pageLimit = requestedLimit - len(ret)
		}
		if pageLimit <= 0 {
			return ret, nil
		}
		pageQuery := query
		pageQuery.CursorAfterID = cursor
		pageQuery.Limit = pageLimit
		event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: pageQuery})
		bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", pageQuery.WorkspaceID, pageQuery.RunID)
		response, err := memoryCommandResponse[memoryevents.ListMemoryRecallTracesResult](r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
		if err != nil {
			return nil, err
		}
		for _, item := range response.Traces {
			trace, err := item.Model()
			if err != nil {
				return nil, err
			}
			ret = append(ret, trace)
			if len(ret) > 100000 {
				return nil, common.ErrConflict
			}
		}
		if requestedLimit > 0 && len(ret) >= requestedLimit {
			return ret[:requestedLimit], nil
		}
		if response.NextCursor == "" {
			return ret, nil
		}
		if len(response.Traces) == 0 {
			return nil, common.ErrConflict
		}
		if _, duplicate := seen[response.NextCursor]; duplicate {
			return nil, common.ErrConflict
		}
		seen[response.NextCursor] = struct{}{}
		cursor = response.NextCursor
	}
	return nil, common.ErrConflict
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandDelete, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: workspaceID, TraceID: id})
	bindOwnerCommandLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), workspaceID, "")
	_, err := memoryCommandResponse[memoryevents.DeleteMemoryRecallTraceResult](r.hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandDelete)
	return err
}

func storeRuntimeStateRecordCommandFromModel(item *runmodel.Run) (runevents.StoreRuntimeStateRecordCommand, error) {
	if item == nil {
		return runevents.StoreRuntimeStateRecordCommand{}, nil
	}
	ret := runevents.StoreRuntimeStateRecordCommand{
		RunID:        item.ID,
		WorkspaceID:  item.WorkspaceID,
		RuntimeState: cloneRuntimeStateRecord(item.RuntimeState),
	}
	if item.RuntimeState != nil {
		revision := item.RuntimeState.StateRevision
		ret.ExpectedRuntimeStateRevision = &revision
		fence, err := runtimeLifecycleFenceFromModel(item)
		if err != nil {
			return runevents.StoreRuntimeStateRecordCommand{}, err
		}
		ret.LifecycleFence = fence
	}
	return ret, nil
}

func runtimeLifecycleFenceFromModel(item *runmodel.Run) (*runtimecontract.RuntimeOpaqueLifecycleFence, error) {
	if item == nil || item.RuntimeState == nil || item.RuntimeBootstrap == nil {
		return nil, nil
	}
	state := item.RuntimeState
	fence := runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, RuntimeStateRevision: state.StateRevision,
		RuntimeStateRef:  runevents.RuntimeStateRef(item.WorkspaceID, item.ID, state.StateRevision),
		RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(*state),
		BootstrapHash:    runtimecontract.StableRuntimeBootstrapHash(*item.RuntimeBootstrap),
	}
	outcome, terminal, err := runtime.DeriveTerminalOutcome(state.Decision, state.Termination)
	if err != nil {
		return nil, err
	}
	if terminal {
		fence.TerminalFenceID = runtimecontract.StableTerminalDeliveryID(item.WorkspaceID, item.ID, state.Decision.AuthoritySnapshot.ID, outcome)
	} else {
		governanceHandoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(state)
		if err != nil {
			return nil, err
		}
		if governed {
			fence.GovernanceFenceID = governanceHandoff.FenceID()
			fence.GovernanceResolutionID = governanceHandoff.ResolutionID
		}
	}
	if fence.GovernanceFenceID == "" && fence.TerminalFenceID == "" {
		return nil, nil
	}
	return &fence, nil
}

type runtimeGovernanceLifecycleHandoffKind string

const (
	runtimeGovernanceLifecycleWaitDefinitionClarification runtimeGovernanceLifecycleHandoffKind = "definition_clarification_wait"
	runtimeGovernanceLifecycleWaitDecision                runtimeGovernanceLifecycleHandoffKind = "decision_wait"
	runtimeGovernanceLifecycleWaitApproval                runtimeGovernanceLifecycleHandoffKind = "approval_wait"
	runtimeGovernanceLifecycleContinuationDecision        runtimeGovernanceLifecycleHandoffKind = "decision_continuation"
	runtimeGovernanceLifecycleContinuationApproval        runtimeGovernanceLifecycleHandoffKind = "approval_continuation"
)

// runtimeGovernanceLifecycleHandoff is the single Orchestrator-authored SM-4
// identity used both for the opaque fence persisted with the aggregate and for
// the immutable fact later delivered to the Run owner.  It is deliberately
// derived only from persisted RuntimeStateRecord facts; an application view or
// an EventHub response cannot manufacture a second lifecycle identity.
type runtimeGovernanceLifecycleHandoff struct {
	Kind                runtimeGovernanceLifecycleHandoffKind
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	TicketID            common.TicketID
	ApprovalID          common.ApprovalID
	ResolutionID        common.ResolutionID
}

func (h runtimeGovernanceLifecycleHandoff) FenceID() string {
	return runtimeGovernanceLifecycleFenceID(h.AuthoritySnapshotID, h.DispatchID)
}

func (h runtimeGovernanceLifecycleHandoff) validate() error {
	if h.Kind == "" || h.AuthoritySnapshotID == "" || h.DefinitionRevision == "" || h.EvidenceRevision == "" || h.DispatchID == "" {
		return fmt.Errorf("runtime governance lifecycle handoff is incomplete")
	}
	hasTicket := h.TicketID != ""
	hasApproval := h.ApprovalID != ""
	if hasTicket == hasApproval {
		return fmt.Errorf("runtime governance lifecycle handoff requires exactly one owner blocker")
	}
	switch h.Kind {
	case runtimeGovernanceLifecycleWaitDefinitionClarification, runtimeGovernanceLifecycleWaitDecision, runtimeGovernanceLifecycleWaitApproval:
		if h.ResolutionID != "" {
			return fmt.Errorf("runtime governance wait cannot carry resolution identity")
		}
	case runtimeGovernanceLifecycleContinuationDecision, runtimeGovernanceLifecycleContinuationApproval:
		if h.ResolutionID == "" {
			return fmt.Errorf("runtime governance continuation requires resolution identity")
		}
	default:
		return fmt.Errorf("unsupported runtime governance lifecycle handoff %q", h.Kind)
	}
	if (h.Kind == runtimeGovernanceLifecycleWaitApproval || h.Kind == runtimeGovernanceLifecycleContinuationApproval) != hasApproval {
		return fmt.Errorf("runtime governance lifecycle handoff owner kind does not match blocker")
	}
	return nil
}

func runtimeGovernanceLifecycleHandoffFromState(state *runtimecontract.RuntimeStateRecord) (runtimeGovernanceLifecycleHandoff, bool, error) {
	if state == nil {
		return runtimeGovernanceLifecycleHandoff{}, false, nil
	}
	base := runtimeGovernanceLifecycleHandoff{
		DefinitionRevision: state.Facts.DefinitionRevision,
		EvidenceRevision:   state.Facts.EvidenceRevision,
	}
	if state.Decision.State == runtimecontract.ConvergenceStateNeedGovernance {
		if state.DefinitionClarification != nil {
			clarification := state.DefinitionClarification.Clone()
			if clarification.Dispatch == nil || clarification.Request == nil {
				// Observation and sealed-dispatch are valid pre-delivery transaction
				// states. No Run lifecycle fact is authorized until the Decision
				// owner request has also been persisted.
				return runtimeGovernanceLifecycleHandoff{}, false, nil
			}
			if clarification.Dispatch.AuthoritySnapshotID != state.Decision.AuthoritySnapshot.ID ||
				clarification.Dispatch.DefinitionRevision != state.Facts.DefinitionRevision ||
				clarification.Request.DispatchID != clarification.Dispatch.DispatchID {
				return runtimeGovernanceLifecycleHandoff{}, false, fmt.Errorf("runtime Definition clarification lifecycle identity is stale")
			}
			handoff := base
			handoff.Kind = runtimeGovernanceLifecycleWaitDefinitionClarification
			handoff.AuthoritySnapshotID = clarification.Dispatch.AuthoritySnapshotID
			handoff.DispatchID = clarification.Dispatch.DispatchID
			handoff.TicketID = clarification.Request.TicketID
			if err := handoff.validate(); err != nil {
				return runtimeGovernanceLifecycleHandoff{}, false, err
			}
			return handoff, true, nil
		}
		if state.ActiveDispatch == nil {
			return runtimeGovernanceLifecycleHandoff{}, false, nil
		}
		for idx := len(state.GovernanceRequests) - 1; idx >= 0; idx-- {
			request := state.GovernanceRequests[idx]
			if request.AuthoritySnapshotID != state.Decision.AuthoritySnapshot.ID || request.DispatchID != state.ActiveDispatch.ID {
				continue
			}
			handoff := base
			handoff.AuthoritySnapshotID = request.AuthoritySnapshotID
			handoff.DispatchID = request.DispatchID
			handoff.TicketID = request.TicketID
			handoff.ApprovalID = request.ApprovalID
			if request.ApprovalID != "" {
				handoff.Kind = runtimeGovernanceLifecycleWaitApproval
			} else {
				handoff.Kind = runtimeGovernanceLifecycleWaitDecision
			}
			if err := handoff.validate(); err != nil {
				return runtimeGovernanceLifecycleHandoff{}, false, err
			}
			return handoff, true, nil
		}
		return runtimeGovernanceLifecycleHandoff{}, false, nil
	}

	if count := len(state.GovernanceResolutions); count != 0 {
		resolution := state.GovernanceResolutions[count-1]
		if resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue {
			// A rejected governance resolution is represented solely by the
			// persisted terminal fence/fact. It never authorizes a continuation.
			return runtimeGovernanceLifecycleHandoff{}, false, nil
		}
		handoff := base
		handoff.AuthoritySnapshotID = resolution.AuthoritySnapshotID
		handoff.DispatchID = resolution.DispatchID
		handoff.TicketID = resolution.TicketID
		handoff.ApprovalID = resolution.ApprovalID
		handoff.ResolutionID = resolution.ResolutionID
		if resolution.ApprovalID != "" {
			handoff.Kind = runtimeGovernanceLifecycleContinuationApproval
		} else {
			handoff.Kind = runtimeGovernanceLifecycleContinuationDecision
		}
		if err := handoff.validate(); err != nil {
			return runtimeGovernanceLifecycleHandoff{}, false, err
		}
		return handoff, true, nil
	}
	return runtimeGovernanceLifecycleHandoff{}, false, nil
}

func runtimeGovernanceLifecycleFenceID(authoritySnapshotID, dispatchID string) string {
	return "runtime-governance/" + authoritySnapshotID + "/" + dispatchID
}

func cloneRuntimeStateRecord(value *runtimecontract.RuntimeStateRecord) *runtimecontract.RuntimeStateRecord {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}
