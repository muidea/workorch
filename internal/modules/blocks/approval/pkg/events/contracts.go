package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicApprovalResolved,
	)...)
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicAcquireRuntimeCapabilities,
		TopicApprovalCommandGet,
		TopicApprovalCommandList,
		TopicApprovalCommandListByRun,
		TopicApprovalCommandListByWorkspace,
	)...)
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicApprovalCommandPurgeWorkspace,
		TopicApprovalCommandEnsureApproved,
		TopicApprovalCommandEnsureRejected,
		TopicApprovalCommandRecordEvidence,
		TopicApprovalCommandRecordResolution,
		TopicOpenRuntimeApprovalRequest,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicAcquireRuntimeCapabilities)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOpenRuntimeApprovalRequest)
	return ret
}
