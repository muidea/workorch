package runports

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ResumeRunFunc func(context.Context, *runmodel.Run) error

func RecoverOrphanDispatchedStep(ctx context.Context, hub events.Hub, item *runmodel.Run, maxIterations int, resume ResumeRunFunc) (*runmodel.Step, bool, error) {
	if hub == nil ||
		item == nil ||
		resume == nil ||
		!runtimecore.IsRecoverableRunningRun(item) {
		return nil, false, nil
	}
	current := item
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, current.WorkspaceID)
	var recovered *runmodel.Step
	for attempt := 0; attempt < maxIterations; attempt++ {
		terminal, err := OrphanDispatchedTerminalStep(ctx, hub, current)
		if err != nil {
			return nil, false, err
		}
		if terminal == nil {
			return recovered, recovered != nil, nil
		}
		previousTerminalID := terminal.ID
		if err := resume(ctx, current); err != nil {
			latest, latestErr := ReadRun(ctx, hub, current.ID)
			if latestErr != nil {
				return nil, false, latestErr
			}
			current = latest
			if !errors.Is(err, common.ErrConflict) {
				return nil, false, err
			}
			nextTerminal, terminalErr := OrphanDispatchedTerminalStep(ctx, hub, current)
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
		nextTerminal, err := OrphanDispatchedTerminalStep(ctx, hub, current)
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
