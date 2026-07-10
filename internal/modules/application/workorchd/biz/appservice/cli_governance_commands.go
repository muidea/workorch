package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleGovernanceCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandDecisionResolve:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveDecisionWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveDecisionCommandFromPayload(cmd.Payload))
		if err == nil {
			err = d.publishDecisionContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandDecisionList:
		items, err := ListDecisionsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandDecisionGet:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ReadDecisionWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandDecisionBatch:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		items, err := BatchResolveDecisionsWithSession(ctx, d.borrow(), clicore.BatchResolveDecisionCommandFromPayload(cmd.Payload))
		if err == nil {
			err = d.publishDecisionContinuations(ctx, cmd, items)
		}
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandApprovalApprove:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveApprovalCommandFromPayload("approve", cmd.Payload))
		if err == nil {
			err = d.publishApprovalContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandApprovalReject:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveApprovalCommandFromPayload("reject", cmd.Payload))
		if err == nil {
			err = d.publishApprovalContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandApprovalList:
		items, err := ListApprovalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandApprovalGet:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ReadApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandApprovalBatch:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		items, err := BatchResolveApprovalsWithSession(ctx, d.borrow(), clicore.BatchResolveApprovalCommandFromPayload(cmd.Payload))
		if err == nil {
			err = d.publishApprovalContinuations(ctx, cmd, items)
		}
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}
