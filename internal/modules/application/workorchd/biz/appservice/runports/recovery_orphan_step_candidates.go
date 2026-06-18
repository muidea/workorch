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
	runSteps, err := ListStepSummariesByRun(ctx, hub, item.ID)
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
	step, _, err := staleOrphanDispatchedRuntimeGap(ctx, hub, item, minAge, now)
	return step, err
}

func staleOrphanDispatchedRuntimeGap(ctx context.Context, hub events.Hub, item *runmodel.Run, minAge time.Duration, now time.Time) (*runmodel.Step, RuntimeGapRecovery, error) {
	ctx = contextWithRunWorkspace(ctx, item)
	terminal, err := OrphanDispatchedTerminalStep(ctx, hub, item)
	if err != nil || terminal == nil {
		return terminal, RuntimeGapRecovery{}, err
	}
	if minAge <= 0 {
		return terminal, runtimeGapRecoveryFromStep(item, terminal, runtimeExecutionGapUnknown, runtimeExecutionGapState{}), nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	checkpoints, err := ListRuntimeCheckpointsByRun(ctx, hub, item.ID)
	if err != nil {
		return nil, RuntimeGapRecovery{}, err
	}
	gap := classifyRuntimeExecutionGapForStep(terminal, checkpoints)
	switch gap.Gap {
	case runtimeExecutionGapTerminalNotFinalized:
		if !gap.HasTerminalResult {
			return nil, RuntimeGapRecovery{}, nil
		}
	case runtimeExecutionGapWorkerLost:
		if !gap.HasWorkerLost {
			return nil, RuntimeGapRecovery{}, nil
		}
	case runtimeExecutionGapAcceptedNoProgress, runtimeExecutionGapAcceptedInProgress:
		if !gap.HasWorkerHeartbeat || gap.HasWorkerStopped {
			return nil, RuntimeGapRecovery{}, nil
		}
	case runtimeExecutionGapFinalized, runtimeExecutionGapUnknown:
		return nil, RuntimeGapRecovery{}, nil
	}
	if gap.LatestStepTime == "" {
		return nil, RuntimeGapRecovery{}, nil
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, gap.LatestStepTime)
	if err != nil {
		return nil, RuntimeGapRecovery{}, nil
	}
	if occurredAt.After(now) || now.Sub(occurredAt) < minAge {
		return nil, RuntimeGapRecovery{}, nil
	}
	return terminal, runtimeGapRecoveryFromStep(item, terminal, gap.Gap, gap), nil
}

func runtimeGapRecoveryFromStep(item *runmodel.Run, step *runmodel.Step, gap runtimeExecutionGap, state runtimeExecutionGapState) RuntimeGapRecovery {
	if item == nil || step == nil {
		return RuntimeGapRecovery{}
	}
	reason := "stale dispatched runtime gap"
	if gap != "" {
		reason += ": " + string(gap)
	}
	return RuntimeGapRecovery{
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.ID,
		StepID:         step.ID,
		StepKind:       step.Kind,
		Gap:            gap,
		RequestID:      state.RequestID,
		QueueKey:       state.QueueKey,
		LatestStepTime: state.LatestStepTime,
		Reason:         reason,
	}
}

func contextWithRunWorkspace(ctx context.Context, item *runmodel.Run) context.Context {
	if item == nil || item.WorkspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
}
