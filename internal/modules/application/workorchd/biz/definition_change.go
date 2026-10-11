package biz

import (
	"context"
	"reflect"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// 待审候选准备不接确认、admission 或执行意图，也不暴露 HTTP 写入口。
func (b *Biz) PrepareDefinitionChange(ctx context.Context, command workspaceevents.PrepareDefinitionChangeCommand) (workspaceevents.PrepareDefinitionChangeResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangePrepare, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response, err := ownerevent.WorkspaceOwner[workspaceevents.PrepareDefinitionChangeResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	response.Candidate = response.Candidate.Clone()
	return response, nil
}

// 派生与六审仍只是待审材料，不执行精确批准或定义应用。
func (b *Biz) DeriveDefinitionChangeAcceptance(ctx context.Context, id common.WorkspaceID, inputHash string) (workspace.DefinitionDerivedAcceptance, error) {
	view, err := b.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if inputHash == "" || view.Candidate == nil || view.Candidate.Record.InputHash != inputHash {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	return b.deriveDefinitionAcceptance(ctx, id, inputHash)
}

func (b *Biz) CompileDefinitionChangeSpecification(ctx context.Context, id common.WorkspaceID, inputHash string) (workspace.GoalSpecification, error) {
	view, err := b.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	if inputHash == "" || view.Candidate == nil || view.Candidate.Record.InputHash != inputHash {
		return workspace.GoalSpecification{}, common.ErrConflict
	}
	return b.compileAuthoringSpecification(ctx, id, inputHash)
}

// 最终登记沿原 Ticket append-only 版本；只增加 owner 已审身份，不产生 Resolution。
func (b *Biz) RegisterReviewedDefinitionChange(ctx context.Context, id common.WorkspaceID, requestID string) (decisionevents.RegisterContractChangeProposalResult, error) {
	view, err := b.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if view.Candidate == nil || view.Reviewed == nil || requestID == "" {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	c := view.Candidate
	ev := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetTicketCommand{WorkspaceID: id, TicketID: c.TicketID})
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(id), SubjectType: "decision_ticket", SubjectID: string(c.TicketID)}))
	response, err := ownerevent.Decision[decisionevents.GetTicketResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if response.Ticket == nil || response.Ticket.ID != c.TicketID || response.Ticket.WorkspaceID != id || response.Ticket.ValidateContractChange() != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	final := decisionmodel.ContractChangeCandidate{ID: view.Reviewed.ID, Revision: view.Reviewed.Revision, Hash: view.Reviewed.Hash}
	for _, revision := range response.Ticket.ContractChangeRevisions {
		if revision.RequestID == requestID {
			if revision.Revision != c.ProposalRevision+1 || revision.Proposal.FinalCandidate == nil || *revision.Proposal.FinalCandidate != final {
				return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
			}
			command := decisionevents.RegisterContractChangeProposalCommand{TicketID: c.TicketID, RequestID: requestID, ExpectedRevision: c.ProposalRevision, ExpectedHash: c.ProposalHash, Proposal: revision.Proposal.Clone()}
			return b.RegisterContractChangeProposal(ctx, command)
		}
	}
	latest := response.Ticket.ContractChangeRevisions[len(response.Ticket.ContractChangeRevisions)-1]
	if latest.Revision != c.ProposalRevision || latest.Hash != c.ProposalHash || latest.Proposal.FinalCandidate != nil && !reflect.DeepEqual(*latest.Proposal.FinalCandidate, final) {
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	proposal := latest.Proposal.Clone()
	proposal.FinalCandidate = &final
	return b.RegisterContractChangeProposal(ctx, decisionevents.RegisterContractChangeProposalCommand{TicketID: c.TicketID, RequestID: requestID, ExpectedRevision: latest.Revision, ExpectedHash: latest.Hash, Proposal: proposal})
}

func (b *Biz) QueryDefinitionChange(ctx context.Context, command workspaceevents.QueryDefinitionChangeCommand) (workspaceevents.QueryDefinitionChangeResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.WorkspaceID == "" {
		return workspaceevents.QueryDefinitionChangeResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangeQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response, err := ownerevent.WorkspaceOwner[workspaceevents.QueryDefinitionChangeResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if response.Candidate != nil {
		candidate := response.Candidate.Clone()
		response.Candidate = &candidate
	}
	if response.Reviewed != nil {
		identity := *response.Reviewed
		response.Reviewed = &identity
	}
	return response, nil
}
