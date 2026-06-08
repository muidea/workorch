package eventdefs

const TopicRuntimeBlockCommandExecutePrefix = "/runtime/block/command/execute/"

func RuntimeBlockExecuteTopic(kind string) string {
	return TopicRuntimeBlockCommandExecutePrefix + kind
}
