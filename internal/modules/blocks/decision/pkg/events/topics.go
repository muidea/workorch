package events

const NotificationDestination = "/decision/consumers/#"

const (
	TopicTicketCreated    = "/ticket/created"
	TopicTicketResolved   = "/ticket/resolved"
	TopicTicketSuperseded = "/ticket/superseded"
	TopicTicketUpdated    = "/ticket/updated"

	TopicDecisionTicketCreated    = "/decision_ticket/created"
	TopicDecisionTicketResolved   = "/decision_ticket/resolved"
	TopicDecisionTicketSuperseded = "/decision_ticket/superseded"

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
	TopicDecisionCommandRecordEvidence   = "/decision/command/record-evidence"
)
