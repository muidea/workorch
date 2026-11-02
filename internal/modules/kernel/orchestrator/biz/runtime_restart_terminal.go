package orchestrator

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A Run can still be active after SM-2 sealed its terminal and before SM-4
// finished delivery. Restart must close that exact terminal, not request a
// competing administrative terminal. Its bounded retry authority is the Run
// owner's already claimed replacement generation, including after automatic
// terminal delivery entered intervention. Neither ledger is reset here.
func (m *Biz) handleRestartTerminalPredecessor(ctx context.Context, cmd orchestratorevents.RequestRuntimeTerminationCommand, result magicevent.Result) bool {
	aggregate, err := m.runtimeController.Load(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	_, terminal, err := runtime.DeriveTerminalOutcome(aggregate.Decision, aggregate.Termination)
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	if !terminal || aggregate.Termination != nil {
		// The normal administrative path shares the same Run-owned claim.
		// A delayed command must not redeliver a completed replacement or use
		// an exhausted/stale generation merely because its reason matches.
		observed, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, cmd.WorkspaceID, cmd.RunID)
		if err == nil {
			err = validateRestartTerminalDeliveryCommand(cmd, observed)
		}
		if err != nil {
			setOrchestratorAck(result, err)
			return true
		}
		return false
	}
	key := runtimeDriveKey(cmd.WorkspaceID, cmd.RunID)
	token, acquired := m.beginRuntimeDriveWithToken(key)
	if !acquired {
		setOrchestratorAck(result, fmt.Errorf("%w: original terminal delivery is owned by the active drive", common.ErrConflict))
		return true
	}
	gate, _ := m.currentRuntimeDriveGate(key)
	settlementCtx := m.orchestratorBackgroundContext(ctx, cmd.WorkspaceID)
	settlement := runtimeDriveGateSettlementRequest{
		WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
		GateEpoch: gate.Epoch, GateToken: token, Source: "restart_original_terminal_delivery", ReleaseWhenNoLease: true,
	}
	defer func() {
		// Failure is returned to the Run replacement reducer, which alone owns
		// its bounded retry/restart schedule. Do not mint an automatic SM-4
		// lease, clear intervention, or retry beside that transaction.
		if !m.releaseRuntimeDriveGateAfterFreshNoLease(settlementCtx, settlement) {
			m.retainOrAcquireRuntimeDriveGateSettlement(settlement)
		}
	}()

	observed, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, cmd.WorkspaceID, cmd.RunID)
	if err == nil {
		err = validateRestartTerminalDeliveryCommand(cmd, observed)
	}
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	// Re-read after taking the single drive gate and validating the current
	// Run-owned claim. Scheduled or running durable drive authority must finish
	// through its existing lane; Restart cannot steal it.
	aggregate, err = m.runtimeController.Load(ctx, cmd.WorkspaceID, cmd.RunID)
	if err == nil && (aggregate.DriveLease != nil || !runtimeTerminalDeliveryReady(aggregate) || aggregate.Termination != nil) {
		err = fmt.Errorf("%w: original terminal is not available for replacement delivery", common.ErrConflict)
	}
	if err == nil {
		err = m.deliverRuntimeTerminal(ctx, aggregate)
	}
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	completed, err := m.runtimeController.Load(ctx, cmd.WorkspaceID, cmd.RunID)
	if err == nil && !runtimeTerminalDeliveryFullyAcknowledged(completed) {
		err = fmt.Errorf("%w: original terminal delivery is not fully acknowledged", common.ErrConflict)
	}
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	request, err := terminalDeliveryRequest(completed)
	if err != nil {
		setOrchestratorAck(result, err)
		return true
	}
	m.setRuntimeTerminationResult(result, request.TerminalDeliveryID, completed)
	return true
}

func validateRestartTerminalDeliveryCommand(cmd orchestratorevents.RequestRuntimeTerminationCommand, observed *runmodel.Run) error {
	if err := cmd.Validate(); err != nil {
		return err
	}
	if observed == nil || observed.WorkspaceID != cmd.WorkspaceID || observed.ID != cmd.RunID ||
		cmd.RequestedBy != "blocks/run" || cmd.Kind.Normalized() != runtimecontract.RuntimeTerminationSuperseded {
		return fmt.Errorf("%w: restart terminal delivery requires the exact Run owner scope", common.ErrConflict)
	}
	intent := observed.ReplacementIntent
	if intent == nil || !intent.ValidFor(observed) ||
		(intent.State != runmodel.ReplacementIntentPending && intent.State != runmodel.ReplacementIntentClarificationResolutionBound) ||
		intent.RequestID != cmd.ReplacementRequestID || runmodel.StableReplacementIntentHash(*intent) != cmd.ReplacementIntentHash ||
		intent.Kind != cmd.Kind.Normalized() || strings.TrimSpace(intent.Reason) != strings.TrimSpace(cmd.Reason) ||
		intent.Recovery.State != runmodel.ReplacementRecoveryRunning || intent.Recovery.Stage != runmodel.ReplacementRecoveryAdministrativeTermination ||
		intent.Recovery.Generation != cmd.RecoveryGeneration {
		return fmt.Errorf("%w: restart terminal delivery has no matching claimed replacement generation", common.ErrConflict)
	}
	return nil
}
