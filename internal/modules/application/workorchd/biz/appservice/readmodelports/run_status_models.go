package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runStatusRelatedModels struct {
	steps       []*runmodel.Step
	checkpoints []*runmodel.RuntimeCheckpoint
	decisions   []*decisionmodel.Ticket
	approvals   []*approvalmodel.Approval
}

func loadRunStatusRelatedModels(ctx context.Context, readers RunStatusReaders, run *runmodel.Run) (runStatusRelatedModels, error) {
	var related runStatusRelatedModels
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
	err := readmodelcore.RunTasks(
		func() error {
			if readers.Steps == nil {
				return nil
			}
			var err error
			related.steps, err = readers.Steps.ListByRun(ctx, run.ID)
			return err
		},
		func() error {
			if readers.Checkpoints == nil {
				return nil
			}
			var err error
			related.checkpoints, err = readers.Checkpoints.ListByRun(ctx, run.ID)
			return err
		},
		func() error {
			if readers.Tickets == nil {
				return nil
			}
			var err error
			related.decisions, err = readers.Tickets.ListByRunID(ctx, run.ID)
			return err
		},
		func() error {
			if readers.Approvals == nil {
				return nil
			}
			var err error
			related.approvals, err = readers.Approvals.ListByRunID(ctx, run.ID)
			return err
		},
	)
	if err != nil {
		return runStatusRelatedModels{}, err
	}
	return related, nil
}
