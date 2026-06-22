package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicResultAccepted,
		TopicResultRejected,
		TopicResultSuperseded,
		TopicArtifactCreated,
	)...)
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicResultCommandGet,
		TopicResultCommandListByWorkspace,
		TopicResultCommandListByRun,
		TopicArtifactCommandGet,
		TopicArtifactCommandListByWorkspace,
		TopicArtifactCommandListByRun,
		TopicArtifactCommandListByResult,
	)...)
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicResultCommandCreate,
		TopicResultCommandUpdate,
		TopicResultCommandDelete,
		TopicArtifactCommandCreate,
		TopicArtifactCommandUpdate,
		TopicArtifactCommandDelete,
	)...)
	return ret
}
