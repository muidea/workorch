package events

const NotificationDestination = "/approval/consumers/#"

const (
	TopicApprovalResolved = "/approval/resolved"

	TopicApprovalCommandGet                   = "/approval/command/get"
	TopicApprovalCommandList                  = "/approval/command/list"
	TopicApprovalCommandListByRun             = "/approval/command/list-by-run"
	TopicApprovalCommandListByWorkspace       = "/approval/command/list-by-workspace"
	TopicApprovalCommandPurgeWorkspace        = "/approval/command/purge-workspace"
	TopicApprovalCommandSealWorkspaceDeletion = "/approval/command/seal-workspace-deletion"
	TopicApprovalCommandEnsureApproved        = "/approval/command/ensure-approved"
	TopicApprovalCommandEnsureRejected        = "/approval/command/ensure-rejected"
	TopicOpenRuntimeApprovalRequest           = "/approval/runtime-governance/open-request"
	TopicAcquireRuntimeCapabilities           = "/approval/runtime-prerequisite/acquire-capabilities"
)
