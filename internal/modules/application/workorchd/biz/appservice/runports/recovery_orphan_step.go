package runports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ResumeRunFunc func(context.Context, *runmodel.Run) error
type RecoverRuntimeGapFunc func(context.Context, *runmodel.Run, RuntimeGapRecovery) error

type RuntimeGapRecovery struct {
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	StepKind       string
	Gap            runtimeExecutionGap
	RequestID      string
	QueueKey       string
	LatestStepTime string
	Reason         string
}

func RecoverOrphanDispatchedStep(ctx context.Context, hub events.Hub, item *runmodel.Run, maxIterations int, resume ResumeRunFunc) (*runmodel.Step, bool, error) {
	return recoverOrphanDispatchedStep(ctx, hub, item, maxIterations, resume, nil)
}

func RecoverStaleOrphanDispatchedStep(ctx context.Context, hub events.Hub, item *runmodel.Run, maxIterations int, resume ResumeRunFunc, minAge time.Duration, now time.Time) (*runmodel.Step, bool, error) {
	if resume == nil {
		return nil, false, nil
	}
	return RecoverStaleRuntimeGapDispatchedStep(ctx, hub, item, maxIterations, func(ctx context.Context, current *runmodel.Run, _ RuntimeGapRecovery) error {
		return resume(ctx, current)
	}, minAge, now)
}

func RecoverStaleRuntimeGapDispatchedStep(ctx context.Context, hub events.Hub, item *runmodel.Run, maxIterations int, recover RecoverRuntimeGapFunc, minAge time.Duration, now time.Time) (*runmodel.Step, bool, error) {
	if minAge <= 0 {
		minAge = DefaultStaleDispatchedStepMinAge
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return recoverOrphanDispatchedStep(ctx, hub, item, maxIterations, nil, func(ctx context.Context, hub events.Hub, current *runmodel.Run) (*runmodel.Step, error) {
		step, _, err := staleOrphanDispatchedRuntimeGap(ctx, hub, current, minAge, now)
		return step, err
	}, func(ctx context.Context, hub events.Hub, current *runmodel.Run, terminal *runmodel.Step) error {
		if recover == nil {
			return common.ErrInvalidArgument
		}
		_, recovery, err := staleOrphanDispatchedRuntimeGap(ctx, hub, current, minAge, now)
		if err != nil {
			return err
		}
		if recovery.StepID == "" && terminal != nil {
			recovery = RuntimeGapRecovery{
				WorkspaceID: current.WorkspaceID,
				RunID:       current.ID,
				StepID:      terminal.ID,
				StepKind:    terminal.Kind,
				Gap:         runtimeExecutionGapUnknown,
			}
		}
		return recover(ctx, current, recovery)
	})
}

type orphanDispatchedStepSelector func(context.Context, events.Hub, *runmodel.Run) (*runmodel.Step, error)
type orphanDispatchedStepRecoveryExecutor func(context.Context, events.Hub, *runmodel.Run, *runmodel.Step) error

func recoverOrphanDispatchedStep(ctx context.Context, hub events.Hub, item *runmodel.Run, maxIterations int, resume ResumeRunFunc, selector orphanDispatchedStepSelector, executors ...orphanDispatchedStepRecoveryExecutor) (*runmodel.Step, bool, error) {
	if hub == nil ||
		item == nil ||
		!runtimecore.IsRecoverableRunningRun(item) {
		return nil, false, nil
	}
	if selector == nil {
		selector = OrphanDispatchedTerminalStep
	}
	current := item
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, current.WorkspaceID)
	var recovered *runmodel.Step
	for attempt := 0; attempt < maxIterations; attempt++ {
		terminal, err := selector(ctx, hub, current)
		if err != nil {
			return nil, false, err
		}
		if terminal == nil {
			return recovered, recovered != nil, nil
		}
		previousTerminalID := terminal.ID
		err = executeOrphanDispatchedRecovery(ctx, hub, current, terminal, resume, executors...)
		if err != nil {
			latest, latestErr := ReadRun(ctx, hub, current.ID)
			if latestErr != nil {
				return nil, false, latestErr
			}
			current = latest
			if !errors.Is(err, common.ErrConflict) {
				return nil, false, err
			}
			nextTerminal, terminalErr := selector(ctx, hub, current)
			if terminalErr != nil {
				return nil, false, terminalErr
			}
			if nextTerminal == nil {
				return terminal, true, nil
			}
			if nextTerminal.ID == previousTerminalID {
				return nil, false, err
			}
			recovered = nextTerminal
			continue
		}
		recovered = terminal
		latest, err := ReadRun(ctx, hub, current.ID)
		if err != nil {
			return nil, false, err
		}
		current = latest
		if !runtimecore.IsRecoverableRunningRun(current) {
			return recovered, true, nil
		}
		nextTerminal, err := selector(ctx, hub, current)
		if err != nil {
			return nil, false, err
		}
		if nextTerminal == nil || nextTerminal.ID == previousTerminalID {
			return recovered, true, nil
		}
		recovered = nextTerminal
	}
	if recovered == nil {
		return nil, false, nil
	}
	return recovered, true, fmt.Errorf("%w: orphan dispatched step recovery exceeded %d iterations", common.ErrConflict, maxIterations)
}

func executeOrphanDispatchedRecovery(ctx context.Context, hub events.Hub, current *runmodel.Run, terminal *runmodel.Step, resume ResumeRunFunc, executors ...orphanDispatchedStepRecoveryExecutor) error {
	if len(executors) > 0 && executors[0] != nil {
		return executors[0](ctx, hub, current, terminal)
	}
	if resume == nil {
		return common.ErrInvalidArgument
	}
	return resume(ctx, current)
}
