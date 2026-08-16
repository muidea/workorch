package orchestrator

// Runtime drive lifecycle is owned by Biz.

import (
	"context"
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

// runtimeFreshActivationRequiredError marks a Run-owned, persisted
// administrative outcome. It is intentionally private to the Orchestrator
// scheduler: the Run owner exposes the user-facing typed resume admission,
// while SM-4 must stop without classifying this as a drive failure or creating
// a recovery lease.
type runtimeFreshActivationRequiredError struct {
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	RuntimeStateRef string
}

func (e *runtimeFreshActivationRequiredError) Error() string {
	if e == nil {
		return "Runtime requires fresh activation"
	}
	return fmt.Sprintf("Runtime requires fresh activation workspace=%s run=%s", e.WorkspaceID, e.RunID)
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

type runtimeProjectionError struct {
	cause error
}

func (e *runtimeProjectionError) Error() string {
	if e == nil || e.cause == nil {
		return "Runtime projection failed"
	}
	return e.cause.Error()
}

func (e *runtimeProjectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func runtimeProjectionFailure(err error) error {
	if err == nil {
		return nil
	}
	return &runtimeProjectionError{cause: err}
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

type runtimeDriveTransitionOutcome struct {
	aggregate runtime.RuntimeAggregate
	result    *orchestratorevents.DriveRuntimeResult
}

func (m *Biz) initializeRuntimeDrive(ctx context.Context, command orchestratorevents.DriveRuntimeCommand) (*runmodel.Run, runtime.RuntimeAggregate, error) {
	run, err := (runEventRepository{hub: m.EventHub()}).GetByWorkspaceID(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		return nil, runtime.RuntimeAggregate{}, err
	}
	if run.RuntimeActivationStatus != nil && run.RuntimeActivationStatus.Disposition == runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		return nil, runtime.RuntimeAggregate{}, &runtimeFreshActivationRequiredError{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, RuntimeStateRef: run.RuntimeActivationStatus.RuntimeStateRef,
		}
	}
	aggregate, err := m.initializeRuntimeFromRun(ctx, run)
	if err == nil {
		return run, aggregate, nil
	}
	incompatibility, incompatible := runtimecontract.RuntimeStateIncompatibilityFromError(err)
	if !incompatible || run.RuntimeState == nil {
		return nil, runtime.RuntimeAggregate{}, err
	}
	runtimeStateRef := runevents.RuntimeStateRef(run.WorkspaceID, run.ID, run.RuntimeState.StateRevision)
	if factErr := m.applyRunRuntimeStateIncompatible(ctx, run.WorkspaceID, run.ID, runtimeStateRef, incompatibility, err); factErr != nil {
		return nil, runtime.RuntimeAggregate{}, factErr
	}
	return nil, runtime.RuntimeAggregate{}, &runtimeFreshActivationRequiredError{WorkspaceID: run.WorkspaceID, RunID: run.ID, RuntimeStateRef: runtimeStateRef}
}

func (m *Biz) driveOneRuntimeTransition(ctx context.Context, run *runmodel.Run, aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand, stablePrefixHash string) (runtimeDriveTransitionOutcome, error) {
	previousRevision := aggregate.StateRevision
	previousConvergence := runtime.RuntimeConvergenceProgressSignature(aggregate)
	if runtimeConvergenceTerminal(aggregate.Decision.State) {
		return terminalRuntimeDriveTransition(aggregate), nil
	}
	if !runtime.SemanticOperationAuthorityReady(aggregate) && aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed {
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

func runtimeConvergenceTerminal(state runtimecontract.ConvergenceState) bool {
	return state == runtimecontract.ConvergenceStateComplete || state == runtimecontract.ConvergenceStateFailed
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
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance && aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.Governance != nil {
		result, err := runtimeGovernedWaitDriveResult(aggregate)
		if err != nil {
			return runtimeDriveTransitionOutcome{}, err
		}
		return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}, nil
	}
	if runtimeConvergenceTerminal(aggregate.Decision.State) {
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
	result, err := m.driveRuntimeLocked(ctx, command)
	if err != nil {
		return err
	}
	switch {
	case result.Terminal == nil && result.State == runtimecontract.ConvergenceStateNeedGovernance:
		run, loadErr := (runEventRepository{hub: m.EventHub()}).GetByWorkspaceID(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			return runtimeProjectionFailure(loadErr)
		}
		if run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID == result.BlockingTicketID {
			return nil
		}
		// RUNTIME-SMO: publish immutable governance-wait fact; Run owner migrates SM-1.
		authID, defRev, evidRev, dispatchID, ticketID, approvalID := governanceWaitFromDriveResult(result)
		return runtimeProjectionFailure(m.applyRunGovernanceWait(ctx, command.WorkspaceID, command.RunID, authID, defRev, evidRev, dispatchID, ticketID, approvalID, result.StateRevision))
	case result.Terminal != nil:
		aggregate, loadErr := m.runtimeController.Load(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			return runtimeProjectionFailure(loadErr)
		}
		return runtimeProjectionFailure(m.deliverRuntimeTerminal(ctx, aggregate))
	default:
		return fmt.Errorf("unsupported Runtime production result state %q", result.State)
	}
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
	if aggregate.ActiveDispatch == nil {
		return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("governed Runtime wait has no active dispatch")
	}
	request, found := governancecheck.RequestForDispatch(aggregate.GovernanceRequests, aggregate.ActiveDispatch.ID, aggregate.Decision.AuthoritySnapshot.ID)
	if found {
		return orchestratorevents.DriveRuntimeResult{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			StateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
			DefinitionRevision:  aggregate.Facts.DefinitionRevision,
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			DispatchID:          request.DispatchID,
			State:               aggregate.Decision.State,
			BlockingTicketID:    request.TicketID,
		}, nil
	}
	return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("governed Runtime wait has no bound ticket")
}

func validateRuntimeDriveResult(result orchestratorevents.DriveRuntimeResult) error {
	if result.WorkspaceID == "" || result.RunID == "" || result.StateRevision == 0 || result.AuthoritySnapshotID == "" || result.State == "" {
		return fmt.Errorf("invalid Runtime drive result")
	}
	switch {
	case result.Terminal == nil && result.State == runtimecontract.ConvergenceStateNeedGovernance:
		if result.State != runtimecontract.ConvergenceStateNeedGovernance || result.BlockingTicketID == "" || result.Terminal != nil {
			return fmt.Errorf("invalid governed Runtime drive result")
		}
	case result.Terminal != nil:
		if (result.State != runtimecontract.ConvergenceStateComplete && result.State != runtimecontract.ConvergenceStateFailed) || result.Terminal == nil || result.BlockingTicketID != "" {
			return fmt.Errorf("invalid terminal Runtime drive result")
		}
	default:
		return fmt.Errorf("invalid Runtime drive result state %q", result.State)
	}
	return nil
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
