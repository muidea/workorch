package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateApprovalCommand = approvalcontracts.CreateApprovalCommand
type EnsureApprovalCommand = approvalcontracts.CreateApprovalCommand
type UpdateApprovalCommand = approvalcontracts.UpdateApprovalCommand
type DeleteApprovalCommand = approvalcontracts.DeleteApprovalCommand
type EnsureApprovedCommand = approvalcontracts.ResolveApprovalCommand
type EnsureRejectedCommand = approvalcontracts.ResolveApprovalCommand

type GetApprovalCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	ApprovalID  common.ApprovalID
}

type ListApprovalsCommand struct{}

type ListApprovalsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListApprovalsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ApprovalChangedData struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
	Action      string
	Context     []ApprovalEvidenceField
	State       common.ApprovalState
}

type ApprovalCommandResult struct {
	Approval  *approvalmodel.Approval
	Approvals []*approvalmodel.Approval
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandCreate).MessageContract(),
		eventcontract.CommandTopic[EnsureApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsure).MessageContract(),
		eventcontract.CommandTopic[GetApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandList).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByRunCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByRun).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByWorkspaceCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[UpdateApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandDelete).MessageContract(),
		eventcontract.CommandTopic[EnsureApprovedCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureApproved).MessageContract(),
		eventcontract.CommandTopic[EnsureRejectedCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureRejected).MessageContract(),
	}
	for _, topic := range []string{
		TopicApprovalCreated,
		TopicApprovalApproved,
		TopicApprovalRejected,
		TopicApprovalResolved,
		TopicApprovalSuperseded,
	} {
		ret = append(ret, eventcontract.DataTopic[ApprovalChangedData]("approval", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
