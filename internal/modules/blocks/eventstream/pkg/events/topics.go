package events

const (
	NotificationDestination                         = "/eventstream/consumers/#"
	TopicEventStreamCommandList                     = "/eventstream/command/list"
	TopicEventStreamCommandHighWatermarks           = "/eventstream/command/high-watermarks"
	TopicEventStreamCommandSealWorkspaceDeletion    = "/eventstream/command/seal-workspace-deletion"
	TopicEventStreamCommandPurgeWorkspace           = "/eventstream/command/purge-workspace"
	TopicEventStreamCommandRecordApprovalResolution = "/eventstream/command/record-approval-resolution"
	TopicEventStreamRecordAppended                  = "/eventstream/record/appended"
)
