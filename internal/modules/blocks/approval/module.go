package approval

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = approvalservice.Service
type CreateApprovalCommand = approvalcontracts.CreateApprovalCommand
type UpdateApprovalCommand = approvalcontracts.UpdateApprovalCommand
type ResolveApprovalCommand = approvalcontracts.ResolveApprovalCommand

type Dependencies struct {
	Approvals repository.ApprovalRepository
	EventHub  magicevent.Hub
}

type Module struct {
	bizPtr     *approvalbiz.Service
	servicePtr *approvalservice.Module
	eventHub   magicevent.Hub
	approvals  repository.ApprovalRepository
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Approvals == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.approvals = deps.Approvals
	m.bizPtr = approvalbiz.NewService(deps.Approvals, deps.EventHub)
	m.servicePtr = approvalservice.New(approvalservice.Dependencies{
		CreateApproval:          m.bizPtr.CreateApproval,
		EnsureApproval:          m.bizPtr.EnsureApproval,
		Approve:                 m.bizPtr.Approve,
		EnsureApproved:          m.bizPtr.EnsureApproved,
		EnsureApprovedAndReturn: m.bizPtr.EnsureApprovedAndReturn,
		Reject:                  m.bizPtr.Reject,
		EnsureRejected:          m.bizPtr.EnsureRejected,
		EnsureRejectedAndReturn: m.bizPtr.EnsureRejectedAndReturn,
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
		approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected,
		approvalevents.TopicApprovalCommandEnsure,
		approvalevents.TopicApprovalCommandCreate,
		approvalevents.TopicApprovalCommandGet,
		approvalevents.TopicApprovalCommandList,
		approvalevents.TopicApprovalCommandListByRun,
		approvalevents.TopicApprovalCommandListByWorkspace,
		approvalevents.TopicApprovalCommandUpdate,
		approvalevents.TopicApprovalCommandDelete,
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
	m.approvals = nil
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
	setApprovalMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (approvalevents.ApprovalCommandResult, error) {
	if m == nil || m.approvals == nil || m.servicePtr == nil {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID := approvalCommandWorkspaceID(data); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch topic {
	case approvalevents.TopicApprovalCommandEnsureApproved:
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		scope := approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
		if err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, scope); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		item, err := m.servicePtr.EnsureApprovedAndReturn(ctx, resolveApprovalContract(cmd))
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if scope.hasScope() && !approvalMatchesCommandScope(item, scope) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
	case approvalevents.TopicApprovalCommandEnsureRejected:
		cmd, ok := data.(approvalevents.EnsureRejectedCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		scope := approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
		if err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, scope); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		item, err := m.servicePtr.EnsureRejectedAndReturn(ctx, resolveApprovalContract(cmd))
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if scope.hasScope() && !approvalMatchesCommandScope(item, scope) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
	case approvalevents.TopicApprovalCommandEnsure:
		cmd, ok := data.(approvalevents.EnsureApprovalCommand)
		if !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.EnsureApproval(ctx, createApprovalContract(cmd)); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{}, nil
	case approvalevents.TopicApprovalCommandCreate:
		cmd, ok := data.(approvalevents.CreateApprovalCommand)
		if !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateApproval(ctx, createApprovalContract(cmd)); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{}, nil
	case approvalevents.TopicApprovalCommandGet:
		cmd, ok := data.(approvalevents.GetApprovalCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.approvals.Get(ctx, cmd.ApprovalID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if !approvalMatchesCommandScope(item, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
	case approvalevents.TopicApprovalCommandList:
		if _, ok := data.(approvalevents.ListApprovalsCommand); !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.List(ctx)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	case approvalevents.TopicApprovalCommandListByRun:
		cmd, ok := data.(approvalevents.ListApprovalsByRunCommand)
		if !ok || cmd.RunID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.ListByRunID(ctx, cmd.RunID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		items = filterApprovalsByCommandScope(items, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	case approvalevents.TopicApprovalCommandListByWorkspace:
		cmd, ok := data.(approvalevents.ListApprovalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.approvals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{Approvals: items}, nil
	case approvalevents.TopicApprovalCommandUpdate:
		cmd, ok := data.(approvalevents.UpdateApprovalCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		item := approvalFromUpdateCommand(cmd)
		if !approvalMatchesCommandScope(item, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		if err := m.approvals.Update(ctx, item); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		m.publishApprovalUpdated(ctx, item)
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
	case approvalevents.TopicApprovalCommandDelete:
		cmd, ok := data.(approvalevents.DeleteApprovalCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if err := m.approvals.Delete(ctx, cmd.ApprovalID); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		return approvalevents.ApprovalCommandResult{}, nil
	default:
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Module) publishApprovalUpdated(ctx context.Context, item *approvalmodel.Approval) {
	if m == nil || m.eventHub == nil || item == nil {
		return
	}
	topic := approvalevents.TopicApprovalResolved
	if item.State == common.ApprovalStateSuperseded {
		topic = approvalevents.TopicApprovalSuperseded
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, approvalevents.ApprovalChangedData{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
		Reason: item.Reason, Action: item.Action, State: item.State,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "approval", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID)}))
	m.eventHub.Post(event)
}

func setApprovalMagicResult(result magicevent.Result, data approvalevents.ApprovalCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

type approvalCommandScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

func (s approvalCommandScope) hasScope() bool {
	return s.WorkspaceID != "" || s.RunID != "" || s.StepID != ""
}

func approvalCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case approvalevents.CreateApprovalCommand:
		return cmd.WorkspaceID
	case approvalevents.UpdateApprovalCommand:
		return cmd.WorkspaceID
	case approvalevents.DeleteApprovalCommand:
		return cmd.WorkspaceID
	case approvalevents.EnsureApprovedCommand:
		return cmd.WorkspaceID
	case approvalevents.GetApprovalCommand:
		return cmd.WorkspaceID
	case approvalevents.ListApprovalsByRunCommand:
		return cmd.WorkspaceID
	case approvalevents.ListApprovalsByWorkspaceCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func approvalMatchesCommandScope(item *approvalmodel.Approval, scope approvalCommandScope) bool {
	if item == nil {
		return false
	}
	if scope.WorkspaceID != "" && item.WorkspaceID != scope.WorkspaceID {
		return false
	}
	if scope.RunID != "" && item.RunID != scope.RunID {
		return false
	}
	if scope.StepID != "" && item.StepID != scope.StepID {
		return false
	}
	return true
}

func filterApprovalsByCommandScope(items []*approvalmodel.Approval, scope approvalCommandScope) []*approvalmodel.Approval {
	if len(items) == 0 || !scope.hasScope() {
		return items
	}
	ret := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if approvalMatchesCommandScope(item, scope) {
			ret = append(ret, item)
		}
	}
	return ret
}

func createApprovalContract(cmd approvalevents.CreateApprovalCommand) approvalcontracts.CreateApprovalCommand {
	return approvalcontracts.CreateApprovalCommand{
		ApprovalID:     cmd.ApprovalID,
		WorkspaceID:    cmd.WorkspaceID,
		RunID:          cmd.RunID,
		StepID:         cmd.StepID,
		Reason:         cmd.Reason,
		Action:         cmd.Action,
		Context:        cmd.Context.Map(),
		AutoGovernance: cmd.AutoGovernance.Map(),
	}
}

func resolveApprovalContract(cmd approvalevents.ResolveApprovalCommand) approvalcontracts.ResolveApprovalCommand {
	return approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		Context:     cmd.Context.Map(),
	}
}

func approvalFromUpdateCommand(cmd approvalevents.UpdateApprovalCommand) *approvalmodel.Approval {
	return &approvalmodel.Approval{
		ID:             cmd.ApprovalID,
		WorkspaceID:    cmd.WorkspaceID,
		RunID:          cmd.RunID,
		StepID:         cmd.StepID,
		Reason:         cmd.Reason,
		Action:         cmd.Action,
		Context:        cmd.Context.Map(),
		State:          cmd.State,
		AutoGovernance: cmd.AutoGovernance.Map(),
		ResolvedAt:     cmd.ResolvedAt,
		ResolvedBy:     cmd.ResolvedBy,
		CreatedAt:      cmd.CreatedAt,
		UpdatedAt:      cmd.UpdatedAt,
	}
}

func (m *Module) ensureApprovalScopeForCommand(ctx context.Context, approvalID common.ApprovalID, scope approvalCommandScope) error {
	if approvalID == "" {
		return common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	if !scope.hasScope() {
		_, err := m.resolveUniqueApprovalForUnscopedCommand(ctx, approvalID)
		return err
	}
	item, err := m.approvals.Get(ctx, approvalID)
	if err != nil {
		return err
	}
	if !approvalMatchesCommandScope(item, scope) {
		return common.ErrNotFound
	}
	return nil
}

func (m *Module) resolveUniqueApprovalForUnscopedCommand(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := m.approvals.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []*approvalmodel.Approval
	var pendingMatches []*approvalmodel.Approval
	for _, item := range items {
		if item == nil || item.ID != approvalID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.ApprovalStatePending {
			pendingMatches = append(pendingMatches, item)
		}
	}
	switch {
	case len(pendingMatches) == 1:
		return pendingMatches[0], nil
	case len(pendingMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}

func cloneMap(value map[string]any) map[string]any {
	ret := make(map[string]any, len(value))
	for key, item := range value {
		ret[key] = item
	}
	return ret
}
