package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const stuckRunningRunFailureReason = "stuck running run recovered: no active execution, blocker, or continuation checkpoint"
const orphanDispatchedStepRecoveryReason = "orphan dispatched step recovered after service restart"
const maxOrphanDispatchedStepRecoveryIterations = 16

func RecoverStuckRunningRunsWithSession(ctx context.Context, session *RuntimeSession) (ret []Run, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := recoverTerminalActiveWorkspacePointers(ctx, exports.Infrastructure.EventHub); err != nil {
		return nil, err
	}

	runs, err := listRunsByOwnerCommand(ctx, exports.Infrastructure.EventHub)
	if err != nil {
		return nil, err
	}
	for _, item := range runs {
		if recoveredStep, changed, err := recoverOrphanDispatchedStep(ctx, session, exports.Infrastructure.EventHub, item); err != nil {
			return nil, err
		} else if changed {
			recovered, err := readRunByOwnerCommand(common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID), exports.Infrastructure.EventHub, item.ID)
			if err != nil {
				return nil, err
			}
			runView := mapRun(recovered)
			runView.FailureReason = orphanDispatchedStepRecoveryReason + ": " + string(recoveredStep.ID)
			ret = append(ret, runView)
			continue
		}
		if !isStuckRunningRunCandidate(ctx, exports.Infrastructure.EventHub, item) {
			continue
		}
		failed, err := failStuckRunningRun(ctx, exports.Infrastructure.EventHub, item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, mapRun(failed))
	}
	return ret, nil
}

func recoverOrphanDispatchedStep(ctx context.Context, session *RuntimeSession, hub events.Hub, item *runmodel.Run) (*runmodel.Step, bool, error) {
	if session == nil ||
		hub == nil ||
		item == nil ||
		!isRecoverableRunningRun(item) {
		return nil, false, nil
	}
	current := item
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, current.WorkspaceID)
	var recovered *runmodel.Step
	for attempt := 0; attempt < maxOrphanDispatchedStepRecoveryIterations; attempt++ {
		terminal, err := orphanDispatchedTerminalStep(ctx, hub, current)
		if err != nil {
			return nil, false, err
		}
		if terminal == nil {
			return recovered, recovered != nil, nil
		}
		previousTerminalID := terminal.ID
		if _, err := ResumeRunWithSession(ctx, session.Borrow(), string(current.ID)); err != nil {
			latest, latestErr := readRunByOwnerCommand(ctx, hub, current.ID)
			if latestErr != nil {
				return nil, false, latestErr
			}
			current = latest
			if !errors.Is(err, common.ErrConflict) {
				return nil, false, err
			}
			nextTerminal, terminalErr := orphanDispatchedTerminalStep(ctx, hub, current)
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
		latest, err := readRunByOwnerCommand(ctx, hub, current.ID)
		if err != nil {
			return nil, false, err
		}
		current = latest
		if !isRecoverableRunningRun(current) {
			return recovered, true, nil
		}
		nextTerminal, err := orphanDispatchedTerminalStep(ctx, hub, current)
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
	return recovered, true, fmt.Errorf("%w: orphan dispatched step recovery exceeded %d iterations", common.ErrConflict, maxOrphanDispatchedStepRecoveryIterations)
}

func orphanDispatchedTerminalStep(ctx context.Context, hub events.Hub, item *runmodel.Run) (*runmodel.Step, error) {
	if hub == nil || !isRecoverableRunningRun(item) {
		return nil, nil
	}
	runSteps, err := listStepsByRunOwnerCommand(ctx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	terminal := terminalStepModel(runSteps)
	if terminal == nil || terminal.State != common.StepStateDispatched {
		return nil, nil
	}
	return terminal, nil
}

func isRecoverableRunningRun(item *runmodel.Run) bool {
	return item != nil &&
		item.State == common.RunStateRunning &&
		item.BlockingTicketID == "" &&
		item.BlockingApprovalID == ""
}

func recoverTerminalActiveWorkspacePointers(ctx context.Context, hub events.Hub) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	items, err := listWorkspacesByOwnerCommand(ctx, hub)
	if err != nil {
		return err
	}
	for _, workspace := range items {
		if workspace == nil || workspace.ActiveRunID == "" {
			continue
		}
		runCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
		activeRun, err := readRunByOwnerCommand(runCtx, hub, workspace.ActiveRunID)
		if err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return err
			}
			workspace.ActiveRunID = ""
			if isActiveWorkspaceStateForRecovery(workspace.State) {
				workspace.State = common.WorkspaceStateActive
			}
			if err := updateWorkspaceByEventHub(ctx, hub, workspace); err != nil {
				return err
			}
			continue
		}
		if activeRun.WorkspaceID != workspace.ID || !isActivationTerminalRunState(activeRun.State) {
			continue
		}
		workspace.ActiveRunID = ""
		workspace.State = recoveredWorkspaceStateForTerminalRun(activeRun.State)
		if err := updateWorkspaceByEventHub(ctx, hub, workspace); err != nil {
			return err
		}
	}
	return nil
}

func failStuckRunningRun(ctx context.Context, hub events.Hub, item *runmodel.Run) (*runmodel.Run, error) {
	if hub == nil || item == nil {
		return nil, common.ErrInvalidArgument
	}
	runCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	if err := publishOwnerCommand(runCtx, hub, events.TopicRunCommandFail, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyRunID:         string(item.ID),
		events.PayloadKeyFailureReason: stuckRunningRunFailureReason,
	}); err != nil {
		return nil, err
	}
	failed, err := readRunByOwnerCommand(runCtx, hub, item.ID)
	if err != nil {
		return nil, err
	}
	workspace, err := readWorkspaceByOwnerCommand(ctx, hub, failed.WorkspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return failed, nil
		}
		return nil, err
	}
	if workspace == nil || workspace.ActiveRunID != failed.ID {
		return failed, nil
	}
	workspace.ActiveRunID = ""
	workspace.State = recoveredWorkspaceStateForTerminalRun(failed.State)
	return failed, updateWorkspaceByEventHub(ctx, hub, workspace)
}

func RecoverStuckRunningRunsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return RecoverStuckRunningRunsWithSession(ctx, session)
}

func isStuckRunningRunCandidate(ctx context.Context, hub events.Hub, item *runmodel.Run) bool {
	if item == nil ||
		item.State != common.RunStateRunning ||
		item.BlockingTicketID != "" ||
		item.BlockingApprovalID != "" {
		return false
	}
	return !hasContinuationCheckpoint(ctx, hub, item.ID)
}

func updateWorkspaceByEventHub(ctx context.Context, hub events.Hub, workspace *workspacemodel.Workspace) error {
	if hub == nil || workspace == nil || workspace.ID == "" {
		return common.ErrInvalidArgument
	}
	return publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandUpdate, "workspace", string(workspace.ID), workspace.ID, "", map[string]any{
		events.PayloadKeyWorkspace: workspace,
	})
}

func recoveredWorkspaceStateForTerminalRun(state common.RunState) common.WorkspaceState {
	switch state {
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isActiveWorkspaceStateForRecovery(state common.WorkspaceState) bool {
	switch state {
	case common.WorkspaceStateRunning, common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked, common.WorkspaceStateValidating:
		return true
	default:
		return false
	}
}

func hasContinuationCheckpoint(ctx context.Context, hub events.Hub, runID common.RunID) bool {
	if hub == nil || runID == "" {
		return false
	}
	items, err := listRuntimeCheckpointsByRunOwnerCommand(ctx, hub, runID)
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

func terminalStepModel(items []*runmodel.Step) *runmodel.Step {
	if len(items) == 0 {
		return nil
	}
	referenced := map[common.StepID]bool{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if previous := previousStepModelID(item); previous != "" {
			referenced[previous] = true
		}
	}
	terminals := make([]*runmodel.Step, 0, len(items))
	for _, item := range items {
		if item == nil || referenced[item.ID] {
			continue
		}
		terminals = append(terminals, item)
	}
	if len(terminals) == 0 {
		return nil
	}
	if len(terminals) == 1 {
		return terminals[0]
	}
	for _, item := range terminals {
		if item.State == common.StepStateDispatched {
			return item
		}
	}
	return terminals[len(terminals)-1]
}

func previousStepModelID(item *runmodel.Step) common.StepID {
	if item == nil || item.Input == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := item.Input[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != item.ID {
			return common.StepID(value)
		}
	}
	return ""
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}
