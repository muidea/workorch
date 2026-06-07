package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func IsStuckRunningRunCandidate(ctx context.Context, hub events.Hub, item *runmodel.Run) bool {
	return runtimecore.IsRecoverableRunningRun(item) && !HasContinuationCheckpoint(ctx, hub, item.ID)
}

func OrphanDispatchedTerminalStep(ctx context.Context, hub events.Hub, item *runmodel.Run) (*runmodel.Step, error) {
	if hub == nil || !runtimecore.IsRecoverableRunningRun(item) {
		return nil, nil
	}
	runSteps, err := ListStepsByRun(ctx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	terminal := runtimecore.TerminalStepModel(runSteps)
	if terminal == nil || terminal.State != common.StepStateDispatched {
		return nil, nil
	}
	return terminal, nil
}
