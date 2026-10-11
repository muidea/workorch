package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// RegisterContractChangeProposal 只登记问题材料；没有 HTTP、批准或应用入口。
// Input/Definition 基线由原 owner 读取反证，材料中的 ProposedBy 不是授权证明。
func (b *Biz) RegisterContractChangeProposal(ctx context.Context, command decisionevents.RegisterContractChangeProposalCommand) (decisionevents.RegisterContractChangeProposalResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if err := command.Validate(); err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.Proposal.WorkspaceID)
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	// 运行期 matching 尚未接通，应用端不开放有Run登记，不虚构替代scope。
	if command.Proposal.RunID != "" {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	current, err := workspaceports.ReadWorkspaceRecord(bound, b.EventHub(), command.Proposal.WorkspaceID)
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if current.ActiveRunID != "" {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	runs, err := ownerreadports.ListRunsByWorkspaceOwnerCommand(bound, b.EventHub(), command.Proposal.WorkspaceID)
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if len(runs) != 0 {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	if command.Proposal.BaseDefinitionRevision != "" {
		// 普通 Workspace DTO 有意不携带原 authoring/journal；不能在裁剪副本上重建持久权威。
		view, err := b.QueryDefinitionAuthoring(bound, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: command.Proposal.WorkspaceID})
		if err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		if view.WorkspaceRevision != current.Revision || view.Confirmation == nil || view.ActivationAdmission == nil {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
		if current.StartupInput == nil || current.StartupInput.DefinitionRevision != command.Proposal.BaseDefinitionRevision || current.StartupInput.StartupInputHash != command.Proposal.BaseStartupInputHash {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
	}
	if command.Proposal.BaseInputRevision != 0 {
		view, err := b.QueryDefinitionAuthoring(bound, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: command.Proposal.WorkspaceID})
		if err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		if view.WorkspaceRevision != current.Revision || view.Record == nil || view.Record.Input.Revision != command.Proposal.BaseInputRevision || view.Record.InputHash != command.Proposal.BaseInputHash {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
	}
	command.Proposal = command.Proposal.Clone()
	ev := magicevent.NewEventWithContext(decisionevents.TopicRegisterContractChangeProposal, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), bound, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.Proposal.WorkspaceID), SubjectType: "decision_ticket", SubjectID: string(command.TicketID)}))
	result, err := ownerevent.Decision[decisionevents.RegisterContractChangeProposalResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if err := result.ValidateFor(command); err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	return result, nil
}
