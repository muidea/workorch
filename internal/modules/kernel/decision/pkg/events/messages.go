package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateTicketCommand = decisioncontracts.CreateTicketCommand
type EnsureTicketCommand = decisioncontracts.CreateTicketCommand
type UpdateTicketCommand = decisioncontracts.UpdateTicketCommand
type DeleteTicketCommand = decisioncontracts.DeleteTicketCommand
type CreateResolutionCommand = decisioncontracts.CreateResolutionCommand
type EnsureResolvedCommand = decisioncontracts.ResolveTicketCommand
type DeleteResolutionCommand = decisioncontracts.DeleteResolutionCommand

type GetTicketCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	TicketID    common.TicketID
}

type ListTicketsCommand struct{}

type ListTicketsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListTicketsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetResolutionCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	TicketID    common.TicketID
}

type DecisionData struct {
	Ticket     *decisionmodel.Ticket
	Resolution *decisionmodel.Resolution
}

type DecisionCommandResult struct {
	Ticket      *decisionmodel.Ticket
	Tickets     []*decisionmodel.Ticket
	Resolution  *decisionmodel.Resolution
	Resolutions []*decisionmodel.Resolution
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandCreateTicket).MessageContract(),
		eventcontract.CommandTopic[EnsureTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureTicket).MessageContract(),
		eventcontract.CommandTopic[GetTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetTicket).MessageContract(),
		eventcontract.CommandTopic[ListTicketsCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTickets).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByRunCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByRun).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByWorkspaceCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByWS).MessageContract(),
		eventcontract.CommandTopic[UpdateTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandUpdateTicket).MessageContract(),
		eventcontract.CommandTopic[DeleteTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteTicket).MessageContract(),
		eventcontract.CommandTopic[CreateResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandCreateResolution).MessageContract(),
		eventcontract.CommandTopic[GetResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetResolution).MessageContract(),
		eventcontract.CommandTopic[DeleteResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteResolution).MessageContract(),
		eventcontract.CommandTopic[EnsureResolvedCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureResolved).MessageContract(),
	}
	for _, topic := range []string{
		TopicTicketCreated,
		TopicTicketResolved,
		TopicTicketSuperseded,
		TopicTicketUpdated,
		TopicDecisionTicketCreated,
		TopicDecisionTicketResolved,
		TopicDecisionTicketSuperseded,
	} {
		ret = append(ret, eventcontract.DataTopic[DecisionData]("decision", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
