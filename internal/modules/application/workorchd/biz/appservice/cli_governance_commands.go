package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleGovernanceCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandDecisionResolve:
		item, err := ResolveDecisionWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.ResolveDecisionCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandDecisionList:
		items, err := ListDecisionsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case events.CommandDecisionGet:
		item, err := ReadDecisionWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandDecisionBatch:
		items, err := BatchResolveDecisionsWithSession(ctx, d.borrow(), clicore.BatchResolveDecisionCommandFromPayload(cmd.Payload))
		return nil, clicore.ResourceMaps(items), err
	case events.CommandApprovalApprove:
		item, err := ApproveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalReject:
		item, err := RejectApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalList:
		items, err := ListApprovalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, clicore.ResourceMaps(items), err
	case events.CommandApprovalGet:
		item, err := ReadApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandApprovalBatch:
		items, err := BatchResolveApprovalsWithSession(ctx, d.borrow(), clicore.BatchResolveApprovalCommandFromPayload(cmd.Payload))
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}
