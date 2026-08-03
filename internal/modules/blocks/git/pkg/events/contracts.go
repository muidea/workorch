package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := eventcontract.Build("git", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities, TopicAcquireRuntimeOperationAdmissibility, TopicProbeRuntimeOperationFeasibility, TopicProbeRuntimeSemanticProofFeasibility, TopicQueryRuntimeCWIReconciliation, TopicAcquireCapabilityState)
	ret = append(ret, eventcontract.Build("git", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteOperation)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicAcquireRuntimeCapabilities, TopicAcquireRuntimeOperationAdmissibility, TopicProbeRuntimeOperationFeasibility, TopicProbeRuntimeSemanticProofFeasibility, TopicQueryRuntimeCWIReconciliation, TopicAcquireCapabilityState)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassExecute, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRunStep, eventcontract.IdentityPolicyRuntimeOperation, true), TopicExecuteOperation)
	return ret
}
