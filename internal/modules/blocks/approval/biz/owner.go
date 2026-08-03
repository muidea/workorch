package approval

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Approvals         repository.ApprovalRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service   *Service
	approvals repository.ApprovalRepository
	topics    []string
	started   bool
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Approvals == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: NewService(deps.Approvals), approvals: deps.Approvals}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.approvals == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = []string{
		approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected,
		approvalevents.TopicApprovalCommandGet,
		approvalevents.TopicApprovalCommandList,
		approvalevents.TopicApprovalCommandListByRun,
		approvalevents.TopicApprovalCommandListByWorkspace,
		approvalevents.TopicApprovalCommandDelete,
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
	m.approvals = nil
	m.started = false
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setApprovalMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (approvalevents.ApprovalCommandResult, error) {
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()
	if m == nil || m.approvals == nil || m.service == nil {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case approvalevents.DeleteApprovalCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.EnsureApprovedCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.GetApprovalCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.ListApprovalsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.ListApprovalsByWorkspaceCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID != "" {
		var scopeErr error
		ctx, scopeErr = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if scopeErr != nil {
			return approvalevents.ApprovalCommandResult{}, scopeErr
		}
	}
	switch topic {
	case approvalevents.TopicApprovalCommandEnsureApproved:
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		scope := approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
		prior, err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, scope)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		item, err := m.service.EnsureApprovedAndReturn(ctx, resolveApprovalContract(cmd))
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if err := m.recordApprovalGovernanceEvidence(ctx, item); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if prior != nil && prior.State == common.ApprovalStatePending {
			m.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, item)
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
		prior, err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, scope)
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		item, err := m.service.EnsureRejectedAndReturn(ctx, resolveApprovalContract(cmd))
		if err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if err := m.recordApprovalGovernanceEvidence(ctx, item); err != nil {
			return approvalevents.ApprovalCommandResult{}, err
		}
		if prior != nil && prior.State == common.ApprovalStatePending {
			m.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, item)
		}
		if scope.hasScope() && !approvalMatchesCommandScope(item, scope) {
			return approvalevents.ApprovalCommandResult{}, common.ErrNotFound
		}
		return approvalevents.ApprovalCommandResult{Approval: item}, nil
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
	case approvalevents.TopicApprovalCommandDelete:
		cmd, ok := data.(approvalevents.DeleteApprovalCommand)
		if !ok || cmd.ApprovalID == "" {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		if _, err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID}); err != nil {
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

func (m *Biz) publishApprovalChanged(ctx context.Context, topic string, item *approvalmodel.Approval) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, approvalevents.ApprovalChangedData{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
		Reason: item.Reason, Action: item.Action, Context: item.Context.Clone().Fields, State: item.State,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(approvalevents.Contracts(), topic, workorchevents.EventHubLaneScope{SubjectType: "approval", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID)}))
	m.PostEvent(event)
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

func resolveApprovalContract(cmd approvalevents.ResolveApprovalCommand) approvalcontracts.ResolveApprovalCommand {
	return approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		Context:     workorcheventpayload.New(cmd.Context...),
	}
}

func (m *Biz) ensureApprovalScopeForCommand(ctx context.Context, approvalID common.ApprovalID, scope approvalCommandScope) (*approvalmodel.Approval, error) {
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return nil, common.ErrInvalidArgument
	}
	if !scope.hasScope() {
		return m.resolveUniqueApprovalForUnscopedCommand(ctx, approvalID)
	}
	item, err := m.approvals.Get(ctx, approvalID)
	if err != nil {
		return nil, err
	}
	if !approvalMatchesCommandScope(item, scope) {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (m *Biz) resolveUniqueApprovalForUnscopedCommand(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
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
