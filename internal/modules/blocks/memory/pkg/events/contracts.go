package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("memory", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicMemoryCreated,
		TopicMemoryUpdated,
		TopicMemoryDeleted,
		TopicMemoryRecallRecorded,
	)...)
	ret = append(ret, eventcontract.Build("memory", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicMemoryCommandGet,
		TopicMemoryCommandListByWorkspace,
		TopicMemoryCommandSearch,
		TopicMemoryRecallTraceCommandList,
	)...)
	ret = append(ret, eventcontract.Build("memory", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicMemoryCommandCreate,
		TopicMemoryCommandUpdate,
		TopicMemoryCommandDelete,
		TopicMemoryRecallTraceCommandUpsert,
		TopicMemoryCommandSealWorkspaceDeletion,
		TopicMemoryCommandPurgeWorkspace,
	)...)
	return ret
}
