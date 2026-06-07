package events

const (
	TopicResultAccepted   = "/result/accepted"
	TopicResultRejected   = "/result/rejected"
	TopicResultSuperseded = "/result/superseded"
	TopicArtifactCreated  = "/artifact/created"

	TopicResultCommandCreate          = "/result/command/create"
	TopicResultCommandGet             = "/result/command/get"
	TopicResultCommandListByWorkspace = "/result/command/list-by-workspace"
	TopicResultCommandListByRun       = "/result/command/list-by-run"
	TopicResultCommandUpdate          = "/result/command/update"
	TopicResultCommandDelete          = "/result/command/delete"

	TopicArtifactCommandCreate          = "/artifact/command/create"
	TopicArtifactCommandGet             = "/artifact/command/get"
	TopicArtifactCommandListByWorkspace = "/artifact/command/list-by-workspace"
	TopicArtifactCommandListByRun       = "/artifact/command/list-by-run"
	TopicArtifactCommandListByResult    = "/artifact/command/list-by-result"
	TopicArtifactCommandUpdate          = "/artifact/command/update"
	TopicArtifactCommandDelete          = "/artifact/command/delete"
)
