package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BlockedRunResumeCompressedContext(ctx context.Context, tickets readmodelcore.DecisionTicketReader, resolutions readmodelcore.DecisionResolutionReader, approvals readmodelcore.ApprovalReader, item *runmodel.Run, language string) (map[string]any, error) {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return nil, nil
	}
	switch item.State {
	case common.RunStateBlockedOnDecision:
		if item.BlockingTicketID == "" || tickets == nil || resolutions == nil {
			return nil, nil
		}
		ticket, err := tickets.Get(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if ticket.State != common.DecisionTicketStateResolved {
			return nil, nil
		}
		resolution, err := resolutions.GetByTicketID(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if resolution.Type != "provide_missing_info" {
			return nil, nil
		}
		blocker, recovery, summary := runtimecore.ResolvedDecisionBlockerContext(item, language)
		return runtimecore.BlockedRunResumeCompressedContext(item, blocker, recovery, summary, language), nil
	case common.RunStateBlockedOnApproval:
		if item.BlockingApprovalID == "" || approvals == nil {
			return nil, nil
		}
		approval, err := approvals.Get(ctx, item.BlockingApprovalID)
		if err != nil {
			return nil, err
		}
		if approval.State != common.ApprovalStateApproved {
			return nil, nil
		}
		blocker, recovery, summary := runtimecore.ApprovedApprovalBlockerContext(item, language)
		return runtimecore.BlockedRunResumeCompressedContext(item, blocker, recovery, summary, language), nil
	default:
		return nil, nil
	}
}
