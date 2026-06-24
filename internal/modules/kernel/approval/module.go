package approval

import (
	"context"

	approvalbiz "github.com/muidea/workorch/internal/modules/kernel/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/kernel/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = approvalservice.Service
type CreateApprovalCommand = approvalcontracts.CreateApprovalCommand
type UpdateApprovalCommand = approvalcontracts.UpdateApprovalCommand
type ResolveApprovalCommand = approvalcontracts.ResolveApprovalCommand

type Dependencies struct {
	Approvals       repository.ApprovalRepository
	EvidenceRecords repository.EvidenceRecordRepository
	EventHub        events.Hub
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
	m.bizPtr = approvalbiz.NewService(deps.Approvals, deps.EventHub, deps.EvidenceRecords)
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
	m.unsub = append(m.unsub, m.eventHub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, m.handleEnsureApprovedCommand))
	m.unsub = append(m.unsub, m.eventHub.Subscribe(approvalevents.TopicApprovalCommandEnsureRejected, m.handleEnsureRejectedCommand))
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandEnsure, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandCreate, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandGet, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandList, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandListByRun, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandListByWorkspace, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandUpdate, m.handleApprovalCommand),
		m.eventHub.Subscribe(approvalevents.TopicApprovalCommandDelete, m.handleApprovalCommand),
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
	cmd := resolveApprovalCommandFromPayload(envelope.Payload)
	if workspaceID := resolveApprovalScopedWorkspaceID(cmd, envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	approvalID := cmd.ApprovalID
	if resolveApprovalScopedWorkspaceID(cmd, envelope) == "" {
		if err := m.ensureApprovalScope(ctx, approvalID, envelope); err != nil {
			return err
		}
	}
	item, err := m.servicePtr.EnsureApprovedAndReturn(ctx, cmd)
	if err != nil {
		return err
	}
	if envelopeHasScope(envelope) && !approvalMatchesScope(item, envelope) {
		return common.ErrNotFound
	}
	return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
}

func (m *Module) handleEnsureRejectedCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	cmd := resolveApprovalCommandFromPayload(envelope.Payload)
	if workspaceID := resolveApprovalScopedWorkspaceID(cmd, envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	approvalID := cmd.ApprovalID
	if resolveApprovalScopedWorkspaceID(cmd, envelope) == "" {
		if err := m.ensureApprovalScope(ctx, approvalID, envelope); err != nil {
			return err
		}
	}
	item, err := m.servicePtr.EnsureRejectedAndReturn(ctx, cmd)
	if err != nil {
		return err
	}
	if envelopeHasScope(envelope) && !approvalMatchesScope(item, envelope) {
		return common.ErrNotFound
	}
	return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
}

func (m *Module) handleApprovalCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	if workspaceID := approvalScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case approvalevents.TopicApprovalCommandEnsure:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.CreateApprovalCommand)
		if !ok {
			item, err := approvalFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = approvalcontracts.CreateApprovalCommand{
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
	case approvalevents.TopicApprovalCommandCreate:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.CreateApprovalCommand)
		if !ok {
			item, err := approvalFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
			cmd = approvalCreateCommandFromModel(item)
		}
		if err := m.servicePtr.CreateApproval(ctx, cmd); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	case approvalevents.TopicApprovalCommandGet:
		item, err := m.approvals.Get(ctx, common.ApprovalID(stringPayload(envelope.Payload, events.PayloadKeyApprovalID)))
		if err != nil {
			return err
		}
		if !approvalMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyApproval: item})
	case approvalevents.TopicApprovalCommandList:
		items, err := m.approvals.List(ctx)
		if err != nil {
			return err
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case approvalevents.TopicApprovalCommandListByRun:
		items, err := m.approvals.ListByRunID(ctx, common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)))
		if err != nil {
			return err
		}
		items = filterApprovalsByScope(items, envelope)
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case approvalevents.TopicApprovalCommandListByWorkspace:
		items, err := m.approvals.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return approvalCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case approvalevents.TopicApprovalCommandUpdate:
		var item *approvalmodel.Approval
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.UpdateApprovalCommand); ok {
			item = approvalFromUpdateCommand(cmd)
		} else {
			var err error
			item, err = approvalFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if !approvalMatchesScope(item, envelope) {
			return common.ErrNotFound
		}
		if err := m.approvals.Update(ctx, item); err != nil {
			return err
		}
		return approvalCommandResponse(envelope, nil)
	case approvalevents.TopicApprovalCommandDelete:
		approvalID := deleteApprovalIDFromPayload(envelope.Payload)
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

func approvalCreateCommandFromModel(item *approvalmodel.Approval) approvalcontracts.CreateApprovalCommand {
	if item == nil {
		return approvalcontracts.CreateApprovalCommand{}
	}
	return approvalcontracts.CreateApprovalCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		Reason:      item.Reason,
		Action:      item.Action,
		Context:     cloneMap(item.Context),
	}
}

func approvalFromUpdateCommand(cmd approvalcontracts.UpdateApprovalCommand) *approvalmodel.Approval {
	return &approvalmodel.Approval{
		ID:          cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		Action:      cmd.Action,
		Context:     cloneMap(cmd.Context),
		State:       cmd.State,
		CreatedAt:   cmd.CreatedAt,
		UpdatedAt:   cmd.UpdatedAt,
	}
}

func resolveApprovalCommandFromPayload(payload map[string]any) approvalcontracts.ResolveApprovalCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand); ok {
		return cmd
	}
	return approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  common.ApprovalID(stringPayload(payload, events.PayloadKeyApprovalID)),
		WorkspaceID: common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		RunID:       common.RunID(stringPayload(payload, events.PayloadKeyRunID)),
		Context:     mapPayload(payload, events.PayloadKeyContext),
	}
}

func deleteApprovalIDFromPayload(payload map[string]any) common.ApprovalID {
	if cmd, ok := payload[events.PayloadKeyCommand].(approvalcontracts.DeleteApprovalCommand); ok && cmd.ApprovalID != "" {
		return cmd.ApprovalID
	}
	return common.ApprovalID(stringPayload(payload, events.PayloadKeyApprovalID))
}

func (m *Module) ensureApprovalScope(ctx context.Context, approvalID common.ApprovalID, envelope events.Envelope) error {
	if approvalID == "" {
		return common.ErrInvalidArgument
	}
	if m == nil || m.approvals == nil {
		return common.ErrInvalidArgument
	}
	if !envelopeHasScope(envelope) {
		_, err := m.resolveUniqueApprovalForUnscopedCommand(ctx, approvalID)
		return err
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

func envelopeHasScope(envelope events.Envelope) bool {
	return stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID) != "" ||
		stringPayload(envelope.Payload, events.PayloadKeyRunID) != "" ||
		stringPayload(envelope.Payload, events.PayloadKeyStepID) != "" ||
		envelope.WorkspaceID != "" ||
		envelope.RunID != "" ||
		envelope.StepID != ""
}

func approvalScopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
}

func resolveApprovalScopedWorkspaceID(cmd approvalcontracts.ResolveApprovalCommand, envelope events.Envelope) common.WorkspaceID {
	if cmd.WorkspaceID != "" {
		return cmd.WorkspaceID
	}
	return approvalScopedWorkspaceID(envelope)
}

func approvalMatchesScope(item *approvalmodel.Approval, envelope events.Envelope) bool {
	if item == nil {
		return false
	}
	workspaceID := approvalScopedWorkspaceID(envelope)
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
	return cloneMap(value)
}

func cloneMap(value map[string]any) map[string]any {
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
