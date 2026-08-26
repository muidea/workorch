package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("eventstream", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicEventStreamCommandList,
	)...)
	ret = append(ret, eventcontract.Build("eventstream", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicEventStreamCommandHighWatermarks,
	)...)
	ret = append(ret, eventcontract.Build("eventstream", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicEventStreamCommandSealWorkspaceDeletion,
		TopicEventStreamCommandPurgeWorkspace,
	)...)
	ret = append(ret, eventcontract.Build("eventstream", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicEventStreamRecordAppended,
	)...)
	return ret
}
