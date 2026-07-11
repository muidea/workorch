package events

const NotificationDestination = "/memory/consumers/#"

const (
	TopicMemoryCreated        = "/memory/created"
	TopicMemoryUpdated        = "/memory/updated"
	TopicMemoryDeleted        = "/memory/deleted"
	TopicMemoryRecallRecorded = "/memory/recall/recorded"

	TopicMemoryCommandCreate            = "/memory/command/create"
	TopicMemoryCommandGet               = "/memory/command/get"
	TopicMemoryCommandListByWorkspace   = "/memory/command/list-by-workspace"
	TopicMemoryCommandSearch            = "/memory/command/search"
	TopicMemoryCommandUpdate            = "/memory/command/update"
	TopicMemoryCommandDelete            = "/memory/command/delete"
	TopicMemoryRecallTraceCommandUpsert = "/memory/recall/command/upsert"
	TopicMemoryRecallTraceCommandGet    = "/memory/recall/command/get"
	TopicMemoryRecallTraceCommandList   = "/memory/recall/command/list"
	TopicMemoryRecallTraceCommandDelete = "/memory/recall/command/delete"
)
