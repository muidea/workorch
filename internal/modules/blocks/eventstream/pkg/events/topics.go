package events

const (
	NotificationDestination                      = "/eventstream/consumers/#"
	TopicEventStreamCommandList                  = "/eventstream/command/list"
	TopicEventStreamCommandHighWatermarks        = "/eventstream/command/high-watermarks"
	TopicEventStreamCommandSealWorkspaceDeletion = "/eventstream/command/seal-workspace-deletion"
	TopicEventStreamCommandPurgeWorkspace        = "/eventstream/command/purge-workspace"
	TopicEventStreamRecordAppended               = "/eventstream/record/appended"
)
