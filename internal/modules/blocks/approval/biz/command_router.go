package approval

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type approvalCommandFamily uint8

const (
	approvalCommandFamilyQuery approvalCommandFamily = iota + 1
	approvalCommandFamilyResolution
	approvalCommandFamilyDelete
)

func approvalCommandTopics() []string {
	return []string{
		approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected,
		approvalevents.TopicApprovalCommandGet,
		approvalevents.TopicApprovalCommandList,
		approvalevents.TopicApprovalCommandListByRun,
		approvalevents.TopicApprovalCommandListByWorkspace,
		approvalevents.TopicApprovalCommandDelete,
	}
}

func approvalCommandFamilyForTopic(topic string) (approvalCommandFamily, bool) {
	switch topic {
	case approvalevents.TopicApprovalCommandGet,
		approvalevents.TopicApprovalCommandList,
		approvalevents.TopicApprovalCommandListByRun,
		approvalevents.TopicApprovalCommandListByWorkspace:
		return approvalCommandFamilyQuery, true
	case approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected:
		return approvalCommandFamilyResolution, true
	case approvalevents.TopicApprovalCommandDelete:
		return approvalCommandFamilyDelete, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setApprovalMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (approvalevents.ApprovalCommandResult, error) {
	if m == nil || m.approvals == nil || m.service == nil || ev == nil {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	ctx, err := bindApprovalCommandScope(ev.Context(), ev.Data())
	if err != nil {
		return approvalevents.ApprovalCommandResult{}, err
	}
	family, ok := approvalCommandFamilyForTopic(ev.ID())
	if !ok {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	switch family {
	case approvalCommandFamilyQuery:
		return m.executeApprovalQueryCommand(ctx, ev)
	case approvalCommandFamilyResolution:
		return m.executeApprovalResolutionCommand(ctx, ev)
	case approvalCommandFamilyDelete:
		return m.executeDeleteApprovalCommand(ctx, ev)
	default:
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
}

func bindApprovalCommandScope(ctx context.Context, data any) (context.Context, error) {
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case approvalevents.DeleteApprovalCommand:
		workspaceID = cmd.WorkspaceID
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
	}
	if workspaceID == "" {
		return ctx, nil
	}
	return common.BindRuntimeWorkspaceContext(ctx, workspaceID)
}

func setApprovalMagicResult(result magicevent.Result, data approvalevents.ApprovalCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data.Clone(), nil)
}
