package approval

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) executeApprovalResolutionCommand(ctx context.Context, ev magicevent.Event) (approvalevents.ApprovalCommandResult, error) {
	switch ev.ID() {
	case approvalevents.TopicApprovalCommandEnsureApproved:
		cmd, ok := ev.Data().(approvalevents.EnsureApprovedCommand)
		if !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		return m.ensureApprovalResolution(ctx, approvalevents.ResolveApprovalCommand(cmd), true)
	case approvalevents.TopicApprovalCommandEnsureRejected:
		cmd, ok := ev.Data().(approvalevents.EnsureRejectedCommand)
		if !ok {
			return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
		}
		return m.ensureApprovalResolution(ctx, approvalevents.ResolveApprovalCommand(cmd), false)
	default:
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) ensureApprovalResolution(ctx context.Context, cmd approvalevents.ResolveApprovalCommand, approved bool) (approvalevents.ApprovalCommandResult, error) {
	if cmd.ApprovalID == "" {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	scope := approvalCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
	prior, err := m.ensureApprovalScopeForCommand(ctx, cmd.ApprovalID, scope)
	if err != nil {
		return approvalevents.ApprovalCommandResult{}, err
	}
	var item *approvalmodel.Approval
	if approved {
		item, err = m.service.EnsureApprovedAndReturn(ctx, resolveApprovalContract(cmd))
	} else {
		item, err = m.service.EnsureRejectedAndReturn(ctx, resolveApprovalContract(cmd))
	}
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
}

func (m *Biz) executeDeleteApprovalCommand(ctx context.Context, ev magicevent.Event) (approvalevents.ApprovalCommandResult, error) {
	if ev.ID() != approvalevents.TopicApprovalCommandDelete {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(approvalevents.DeleteApprovalCommand)
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
}

func resolveApprovalContract(cmd approvalevents.ResolveApprovalCommand) approvalcontracts.ResolveApprovalCommand {
	return approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		Context:     workorcheventpayload.New(cmd.Context...),
	}
}
