package decision

import (
	"context"
	"errors"
	"reflect"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handleRegisterContractChangeProposal(ev magicevent.Event, result magicevent.Result) {
	if ev == nil {
		setDefinitionClarificationResult(result, decisionevents.RegisterContractChangeProposalResult{}, common.ErrInvalidArgument)
		return
	}
	cmd, ok := ev.Data().(decisionevents.RegisterContractChangeProposalCommand)
	// 本片只有 Application 的受控提案入口，不从 caller 的 ProposedBy 推断批准权。
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.service == nil {
		setDefinitionClarificationResult(result, decisionevents.RegisterContractChangeProposalResult{}, common.ErrInvalidArgument)
		return
	}
	if err := cmd.Validate(); err != nil {
		setDefinitionClarificationResult(result, decisionevents.RegisterContractChangeProposalResult{}, err)
		return
	}
	response, err := m.registerContractChangeProposal(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) registerContractChangeProposal(ctx context.Context, cmd decisionevents.RegisterContractChangeProposalCommand) (decisionevents.RegisterContractChangeProposalResult, error) {
	if err := cmd.Validate(); err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	// 跨 owner 查询先于本 owner mutation 锁，避免与 Workspace 删除协调互等。
	if cmd.Proposal.FinalCandidate != nil {
		bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.Proposal.WorkspaceID)
		if err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		existing, err := m.tickets.Get(bound, cmd.TicketID)
		if err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		replay := false
		for _, revision := range existing.ContractChangeRevisions {
			replay = replay || revision.RequestID == cmd.RequestID
		}
		if !replay {
			if err := m.verifyContractChangeFinalCandidate(ctx, cmd); err != nil {
				return decisionevents.RegisterContractChangeProposalResult{}, err
			}
		}
	}
	unlock := m.lockWorkspaceMutation(cmd.Proposal.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ctx, cmd.Proposal.WorkspaceID); err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.Proposal.WorkspaceID)
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if _, err := m.resolutions.GetByTicketID(bound, cmd.TicketID); !errors.Is(err, common.ErrNotFound) {
		if err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
	}
	current, err := m.tickets.Get(bound, cmd.TicketID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	if current != nil {
		if err := current.ValidateContractChange(); err != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, err
		}
		if current.WorkspaceID != cmd.Proposal.WorkspaceID || current.RunID != cmd.Proposal.RunID {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
		for _, revision := range current.ContractChangeRevisions {
			if revision.RequestID != cmd.RequestID {
				continue
			}
			if revision.Revision != cmd.ExpectedRevision+1 || revision.Hash != cmd.Proposal.Hash() || !reflect.DeepEqual(revision.Proposal, cmd.Proposal) ||
				(cmd.ExpectedRevision != 0 && current.ContractChangeRevisions[cmd.ExpectedRevision-1].Hash != cmd.ExpectedHash) {
				return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
			}
			return contractChangeRegistrationResult(current, revision, "duplicate"), nil
		}
		last := current.ContractChangeRevisions[len(current.ContractChangeRevisions)-1]
		if last.Revision != cmd.ExpectedRevision || last.Hash != cmd.ExpectedHash {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
		current, err = m.service.applyLifecycleInternalEvent(bound, current, lifecycleInternalEvent{
			Kind: lifecycleInternalEventContractChangeRegistered, TicketID: cmd.TicketID,
			ContractChange: &decisionmodel.ContractChangeRevision{RequestID: cmd.RequestID, Revision: cmd.ExpectedRevision + 1, Hash: cmd.Proposal.Hash(), Proposal: cmd.Proposal.Clone()},
		})
	} else {
		if cmd.ExpectedRevision != 0 || cmd.Proposal.FinalCandidate != nil {
			return decisionevents.RegisterContractChangeProposalResult{}, common.ErrConflict
		}
		seed := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.Proposal.WorkspaceID, RunID: cmd.Proposal.RunID,
			Reason: cmd.Proposal.WhyChange, DecisionKind: decisionmodel.ContractChangeDecisionKind,
			ContractChangeRevisions: []decisionmodel.ContractChangeRevision{{RequestID: cmd.RequestID, Revision: 1, Hash: cmd.Proposal.Hash(), Proposal: cmd.Proposal.Clone()}},
		}
		current, err = m.service.applyLifecycleInternalEvent(bound, nil, lifecycleInternalEvent{Kind: lifecycleInternalEventOpened, TicketID: cmd.TicketID, Ticket: seed})
	}
	if err != nil {
		return decisionevents.RegisterContractChangeProposalResult{}, err
	}
	// 登记尚无批准合同，故不发送可被普通治理消费的 created/resolved 通知。
	return contractChangeRegistrationResult(current, current.ContractChangeRevisions[len(current.ContractChangeRevisions)-1], "registered"), nil
}

func contractChangeRegistrationResult(ticket *decisionmodel.Ticket, revision decisionmodel.ContractChangeRevision, disposition string) decisionevents.RegisterContractChangeProposalResult {
	return decisionevents.RegisterContractChangeProposalResult{TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID, RunID: ticket.RunID,
		RequestID: revision.RequestID, Revision: revision.Revision, Hash: revision.Hash, Disposition: disposition}
}
