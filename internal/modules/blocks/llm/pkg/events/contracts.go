package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	return eventcontract.Build("llm", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicLLMCommandCompleteProvider,
		TopicLLMCommandGenerateEmbedding,
	)
}
