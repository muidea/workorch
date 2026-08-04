package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func BatchResolveApprovalViews(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, cmd appmodelcore.BatchResolveApprovalCommand, scope common.WorkspaceID) ([]workorchprojectioncontract.Approval, error) {
	semantic := approvalSemanticFromCommand(appmodelcore.ResolveApprovalCommand{Action: cmd.Action, Payload: cmd.Payload}, "")
	action, err := governancecore.NormalizeApprovalBatchAction(semantic.Action)
	if err != nil {
		return nil, err
	}
	if err := governancecore.ValidateApprovalIDs(cmd.ApprovalIDs); err != nil {
		return nil, err
	}
	type approvalRead struct {
		item *approvalmodel.Approval
		err  error
	}
	readCtx := contextWithGovernanceScope(ctx, scope)
	reads := make([]approvalRead, len(cmd.ApprovalIDs))
	for i, approvalID := range cmd.ApprovalIDs {
		reads[i].item, reads[i].err = ReadApproval(readCtx, hub, common.ApprovalID(approvalID))
	}
	for _, r := range reads {
		if r.err != nil {
			return nil, r.err
		}
		if err := governancecore.EnsureApprovalWorkspaceScope(scope, r.item); err != nil {
			return nil, err
		}
		if r.item.State != common.ApprovalStatePending {
			return nil, common.ErrConflict
		}
	}

	ret := make([]workorchprojectioncontract.Approval, 0, len(cmd.ApprovalIDs))
	for _, r := range reads {
		resolutionContext := governancecore.WithApprovalSemanticPayload(cmd.Payload, semantic, approvalSemanticSource(cmd.Payload))
		item, err := ResolveApprovalItemWithContext(ctx, hub, r.item, action, resolutionContext)
		if err != nil {
			return nil, err
		}
		view, err := ApprovalViewForItem(ctx, traces, item, scope)
		if err != nil {
			return nil, err
		}
		ret = append(ret, view)
	}
	return ret, nil
}
