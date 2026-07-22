package orchestrator

// Runtime V2 drive lifecycle is owned by Biz.

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeV2DriveTransitionLimit = 1024

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
			return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("%w: Runtime V2 drive made no persisted progress at revision %d", common.ErrConflict, previousRevision)
		}
		if runtimev2.RuntimeConvergenceProgressSignature(aggregate) == previousConvergence {
			return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("%w: Runtime V2 drive changed only retry bookkeeping at revision %d", common.ErrConflict, aggregate.StateRevision)
		}
	}
	return orchestratorevents.DriveRuntimeV2Result{}, fmt.Errorf("%w: Runtime V2 drive exceeded %d persisted transitions", common.ErrConflict, runtimeV2DriveTransitionLimit)
}

func (m *Biz) driveRuntimeV2Decision(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	var err error
	if aggregate.ActiveDispatch == nil {
		if feasibility, feasibilityErr := runtimeV2FeasibilityFact(aggregate); feasibilityErr != nil {
			return runtimev2.RuntimeAggregate{}, feasibilityErr
		} else if feasibility != nil {
			return m.bizPtr.RecordRuntimeV2FeasibilityFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *feasibility)
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
		return m.bizPtr.CommitRuntimeV2Assessment(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
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
		return runtimeV2ProjectionFailure(m.bizPtr.BlockOnDecision(ctx, runID, result.BlockingTicketID))
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
				AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, Action: aggregate.Decision.Action,
				BlockingTicketID: request.TicketID,
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
