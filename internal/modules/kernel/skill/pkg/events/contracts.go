package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("skill", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicSkillCommandGet,
		TopicSkillCommandList,
	)...)
	ret = append(ret, eventcontract.Build("skill", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicSkillCommandCreate,
		TopicSkillCommandUpdate,
	)...)
	return ret
}
