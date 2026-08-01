package orchestrator

// Governance adapter is internal to the Runtime owner Biz.

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

func (m *Biz) handleRuntimeGovernanceTicketResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeGovernanceTicketResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeGovernanceTicketResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand)
	if !ok || m == nil || m.bizPtr == nil || command.ResolutionID == "" || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Summary) == "" {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	aggregate, err := m.bizPtr.LoadRuntime(event.Context(), command.WorkspaceID, command.RunID)
	if err != nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	var binding *runtimecontract.RuntimeGovernanceRequestRecord
	for idx := range aggregate.GovernanceRequests {
		if aggregate.GovernanceRequests[idx].TicketID == command.TicketID {
			value := aggregate.GovernanceRequests[idx]
			binding = &value
			break
		}
	}
	if binding == nil {
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

func CommitRuntimeGovernanceResolution(ctx context.Context, hub magicevent.Hub, command orchestratorevents.CommitRuntimeGovernanceResolutionCommand) (orchestratorevents.CommitRuntimeGovernanceResolutionResult, error) {
	if hub == nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	if err := command.Resolution.Validate(); err != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		"runtime-governance-continuation",
		ModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if !ok || response.ResolutionID != command.Resolution.ResolutionID {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, fmt.Errorf("orchestrator returned an invalid runtime governance resolution result")
	}
	return response, nil
}

func (m *Biz) handleRuntimeGovernanceResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeGovernanceResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeGovernanceResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceResolutionCommand)
	if !ok || m == nil || m.bizPtr == nil {
		setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	commitEvent, err := m.submitRuntimeInternalEvent(event.Context(), runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, WorkspaceID: command.Resolution.WorkspaceID, RunID: command.Resolution.RunID, GovernanceResolution: &command.Resolution,
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
		ResolutionID:        command.Resolution.ResolutionID,
		Disposition:         runtimeGovernanceDisposition(commit.Disposition),
		StateRevision:       commit.Aggregate.StateRevision,
		EvidenceRevision:    commit.Aggregate.Facts.EvidenceRevision,
		AuthoritySnapshotID: commit.Aggregate.Decision.AuthoritySnapshot.ID,
		State:               commit.Aggregate.Decision.State,
		Terminal:            commit.Aggregate.Decision.Terminal,
	}
	if commit.Disposition == runtime.CommitDispositionCommitted || commit.Disposition == runtime.CommitDispositionDuplicate {
		if err := m.continueRuntimeGovernance(event.Context(), response, command.Resolution); err != nil {
			setRuntimeGovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
			return
		}
	}
	setRuntimeGovernanceResult(result, response, nil)
}

func (m *Biz) continueRuntimeGovernance(ctx context.Context, response orchestratorevents.CommitRuntimeGovernanceResolutionResult, resolution runtimecontract.RuntimeGovernanceResolution) error {
	workspaceID := resolution.WorkspaceID
	runID := resolution.RunID
	if response.State == runtimecontract.ConvergenceStateComplete || response.State == runtimecontract.ConvergenceStateFailed {
		aggregate, err := m.bizPtr.LoadRuntime(ctx, workspaceID, runID)
		if err != nil {
			return err
		}
		return m.deliverRuntimeTerminal(ctx, aggregate)
	}
	// RUNTIME-SMO: persist the governance resolution first, then submit the
	// immutable continuation fact to Run. The subsequent RunStateChanged(active)
	// notification is the only trigger for the next SM-4 drive.
	aggregate, err := m.bizPtr.LoadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	authID, defRev, evidRev, dispatchID, resolutionID, ticketID, approvalID, stateRevision := continuationFromGovernanceCommit(resolution, response, aggregate)
	if err := m.applyRunContinuationEligible(ctx, workspaceID, runID, authID, defRev, evidRev, dispatchID, resolutionID, ticketID, approvalID, stateRevision); err != nil {
		return err
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
