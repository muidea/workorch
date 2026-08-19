package decision

import (
	"context"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) executeEnsureResolvedCommand(ctx context.Context, data any) (decisionevents.EnsureResolvedResult, error) {
	cmd, ok := data.(decisionevents.EnsureResolvedCommand)
	if !ok {
		return decisionevents.EnsureResolvedResult{}, common.ErrInvalidArgument
	}
	if !decisionCommandHasScope(cmd) {
		ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
		if err != nil {
			return decisionevents.EnsureResolvedResult{}, err
		}
		cmd.WorkspaceID = ticket.WorkspaceID
		cmd.RunID = ticket.RunID
		cmd.StepID = ticket.StepID
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
		if err != nil {
			return decisionevents.EnsureResolvedResult{}, err
		}
	}
	resolveCommand := resolveTicketContract(cmd)
	if cmd.DecisionKind == "runtime_governance" {
		return m.executeRuntimeGovernanceResolution(ctx, cmd, resolveCommand)
	}
	ticket, resolution, transitioned, err := m.service.EnsureResolvedTicketAndReturnWithTransition(ctx, resolveCommand)
	if err != nil {
		return decisionevents.EnsureResolvedResult{}, err
	}
	if err := m.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
		return decisionevents.EnsureResolvedResult{}, err
	}
	return decisionevents.EnsureResolvedResult{Ticket: decisionmodel.CloneTicket(ticket), Resolution: decisionmodel.CloneResolution(resolution)}, nil
}

func (m *Biz) executeGetResolutionCommand(ctx context.Context, data any) (decisionevents.GetResolutionResult, error) {
	cmd, ok := data.(decisionevents.GetResolutionCommand)
	if !ok || cmd.TicketID == "" {
		return decisionevents.GetResolutionResult{}, common.ErrInvalidArgument
	}
	resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, cmd.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
	if err != nil {
		return decisionevents.GetResolutionResult{}, err
	}
	item, err := m.resolutions.GetByTicketID(resolutionCtx, cmd.TicketID)
	if err != nil {
		return decisionevents.GetResolutionResult{}, err
	}
	return decisionevents.GetResolutionResult{Resolution: decisionmodel.CloneResolution(item)}, nil
}

func decisionCommandHasScope(cmd decisionevents.EnsureResolvedCommand) bool {
	return cmd.WorkspaceID != "" || cmd.RunID != "" || cmd.StepID != ""
}

func resolveTicketContract(cmd decisionevents.EnsureResolvedCommand) decisioncontracts.ResolveTicketCommand {
	return decisioncontracts.ResolveTicketCommand{
		ResolutionID:       cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		WorkspaceID:        cmd.WorkspaceID,
		RunID:              cmd.RunID,
		StepID:             cmd.StepID,
		Type:               cmd.Type,
		Payload:            workorcheventpayload.New(cmd.Payload...),
		ContinuationAction: cmd.ContinuationAction,
		ResumeRunID:        cmd.ResumeRunID,
		SuccessorRunID:     cmd.SuccessorRunID,
	}
}
