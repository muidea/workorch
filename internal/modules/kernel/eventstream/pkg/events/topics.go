package events

const (
	NotificationDestination               = "/eventstream/consumers/#"
	TopicEventStreamCommandList           = "/eventstream/command/list"
	TopicEventStreamCommandHighWatermarks = "/eventstream/command/high-watermarks"
	TopicEventStreamCommandPurgeWorkspace = "/eventstream/command/purge-workspace"
	TopicEventStreamRecordAppended        = "/eventstream/record/appended"
)
