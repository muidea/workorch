package events

const NotificationDestination = "/decision/consumers/#"

const (
	TopicTicketCreated  = "/ticket/created"
	TopicTicketResolved = "/ticket/resolved"

	TopicDecisionCommandGetTicket        = "/decision/command/get-ticket"
	TopicDecisionCommandListTickets      = "/decision/command/list-tickets"
	TopicDecisionCommandListTicketsByRun = "/decision/command/list-tickets-by-run"
	TopicDecisionCommandListTicketsByWS  = "/decision/command/list-tickets-by-workspace"
	TopicDecisionCommandDeleteTicket     = "/decision/command/delete-ticket"
	TopicDecisionCommandGetResolution    = "/decision/command/get-resolution"
	TopicDecisionCommandDeleteResolution = "/decision/command/delete-resolution"
	TopicDecisionCommandEnsureResolved   = "/decision/command/ensure-resolved"
	TopicDecisionCommandRecordEvidence   = "/decision/command/record-evidence"
	TopicOpenRuntimeGovernanceRequest    = "/decision/runtime-governance/open-request"
	TopicAcquireRuntimeCapabilities      = "/decision/runtime-prerequisite/acquire-capabilities"
)
