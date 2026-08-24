package orchestrator

// Runtime drive lifecycle is owned by Biz.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeDriveTransitionLimit = 1024
const runtimeInternalDriveSource = ModuleID

// runtimePersistedDrivePendingError means an owner-local transition was
// durably committed but changed only bounded request/recovery bookkeeping. It
// is not business convergence and not a CAS failure; the scheduler may re-drive
// the newly persisted revision without consuming another recovery ledger.
type runtimePersistedDrivePendingError struct {
	Revision uint64
}

func (e *runtimePersistedDrivePendingError) Error() string {
	if e == nil {
		return "Runtime has persisted pending drive work"
	}
	return fmt.Sprintf("Runtime has persisted pending drive work at revision %d", e.Revision)
}

type runtimeInvariantError struct {
	Message string
}

func (e *runtimeInvariantError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "Runtime drive invariant failed"
	}
	return strings.TrimSpace(e.Message)
}

type runtimeTerminalDeliveryError struct {
	cause error
}

func (e *runtimeTerminalDeliveryError) Error() string {
	if e == nil || e.cause == nil {
		return "Runtime terminal delivery failed"
	}
	return e.cause.Error()
}

func (e *runtimeTerminalDeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func runtimeTerminalDeliveryFailure(err error) error {
	if err == nil {
		return nil
	}
	return &runtimeTerminalDeliveryError{cause: err}
}

// runtimeLifecycleOwnerDeliveryError is an SM-4 handoff failure, not a
// projection or terminal-delivery failure. Its exact persisted governance
// identity is retained so the existing DriveLease mechanism can re-deliver the
// same immutable fact without consuming the terminal delivery budget.
type runtimeLifecycleOwnerDeliveryError struct {
	handoff runtimeGovernanceLifecycleHandoff
	cause   error
}

func (e *runtimeLifecycleOwnerDeliveryError) Error() string {
	if e == nil || e.cause == nil {
		return "Runtime lifecycle owner delivery failed"
	}
	return e.cause.Error()
}

func (e *runtimeLifecycleOwnerDeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *runtimeLifecycleOwnerDeliveryError) leaseSignature() string {
	if e == nil {
		return "lifecycle-owner-delivery/invalid"
	}
	h := e.handoff
	return fmt.Sprintf("lifecycle-owner-delivery/%s/%s/%s/%s/%s/%s/%s", h.Kind, h.AuthoritySnapshotID, h.DispatchID, h.TicketID, h.ApprovalID, h.ResolutionID, h.FenceID())
}

func runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff runtimeGovernanceLifecycleHandoff, err error) error {
	if err == nil {
		return nil
	}
	if handoff.validate() != nil {
		return &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: fmt.Errorf("runtime lifecycle owner delivery has invalid persisted handoff: %w", err)}
	}
	return &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: err}
}

func runtimeLifecycleOwnerDeliveryFailure(result orchestratorevents.DriveRuntimeResult, err error) error {
	if err == nil {
		return nil
	}
	handoff := runtimeGovernanceLifecycleHandoff{
		AuthoritySnapshotID: result.AuthoritySnapshotID,
		DefinitionRevision:  result.DefinitionRevision,
		EvidenceRevision:    result.EvidenceRevision,
		DispatchID:          result.DispatchID,
		TicketID:            result.BlockingTicketID,
		ApprovalID:          result.BlockingApprovalID,
	}
	if handoff.ApprovalID != "" {
		handoff.Kind = runtimeGovernanceLifecycleWaitApproval
	} else {
		handoff.Kind = runtimeGovernanceLifecycleWaitDecision
	}
	return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
}

func DriveRuntime(ctx context.Context, hub magicevent.Hub, command orchestratorevents.DriveRuntimeCommand) (orchestratorevents.DriveRuntimeResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorDriveRuntime,
		runtimeInternalDriveSource,
		ModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.DriveRuntimeResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.DriveRuntimeResult)
	if !ok || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID {
		return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("orchestrator returned an invalid Runtime drive result")
	}
	if err := validateRuntimeDriveResult(response); err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	return response, nil
}

func (m *Biz) handleRuntimeDrive(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.DriveRuntimeCommand)
	if !ok || m == nil || event.Source() != runtimeInternalDriveSource {
		setRuntimeDriveResult(result, orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument)
		return
	}
	response, err := m.driveRuntime(event.Context(), command)
	setRuntimeDriveResult(result, response, err)
}

func (m *Biz) driveRuntime(ctx context.Context, command orchestratorevents.DriveRuntimeCommand) (orchestratorevents.DriveRuntimeResult, error) {
	if m == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	return m.driveRuntimeLocked(ctx, command)
}

// driveRuntimeLocked is called either by the synchronous EventHub handler
// through driveRuntime or by the background drive while lifecycleMu is held.
// Keeping this split avoids recursive RLock acquisition when Stop is pending.
func (m *Biz) driveRuntimeLocked(ctx context.Context, command orchestratorevents.DriveRuntimeCommand) (orchestratorevents.DriveRuntimeResult, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument
	}
	run, aggregate, err := m.initializeRuntimeDrive(ctx, command)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	for transition := 0; transition < runtimeDriveTransitionLimit; transition++ {
		if transition != 0 {
			// Runtime aggregate projection updates Run's opaque lifecycle fence on
			// every persisted transition. Re-read that owner state before comparing
			// a later aggregate revision; retaining the initialization pointer would
			// falsely reject a valid exact resolution after the first CAS.
			run, err = (runSnapshotReader{hub: m.EventHub()}).Read(ctx, command.WorkspaceID, command.RunID)
			if err != nil {
				return orchestratorevents.DriveRuntimeResult{}, err
			}
		}
		// The activation event above, and every owner-private transition below,
		// returns the authoritative persisted aggregate. A governance resolution
		// may have committed after driveAndProject's recovery preflight but before
		// this drive loaded that aggregate. Never let such an old drive cross into
		// SM-2/SM-3/provider/capability work until the Run owner proves the exact
		// persisted continuation resolution in its own lifecycle fence.
		if err := runtimeDriveContinuationFence(aggregate, run); err != nil {
			return orchestratorevents.DriveRuntimeResult{}, err
		}
		outcome, err := m.driveOneRuntimeTransition(ctx, run, aggregate, command, stablePrefixHash)
		if err != nil {
			return orchestratorevents.DriveRuntimeResult{}, err
		}
		if outcome.result != nil {
			return *outcome.result, nil
		}
		aggregate = outcome.aggregate
	}
	return orchestratorevents.DriveRuntimeResult{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime drive exceeded %d persisted transitions", runtimeDriveTransitionLimit)}
}

func runtimeDriveContinuationFence(aggregate runtime.RuntimeAggregate, run *runmodel.Run) error {
	if runtimeConvergenceTerminal(aggregate.Decision.State) || aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance {
		return nil
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&record)
	if err != nil {
		return err
	}
	if !governed || (handoff.Kind != runtimeGovernanceLifecycleContinuationDecision && handoff.Kind != runtimeGovernanceLifecycleContinuationApproval) {
		return nil
	}
	fact, err := runContinuationEligibleFact(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		handoff.AuthoritySnapshotID,
		handoff.DefinitionRevision,
		handoff.EvidenceRevision,
		handoff.DispatchID,
		handoff.ResolutionID,
		handoff.TicketID,
		handoff.ApprovalID,
	)
	if err != nil {
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
	}
	// This is a read-only proof against Run-owned state. It neither delivers a
	// lifecycle command nor writes Run state: failure returns to the current
	// SM-4 gate owner, which retains the same persisted handoff in DriveLease.
	return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, validateRunContinuationAcknowledgement(
		runevents.RunLifecycleCommandResult{Run: run}, fact, aggregate.StateRevision,
	))
}

type runtimeDriveTransitionOutcome struct {
	aggregate runtime.RuntimeAggregate
	result    *orchestratorevents.DriveRuntimeResult
}

func (m *Biz) initializeRuntimeDrive(ctx context.Context, command orchestratorevents.DriveRuntimeCommand) (*runmodel.Run, runtime.RuntimeAggregate, error) {
	run, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		return nil, runtime.RuntimeAggregate{}, err
	}
	aggregate, err := m.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		return nil, runtime.RuntimeAggregate{}, err
	}
	return run, aggregate, nil
}

func (m *Biz) driveOneRuntimeTransition(ctx context.Context, run *runmodel.Run, aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand, stablePrefixHash string) (runtimeDriveTransitionOutcome, error) {
	previousRevision := aggregate.StateRevision
	previousConvergence := runtime.RuntimeConvergenceProgressSignature(aggregate)
	// A catalog retirement can persist a fresh SM-3 semantic generation before
	// SM-2 has a binding for the newly eligible CWI. The reducer decision stored
	// in that same owner-local CAS is not operation authority until the semantic
	// generation is sealed. In particular, a target-grounding terminal produced
	// from the binding-free focus must not cross into SM-4 delivery. A terminal
	// is final at this gate only when semantic operation authority is already
	// usable, or when the semantic transaction itself has failed.
	semanticAuthorityPending := runtimeSemanticAuthorityPending(aggregate)
	if runtimeTerminalDeliveryReady(aggregate) {
		return terminalRuntimeDriveTransition(aggregate), nil
	}
	if wait, pendingRetirement := runtimeRecoverableWaitPendingRetirement(aggregate); pendingRetirement {
		updated, retireErr := m.runtimeRetireRecoverableWait(ctx, aggregate, wait.WaitID)
		if retireErr != nil {
			return runtimeDriveTransitionOutcome{}, retireErr
		}
		if updated.StateRevision <= previousRevision {
			return runtimeDriveTransitionOutcome{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime recoverable wait retirement made no persisted progress at revision %d", previousRevision)}
		}
		return runtimeDriveTransitionOutcome{aggregate: updated}, nil
	}
	if _, waiting := runtimeCurrentRecoverableWait(aggregate); waiting {
		return m.driveRuntimeRecoverableWait(ctx, aggregate, command.RecoveryLeaseGeneration)
	}
	if aggregate.DefinitionClarification != nil {
		if aggregate.DefinitionClarification.Request != nil {
			result, resultErr := runtimeDefinitionClarificationWaitDriveResult(aggregate)
			if resultErr != nil {
				return runtimeDriveTransitionOutcome{}, resultErr
			}
			return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}, nil
		}
		updated, driveErr := m.driveRuntimeDefinitionClarification(ctx, aggregate)
		if driveErr != nil {
			return runtimeDriveTransitionOutcome{}, driveErr
		}
		if updated.StateRevision <= previousRevision {
			return runtimeDriveTransitionOutcome{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime Definition clarification made no persisted progress at revision %d", previousRevision)}
		}
		return runtimeDriveTransitionOutcome{aggregate: updated}, nil
	}
	if semanticAuthorityPending {
		return m.driveOneRuntimeSemanticTransition(ctx, aggregate, previousRevision, previousConvergence)
	}
	if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return terminalRuntimeDriveTransition(aggregate), nil
	}
	updated, changed, err := m.driveOneRuntimeReconciliationTransition(ctx, run, aggregate, command)
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	if changed {
		if updated.StateRevision <= previousRevision {
			return runtimeDriveTransitionOutcome{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime reconciliation made no persisted progress at revision %d", previousRevision)}
		}
		return runtimeDriveTransitionOutcome{aggregate: updated}, nil
	}
	updated, err = m.advanceRuntimeConvergence(ctx, updated, stablePrefixHash)
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	return finishRuntimeConvergenceTransition(updated, previousRevision, previousConvergence)
}

func (m *Biz) driveRuntimeDefinitionClarification(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if aggregate.DefinitionClarification == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	state := aggregate.DefinitionClarification.Clone()
	if state.Dispatch == nil {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventDefinitionClarificationDispatchSealed,
			Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
			Transaction: &runtime.RuntimeInternalTransactionPayload{},
		})
		return updated.Aggregate, err
	}
	return m.prepareRuntimeDefinitionClarificationRequest(ctx, aggregate)
}

func runtimeConvergenceTerminal(state runtimecontract.ConvergenceState) bool {
	return state == runtimecontract.ConvergenceStateComplete || state == runtimecontract.ConvergenceStateFailed
}

// runtimeSemanticAuthorityPending is the SM-3-before-SM-4 ordering fence. A
// retired/requesting semantic generation is still owner-local work whenever
// the current CWI cannot reuse an execution-locked sealed binding.
func runtimeSemanticAuthorityPending(aggregate runtime.RuntimeAggregate) bool {
	return aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed &&
		!runtime.SemanticOperationAuthorityReady(aggregate)
}

// runtimeTerminalDeliveryReady prevents the one reducer terminal that can be
// computed from a binding-free focus while a new semantic generation is still
// pending from escaping as an SM-4 outcome. Other reducer terminals already
// express durable authority (for example a contract defect or exhausted
// budget); a failed semantic transaction is likewise final.
func runtimeTerminalDeliveryReady(aggregate runtime.RuntimeAggregate) bool {
	if !runtimeConvergenceTerminal(aggregate.Decision.State) || aggregate.Decision.Terminal == nil {
		return false
	}
	if aggregate.Decision.Terminal.Code != "target_grounding_unresolved" ||
		aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return true
	}
	return runtime.SemanticOperationAuthorityReady(aggregate)
}

func terminalRuntimeDriveTransition(aggregate runtime.RuntimeAggregate) runtimeDriveTransitionOutcome {
	result := runtimeTerminalDriveResult(aggregate)
	return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}
}

func (m *Biz) driveOneRuntimeSemanticTransition(ctx context.Context, aggregate runtime.RuntimeAggregate, previousRevision uint64, previousConvergence string) (runtimeDriveTransitionOutcome, error) {
	updated, err := m.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	if updated.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return terminalRuntimeDriveTransition(updated), nil
	}
	if updated.StateRevision <= previousRevision {
		return runtimeDriveTransitionOutcome{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime semantic planning made no persisted progress at revision %d", previousRevision)}
	}
	if runtime.RuntimeConvergenceProgressSignature(updated) == previousConvergence {
		return runtimeDriveTransitionOutcome{}, &runtimePersistedDrivePendingError{Revision: updated.StateRevision}
	}
	return runtimeDriveTransitionOutcome{aggregate: updated}, nil
}

func (m *Biz) driveOneRuntimeReconciliationTransition(ctx context.Context, run *runmodel.Run, aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand) (runtime.RuntimeAggregate, bool, error) {
	required := aggregate.Facts.ReconciliationContract.SchemaVersion != "" &&
		(aggregate.Reconciliation.Batch == nil || reconciliationRequestNeedsRefresh(run, aggregate, command)) &&
		aggregate.ActiveDispatch == nil && !runtimeConvergenceTerminal(aggregate.Decision.State)
	if !required {
		return aggregate, false, nil
	}
	return m.reconcileRuntimeWorkspaceWithCommand(ctx, run, aggregate, command)
}

func (m *Biz) advanceRuntimeConvergence(ctx context.Context, aggregate runtime.RuntimeAggregate, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedPrerequisite:
		return m.prepareRuntimePrerequisites(ctx, aggregate)
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance:
		return m.driveRuntimeDecision(ctx, aggregate, stablePrefixHash)
	case runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateFailed:
		return aggregate, nil
	default:
		return runtime.RuntimeAggregate{}, fmt.Errorf("unsupported Runtime convergence state %q", aggregate.Decision.State)
	}
}

func finishRuntimeConvergenceTransition(aggregate runtime.RuntimeAggregate, previousRevision uint64, previousConvergence string) (runtimeDriveTransitionOutcome, error) {
	if _, waiting := runtimeCurrentRecoverableWait(aggregate); waiting {
		result, err := runtimeRecoverableWaitDriveResult(aggregate)
		return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}, err
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance && aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.Governance != nil {
		result, err := runtimeGovernedWaitDriveResult(aggregate)
		if err != nil {
			return runtimeDriveTransitionOutcome{}, err
		}
		return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}, nil
	}
	if runtimeTerminalDeliveryReady(aggregate) {
		return terminalRuntimeDriveTransition(aggregate), nil
	}
	if aggregate.StateRevision <= previousRevision {
		return runtimeDriveTransitionOutcome{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime drive made no persisted progress at revision %d", previousRevision)}
	}
	if runtime.RuntimeConvergenceProgressSignature(aggregate) == previousConvergence {
		return runtimeDriveTransitionOutcome{}, &runtimePersistedDrivePendingError{Revision: aggregate.StateRevision}
	}
	return runtimeDriveTransitionOutcome{aggregate: aggregate}, nil
}

func (m *Biz) driveRuntimeDecision(ctx context.Context, aggregate runtime.RuntimeAggregate, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	var err error
	if aggregate.ActiveDispatch == nil {
		// SMO-7A: a Shell validation diagnostic is only a candidate repair
		// surface. Before any next provider request, obtain File's current
		// read-only confirmation and feed it back through the Orchestrator's
		// internal-event/CAS boundary.
		if aggregate.Facts.Failure != nil && aggregate.Facts.Failure.ValidationFailureObservation != nil && aggregate.Facts.Resources.FailureTarget == nil {
			updated, handled, handoffErr := m.confirmRuntimeRepairSurface(ctx, aggregate)
			if handoffErr != nil {
				return runtime.RuntimeAggregate{}, handoffErr
			}
			if handled {
				return updated, nil
			}
		}
		// This gate runs before BeginCandidateAttempt. A stale selection or
		// focus-scoped prerequisite is SM-2 authority debt, not a malformed
		// provider candidate; persist the deterministic cleanup and let the
		// outer drive acquire the current owner fact on the next transition.
		if report := runtime.InspectRuntimeAuthorityInvariants(aggregate); len(report.Violations) != 0 {
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventReconcileAuthority, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
			})
			return updated.Aggregate, err
		}
		stale, staleErr := InspectValidationScratchPrerequisite(aggregate.Facts)
		if staleErr != nil {
			return runtime.RuntimeAggregate{}, staleErr
		}
		if stale != nil {
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{Stale: stale},
			})
			return updated.Aggregate, err
		}
		if feasibility, feasibilityErr := runtimeFeasibilityFact(aggregate); feasibilityErr != nil {
			return runtime.RuntimeAggregate{}, feasibilityErr
		} else if feasibility != nil {
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FeasibilityFact: feasibility},
			})
			return updated.Aggregate, err
		}
		aggregate, err = m.requestAndSealRuntimeCandidateWithRetry(ctx, aggregate, stablePrefixHash)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
	}
	if aggregate.ActiveDispatch == nil {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch.Candidate.Governance != nil {
		return m.prepareRuntimeGovernanceRequest(ctx, aggregate)
	}
	if aggregate.ActiveDispatch.Candidate.Assessment != nil {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventAssessmentObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
		})
		return updated.Aggregate, err
	}
	return m.executeRuntimeActiveDispatch(ctx, aggregate)
}

// driveAndProjectRuntimeLocked is only called from the background drive
// while lifecycleMu is held across recovery accounting and terminal delivery.
func (m *Biz) driveAndProjectRuntimeLocked(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	return m.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: workspaceID, RunID: runID})
}

func (m *Biz) driveAndProjectRuntimeLockedWithCommand(ctx context.Context, command orchestratorevents.DriveRuntimeCommand) error {
	continuationDelivered, err := m.deliverPersistedRuntimeGovernanceContinuation(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		return err
	}
	if continuationDelivered && command.RecoveryLeaseGeneration == 0 {
		// An ordinary drive stops after Run's owner event and lets that event
		// schedule the successor. A claimed governance-continuation lease is
		// different: the same durable generation is already the successor, so it
		// continues below only after the exact Run owner acknowledgement. This
		// closes the active-notification/gate race without inventing another
		// scheduler or allowing a timer to bypass the gate.
		return nil
	}
	result, err := m.driveRuntimeLocked(ctx, command)
	if err != nil {
		return err
	}
	switch {
	case result.RecoverableWait != nil:
		return &runtimeRecoverableWaitPendingError{Wait: result.RecoverableWait.Clone()}
	case result.Terminal == nil && result.State == runtimecontract.ConvergenceStateNeedGovernance:
		aggregate, aggregateErr := m.runtimeController.Load(ctx, command.WorkspaceID, command.RunID)
		if aggregateErr != nil {
			return runtimeLifecycleOwnerDeliveryFailure(result, aggregateErr)
		}
		record := runtimeStateRecordFromAggregate(aggregate)
		handoff, governed, handoffErr := runtimeGovernanceLifecycleHandoffFromState(&record)
		if handoffErr != nil || !governed || (handoff.Kind != runtimeGovernanceLifecycleWaitDefinitionClarification && handoff.Kind != runtimeGovernanceLifecycleWaitDecision && handoff.Kind != runtimeGovernanceLifecycleWaitApproval) {
			if handoffErr == nil {
				handoffErr = fmt.Errorf("governed Runtime result has no persisted lifecycle wait handoff")
			}
			return runtimeLifecycleOwnerDeliveryFailure(result, handoffErr)
		}
		run, loadErr := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, loadErr)
		}
		if (run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID == result.BlockingTicketID) ||
			(run.State == common.RunStateBlockedOnApproval && run.BlockingApprovalID == result.BlockingApprovalID) {
			fact, factErr := runGovernanceWaitFact(
				command.WorkspaceID, command.RunID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision,
				handoff.EvidenceRevision, handoff.DispatchID, handoff.TicketID, handoff.ApprovalID,
			)
			if factErr != nil {
				return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, factErr)
			}
			return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, validateRunGovernanceWaitAcknowledgement(
				runevents.RunLifecycleCommandResult{Run: run}, fact, aggregate.StateRevision,
			))
		}
		// RUNTIME-SMO: publish immutable governance-wait fact; Run owner transitions SM-1.
		authID, defRev, evidRev, dispatchID, ticketID, approvalID := governanceWaitFromDriveResult(result)
		return runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, m.applyRunGovernanceWait(ctx, command.WorkspaceID, command.RunID, authID, defRev, evidRev, dispatchID, ticketID, approvalID, result.StateRevision))
	case result.Terminal != nil:
		aggregate, loadErr := m.runtimeController.Load(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			return runtimeTerminalDeliveryFailure(loadErr)
		}
		return runtimeTerminalDeliveryFailure(m.deliverRuntimeTerminal(ctx, aggregate))
	default:
		return fmt.Errorf("unsupported Runtime production result state %q", result.State)
	}
}

// deliverPersistedRuntimeGovernanceContinuation closes the crash cut between
// the SM-4 governance-resolution CAS and Run's SM-1 continuation fact. It
// derives the exact fact only from the persisted aggregate and does not choose
// or mutate Run state. A blocked Run receives the owner command and this drive
// stops; an already-active Run means the same fact was acknowledged (including
// response-loss) and normal aggregate convergence may continue.
func (m *Biz) deliverPersistedRuntimeGovernanceContinuation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (bool, error) {
	if m == nil || m.runtimeController == nil || workspaceID == "" || runID == "" {
		return false, common.ErrInvalidArgument
	}
	aggregate, err := m.runtimeController.Load(ctx, workspaceID, runID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed || aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance {
		return false, nil
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&record)
	if err != nil {
		return false, err
	}
	if !governed || (handoff.Kind != runtimeGovernanceLifecycleContinuationDecision && handoff.Kind != runtimeGovernanceLifecycleContinuationApproval) {
		return false, nil
	}
	run, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, workspaceID, runID)
	if err != nil {
		return false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
	}
	if run.State == common.RunStateActive && run.BlockingTicketID == "" && run.BlockingApprovalID == "" {
		fact, factErr := runContinuationEligibleFact(
			workspaceID, runID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision,
			handoff.DispatchID, handoff.ResolutionID, handoff.TicketID, handoff.ApprovalID,
		)
		if factErr != nil {
			return false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, factErr)
		}
		// Response loss is acknowledged only by the exact persisted Run-owned
		// lifecycle fence. Active state alone is insufficient because an older
		// resolution for the same governance fence must fail closed.
		return false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, validateRunContinuationAcknowledgement(
			runevents.RunLifecycleCommandResult{Run: run}, fact, aggregate.StateRevision,
		))
	}
	exactBlocked := (handoff.Kind == runtimeGovernanceLifecycleContinuationDecision && run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID == handoff.TicketID && run.BlockingApprovalID == "") ||
		(handoff.Kind == runtimeGovernanceLifecycleContinuationApproval && run.State == common.RunStateBlockedOnApproval && run.BlockingApprovalID == handoff.ApprovalID && run.BlockingTicketID == "")
	if !exactBlocked {
		return false, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, fmt.Errorf("run lifecycle does not match persisted governance continuation blocker"))
	}
	err = m.applyRunContinuationEligible(ctx, workspaceID, runID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision, handoff.DispatchID, handoff.ResolutionID, handoff.TicketID, handoff.ApprovalID, aggregate.StateRevision)
	return err == nil, runtimeLifecycleOwnerDeliveryFailureForHandoff(handoff, err)
}

func runtimeTerminalDriveResult(aggregate runtime.RuntimeAggregate) orchestratorevents.DriveRuntimeResult {
	return orchestratorevents.DriveRuntimeResult{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DefinitionRevision:  aggregate.Facts.DefinitionRevision,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, State: aggregate.Decision.State,
		Terminal: aggregate.Decision.Terminal,
	}
}

func runtimeGovernedWaitDriveResult(aggregate runtime.RuntimeAggregate) (orchestratorevents.DriveRuntimeResult, error) {
	record := runtimeStateRecordFromAggregate(aggregate)
	handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&record)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	if !governed || (handoff.Kind != runtimeGovernanceLifecycleWaitDefinitionClarification &&
		handoff.Kind != runtimeGovernanceLifecycleWaitDecision && handoff.Kind != runtimeGovernanceLifecycleWaitApproval) {
		return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("governed Runtime wait has no bound owner request")
	}
	if handoff.Kind != runtimeGovernanceLifecycleWaitDefinitionClarification {
		request, found := governancecheck.RequestForDispatch(aggregate.GovernanceRequests, handoff.DispatchID, handoff.AuthoritySnapshotID)
		if !found || request.TicketID != handoff.TicketID || request.ApprovalID != handoff.ApprovalID {
			return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("governed Runtime wait lifecycle identity does not match its owner request")
		}
	}
	return orchestratorevents.DriveRuntimeResult{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StateRevision: aggregate.StateRevision, EvidenceRevision: handoff.EvidenceRevision,
		DefinitionRevision:  handoff.DefinitionRevision,
		AuthoritySnapshotID: handoff.AuthoritySnapshotID,
		DispatchID:          handoff.DispatchID,
		State:               aggregate.Decision.State,
		BlockingTicketID:    handoff.TicketID,
		BlockingApprovalID:  handoff.ApprovalID,
	}, nil
}

func runtimeDefinitionClarificationWaitDriveResult(aggregate runtime.RuntimeAggregate) (orchestratorevents.DriveRuntimeResult, error) {
	result, err := runtimeGovernedWaitDriveResult(aggregate)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	handoff, governed, handoffErr := runtimeGovernanceLifecycleHandoffFromState(&record)
	if handoffErr != nil || !governed || handoff.Kind != runtimeGovernanceLifecycleWaitDefinitionClarification {
		if handoffErr != nil {
			return orchestratorevents.DriveRuntimeResult{}, handoffErr
		}
		return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("runtime Definition clarification wait is not fully bound")
	}
	return result, nil
}

func validateRuntimeDriveResult(result orchestratorevents.DriveRuntimeResult) error {
	if result.WorkspaceID == "" || result.RunID == "" || result.StateRevision == 0 || result.AuthoritySnapshotID == "" || result.State == "" {
		return fmt.Errorf("invalid Runtime drive result")
	}
	switch {
	case result.RecoverableWait != nil:
		wait := result.RecoverableWait
		if result.Terminal != nil || result.BlockingTicketID != "" || result.BlockingApprovalID != "" ||
			result.State != runtimecontract.ConvergenceStateNeedPrerequisite || wait.WaitID == "" || !waitStatusActive(wait.Status) || wait.StateRevision != result.StateRevision {
			return fmt.Errorf("invalid recoverable-wait Runtime drive result")
		}
	case result.Terminal == nil && result.State == runtimecontract.ConvergenceStateNeedGovernance:
		hasTicket := result.BlockingTicketID != ""
		hasApproval := result.BlockingApprovalID != ""
		if result.State != runtimecontract.ConvergenceStateNeedGovernance || hasTicket == hasApproval || result.Terminal != nil {
			return fmt.Errorf("invalid governed Runtime drive result")
		}
	case result.Terminal != nil:
		if (result.State != runtimecontract.ConvergenceStateComplete && result.State != runtimecontract.ConvergenceStateFailed) || result.Terminal == nil || result.BlockingTicketID != "" || result.BlockingApprovalID != "" {
			return fmt.Errorf("invalid terminal Runtime drive result")
		}
	default:
		return fmt.Errorf("invalid Runtime drive result state %q", result.State)
	}
	return nil
}

func waitStatusActive(status runtimecontract.RuntimeRecoverableWaitStatus) bool {
	return status == runtimecontract.RuntimeRecoverableWaitWaiting || status == runtimecontract.RuntimeRecoverableWaitProbeClaimed
}

func setRuntimeDriveResult(result magicevent.Result, response orchestratorevents.DriveRuntimeResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
