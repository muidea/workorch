package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
const CommandDestination = modulecommon.ModuleID

type DeleteApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}
type ResolveApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Context     []ApprovalEvidenceField
}

type EnsureApprovedCommand = ResolveApprovalCommand
type EnsureRejectedCommand = ResolveApprovalCommand

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
		eventcontract.CommandTopic[GetApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandList).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByRunCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByRun).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByWorkspaceCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[DeleteApprovalCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandDelete).MessageContract(),
		eventcontract.CommandTopic[EnsureApprovedCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureApproved).MessageContract(),
		eventcontract.CommandTopic[EnsureRejectedCommand, ApprovalCommandResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureRejected).MessageContract(),
		eventcontract.CommandTopic[RecordApprovalEvidenceCommand, RecordApprovalEvidenceResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandRecordEvidence).MessageContract(),
	}
	for _, topic := range []string{
		TopicApprovalResolved,
	} {
		ret = append(ret, eventcontract.DataTopic[ApprovalChangedData]("approval", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
