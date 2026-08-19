package events

const NotificationDestination = "/approval/consumers/#"

const (
	TopicApprovalResolved = "/approval/resolved"

	TopicApprovalCommandGet              = "/approval/command/get"
	TopicApprovalCommandList             = "/approval/command/list"
	TopicApprovalCommandListByRun        = "/approval/command/list-by-run"
	TopicApprovalCommandListByWorkspace  = "/approval/command/list-by-workspace"
	TopicApprovalCommandPurgeWorkspace   = "/approval/command/purge-workspace"
	TopicApprovalCommandEnsureApproved   = "/approval/command/ensure-approved"
	TopicApprovalCommandEnsureRejected   = "/approval/command/ensure-rejected"
	TopicApprovalCommandRecordEvidence   = "/approval/command/record-evidence"
	TopicApprovalCommandRecordResolution = "/approval/command/record-resolution"
	TopicOpenRuntimeApprovalRequest      = "/approval/runtime-governance/open-request"
	TopicAcquireRuntimeCapabilities      = "/approval/runtime-prerequisite/acquire-capabilities"
)
