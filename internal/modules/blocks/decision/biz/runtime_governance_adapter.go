package decision

import (
	"context"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeGovernanceRequest(event magicevent.Event, result magicevent.Result) {
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument)
		return
	}
	command, ok := event.Data().(decisionevents.OpenRuntimeGovernanceRequestCommand)
	if !ok || m == nil || m.service == nil || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" ||
		command.DispatchID == "" || command.Transaction.AuthoritySnapshotID == "" || command.Dependency.Kind == "" || command.Dependency.Reason == "" ||
		!modulecommon.IsSupportedResolutionType(command.ContinueResolutionType) || !modulecommon.IsSupportedResolutionType(command.FailResolutionType) {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(event.Context(), command.WorkspaceID)
	if err != nil {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
		return
	}
	unlock := m.lockWorkspaceMutation(command.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ctx, command.WorkspaceID); err != nil {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
		return
	}
	ticket, _, err := m.service.EnsureTicketAndReturn(ctx, decisioncontracts.CreateTicketCommand{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID: common.StepID("runtime/" + command.DispatchID), Reason: command.Dependency.Reason,
		DecisionKind: "runtime_governance", Question: command.Dependency.Reason,
		FailureClass: "governance_dependency", FailureSignature: command.Dependency.Kind + ":" + command.Dependency.SubjectRef,
		AllowedRecoveryActions: []string{command.ContinueResolutionType, command.FailResolutionType},
	})
	if err != nil {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
		return
	}
	if err := m.ensureTicketCreatedNotification(ctx, ticket); err != nil {
		setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{}, err)
		return
	}
	setRuntimeGovernanceRequestResult(result, decisionevents.OpenRuntimeGovernanceRequestResult{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		DispatchID: command.DispatchID, AuthoritySnapshotID: command.Transaction.AuthoritySnapshotID,
	}, nil)
}

func setRuntimeGovernanceRequestResult(result magicevent.Result, response decisionevents.OpenRuntimeGovernanceRequestResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) executeRuntimeGovernanceResolution(ctx context.Context, cmd decisionevents.EnsureResolvedCommand, resolveCommand decisioncontracts.ResolveTicketCommand) (decisionevents.EnsureResolvedResult, error) {
	currentTicket, err := m.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return decisionevents.EnsureResolvedResult{}, err
	}
	if currentTicket.DecisionKind != cmd.DecisionKind {
		return decisionevents.EnsureResolvedResult{}, common.ErrConflict
	}
	ticket, proposal, err := m.service.EnsureRuntimeGovernanceResolutionProposal(ctx, resolveCommand)
	if err != nil {
		return decisionevents.EnsureResolvedResult{}, err
	}
	commit, err := m.commitRuntimeGovernanceTicketProposal(ctx, ticket, proposal)
	if err != nil {
		return decisionevents.EnsureResolvedResult{}, err
	}
	switch commit.Disposition {
	case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
		orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
		ticket, resolution, transitioned, err := m.service.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, resolveCommand)
		if err != nil {
			return decisionevents.EnsureResolvedResult{}, err
		}
		if err := m.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
			return decisionevents.EnsureResolvedResult{}, err
		}
		return decisionevents.EnsureResolvedResult{Ticket: decisionmodel.CloneTicket(ticket), Resolution: decisionmodel.CloneResolution(resolution)}, nil
	case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
		if err := m.service.RejectRuntimeGovernanceResolutionProposal(ctx, resolveCommand, "aggregate_commit_stale", "runtime governance proposal belongs to a stale aggregate authority"); err != nil {
			return decisionevents.EnsureResolvedResult{}, err
		}
		return decisionevents.EnsureResolvedResult{}, common.ErrConflict
	default:
		return decisionevents.EnsureResolvedResult{}, common.ErrConflict
	}
}

func (m *Biz) commitRuntimeGovernanceTicketProposal(ctx context.Context, ticket *decisionmodel.Ticket, proposal *decisionmodel.RuntimeGovernanceResolutionProposal) (orchestratorevents.CommitRuntimeGovernanceResolutionResult, error) {
	if m == nil || m.EventHub() == nil || ticket == nil || proposal == nil || ticket.DecisionKind != "runtime_governance" {
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	outcome := runtimecontract.RuntimeGovernanceResolutionOutcome("")
	switch proposal.ResolutionType {
	case modulecommon.ResolutionTypeProvideMissingInfo:
		outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeContinue
	case modulecommon.ResolutionTypeRejectCurrentPath:
		outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeFail
	default:
		return orchestratorevents.CommitRuntimeGovernanceResolutionResult{}, common.ErrInvalidArgument
	}
	var response orchestratorevents.CommitRuntimeGovernanceResolutionResult
	err := m.withWorkspaceActiveDelivery(ctx, ticket.WorkspaceID, func(guarded context.Context) error {
		result := m.SendEvent(magicevent.NewEventWithContext(
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
			ModuleID,
			orchestratorevents.CommandDestination,
			magicevent.NewHeader(),
			guarded,
			orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand{
				ResolutionID: proposal.ResolutionID, TicketID: ticket.ID,
				WorkspaceID: ticket.WorkspaceID, RunID: ticket.RunID,
				Outcome: outcome, Summary: runtimeGovernanceProposalSummary(ticket, proposal),
			},
		))
		if result == nil {
			return common.ErrInvalidArgument
		}
		if resultErr := result.Error(); resultErr != nil {
			return resultErr
		}
		data, resultErr := result.Get()
		if resultErr != nil {
			return resultErr
		}
		value, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
		if !ok || value.WorkspaceID != ticket.WorkspaceID || value.RunID != ticket.RunID || value.ResolutionID != proposal.ResolutionID {
			return common.ErrInvalidArgument
		}
		if err := value.Validate(); err != nil {
			return err
		}
		response = value
		return nil
	})
	return response, err
}

func runtimeGovernanceProposalSummary(ticket *decisionmodel.Ticket, proposal *decisionmodel.RuntimeGovernanceResolutionProposal) string {
	if proposal != nil {
		if value, ok := proposal.Payload.String("summary"); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if ticket != nil && strings.TrimSpace(ticket.Reason) != "" {
		return strings.TrimSpace(ticket.Reason)
	}
	return "runtime governance resolution committed"
}
