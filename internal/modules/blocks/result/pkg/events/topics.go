package events

const NotificationDestination = "/result/consumers/#"

const (
	TopicResultCreated    = "/result/created"
	TopicResultAccepted   = "/result/accepted"
	TopicResultRejected   = "/result/rejected"
	TopicResultSuperseded = "/result/superseded"

	TopicResultCommandGet             = "/result/command/get"
	TopicResultCommandListByWorkspace = "/result/command/list-by-workspace"
	TopicResultCommandListByRun       = "/result/command/list-by-run"
	TopicResultCommandDelete          = "/result/command/delete"
	// RUNTIME-SMO terminal delivery: Result owner records sealed outcome.
	TopicResultFactTerminalDelivery = "/result/fact/terminal-delivery"

	TopicArtifactCommandListByWorkspace = "/artifact/command/list-by-workspace"
	TopicArtifactCommandListByRun       = "/artifact/command/list-by-run"
	TopicArtifactCommandListByResult    = "/artifact/command/list-by-result"
	TopicArtifactCommandDelete          = "/artifact/command/delete"
)
