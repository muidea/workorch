package approval

import (
	"context"

	approvalbiz "github.com/muidea/workorch/internal/modules/kernel/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/kernel/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = approvalservice.Service
type CreateApprovalCommand = approvalservice.CreateApprovalCommand
type ResolveApprovalCommand = approvalservice.ResolveApprovalCommand

type Dependencies struct {
	Approvals repository.ApprovalRepository
	EventHub  events.Hub
}

type Module struct {
	bizPtr     *approvalbiz.Service
	servicePtr *approvalservice.Module
	eventHub   events.Hub
	approvals  repository.ApprovalRepository
	unsub      []func()
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
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub, m.eventHub.Subscribe(events.TopicApprovalCommandEnsureApproved, m.handleEnsureApprovedCommand))
	m.unsub = append(m.unsub, m.eventHub.Subscribe(events.TopicApprovalCommandEnsureRejected, m.handleEnsureRejectedCommand))
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicApprovalCommandEnsure, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandCreate, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandGet, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandList, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandListByRun, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandListByWorkspace, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandUpdate, m.handleApprovalCommand),
		m.eventHub.Subscribe(events.TopicApprovalCommandDelete, m.handleApprovalCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	m.approvals = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleEnsureApprovedCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	approvalID := common.ApprovalID(stringPayload(envelope.Payload, events.PayloadKeyApprovalID))
	if err := m.ensureApprovalScope(ctx, approvalID, envelope); err != nil {
		return err
	}
	item, err := m.servicePtr.EnsureApprovedAndReturn(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID})
	if err != nil {
		return err
	}
	return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
}

func (m *Module) handleEnsureRejectedCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	approvalID := common.ApprovalID(stringPayload(envelope.Payload, events.PayloadKeyApprovalID))
	if err := m.ensureApprovalScope(ctx, approvalID, envelope); err != nil {
		return err
	}
	item, err := m.servicePtr.EnsureRejectedAndReturn(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID})
	if err != nil {
		return err
	}
	return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
}

func (m *Module) handleApprovalCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicApprovalCommandEnsure:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalservice.CreateApprovalCommand)
		if !ok {
			item, err := approvalFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = approvalservice.CreateApprovalCommand{
				ApprovalID:  item.ID,
				WorkspaceID: item.WorkspaceID,
				RunID:       item.RunID,
				StepID:      item.StepID,
				Reason:      item.Reason,
				Action:      item.Action,
				Context:     item.Context,
			}
		}
		if err := m.servicePtr.EnsureApproval(ctx, cmd); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	case events.TopicApprovalCommandCreate:
		item, err := approvalFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateApproval(ctx, approvalservice.CreateApprovalCommand{
			ApprovalID:  item.ID,
			WorkspaceID: item.WorkspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
			Reason:      item.Reason,
			Action:      item.Action,
			Context:     item.Context,
		}); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	case events.TopicApprovalCommandGet:
		item, err := m.approvals.Get(ctx, common.ApprovalID(stringPayload(envelope.Payload, events.PayloadKeyApprovalID)))
		if err != nil {
			return err
		}
		if !approvalMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
	case events.TopicApprovalCommandList:
		items, err := m.approvals.List(ctx)
		if err != nil {
			return err
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicApprovalCommandListByRun:
		items, err := m.approvals.ListByRunID(ctx, common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)))
		if err != nil {
			return err
		}
		items = filterApprovalsByScope(items, envelope)
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicApprovalCommandListByWorkspace:
		items, err := m.approvals.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicApprovalCommandUpdate:
		item, err := approvalFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if !approvalMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		if err := m.approvals.Update(ctx, item); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	case events.TopicApprovalCommandDelete:
		approvalID := common.ApprovalID(stringPayload(envelope.Payload, events.PayloadKeyApprovalID))
		if err := m.ensureApprovalScope(ctx, approvalID, envelope); err != nil {
			return err
		}
		if err := m.approvals.Delete(ctx, approvalID); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func (m *Module) ensureApprovalScope(ctx context.Context, approvalID common.ApprovalID, envelope events.Envelope) error {
	if approvalID == "" {
		return common.ErrInvalidArgument
	}
	if !envelopeHasScope(envelope) {
		return nil
	}
	if m == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	item, err := m.approvals.Get(ctx, approvalID)
	if err != nil {
		return err
	}
	if !approvalMatchesScope(item, envelope) {
		return common.ErrNotFound
	}
	return nil
}

func envelopeHasScope(envelope events.Envelope) bool {
	return stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID) != "" ||
		stringPayload(envelope.Payload, events.PayloadKeyRunID) != "" ||
		stringPayload(envelope.Payload, events.PayloadKeyStepID) != "" ||
		envelope.WorkspaceID != "" ||
		envelope.RunID != "" ||
		envelope.StepID != ""
}

func approvalMatchesScope(item *approvalmodel.Approval, envelope events.Envelope) bool {
	if item == nil {
		return false
	}
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	if workspaceID != "" && item.WorkspaceID != workspaceID {
		return false
	}
	runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
	if runID == "" {
		runID = common.RunID(envelope.RunID)
	}
	if runID != "" && item.RunID != runID {
		return false
	}
	stepID := common.StepID(stringPayload(envelope.Payload, events.PayloadKeyStepID))
	if stepID == "" {
		stepID = common.StepID(envelope.StepID)
	}
	if stepID != "" && item.StepID != stepID {
		return false
	}
	return true
}

func filterApprovalsByScope(items []*approvalmodel.Approval, envelope events.Envelope) []*approvalmodel.Approval {
	if len(items) == 0 {
		return items
	}
	ret := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if approvalMatchesScope(item, envelope) {
			ret = append(ret, item)
		}
	}
	return ret
}

func approvalFromPayload(payload map[string]any) (*approvalmodel.Approval, error) {
	item, ok := payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if ok && item != nil {
		return item, nil
	}
	approvalID := common.ApprovalID(stringPayload(payload, events.PayloadKeyApprovalID))
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	return &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		RunID:       common.RunID(stringPayload(payload, events.PayloadKeyRunID)),
		StepID:      common.StepID(stringPayload(payload, events.PayloadKeyStepID)),
		Reason:      stringPayload(payload, events.PayloadKeyReason),
		Action:      stringPayload(payload, events.PayloadKeyAction),
		Context:     mapPayload(payload, events.PayloadKeyContext),
	}, nil
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func mapPayload(payload map[string]any, key string) map[string]any {
	value, _ := payload[key].(map[string]any)
	if value == nil {
		return nil
	}
	ret := make(map[string]any, len(value))
	for key, item := range value {
		ret[key] = item
	}
	return ret
}

func approvalCommandResponse(request events.Envelope, payload map[string]any) error {
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
