package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicTicketCreated,
		TopicTicketResolved,
		TopicTicketSuperseded,
		TopicTicketUpdated,
		TopicDecisionTicketCreated,
		TopicDecisionTicketResolved,
		TopicDecisionTicketSuperseded,
	)...)
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicDecisionCommandGetTicket,
		TopicDecisionCommandListTickets,
		TopicDecisionCommandListTicketsByRun,
		TopicDecisionCommandListTicketsByWS,
		TopicDecisionCommandGetResolution,
	)...)
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicDecisionCommandCreateTicket,
		TopicDecisionCommandEnsureTicket,
		TopicDecisionCommandUpdateTicket,
		TopicDecisionCommandDeleteTicket,
		TopicDecisionCommandCreateResolution,
		TopicDecisionCommandDeleteResolution,
		TopicDecisionCommandEnsureResolved,
		TopicDecisionCommandRecordEvidence,
	)...)
	return ret
}
