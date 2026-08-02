package orchestrator

// Runtime drive lifecycle is owned by Biz.

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
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
	if m == nil || m.bizPtr == nil || m.EventHub() == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeResult{}, common.ErrInvalidArgument
	}
	run, err := (runEventRepository{hub: m.EventHub()}).GetByWorkspaceID(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	aggregate, err := m.bizPtr.InitializeRuntimeFromRun(ctx, run)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	for transition := 0; transition < runtimeDriveTransitionLimit; transition++ {
		previousRevision := aggregate.StateRevision
		previousConvergence := runtime.RuntimeConvergenceProgressSignature(aggregate)
		// A sealed SM-2 terminal is immutable. SM-4 may only deliver/replay it;
		// semantic planning must never rewrite an already judged outcome during
		// startup recovery.
		if aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
			return runtimeTerminalDriveResult(aggregate), nil
		}
		// Semantic planning is a persisted SM-2/SM-3 prerequisite for every
		// operation authority. SM-4 may only re-drive its current phase; it must
		// not let the provisional bootstrap focus enter the operations pipeline.
		if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed && aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed {
			aggregate, err = m.driveRuntimeSemanticPlanning(ctx, aggregate)
			if err != nil {
				return orchestratorevents.DriveRuntimeResult{}, err
			}
			if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
				return runtimeTerminalDriveResult(aggregate), nil
			}
			if aggregate.StateRevision <= previousRevision {
				return orchestratorevents.DriveRuntimeResult{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime semantic planning made no persisted progress at revision %d", previousRevision)}
			}
			if runtime.RuntimeConvergenceProgressSignature(aggregate) == previousConvergence {
				return orchestratorevents.DriveRuntimeResult{}, &runtimePersistedDrivePendingError{Revision: aggregate.StateRevision}
			}
			continue
		}
		if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
			return runtimeTerminalDriveResult(aggregate), nil
		}
		// Reconciliation is an SM-2 preflight input. The request and complete
		// owner-observation batch are persisted through the same internal-event
		// CAS boundary before any candidate, operation or governance path runs.
		if aggregate.Facts.ReconciliationContract.SchemaVersion != "" &&
			(aggregate.Reconciliation.Batch == nil || reconciliationRequestNeedsRefresh(aggregate, command)) &&
			aggregate.ActiveDispatch == nil && aggregate.Decision.State != runtimecontract.ConvergenceStateComplete && aggregate.Decision.State != runtimecontract.ConvergenceStateFailed {
			updated, changed, reconciliationErr := m.reconcileRuntimeWorkspaceWithCommand(ctx, run, aggregate, command)
			if reconciliationErr != nil {
				return orchestratorevents.DriveRuntimeResult{}, reconciliationErr
			}
			aggregate = updated
			if changed {
				if aggregate.StateRevision <= previousRevision {
					return orchestratorevents.DriveRuntimeResult{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime reconciliation made no persisted progress at revision %d", previousRevision)}
				}
				continue
			}
		}
		switch aggregate.Decision.State {
		case runtimecontract.ConvergenceStateNeedPrerequisite:
			aggregate, err = m.prepareRuntimePrerequisites(ctx, aggregate)
		case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance:
			aggregate, err = m.driveRuntimeDecision(ctx, aggregate, stablePrefixHash)
		case runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateFailed:
			return runtimeTerminalDriveResult(aggregate), nil
		default:
			return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("unsupported Runtime convergence state %q", aggregate.Decision.State)
		}
		if err != nil {
			return orchestratorevents.DriveRuntimeResult{}, err
		}
		if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance && aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.Governance != nil {
			return runtimeGovernedWaitDriveResult(aggregate)
		}
		if aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
			return runtimeTerminalDriveResult(aggregate), nil
		}
		if aggregate.StateRevision <= previousRevision {
			return orchestratorevents.DriveRuntimeResult{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime drive made no persisted progress at revision %d", previousRevision)}
		}
		if runtime.RuntimeConvergenceProgressSignature(aggregate) == previousConvergence {
			return orchestratorevents.DriveRuntimeResult{}, &runtimePersistedDrivePendingError{Revision: aggregate.StateRevision}
		}
	}
	return orchestratorevents.DriveRuntimeResult{}, &runtimeInvariantError{Message: fmt.Sprintf("Runtime drive exceeded %d persisted transitions", runtimeDriveTransitionLimit)}
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
				Kind: runtime.RuntimeInternalEventReconcileAuthority, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			})
			return updated.Aggregate, err
		}
		stale, staleErr := InspectValidationScratchPrerequisite(aggregate.Facts)
		if staleErr != nil {
			return runtime.RuntimeAggregate{}, staleErr
		}
		if stale != nil {
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Stale: stale,
			})
			return updated.Aggregate, err
		}
		if feasibility, feasibilityErr := runtimeFeasibilityFact(aggregate); feasibilityErr != nil {
			return runtime.RuntimeAggregate{}, feasibilityErr
		} else if feasibility != nil {
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, FeasibilityFact: feasibility,
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
			Kind: runtime.RuntimeInternalEventAssessmentObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
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
	switch result.Disposition {
	case orchestratorevents.RuntimeDriveDispositionGovernedWait:
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
	case orchestratorevents.RuntimeDriveDispositionTerminal:
		aggregate, loadErr := m.bizPtr.LoadRuntime(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			return runtimeProjectionFailure(loadErr)
		}
		return runtimeProjectionFailure(m.deliverRuntimeTerminal(ctx, aggregate))
	default:
		return fmt.Errorf("unsupported Runtime production disposition %q", result.Disposition)
	}
}

func runtimeTerminalDriveResult(aggregate runtime.RuntimeAggregate) orchestratorevents.DriveRuntimeResult {
	return orchestratorevents.DriveRuntimeResult{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Disposition:   orchestratorevents.RuntimeDriveDispositionTerminal,
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
	for _, request := range aggregate.GovernanceRequests {
		if request.DispatchID == aggregate.ActiveDispatch.ID && request.AuthoritySnapshotID == aggregate.Decision.AuthoritySnapshot.ID {
			return orchestratorevents.DriveRuntimeResult{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				Disposition:   orchestratorevents.RuntimeDriveDispositionGovernedWait,
				StateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
				DefinitionRevision:  aggregate.Facts.DefinitionRevision,
				AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
				DispatchID:          request.DispatchID,
				State:               aggregate.Decision.State,
				BlockingTicketID:    request.TicketID,
			}, nil
		}
	}
	return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("governed Runtime wait has no bound ticket")
}

func validateRuntimeDriveResult(result orchestratorevents.DriveRuntimeResult) error {
	if result.WorkspaceID == "" || result.RunID == "" || result.StateRevision == 0 || result.AuthoritySnapshotID == "" || result.State == "" {
		return fmt.Errorf("invalid Runtime drive result")
	}
	switch result.Disposition {
	case orchestratorevents.RuntimeDriveDispositionGovernedWait:
		if result.State != runtimecontract.ConvergenceStateNeedGovernance || result.BlockingTicketID == "" || result.Terminal != nil {
			return fmt.Errorf("invalid governed Runtime drive result")
		}
	case orchestratorevents.RuntimeDriveDispositionTerminal:
		if (result.State != runtimecontract.ConvergenceStateComplete && result.State != runtimecontract.ConvergenceStateFailed) || result.Terminal == nil || result.BlockingTicketID != "" {
			return fmt.Errorf("invalid terminal Runtime drive result")
		}
	default:
		return fmt.Errorf("invalid Runtime drive disposition %q", result.Disposition)
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
