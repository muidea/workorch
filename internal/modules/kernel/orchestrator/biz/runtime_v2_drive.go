package orchestrator

// Runtime V2 drive lifecycle is owned by Biz.

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeV2DriveTransitionLimit = 1024

// runtimeV2PersistedDrivePendingError means an owner-local transition was
// durably committed but changed only bounded request/recovery bookkeeping. It
// is not business convergence and not a CAS failure; the scheduler may re-drive
// the newly persisted revision without consuming another recovery ledger.
type runtimeV2PersistedDrivePendingError struct {
	Revision uint64
}

func (e *runtimeV2PersistedDrivePendingError) Error() string {
	if e == nil {
		return "Runtime V2 has persisted pending drive work"
	}
	return fmt.Sprintf("Runtime V2 has persisted pending drive work at revision %d", e.Revision)
}

type runtimeV2InvariantError struct {
	Message string
}

func (e *runtimeV2InvariantError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "Runtime V2 drive invariant failed"
	}
	return strings.TrimSpace(e.Message)
}

type runtimeV2ProjectionError struct {
	cause error
}

func (e *runtimeV2ProjectionError) Error() string {
	if e == nil || e.cause == nil {
		return "Runtime V2 projection failed"
	}
	return e.cause.Error()
}

func (e *runtimeV2ProjectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func runtimeV2ProjectionFailure(err error) error {
	if err == nil {
		return nil
	}
	return &runtimeV2ProjectionError{cause: err}
}

func DriveRuntimeV2(ctx context.Context, hub magicevent.Hub, command orchestratorevents.DriveRuntimeV2Command) (orchestratorevents.DriveRuntimeV2Result, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeV2Result{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorDriveRuntimeV2,
		"runtime-v2-driver",
		ModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.DriveRuntimeV2Result{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.DriveRuntimeV2Result)
	if !ok || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID {
		return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("orchestrator returned an invalid Runtime V2 drive result")
	}
	if err := validateRuntimeV2DriveResult(response); err != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, err
	}
	return response, nil
}

func (m *Biz) handleRuntimeV2Drive(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.DriveRuntimeV2Command)
	if !ok || m == nil {
		setRuntimeV2DriveResult(result, orchestratorevents.DriveRuntimeV2Result{}, common.ErrInvalidArgument)
		return
	}
	response, err := m.driveRuntimeV2(event.Context(), command)
	setRuntimeV2DriveResult(result, response, err)
}

func (m *Biz) driveRuntimeV2(ctx context.Context, command orchestratorevents.DriveRuntimeV2Command) (orchestratorevents.DriveRuntimeV2Result, error) {
	if m == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeV2Result{}, common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, err
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	return m.driveRuntimeV2Locked(ctx, command)
}

// driveRuntimeV2Locked is called either by the synchronous EventHub handler
// through driveRuntimeV2 or by the background drive while lifecycleMu is held.
// Keeping this split avoids recursive RLock acquisition when Stop is pending.
func (m *Biz) driveRuntimeV2Locked(ctx context.Context, command orchestratorevents.DriveRuntimeV2Command) (orchestratorevents.DriveRuntimeV2Result, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.DriveRuntimeV2Result{}, common.ErrInvalidArgument
	}
	run, err := (runEventRepository{hub: m.EventHub()}).GetByWorkspaceID(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, err
	}
	aggregate, err := m.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		return orchestratorevents.DriveRuntimeV2Result{}, err
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	for transition := 0; transition < runtimeV2DriveTransitionLimit; transition++ {
		previousRevision := aggregate.StateRevision
		previousConvergence := runtimev2.RuntimeConvergenceProgressSignature(aggregate)
		switch aggregate.Decision.Action {
		case runtimecontract.RuntimeActionAcquirePrerequisite:
			aggregate, err = m.prepareRuntimeV2Prerequisites(ctx, aggregate)
		case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment, runtimecontract.RuntimeActionRequestGovernance:
			aggregate, err = m.driveRuntimeV2Decision(ctx, aggregate, stablePrefixHash)
		case runtimecontract.RuntimeActionComplete, runtimecontract.RuntimeActionFail:
			return runtimeV2TerminalDriveResult(aggregate), nil
		default:
			return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("unsupported Runtime V2 action %q", aggregate.Decision.Action)
		}
		if err != nil {
			return orchestratorevents.DriveRuntimeV2Result{}, err
		}
		if aggregate.Decision.Action == runtimecontract.RuntimeActionRequestGovernance && aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.Governance != nil {
			return runtimeV2GovernedWaitDriveResult(aggregate)
		}
		if aggregate.Decision.Action == runtimecontract.RuntimeActionComplete || aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
			return runtimeV2TerminalDriveResult(aggregate), nil
		}
		if aggregate.StateRevision <= previousRevision {
			return orchestratorevents.DriveRuntimeV2Result{}, &runtimeV2InvariantError{Message: fmt.Sprintf("Runtime V2 drive made no persisted progress at revision %d", previousRevision)}
		}
		if runtimev2.RuntimeConvergenceProgressSignature(aggregate) == previousConvergence {
			return orchestratorevents.DriveRuntimeV2Result{}, &runtimeV2PersistedDrivePendingError{Revision: aggregate.StateRevision}
		}
	}
	return orchestratorevents.DriveRuntimeV2Result{}, &runtimeV2InvariantError{Message: fmt.Sprintf("Runtime V2 drive exceeded %d persisted transitions", runtimeV2DriveTransitionLimit)}
}

func (m *Biz) driveRuntimeV2Decision(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	var err error
	if aggregate.ActiveDispatch == nil {
		// SMO-7A: a Shell validation diagnostic is only a candidate repair
		// surface. Before any next provider request, obtain File's current
		// read-only confirmation and feed it back through the Orchestrator's
		// internal-event/CAS boundary.
		if aggregate.Facts.Failure != nil && aggregate.Facts.Failure.ValidationFailureObservation != nil && aggregate.Facts.Resources.FailureTarget == nil {
			updated, handled, handoffErr := m.confirmRuntimeV2RepairSurface(ctx, aggregate)
			if handoffErr != nil {
				return runtimev2.RuntimeAggregate{}, handoffErr
			}
			if handled {
				return updated, nil
			}
		}
		// This gate runs before BeginCandidateAttempt. A stale selection or
		// focus-scoped prerequisite is SM-2 authority debt, not a malformed
		// provider candidate; persist the deterministic cleanup and let the
		// outer drive acquire the current owner fact on the next transition.
		if report := runtimev2.InspectRuntimeAuthorityInvariants(aggregate); len(report.Violations) != 0 {
			updated, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
				Kind: runtimev2.RuntimeInternalEventReconcileAuthority, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			})
			return updated.Aggregate, err
		}
		stale, staleErr := InspectValidationScratchPrerequisite(aggregate.Facts)
		if staleErr != nil {
			return runtimev2.RuntimeAggregate{}, staleErr
		}
		if stale != nil {
			updated, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
				Kind: runtimev2.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Stale: stale,
			})
			return updated.Aggregate, err
		}
		if feasibility, feasibilityErr := runtimeV2FeasibilityFact(aggregate); feasibilityErr != nil {
			return runtimev2.RuntimeAggregate{}, feasibilityErr
		} else if feasibility != nil {
			updated, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
				Kind: runtimev2.RuntimeInternalEventFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, FeasibilityFact: feasibility,
			})
			return updated.Aggregate, err
		}
		aggregate, err = m.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, stablePrefixHash)
		if err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	if aggregate.ActiveDispatch == nil {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch.Candidate.Governance != nil {
		return m.prepareRuntimeV2GovernanceRequest(ctx, aggregate)
	}
	if aggregate.ActiveDispatch.Candidate.Assessment != nil {
		updated, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
			Kind: runtimev2.RuntimeInternalEventAssessmentObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		})
		return updated.Aggregate, err
	}
	return m.executeRuntimeV2ActiveDispatch(ctx, aggregate)
}

// driveAndProjectRuntimeV2Locked is only called from the background drive
// while lifecycleMu is held across recovery accounting and terminal delivery.
func (m *Biz) driveAndProjectRuntimeV2Locked(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	result, err := m.driveRuntimeV2Locked(ctx, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return err
	}
	switch result.Disposition {
	case orchestratorevents.RuntimeV2DriveDispositionGovernedWait:
		run, loadErr := (runEventRepository{hub: m.EventHub()}).GetByWorkspaceID(ctx, workspaceID, runID)
		if loadErr != nil {
			return runtimeV2ProjectionFailure(loadErr)
		}
		if run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID == result.BlockingTicketID {
			return nil
		}
		// RUNTIME-SMO: publish immutable governance-wait fact; Run owner migrates SM-1.
		authID, defRev, evidRev, dispatchID, ticketID, approvalID := governanceWaitFromDriveResult(result)
		return runtimeV2ProjectionFailure(m.applyRunGovernanceWait(ctx, workspaceID, runID, authID, defRev, evidRev, dispatchID, ticketID, approvalID, result.StateRevision))
	case orchestratorevents.RuntimeV2DriveDispositionTerminal:
		aggregate, loadErr := m.bizPtr.LoadRuntimeV2(ctx, workspaceID, runID)
		if loadErr != nil {
			return runtimeV2ProjectionFailure(loadErr)
		}
		return runtimeV2ProjectionFailure(m.deliverRuntimeV2Terminal(ctx, aggregate))
	default:
		return fmt.Errorf("unsupported Runtime V2 production disposition %q", result.Disposition)
	}
}

func runtimeV2TerminalDriveResult(aggregate runtimev2.RuntimeAggregate) orchestratorevents.DriveRuntimeV2Result {
	return orchestratorevents.DriveRuntimeV2Result{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Disposition:   orchestratorevents.RuntimeV2DriveDispositionTerminal,
		StateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DefinitionRevision:  aggregate.Facts.DefinitionRevision,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, Action: aggregate.Decision.Action,
		Terminal: aggregate.Decision.Terminal,
	}
}

func runtimeV2GovernedWaitDriveResult(aggregate runtimev2.RuntimeAggregate) (orchestratorevents.DriveRuntimeV2Result, error) {
	if aggregate.ActiveDispatch == nil {
		return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("governed Runtime V2 wait has no active dispatch")
	}
	for _, request := range aggregate.GovernanceRequests {
		if request.DispatchID == aggregate.ActiveDispatch.ID && request.AuthoritySnapshotID == aggregate.Decision.AuthoritySnapshot.ID {
			return orchestratorevents.DriveRuntimeV2Result{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				Disposition:   orchestratorevents.RuntimeV2DriveDispositionGovernedWait,
				StateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
				DefinitionRevision:  aggregate.Facts.DefinitionRevision,
				AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
				DispatchID:          request.DispatchID,
				Action:              aggregate.Decision.Action,
				BlockingTicketID:    request.TicketID,
			}, nil
		}
	}
	return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("governed Runtime V2 wait has no bound ticket")
}

func validateRuntimeV2DriveResult(result orchestratorevents.DriveRuntimeV2Result) error {
	if result.WorkspaceID == "" || result.RunID == "" || result.StateRevision == 0 || result.AuthoritySnapshotID == "" || result.Action == "" {
		return fmt.Errorf("invalid Runtime V2 drive result")
	}
	switch result.Disposition {
	case orchestratorevents.RuntimeV2DriveDispositionGovernedWait:
		if result.Action != runtimecontract.RuntimeActionRequestGovernance || result.BlockingTicketID == "" || result.Terminal != nil {
			return fmt.Errorf("invalid governed Runtime V2 drive result")
		}
	case orchestratorevents.RuntimeV2DriveDispositionTerminal:
		if (result.Action != runtimecontract.RuntimeActionComplete && result.Action != runtimecontract.RuntimeActionFail) || result.Terminal == nil || result.BlockingTicketID != "" {
			return fmt.Errorf("invalid terminal Runtime V2 drive result")
		}
	default:
		return fmt.Errorf("invalid Runtime V2 drive disposition %q", result.Disposition)
	}
	return nil
}

func setRuntimeV2DriveResult(result magicevent.Result, response orchestratorevents.DriveRuntimeV2Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
