package appservice

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementports"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeV1ProjectionRebuild func(context.Context) (int64, error)

type RuntimeV1RetirementResult struct {
	DryRun                    bool
	Report                    retirementcore.RuntimeV1RetirementReport
	ProjectionRebuildRevision int64
}

func ExecuteRuntimeV1Retirement(ctx context.Context, hub magicevent.Hub, dryRun bool, rebuild RuntimeV1ProjectionRebuild) (RuntimeV1RetirementResult, error) {
	if hub == nil {
		return RuntimeV1RetirementResult{}, common.ErrInvalidArgument
	}
	inventory, err := retirementports.LoadRuntimeV1Inventory(ctx, hub)
	if err != nil {
		return RuntimeV1RetirementResult{}, err
	}
	plan := retirementcore.BuildRuntimeV1RetirementPlan(inventory)
	result := RuntimeV1RetirementResult{DryRun: dryRun, Report: plan.Report}
	if dryRun {
		return result, nil
	}
	released, err := retirementports.ReleaseRuntimeV1Resources(ctx, hub, plan)
	if err != nil {
		return RuntimeV1RetirementResult{}, err
	}
	result.Report.ReleasedResourceScopes = released
	if err := retirementports.ApplyRuntimeV1RetirementPlan(ctx, hub, inventory, plan); err != nil {
		return RuntimeV1RetirementResult{}, err
	}
	after, err := retirementports.LoadRuntimeV1Inventory(ctx, hub)
	if err != nil {
		return RuntimeV1RetirementResult{}, err
	}
	remaining := retirementcore.BuildRuntimeV1RetirementPlan(after)
	if len(remaining.Runs) != 0 || len(remaining.Workspaces) != 0 || len(remaining.Tickets) != 0 || len(remaining.Approvals) != 0 {
		return RuntimeV1RetirementResult{}, fmt.Errorf("%w: Runtime V1 retirement apply left pending actions", common.ErrConflict)
	}
	if rebuild == nil {
		return RuntimeV1RetirementResult{}, fmt.Errorf("%w: workspace projection rebuild is not configured", common.ErrInvalidArgument)
	}
	revision, err := rebuild(ctx)
	if err != nil {
		return RuntimeV1RetirementResult{}, err
	}
	result.ProjectionRebuildRevision = revision
	return result, nil
}
