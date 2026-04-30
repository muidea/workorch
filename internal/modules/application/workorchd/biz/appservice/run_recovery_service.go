package appservice

import (
	"context"

	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const stuckRunningRunFailureReason = "stuck running run recovered: no active execution, blocker, or continuation checkpoint"

func RecoverStuckRunningRunsWithSession(ctx context.Context, session *RuntimeSession) (ret []Run, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Runs == nil ||
		exports.Repositories.RuntimeCheckpoints == nil ||
		exports.Services.RunService == nil {
		return nil, common.ErrInvalidArgument
	}

	runs, err := exports.Repositories.Runs.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range runs {
		if !isStuckRunningRunCandidate(ctx, exports.Repositories.RuntimeCheckpoints, item) {
			continue
		}
		if err := exports.Services.RunService.FailRun(ctx, runmodule.FailRunCommand{
			RunID:  item.ID,
			Reason: stuckRunningRunFailureReason,
		}); err != nil {
			return nil, err
		}
		recovered, err := exports.Repositories.Runs.Get(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, mapRun(recovered))
	}
	return ret, nil
}

func RecoverStuckRunningRunsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return RecoverStuckRunningRunsWithSession(ctx, session)
}

func isStuckRunningRunCandidate(ctx context.Context, checkpoints checkpointLister, item *runmodel.Run) bool {
	if item == nil ||
		item.State != common.RunStateRunning ||
		item.BlockingTicketID != "" ||
		item.BlockingApprovalID != "" {
		return false
	}
	return !hasContinuationCheckpoint(ctx, checkpoints, item.ID)
}

type checkpointLister interface {
	ListByRun(context.Context, common.RunID) ([]*runmodel.RuntimeCheckpoint, error)
}

func hasContinuationCheckpoint(ctx context.Context, checkpoints checkpointLister, runID common.RunID) bool {
	if checkpoints == nil || runID == "" {
		return false
	}
	items, err := checkpoints.ListByRun(ctx, runID)
	if err != nil || len(items) == 0 {
		return false
	}
	latest := latestRuntimeCheckpointModel(items)
	if latest == nil || latest.CheckpointType != common.CheckpointTypeRecovery {
		return false
	}
	return nonEmptyString(latest.Data["continuationAction"]) ||
		nonEmptyString(latest.Data["resumeAction"]) ||
		nonEmptyString(latest.Data["nextAction"])
}

func latestRuntimeCheckpointModel(items []*runmodel.RuntimeCheckpoint) *runmodel.RuntimeCheckpoint {
	var latest *runmodel.RuntimeCheckpoint
	for _, item := range items {
		if item == nil {
			continue
		}
		if latest == nil ||
			item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}
