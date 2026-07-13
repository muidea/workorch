package events

const NotificationDestination = "/workorchd/consumers/#"

const (
	TopicWorkorchDCommandDone                  = "/workorchd/command/done"
	TopicWorkorchDCommandWorkspaceRunRequested = "/workorchd/command/workspace-run/requested"
	TopicWorkorchDCommandRuntimeV1Retirement   = "/workorchd/command/runtime-v1-retirement"
	TopicWorkspaceProjectionChanged            = "/workorchd/workspace/projection/changed"
	TopicCLICommand                            = "/cli/command"
	TopicCLIProgress                           = "/cli/progress"
)
