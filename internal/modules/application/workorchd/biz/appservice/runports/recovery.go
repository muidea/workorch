package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const StuckRunningRunFailureReason = "stuck running run recovered: no active execution, blocker, or continuation checkpoint"
const OrphanDispatchedStepRecoveryReason = "orphan dispatched step recovered after service restart"
const MaxOrphanDispatchedStepRecoveryIterations = 16

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
