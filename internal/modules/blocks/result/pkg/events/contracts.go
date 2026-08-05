package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicResultCreated,
		TopicResultAccepted,
		TopicResultRejected,
		TopicResultSuperseded,
	)...)
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicResultCommandGet,
		TopicResultCommandListByWorkspace,
		TopicResultCommandListByRun,
		TopicArtifactCommandListByWorkspace,
		TopicArtifactCommandListByRun,
		TopicArtifactCommandListByResult,
	)...)
	ret = append(ret, eventcontract.Build("result", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicResultCommandDelete,
		TopicResultCommandApplyTerminalDelivery,
		TopicArtifactCommandDelete,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicResultCommandApplyTerminalDelivery)
	return ret
}
