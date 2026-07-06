package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicApprovalCreated,
		TopicApprovalApproved,
		TopicApprovalRejected,
		TopicApprovalResolved,
		TopicApprovalSuperseded,
	)...)
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicApprovalCommandGet,
		TopicApprovalCommandList,
		TopicApprovalCommandListByRun,
		TopicApprovalCommandListByWorkspace,
	)...)
	ret = append(ret, eventcontract.Build("approval", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicApprovalCommandCreate,
		TopicApprovalCommandEnsure,
		TopicApprovalCommandUpdate,
		TopicApprovalCommandDelete,
		TopicApprovalCommandEnsureApproved,
		TopicApprovalCommandEnsureRejected,
	)...)
	return ret
}
