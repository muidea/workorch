package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("governance", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
		TopicGovernanceAutoEvaluated,
		TopicGovernanceAutoResolved,
		TopicGovernanceAutoEscalated,
		TopicGovernanceAutoBlocked,
	)...)
	ret = append(ret, eventcontract.Build("governance", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicGovernanceAutoCommandListTraces,
	)...)
	return ret
}
