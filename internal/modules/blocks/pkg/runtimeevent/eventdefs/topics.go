package eventdefs

import "github.com/muidea/workorch/internal/pkg/eventcontract"

const TopicRuntimeBlockCommandExecutePrefix = "/runtime/block/command/execute/"

const (
	TopicRuntimeBlockExecuteAccepted  = "/runtime/block/execute/accepted"
	TopicRuntimeBlockExecuteProgress  = "/runtime/block/execute/progress"
	TopicRuntimeBlockExecuteCompleted = "/runtime/block/execute/completed"
	TopicRuntimeBlockExecuteFailed    = "/runtime/block/execute/failed"
)

func RuntimeBlockExecuteTopic(kind string) string {
	return TopicRuntimeBlockCommandExecutePrefix + kind
}

func Contracts() []eventcontract.Contract {
	return eventcontract.Build("runtimeevent", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
		TopicRuntimeBlockExecuteAccepted,
		TopicRuntimeBlockExecuteProgress,
		TopicRuntimeBlockExecuteCompleted,
		TopicRuntimeBlockExecuteFailed,
	)
}
