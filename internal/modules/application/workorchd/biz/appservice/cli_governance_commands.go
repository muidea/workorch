package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleGovernanceCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandDecisionResolve:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveDecisionWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveDecisionCommandFromPayload(cmd.Payload))
		if err == nil {
			err = d.publishDecisionContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case events.CommandDecisionList:
		items, err := ListDecisionsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case events.CommandDecisionGet:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ReadDecisionWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandDecisionBatch:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		items, err := BatchResolveDecisionsWithSession(ctx, d.borrow(), clicore.BatchResolveDecisionCommandFromPayload(cmd.Payload))
		if err == nil {
			err = d.publishDecisionContinuations(ctx, cmd, items)
		}
		return nil, clicore.ResourceMaps(items), err
	case events.CommandApprovalApprove:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveApprovalCommandFromPayload("approve", cmd.Payload))
		if err == nil {
			err = d.publishApprovalContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalReject:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveApprovalCommandFromPayload("reject", cmd.Payload))
		if err == nil {
			err = d.publishApprovalContinuation(ctx, cmd, item)
		}
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalList:
		items, err := ListApprovalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case events.CommandApprovalGet:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ReadApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalBatch:
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
