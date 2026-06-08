package events

const (
	TopicApprovalCreated  = "/approval/created"
	TopicApprovalResolved = "/approval/resolved"

	TopicApprovalCommandCreate          = "/approval/command/create"
	TopicApprovalCommandEnsure          = "/approval/command/ensure"
	TopicApprovalCommandGet             = "/approval/command/get"
	TopicApprovalCommandList            = "/approval/command/list"
	TopicApprovalCommandListByRun       = "/approval/command/list-by-run"
	TopicApprovalCommandListByWorkspace = "/approval/command/list-by-workspace"
	TopicApprovalCommandUpdate          = "/approval/command/update"
	TopicApprovalCommandDelete          = "/approval/command/delete"
	TopicApprovalCommandEnsureApproved  = "/approval/command/ensure-approved"
	TopicApprovalCommandEnsureRejected  = "/approval/command/ensure-rejected"
)
