package runports

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const StuckRunningRunFailureReason = "stuck running run recovered: no active execution, blocker, or continuation checkpoint"
const OrphanDispatchedStepRecoveryReason = "orphan dispatched step recovered after service restart"
const StaleDispatchedStepRecoveryReason = "stale dispatched step recovered by run watchdog"
const MaxOrphanDispatchedStepRecoveryIterations = 16
const DefaultStaleDispatchedStepMinAge = 10 * time.Minute

func RecoverStuckRunningRunViews(ctx context.Context, hub events.Hub, resume ResumeRunFunc) ([]appmodelcore.Run, error) {
	if err := RecoverTerminalActiveWorkspacePointers(ctx, hub); err != nil {
		return nil, err
	}

	runs, err := ListRuns(ctx, hub)
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

func RecoverStaleDispatchedRunViews(ctx context.Context, hub events.Hub, resume ResumeRunFunc, minAge time.Duration, now time.Time) ([]appmodelcore.Run, error) {
	if minAge <= 0 {
		minAge = DefaultStaleDispatchedStepMinAge
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runs, err := ListRuns(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Run, 0)
	for _, item := range runs {
		recoveredStep, changed, err := RecoverStaleOrphanDispatchedStep(ctx, hub, item, MaxOrphanDispatchedStepRecoveryIterations, resume, minAge, now)
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
