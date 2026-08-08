package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicLLMCommandCompleteProvider,
		TopicLLMCommandGenerateEmbedding,
		TopicLLMCommandRefreshProviderModelCatalog,
		TopicRequestRuntimeCandidate,
	)...)
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicAcquireRuntimeCapabilities,
		TopicLLMCommandGetProviderModelCatalog,
		TopicLLMCommandValidateInferenceSelection,
		TopicAcquireProviderCeiling,
		TopicPreflightRuntimeCandidate,
	)...)
	ret = append(ret, eventcontract.Build("llm", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicLLMUsageRecorded,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicAcquireRuntimeCapabilities)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicAcquireProviderCeiling, TopicPreflightRuntimeCandidate)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassExecute, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicRequestRuntimeCandidate)
	ret = eventcontract.AnnotateLaneIdentity(ret, "llm-provider-completion", TopicLLMCommandCompleteProvider)
	ret = eventcontract.AnnotateLaneIdentity(ret, "llm-embedding", TopicLLMCommandGenerateEmbedding)
	return ret
}
