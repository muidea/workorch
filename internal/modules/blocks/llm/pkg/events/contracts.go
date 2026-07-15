package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicLLMCommandCompleteProvider,
		TopicLLMCommandGenerateEmbedding,
		TopicLLMCommandRefreshProviderModelCatalog,
	)...)
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicLLMCommandGetProviderModelCatalog,
		TopicLLMCommandResolveProviderModelTarget,
	)...)
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicLLMUsageRecorded,
	)...)
	return ret
}
