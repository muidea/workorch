package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("soul", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicSoulCommandGet,
		TopicSoulCommandList,
		TopicSoulCommandValidateCapability,
		TopicSoulCommandQueryBuiltinDefaults,
	)...)
	ret = append(ret, eventcontract.Build("soul", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicSoulCommandCreate,
		TopicSoulCommandUpdate,
		TopicSoulCommandDelete,
		TopicSoulCommandEnsureDefaults,
		TopicSoulCommandUpgradeBuiltinDefault,
	)...)
	return ret
}
