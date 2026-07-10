package runports

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const StuckRunningRunFailureReason = "stuck running run recovered: no active execution, blocker, or continuation checkpoint"
const OrphanDispatchedStepRecoveryReason = "orphan dispatched step recovered after service restart"
const StaleDispatchedStepRecoveryReason = "stale dispatched step recovered by run watchdog"
const MaxOrphanDispatchedStepRecoveryIterations = 16
const DefaultStaleDispatchedStepMinAge = 2 * time.Minute

func RecoverStuckRunningRunViews(ctx context.Context, hub magicevent.Hub, resume ResumeRunFunc) ([]appmodelcore.Run, error) {
	if err := RecoverTerminalActiveWorkspacePointers(ctx, hub); err != nil {
		return nil, err
	}

	runs, err := ListRecoverableRuntimeGapRunCandidates(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Run, 0)
	for _, item := range runs {
		recoveredStep, changed, err := RecoverOrphanDispatchedStep(ctx, hub, item, MaxOrphanDispatchedStepRecoveryIterations, resume)
		if err != nil {
			return nil, err
		} else if changed {
			recovered, err := ReadRun(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), hub, item.ID)
			if err != nil {
				return nil, err
			}
			ret = append(ret, RunViewWithFailureReason(recovered, OrphanDispatchedStepRecoveryReason+": "+string(recoveredStep.ID)))
			continue
		}
		if TerminalStepCanDriveRunningRunResume(ctx, hub, item) {
			if err := resume(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), item); err != nil {
				latest, latestErr := ReadRun(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), hub, item.ID)
				if latestErr != nil {
					return nil, latestErr
				}
				if !IsStuckRunningRunCandidate(ctx, hub, latest) {
					ret = append(ret, RunViewWithFailureReason(latest, OrphanDispatchedStepRecoveryReason+": terminal step resume"))
					continue
				}
				if !IsStuckRunningRunCandidate(ctx, hub, item) {
					return nil, err
				}
			} else {
				recovered, err := ReadRun(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), hub, item.ID)
				if err != nil {
					return nil, err
				}
				ret = append(ret, RunViewWithFailureReason(recovered, OrphanDispatchedStepRecoveryReason+": terminal step resume"))
				continue
			}
		}
		if !IsStuckRunningRunCandidate(ctx, hub, item) {
			continue
		}
		failed, err := FailRunAndRecoverActiveWorkspacePointer(ctx, hub, item, StuckRunningRunFailureReason)
		if err != nil {
			return nil, err
		}
		ret = append(ret, RunView(failed))
	}
	return ret, nil
}

func RecoverStaleDispatchedRunViews(ctx context.Context, hub magicevent.Hub, resume ResumeRunFunc, minAge time.Duration, now time.Time) ([]appmodelcore.Run, error) {
	if resume == nil {
		return nil, common.ErrInvalidArgument
	}
	return RecoverStaleRuntimeGapRunViews(ctx, hub, func(ctx context.Context, current *runmodel.Run, _ RuntimeGapRecovery) error {
		return resume(ctx, current)
	}, minAge, now)
}

func RecoverStaleRuntimeGapRunViews(ctx context.Context, hub magicevent.Hub, recover RecoverRuntimeGapFunc, minAge time.Duration, now time.Time) ([]appmodelcore.Run, error) {
	if minAge <= 0 {
		minAge = DefaultStaleDispatchedStepMinAge
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runs, err := ListRecoverableRuntimeGapRunCandidates(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Run, 0)
	for _, item := range runs {
		recoveredStep, changed, err := RecoverStaleRuntimeGapDispatchedStep(ctx, hub, item, MaxOrphanDispatchedStepRecoveryIterations, recover, minAge, now)
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		recovered, err := ReadRun(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), hub, item.ID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, RunViewWithFailureReason(recovered, StaleDispatchedStepRecoveryReason+": "+string(recoveredStep.ID)))
	}
	return ret, nil
}
