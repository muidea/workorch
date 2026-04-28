package events

const (
	TopicWorkspaceCreated      = "/workspace/created"
	TopicWorkspaceStateChanged = "/workspace/state/changed"
	TopicGoalDefined           = "/goal/defined"
	TopicDoDDefined            = "/dod/defined"
	TopicRunCreated            = "/run/created"
	TopicRunStateChanged       = "/run/state/changed"
	TopicStepDispatched        = "/step/dispatched"
	TopicStepStateChanged      = "/step/state/changed"
	TopicTicketCreated         = "/ticket/created"
	TopicTicketResolved        = "/ticket/resolved"
	TopicApprovalCreated       = "/approval/created"
	TopicApprovalResolved      = "/approval/resolved"
	TopicResultAccepted        = "/result/accepted"
	TopicArtifactCreated       = "/artifact/created"
)
