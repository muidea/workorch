package runports

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func IsStuckRunningRunCandidate(ctx context.Context, hub events.Hub, item *runmodel.Run) bool {
	return runtimecore.IsRecoverableRunningRun(item) && !HasContinuationCheckpointForRun(ctx, hub, item)
}

func TerminalStepCanDriveRunningRunResume(ctx context.Context, hub events.Hub, item *runmodel.Run) bool {
	ctx = contextWithRunWorkspace(ctx, item)
	terminal, err := TerminalStepForRunningRun(ctx, hub, item)
	return err == nil && terminal != nil && terminal.State != common.StepStateDispatched
}

func TerminalStepForRunningRun(ctx context.Context, hub events.Hub, item *runmodel.Run) (*runmodel.Step, error) {
	if hub == nil || !runtimecore.IsRecoverableRunningRun(item) {
		return nil, nil
	}
	ctx = contextWithRunWorkspace(ctx, item)
	runSteps, err := ListStepsByRun(ctx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	return runtimecore.TerminalStepModel(runSteps), nil
}

func OrphanDispatchedTerminalStep(ctx context.Context, hub events.Hub, item *runmodel.Run) (*runmodel.Step, error) {
	ctx = contextWithRunWorkspace(ctx, item)
	terminal, err := TerminalStepForRunningRun(ctx, hub, item)
	if err != nil {
		return nil, err
	}
	if terminal == nil || terminal.State != common.StepStateDispatched {
		return nil, nil
	}
	return terminal, nil
}

func StaleOrphanDispatchedTerminalStep(ctx context.Context, hub events.Hub, item *runmodel.Run, minAge time.Duration, now time.Time) (*runmodel.Step, error) {
	ctx = contextWithRunWorkspace(ctx, item)
	terminal, err := OrphanDispatchedTerminalStep(ctx, hub, item)
	if err != nil || terminal == nil {
		return terminal, err
	}
	if minAge <= 0 {
		return terminal, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	checkpoints, err := ListRuntimeCheckpointsByRun(ctx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	latest := runtimecore.LatestRuntimeCheckpointModel(checkpoints)
	if latest == nil || latest.CurrentStepID != terminal.ID {
		return nil, nil
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, latest.OccurredAt)
	if err != nil {
		return nil, nil
	}
	if occurredAt.After(now) || now.Sub(occurredAt) < minAge {
		return nil, nil
	}
	return terminal, nil
}

func contextWithRunWorkspace(ctx context.Context, item *runmodel.Run) context.Context {
	if item == nil || item.WorkspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
}
