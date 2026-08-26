package approval

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	approvalApplicationSource = "application/workorchd/appservice"
	approvalGovernanceSource  = "kernel/governance"
)

type approvalCommandFamily uint8

const (
	approvalCommandFamilyQuery approvalCommandFamily = iota + 1
	approvalCommandFamilyResolution
	approvalCommandFamilyPurge
)

func approvalCommandTopics() []string {
	return []string{
		approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.TopicApprovalCommandEnsureRejected,
		approvalevents.TopicApprovalCommandGet, approvalevents.TopicApprovalCommandList,
		approvalevents.TopicApprovalCommandListByRun, approvalevents.TopicApprovalCommandListByWorkspace,
		approvalevents.TopicApprovalCommandPurgeWorkspace,
		approvalevents.TopicApprovalCommandSealWorkspaceDeletion,
	}
}

func approvalCommandFamilyForTopic(topic string) (approvalCommandFamily, bool) {
	switch topic {
	case approvalevents.TopicApprovalCommandGet, approvalevents.TopicApprovalCommandList, approvalevents.TopicApprovalCommandListByRun, approvalevents.TopicApprovalCommandListByWorkspace:
		return approvalCommandFamilyQuery, true
	case approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.TopicApprovalCommandEnsureRejected:
		return approvalCommandFamilyResolution, true
	case approvalevents.TopicApprovalCommandPurgeWorkspace, approvalevents.TopicApprovalCommandSealWorkspaceDeletion:
		return approvalCommandFamilyPurge, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	if m == nil || m.approvals == nil || m.service == nil || ev == nil {
		setApprovalMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	ctx, err := bindApprovalCommandScope(ev.Context(), ev.Data())
	if err != nil {
		setApprovalMagicResult(result, struct{}{}, err)
		return
	}
	workspaceID := approvalCommandWorkspaceID(ev.Data())
	if workspaceID != "" {
		var unlock func()
		if approvalCommandMutatesLifecycle(ev.ID()) {
			unlock = m.lockWorkspaceLifecycleMutation(workspaceID)
		} else {
			unlock = m.lockWorkspaceMutation(workspaceID)
		}
		defer unlock()
		if ev.ID() != approvalevents.TopicApprovalCommandSealWorkspaceDeletion && ev.ID() != approvalevents.TopicApprovalCommandPurgeWorkspace {
			if err := m.rejectFencedWorkspace(ctx, workspaceID); err != nil {
				setApprovalMagicResult(result, struct{}{}, err)
				return
			}
		}
	}
	switch ev.ID() {
	case approvalevents.TopicApprovalCommandSealWorkspaceDeletion:
		response, err := m.executeSealWorkspaceDeletion(ctx, ev)
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandGet:
		response, err := m.executeGetApprovalCommand(ctx, ev.Data())
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandList:
		response, err := m.executeListApprovalsCommand(ctx, ev.Data())
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandListByRun:
		response, err := m.executeListApprovalsByRunCommand(ctx, ev.Data())
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandListByWorkspace:
		response, err := m.executeListApprovalsByWorkspaceCommand(ctx, ev.Data())
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandEnsureApproved:
		response, err := m.executeEnsureApprovedCommand(ctx, ev)
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandEnsureRejected:
		response, err := m.executeEnsureRejectedCommand(ctx, ev)
		setApprovalMagicResult(result, response, err)
	case approvalevents.TopicApprovalCommandPurgeWorkspace:
		response, err := m.executePurgeWorkspaceCommand(ctx, ev)
		setApprovalMagicResult(result, response, err)
	default:
		setApprovalMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func approvalCommandMutatesLifecycle(topic string) bool {
	switch topic {
	case approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected,
		approvalevents.TopicApprovalCommandSealWorkspaceDeletion,
		approvalevents.TopicApprovalCommandPurgeWorkspace:
		return true
	default:
		return false
	}
}

func bindApprovalCommandScope(ctx context.Context, data any) (context.Context, error) {
	var workspaceID common.WorkspaceID
	switch cmd := data.(type) {
	case approvalevents.EnsureApprovedCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.EnsureRejectedCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.GetApprovalCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.ListApprovalsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.ListApprovalsByWorkspaceCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.PurgeWorkspaceApprovalsCommand:
		workspaceID = cmd.WorkspaceID
	case approvalevents.SealWorkspaceApprovalDeletionCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID == "" {
		return ctx, nil
	}
	return common.BindRuntimeWorkspaceContext(ctx, workspaceID)
}

func approvalCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case approvalevents.EnsureApprovedCommand:
		return cmd.WorkspaceID
	case approvalevents.EnsureRejectedCommand:
		return cmd.WorkspaceID
	case approvalevents.GetApprovalCommand:
		return cmd.WorkspaceID
	case approvalevents.ListApprovalsByRunCommand:
		return cmd.WorkspaceID
	case approvalevents.ListApprovalsByWorkspaceCommand:
		return cmd.WorkspaceID
	case approvalevents.PurgeWorkspaceApprovalsCommand:
		return cmd.WorkspaceID
	case approvalevents.SealWorkspaceApprovalDeletionCommand:
		return cmd.WorkspaceID
	}
	return ""
}

func validateApprovalResolutionSource(ev magicevent.Event, principal approvalmodel.ApprovalResolutionPrincipal) error {
	if ev == nil || principal.Validate() != nil {
		return common.ErrInvalidArgument
	}
	principal = principal.Normalized()
	switch ev.Source() {
	case approvalApplicationSource:
		if principal.Kind != approvalmodel.ApprovalResolutionPrincipalHumanOperator || principal.SubjectID != approvalApplicationSource {
			return common.ErrInvalidArgument
		}
	case approvalGovernanceSource:
		if principal.Kind != approvalmodel.ApprovalResolutionPrincipalAutoGovernance || principal.SubjectID != approvalGovernanceSource {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}

func setApprovalMagicResult[Result any](result magicevent.Result, data Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
