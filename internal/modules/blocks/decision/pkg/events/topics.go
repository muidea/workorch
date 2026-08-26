package events

const NotificationDestination = "/decision/consumers/#"

const (
	TopicTicketCreated  = "/ticket/created"
	TopicTicketResolved = "/ticket/resolved"

	TopicDecisionCommandGetTicket                           = "/decision/command/get-ticket"
	TopicDecisionCommandListTickets                         = "/decision/command/list-tickets"
	TopicDecisionCommandListTicketsByRun                    = "/decision/command/list-tickets-by-run"
	TopicDecisionCommandListTicketsByWS                     = "/decision/command/list-tickets-by-workspace"
	TopicDecisionCommandGetResolution                       = "/decision/command/get-resolution"
	TopicDecisionCommandEnsureResolved                      = "/decision/command/ensure-resolved"
	TopicDecisionCommandPurgeWorkspace                      = "/decision/command/purge-workspace"
	TopicDecisionCommandSealWorkspaceDeletion               = "/decision/command/seal-workspace-deletion"
	TopicDecisionCommandRecordEvidence                      = "/decision/command/record-evidence"
	TopicOpenRuntimeGovernanceRequest                       = "/decision/runtime-governance/open-request"
	TopicAcquireRuntimeCapabilities                         = "/decision/runtime-prerequisite/acquire-capabilities"
	TopicOpenWorkspaceDefinitionClarification               = "/decision/definition-clarification/workspace/open"
	TopicReconcileWorkspaceDefinitionClarificationCandidate = "/decision/definition-clarification/workspace/reconcile-candidate"
	TopicResolveWorkspaceDefinitionClarificationByAdmission = "/decision/definition-clarification/workspace/resolve-by-admission"
	TopicQueryWorkspaceDefinitionClarificationFact          = "/decision/definition-clarification/workspace/query-persisted-fact"
	TopicOpenRuntimeDefinitionClarification                 = "/decision/definition-clarification/runtime/open"
	TopicResolveRuntimeDefinitionClarificationByReplacement = "/decision/definition-clarification/runtime/resolve-by-replacement"
)
