package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
		TopicRunCreated,
		TopicRunStateChanged,
		TopicRunLLMStreamProgress,
		TopicRunLLMUsageRecorded,
		TopicStepDispatched,
		TopicStepStateChanged,
	)...)
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicRunCommandGet,
		TopicRunCommandList,
		TopicRunCommandListByWorkspace,
		TopicRunCommandGetStep,
		TopicRunCommandListStepsByRun,
		TopicRunCommandGetRuntimeCheckpoint,
		TopicRunCommandListCheckpointsByRun,
	)...)
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicRunCommandCreate,
		TopicRunCommandEnsure,
		TopicRunCommandUpdate,
		TopicRunCommandDelete,
		TopicRunCommandStart,
		TopicRunCommandSupersede,
		TopicRunCommandFail,
		TopicRunCommandEnsureResumed,
	)...)
	return ret
}
