package orchestrator

// Governance adapter is internal to the Runtime V2 owner Biz.

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

func (m *Biz) handleRuntimeV2GovernanceTicketResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeV2GovernanceTicketResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeV2GovernanceTicketResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand)
	if !ok || m == nil || m.bizPtr == nil || command.ResolutionID == "" || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Summary) == "" {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	aggregate, err := m.bizPtr.LoadRuntimeV2(event.Context(), command.WorkspaceID, command.RunID)
	if err != nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
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
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, fmt.Errorf("%w: runtime governance ticket has no persisted binding", common.ErrConflict))
		return
	}
	m.handleRuntimeV2GovernanceResolutionLocked(magicevent.NewEventWithContext(
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

func (m *Biz) handleRuntimeV2GovernanceResolution(event magicevent.Event, result magicevent.Result) {
	if m == nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	m.handleRuntimeV2GovernanceResolutionLocked(event, result)
}

func (m *Biz) handleRuntimeV2GovernanceResolutionLocked(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceResolutionCommand)
	if !ok || m == nil || m.bizPtr == nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	commitEvent, err := m.submitRuntimeV2InternalEvent(event.Context(), runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventGovernanceResolutionObserved, WorkspaceID: command.Resolution.WorkspaceID, RunID: command.Resolution.RunID, GovernanceResolution: &command.Resolution,
	})
	if err != nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
		return
	}
	if commitEvent.Evidence == nil {
		setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument)
		return
	}
	commit := *commitEvent.Evidence
	response := orchestratorevents.CommitRuntimeGovernanceResolutionResult{
		ResolutionID:        command.Resolution.ResolutionID,
		Disposition:         runtimeGovernanceDisposition(commit.Disposition),
		StateRevision:       commit.Aggregate.StateRevision,
		EvidenceRevision:    commit.Aggregate.Facts.EvidenceRevision,
		AuthoritySnapshotID: commit.Aggregate.Decision.AuthoritySnapshot.ID,
		Action:              commit.Aggregate.Decision.Action,
		Terminal:            commit.Aggregate.Decision.Terminal,
	}
	if commit.Disposition == runtimev2.CommitDispositionCommitted || commit.Disposition == runtimev2.CommitDispositionDuplicate {
		if err := m.continueRuntimeV2Governance(event.Context(), response, command.Resolution); err != nil {
			setRuntimeV2GovernanceResult(result, orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, err)
			return
		}
	}
	setRuntimeV2GovernanceResult(result, response, nil)
}

func (m *Biz) continueRuntimeV2Governance(ctx context.Context, response orchestratorevents.CommitRuntimeGovernanceResolutionResult, resolution runtimecontract.RuntimeGovernanceResolution) error {
	workspaceID := resolution.WorkspaceID
	runID := resolution.RunID
	if response.Action == runtimecontract.RuntimeActionComplete || response.Action == runtimecontract.RuntimeActionFail {
		aggregate, err := m.bizPtr.LoadRuntimeV2(ctx, workspaceID, runID)
		if err != nil {
			return err
		}
		return m.deliverRuntimeV2Terminal(ctx, aggregate)
	}
	// RUNTIME-SMO: persist the governance resolution first, then submit the
	// immutable continuation fact to Run. The subsequent RunStateChanged(active)
	// notification is the only trigger for the next SM-4 drive.
	aggregate, err := m.bizPtr.LoadRuntimeV2(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	authID, defRev, evidRev, dispatchID, resolutionID, ticketID, approvalID, stateRevision := continuationFromGovernanceCommit(resolution, response, aggregate)
	if err := m.applyRunContinuationEligible(ctx, workspaceID, runID, authID, defRev, evidRev, dispatchID, resolutionID, ticketID, approvalID, stateRevision); err != nil {
		return err
	}
	return nil
}

func runtimeGovernanceDisposition(value runtimev2.CommitDisposition) orchestratorevents.RuntimeGovernanceResolutionDisposition {
	switch value {
	case runtimev2.CommitDispositionCommitted:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted
	case runtimev2.CommitDispositionStale:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionStale
	case runtimev2.CommitDispositionDuplicate:
		return orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate
	default:
		return ""
	}
}

func setRuntimeV2GovernanceResult(result magicevent.Result, response orchestratorevents.CommitRuntimeGovernanceResolutionResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
