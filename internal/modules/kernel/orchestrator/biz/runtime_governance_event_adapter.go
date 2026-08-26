package orchestrator

// Governance adapter is internal to the Runtime owner Biz.

import (
	"context"
	"fmt"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeGovernanceApprovalResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil || event == nil || event.Source() != approvalcommon.ModuleID {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceApprovalResolutionCommand)
	if !ok || m.runtimeController == nil || command.ResolutionID == "" || command.ApprovalID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Summary) == "" {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	aggregate, err := m.runtimeController.Load(event.Context(), command.WorkspaceID, command.RunID)
	if err != nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	binding, found := governancecheck.RequestForApproval(aggregate.GovernanceRequests, command.ApprovalID)
	if !found {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, fmt.Errorf("%w: runtime governance approval has no persisted binding", common.ErrConflict))
		return
	}
	m.handleRuntimeGovernanceResolutionLocked(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		event.Source(), ModuleID, magicevent.NewHeader(), event.Context(),
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: runtimecontract.RuntimeGovernanceResolution{
			ResolutionID: command.ResolutionID, ApprovalID: command.ApprovalID,
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			AuthoritySnapshotID: binding.AuthoritySnapshotID, DispatchID: binding.DispatchID,
			DependencyKind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
			Outcome: command.Outcome, Summary: command.Summary, EvidenceRefs: append([]string(nil), command.EvidenceRefs...),
		}},
	), result)
}

func (m *Biz) handleRuntimeGovernanceTicketResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil || event == nil || event.Source() != decisioncommon.ModuleID {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeGovernanceTicketResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeGovernanceTicketResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand)
	if !ok || m == nil || m.runtimeController == nil || command.ResolutionID == "" || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Summary) == "" {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	aggregate, err := m.runtimeController.Load(event.Context(), command.WorkspaceID, command.RunID)
	if err != nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	binding, found := governancecheck.RequestForTicket(aggregate.GovernanceRequests, command.TicketID)
	if !found {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, fmt.Errorf("%w: runtime governance ticket has no persisted binding", common.ErrConflict))
		return
	}
	m.handleRuntimeGovernanceResolutionLocked(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		event.Source(), ModuleID, magicevent.NewHeader(), event.Context(),
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: runtimecontract.RuntimeGovernanceResolution{
			ResolutionID: command.ResolutionID, TicketID: command.TicketID,
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			AuthoritySnapshotID: binding.AuthoritySnapshotID, DispatchID: binding.DispatchID,
			DependencyKind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
			Outcome: command.Outcome, Summary: command.Summary, EvidenceRefs: append([]string(nil), command.EvidenceRefs...),
		}},
	), result)
}

func (m *Biz) handleRuntimeGovernanceResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil || event == nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeGovernanceResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeGovernanceResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceResolutionCommand)
	if !ok || m == nil || m.runtimeController == nil || !runtimeGovernanceResolutionSourceMatchesOwner(event.Source(), command.Resolution) {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	transaction := &runtime.RuntimeInternalTransactionPayload{GovernanceResolution: &command.Resolution}
	if command.Resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeContinue || command.Resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeFail {
		signature, signatureErr := runtime.RuntimeGovernanceResolutionDriveLeaseSignature(command.Resolution)
		if signatureErr != nil {
			setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, signatureErr)
			return
		}
		transaction.GovernanceContinuationDriveLease = &runtimecontract.RuntimeDriveLease{
			WorkspaceID:      command.Resolution.WorkspaceID,
			RunID:            command.Resolution.RunID,
			Reason:           runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry,
			FailureSignature: signature,
			DueAt:            time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		}
	}
	commitEvent, err := m.submitRuntimeInternalEvent(event.Context(), runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: command.Resolution.WorkspaceID, RunID: command.Resolution.RunID}, Transaction: transaction,
	})
	if err != nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	if commitEvent.Evidence == nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	commit := *commitEvent.Evidence
	response := orchestratorevents.CommitRuntimeGovernanceResolutionResult{
		WorkspaceID:         command.Resolution.WorkspaceID,
		RunID:               command.Resolution.RunID,
		ResolutionID:        command.Resolution.ResolutionID,
		Disposition:         runtimeGovernanceDisposition(commit.Disposition),
		StateRevision:       commit.Aggregate.StateRevision,
		EvidenceRevision:    commit.Aggregate.Facts.EvidenceRevision,
		AuthoritySnapshotID: commit.Aggregate.Decision.AuthoritySnapshot.ID,
		State:               commit.Aggregate.Decision.State,
		Terminal:            commit.Aggregate.Decision.Terminal,
	}
	if err := response.Validate(); err != nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	if commit.Disposition == runtime.CommitDispositionCommitted || commit.Disposition == runtime.CommitDispositionDuplicate {
		if err := m.continueRuntimeGovernance(event.Context(), response, command.Resolution, commitEvent.DriveLease); err != nil {
			setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
			return
		}
	}
	setRuntimeGovernanceResult(result, response, nil)
}

func runtimeGovernanceResolutionSourceMatchesOwner(source string, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	if resolution.ApprovalID != "" {
		return source == approvalcommon.ModuleID
	}
	return resolution.TicketID != "" && source == decisioncommon.ModuleID
}

func (m *Biz) continueRuntimeGovernance(
	ctx context.Context,
	response orchestratorevents.CommitRuntimeGovernanceResolutionResult,
	resolution runtimecontract.RuntimeGovernanceResolution,
	continuationLease *runtimecontract.RuntimeDriveLease,
) error {
	workspaceID := resolution.WorkspaceID
	runID := resolution.RunID
	if response.State == runtimecontract.ConvergenceStateComplete || response.State == runtimecontract.ConvergenceStateFailed {
		aggregate, err := m.runtimeController.Load(ctx, workspaceID, runID)
		if err != nil {
			return err
		}
		if continuationLease == nil {
			if response.Disposition == orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate {
				if runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
					return nil
				}
				// The exact resolution successor is intentionally renewed to the
				// canonical terminal-delivery identity after a partial owner ack. A
				// response-loss duplicate joins that current durable successor; it
				// must not require the original handoff signature to live forever or
				// create a second timer chain.
				if lease, ok := runtimeTerminalDeliverySuccessorLease(aggregate); ok {
					return m.mountCurrentRuntimeTerminalDeliverySuccessor(ctx, lease, "governance_resolution_terminal_duplicate")
				}
			}
			return fmt.Errorf("%w: terminal governance resolution has no atomic successor lease", common.ErrConflict)
		}
		lease := continuationLease.Clone()
		signature, signatureErr := runtime.RuntimeGovernanceResolutionDriveLeaseSignature(resolution)
		if signatureErr != nil {
			return signatureErr
		}
		if err := m.validateRuntimeLifecycleOwnerDeliveryLease(lease, signature); err != nil {
			return err
		}
		// Terminal delivery is never attempted naked from the governance command.
		// The exact lease is claimed first, then the ordinary SM-4 terminal lane
		// owns partial acknowledgements, retry budget, Stop and fresh restart.
		return m.mountPersistedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, "governance_resolution_terminal", signature)
	}
	// RUNTIME-SMO: persist the governance resolution first, then submit the
	// immutable continuation fact to Run. The subsequent RunStateChanged(active)
	// notification is the only trigger for the next SM-4 drive.
	aggregate, err := m.runtimeController.Load(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	if continuationLease == nil && response.Disposition == orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate {
		return m.acknowledgePersistedRuntimeGovernanceContinuationDuplicate(ctx, resolution, response, aggregate)
	}
	handoff, stateRevision, err := continuationFromGovernanceCommit(resolution, response, aggregate)
	if err != nil {
		return err
	}
	deliveryIdentity := &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: common.ErrConflict}
	if continuationLease == nil {
		if response.Disposition == orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted {
			return fmt.Errorf("%w: committed governance continuation has no atomic successor lease", common.ErrConflict)
		}
		return fmt.Errorf("%w: duplicate governance continuation has no persisted resolution proof", common.ErrConflict)
	}
	lease := continuationLease.Clone()
	if err := m.validateRuntimeLifecycleOwnerDeliveryLease(lease, deliveryIdentity.leaseSignature()); err != nil {
		return err
	}
	ownsLeaseGate, err := m.retainRuntimeLifecycleOwnerDeliveryLeaseGate(lease, deliveryIdentity.leaseSignature())
	if err != nil {
		return err
	}
	deliveryErr := m.applyRunContinuationEligible(
		ctx, workspaceID, runID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision,
		handoff.DispatchID, handoff.ResolutionID, handoff.TicketID, handoff.ApprovalID, stateRevision,
	)
	var mountErr error
	if ownsLeaseGate {
		mountErr = m.mountRetainedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, "governance_resolution")
	}
	if deliveryErr != nil {
		typedDeliveryErr := &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: deliveryErr}
		if mountErr != nil {
			return fmt.Errorf("%w; mount persisted lifecycle successor: %v", typedDeliveryErr, mountErr)
		}
		return typedDeliveryErr
	}
	return mountErr
}

// acknowledgePersistedRuntimeGovernanceContinuationDuplicate separates two
// response-loss cuts without replaying an owner transition:
//   - while the exact continuation is still the current Runtime handoff, Run
//     must prove the exact persisted ResolutionID;
//   - once Runtime has crossed that fence and persisted a later authority, the
//     exact historical resolution record plus Run's ownership of that later
//     aggregate is the durable acknowledgement. Re-delivering the old fact
//     would incorrectly compare it with (or mutate) the new blocker.
func (m *Biz) acknowledgePersistedRuntimeGovernanceContinuationDuplicate(
	ctx context.Context,
	resolution runtimecontract.RuntimeGovernanceResolution,
	response orchestratorevents.CommitRuntimeGovernanceResolutionResult,
	aggregate runtime.RuntimeAggregate,
) error {
	handoff, found, err := persistedRuntimeGovernanceContinuationHandoff(resolution, aggregate)
	if err != nil {
		return err
	}
	if !found || response.StateRevision != aggregate.StateRevision {
		return fmt.Errorf("%w: duplicate governance continuation has no exact persisted resolution record", common.ErrConflict)
	}
	run, readErr := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, resolution.WorkspaceID, resolution.RunID)
	if readErr != nil {
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, readErr)
	}

	record := runtimeStateRecordFromAggregate(aggregate)
	current, governed, handoffErr := runtimeGovernanceLifecycleHandoffFromState(&record)
	if handoffErr != nil {
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, handoffErr)
	}
	if governed && sameRuntimeGovernanceLifecycleHandoffIdentity(current, handoff) {
		fact, factErr := runContinuationEligibleFact(
			resolution.WorkspaceID, resolution.RunID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision,
			handoff.DispatchID, handoff.ResolutionID, handoff.TicketID, handoff.ApprovalID,
		)
		if factErr != nil {
			return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, factErr)
		}
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, validateRunContinuationAcknowledgement(
			runevents.RunLifecycleCommandResult{Run: run}, fact, aggregate.StateRevision,
		))
	}

	owned, ownershipErr := runtimeRunOwnsExactAggregateSnapshot(run, aggregate)
	if ownershipErr != nil {
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, ownershipErr)
	}
	if !owned {
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, fmt.Errorf(
			"%w: Run does not own the later Runtime aggregate for historical governance continuation", common.ErrConflict,
		))
	}
	return nil
}

func persistedRuntimeGovernanceContinuationHandoff(
	resolution runtimecontract.RuntimeGovernanceResolution,
	aggregate runtime.RuntimeAggregate,
) (runtimeGovernanceLifecycleHandoff, bool, error) {
	resolution = resolution.Normalized()
	if err := resolution.Validate(); err != nil || resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue {
		return runtimeGovernanceLifecycleHandoff{}, false, err
	}
	matchedRecord := false
	wantRecord := runtimecontract.RuntimeGovernanceResolutionRecordFromResolution(resolution)
	for _, record := range aggregate.GovernanceResolutions {
		if runtimecontract.SameRuntimeGovernanceResolutionRecord(record, wantRecord) {
			matchedRecord = true
			break
		}
	}
	if !matchedRecord {
		return runtimeGovernanceLifecycleHandoff{}, false, nil
	}
	if !governancecheck.RequestsBindResolution(aggregate.GovernanceRequests, resolution) {
		return runtimeGovernanceLifecycleHandoff{}, false, nil
	}
	handoff := runtimeGovernanceLifecycleHandoff{
		Kind: runtimeGovernanceLifecycleContinuationDecision, AuthoritySnapshotID: resolution.AuthoritySnapshotID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DispatchID: resolution.DispatchID, TicketID: resolution.TicketID, ApprovalID: resolution.ApprovalID,
		ResolutionID: resolution.ResolutionID,
	}
	if resolution.ApprovalID != "" {
		handoff.Kind = runtimeGovernanceLifecycleContinuationApproval
	}
	if err := handoff.validate(); err != nil {
		return runtimeGovernanceLifecycleHandoff{}, false, err
	}
	return handoff, true, nil
}

func sameRuntimeGovernanceLifecycleHandoffIdentity(left, right runtimeGovernanceLifecycleHandoff) bool {
	return left.Kind == right.Kind && left.AuthoritySnapshotID == right.AuthoritySnapshotID && left.DispatchID == right.DispatchID &&
		left.TicketID == right.TicketID && left.ApprovalID == right.ApprovalID && left.ResolutionID == right.ResolutionID
}

func runtimeRunOwnsExactAggregateSnapshot(run *runmodel.Run, aggregate runtime.RuntimeAggregate) (bool, error) {
	if run == nil || run.RuntimeBootstrap == nil || run.RuntimeState == nil || run.WorkspaceID != aggregate.Facts.WorkspaceID || run.ID != aggregate.Facts.RunID ||
		run.RuntimeState.StateRevision != aggregate.StateRevision {
		return false, nil
	}
	wantState := runtimeStateRecordFromAggregate(aggregate)
	if runtimecontract.StableRuntimeStateRecordHash(*run.RuntimeState) != runtimecontract.StableRuntimeStateRecordHash(wantState) {
		return false, nil
	}
	wantFence, err := runtimeLifecycleFenceFromModel(run)
	if err != nil {
		return false, err
	}
	if wantFence == nil || run.RuntimeLifecycleFence == nil {
		return wantFence == nil && run.RuntimeLifecycleFence == nil, nil
	}
	want := wantFence.Normalized()
	got := run.RuntimeLifecycleFence.Normalized()
	if err := got.Validate(); err != nil {
		return false, err
	}
	return want == got, nil
}

func runtimeTerminalDeliverySuccessorLease(aggregate runtime.RuntimeAggregate) (runtimecontract.RuntimeDriveLease, bool) {
	if aggregate.Decision.Terminal == nil || aggregate.DriveLease == nil || aggregate.DriveLease.Validate() != nil ||
		aggregate.DriveLease.WorkspaceID != aggregate.Facts.WorkspaceID || aggregate.DriveLease.RunID != aggregate.Facts.RunID ||
		aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		aggregate.DriveLease.FailureSignature != "terminal-delivery" {
		return runtimecontract.RuntimeDriveLease{}, false
	}
	return aggregate.DriveLease.Clone(), true
}

func (m *Biz) mountCurrentRuntimeTerminalDeliverySuccessor(ctx context.Context, lease runtimecontract.RuntimeDriveLease, source string) error {
	if lease.State == runtimecontract.RuntimeDriveLeaseRunning {
		key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
		m.driveMu.Lock()
		_, locallyRunning := m.drivingRuns[key]
		m.driveMu.Unlock()
		if locallyRunning {
			return nil
		}
		var err error
		lease, _, err = m.runtimeRecoverDriveLease(ctx, lease.WorkspaceID, lease.RunID, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
	}
	return m.mountPersistedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, source, "terminal-delivery")
}

func (m *Biz) validateRuntimeLifecycleOwnerDeliveryLease(lease runtimecontract.RuntimeDriveLease, expectedSignature string) error {
	if lease.Validate() != nil || lease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		lease.FailureSignature != expectedSignature {
		return fmt.Errorf("%w: governance continuation successor lease does not match the exact persisted resolution", common.ErrConflict)
	}
	return nil
}

func runtimeGovernanceDisposition(value runtime.CommitDisposition) orchestratorevents.RuntimeGovernanceResolutionDisposition {
	switch value {
	case runtime.CommitDispositionCommitted:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted
	case runtime.CommitDispositionStale:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionStale
	case runtime.CommitDispositionDuplicate:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate
	default:
		return ""
	}
}

func setRuntimeGovernanceResult(result magicevent.Result, response orchestratorevents.CommitRuntimeGovernanceResolutionResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
