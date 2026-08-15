package decision

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) executeResolutionCommand(ctx context.Context, ev magicevent.Event) (decisionevents.DecisionCommandResult, error) {
	switch ev.ID() {
	case decisionevents.TopicDecisionCommandEnsureResolved:
		return m.executeEnsureResolvedCommand(ctx, ev.Data())
	case decisionevents.TopicDecisionCommandGetResolution:
		return m.executeGetResolutionCommand(ctx, ev.Data())
	case decisionevents.TopicDecisionCommandDeleteResolution:
		return m.executeDeleteResolutionCommand(ctx, ev.Data())
	default:
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) executeEnsureResolvedCommand(ctx context.Context, data any) (decisionevents.DecisionCommandResult, error) {
	cmd, ok := data.(decisionevents.EnsureResolvedCommand)
	if !ok {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	if !decisionCommandHasScope(cmd) {
		ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
		cmd.WorkspaceID = ticket.WorkspaceID
		cmd.RunID = ticket.RunID
		cmd.StepID = ticket.StepID
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
		if err != nil {
			return decisionevents.DecisionCommandResult{}, err
		}
	}
	resolveCommand := resolveTicketContract(cmd)
	if cmd.DecisionKind == "runtime_governance" {
		return m.executeRuntimeGovernanceResolution(ctx, cmd, resolveCommand)
	}
	ticket, resolution, transitioned, err := m.service.EnsureResolvedTicketAndReturnWithTransition(ctx, resolveCommand)
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	if err := m.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}, nil
}

func (m *Biz) executeGetResolutionCommand(ctx context.Context, data any) (decisionevents.DecisionCommandResult, error) {
	cmd, ok := data.(decisionevents.GetResolutionCommand)
	if !ok || cmd.TicketID == "" {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, cmd.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID})
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	item, err := m.resolutions.GetByTicketID(resolutionCtx, cmd.TicketID)
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	return decisionevents.DecisionCommandResult{Resolution: item}, nil
}

func (m *Biz) executeDeleteResolutionCommand(ctx context.Context, data any) (decisionevents.DecisionCommandResult, error) {
	cmd, ok := data.(decisionevents.DeleteResolutionCommand)
	if !ok || cmd.TicketID == "" {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	resolutionCtx, err := m.resolveResolutionTicketContextForCommand(ctx, cmd.TicketID, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID})
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	if err := m.resolutions.DeleteByTicketID(resolutionCtx, cmd.TicketID); err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	return decisionevents.DecisionCommandResult{}, nil
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
