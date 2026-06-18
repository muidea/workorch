package events

const (
	TopicTicketCreated  = "/ticket/created"
	TopicTicketResolved = "/ticket/resolved"
	TopicTicketUpdated  = "/ticket/updated"

	TopicDecisionCommandCreateTicket     = "/decision/command/create-ticket"
	TopicDecisionCommandEnsureTicket     = "/decision/command/ensure-ticket"
	TopicDecisionCommandGetTicket        = "/decision/command/get-ticket"
	TopicDecisionCommandListTickets      = "/decision/command/list-tickets"
	TopicDecisionCommandListTicketsByRun = "/decision/command/list-tickets-by-run"
	TopicDecisionCommandListTicketsByWS  = "/decision/command/list-tickets-by-workspace"
	TopicDecisionCommandUpdateTicket     = "/decision/command/update-ticket"
	TopicDecisionCommandDeleteTicket     = "/decision/command/delete-ticket"
	TopicDecisionCommandCreateResolution = "/decision/command/create-resolution"
	TopicDecisionCommandGetResolution    = "/decision/command/get-resolution"
	TopicDecisionCommandDeleteResolution = "/decision/command/delete-resolution"
	TopicDecisionCommandEnsureResolved   = "/decision/command/ensure-resolved"
)
