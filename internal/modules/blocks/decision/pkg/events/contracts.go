package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicTicketCreated,
		TopicTicketResolved,
	)...)
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicAcquireRuntimeCapabilities,
		TopicDecisionCommandGetTicket,
		TopicDecisionCommandListTickets,
		TopicDecisionCommandListTicketsByRun,
		TopicDecisionCommandListTicketsByWS,
		TopicDecisionCommandGetResolution,
		TopicQueryWorkspaceDefinitionClarificationFact,
	)...)
	ret = append(ret, eventcontract.Build("decision", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicRegisterContractChangeProposal,
		TopicDecisionCommandEnsureResolved,
		TopicDecisionCommandPurgeWorkspace,
		TopicDecisionCommandSealWorkspaceDeletion,
		TopicOpenRuntimeGovernanceRequest,
		TopicOpenWorkspaceDefinitionClarification,
		TopicReconcileWorkspaceDefinitionClarificationCandidate,
		TopicResolveWorkspaceDefinitionClarificationByAdmission,
		TopicOpenRuntimeDefinitionClarification,
		TopicResolveRuntimeDefinitionClarificationByReplacement,
		TopicRepairRuntimeDefinitionClarificationRequestContract,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicAcquireRuntimeCapabilities)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOpenRuntimeGovernanceRequest)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOpenRuntimeDefinitionClarification)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicResolveRuntimeDefinitionClarificationByReplacement)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicRepairRuntimeDefinitionClarificationRequestContract)
	return ret
}
