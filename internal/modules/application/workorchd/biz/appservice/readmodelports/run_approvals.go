package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type RunApprovals struct {
	Items            []projectioncontract.Approval
	SelectedApproval *approvalmodel.Approval
}

func ApprovalsForRun(ctx context.Context, approvals readmodelcore.ApprovalReader, traces readmodelcore.AutoGovernanceTraceReader, run *runmodel.Run) (RunApprovals, error) {
	if run == nil || approvals == nil {
		return RunApprovals{}, nil
	}
	items, err := approvals.ListByRunID(ctx, run.ID)
	if err != nil {
		return RunApprovals{}, err
	}

	ret := RunApprovals{Items: make([]projectioncontract.Approval, 0, len(items))}
	for _, item := range items {
		if item == nil || item.RunID != run.ID {
			continue
		}
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return RunApprovals{}, err
		}
		ret.Items = append(ret.Items, readmodelprojection.GovernanceProjection().MapApproval(item, summary))
		if run.BlockingApprovalID != "" && item.ID == run.BlockingApprovalID {
			ret.SelectedApproval = item
			continue
		}
		if ret.SelectedApproval == nil || run.BlockingApprovalID == "" {
			ret.SelectedApproval = item
		}
	}
	return ret, nil
}

func ApprovalsForWorkspaceItems(ctx context.Context, traces readmodelcore.AutoGovernanceTraceReader, workspaceID common.WorkspaceID, items []*approvalmodel.Approval) ([]projectioncontract.Approval, error) {
	ret := make([]projectioncontract.Approval, 0, len(items))
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return nil, err
		}
		ret = append(ret, readmodelprojection.GovernanceProjection().MapApproval(item, summary))
	}
	return ret, nil
}
