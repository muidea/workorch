package orchestrator

import (
	"context"
	"errors"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateCancelled, "")
}

func (o *RuntimeOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateSuperseded, "")
}

func (o *RuntimeOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	if runID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateFailed, reason)
}

func (o *RuntimeOrchestrator) closeRun(ctx context.Context, runID common.RunID, state common.RunState, reason string) error {
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	return o.closeLoadedRun(ctx, run, state, reason)
}

func (o *RuntimeOrchestrator) closeLoadedRun(ctx context.Context, run *runModel.Run, state common.RunState, reason string) error {
	if run == nil || run.ID == "" {
		return common.ErrInvalidArgument
	}
	if run.State == state {
		_, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, workspaceStateForTerminalRun(state))
		if err != nil {
			return err
		}
		if !changed {
			return common.ErrConflict
		}
		return nil
	}
	if !canCloseRunAsUnsuccessfulTerminal(run.State, state) {
		return common.ErrConflict
	}
	if err := o.dismissOpenDecisionBlocker(ctx, run); err != nil {
		return err
	}
	run.State = state
	if state == common.RunStateSuperseded && (strings.TrimSpace(run.Lineage.LineageState) == "" || run.Lineage.LineageState == "active") {
		run.Lineage.LineageState = "superseded"
	}
	run.BlockingTicketID = ""
	run.BlockingApprovalID = ""
	run.FailureReason = reason
	if strings.TrimSpace(run.EndedAt) == "" {
		run.EndedAt = runtimeEvidenceTimestamp()
	}
	_, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, workspaceStateForTerminalRun(state))
	if err != nil {
		return err
	}
	if !changed {
		return common.ErrConflict
	}
	return o.runs.Update(ctx, run)
}

func (o *RuntimeOrchestrator) dismissOpenDecisionBlocker(ctx context.Context, run *runModel.Run) error {
	if o == nil || o.tickets == nil || run == nil || run.BlockingTicketID == "" {
		return nil
	}
	ticket, err := o.tickets.Get(ctx, run.BlockingTicketID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	if ticket.RunID != run.ID || ticket.State != common.DecisionTicketStateOpen {
		return nil
	}
	ticket.State = common.DecisionTicketStateSuperseded
	ticket.UpdatedAt = runtimeEvidenceTimestamp()
	return o.tickets.Update(ctx, ticket)
}

func workspaceStateForTerminalRun(state common.RunState) common.WorkspaceState {
	if state == common.RunStateSuperseded {
		return common.WorkspaceStateSuperseded
	}
	return common.WorkspaceStateActive
}

func isTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func canCloseRunAsUnsuccessfulTerminal(current common.RunState, target common.RunState) bool {
	switch target {
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		switch current {
		case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
			return true
		}
	}
	return false
}
