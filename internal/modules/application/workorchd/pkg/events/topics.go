package events

const (
	TopicWorkorchDCommandDone                            = "/workorchd/command/done"
	TopicWorkorchDCommandWorkspaceActivationRequested    = "/workorchd/command/workspace-activation/requested"
	TopicWorkorchDCommandRunContinuationRequested        = "/workorchd/command/run-continuation/requested"
	TopicWorkorchDCommandGovernanceContinuationRequested = "/workorchd/command/governance-continuation/requested"
	TopicWorkorchDCommandReplanContinuationRequested     = "/workorchd/command/replan-continuation/requested"
	TopicWorkorchDCommandWorkspaceRestartRequested       = "/workorchd/command/workspace-restart/requested"
	TopicWorkspaceProjectionChanged                      = "/workorchd/workspace/projection/changed"
	TopicCLICommand                                      = "/cli/command"
	TopicCLIResult                                       = "/cli/result"
	TopicCLIProgress                                     = "/cli/progress"
)
