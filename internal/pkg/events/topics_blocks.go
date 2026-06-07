package events

const (
	TopicRuntimeBlockCommandExecutePrefix = "/runtime/block/command/execute/"
	TopicLLMCommandCompleteProvider       = "/llm/command/complete-provider"
	TopicLLMCommandGenerateEmbedding      = "/llm/command/generate-embedding"
)

func RuntimeBlockExecuteTopic(kind string) string {
	return TopicRuntimeBlockCommandExecutePrefix + kind
}
