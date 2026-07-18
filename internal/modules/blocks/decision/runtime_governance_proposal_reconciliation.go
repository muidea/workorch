package decision

import (
	"context"
	"errors"
	"time"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const runtimeGovernanceProposalReconciliationInterval = time.Second

func (m *Module) startRuntimeGovernanceProposalReconciliation() error {
	if m == nil || m.background == nil {
		return nil
	}
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	if m.reconcileCancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.reconcileCancel = cancel
	m.reconcileDone = done
	if err := m.background.AsyncFunction(func() {
		defer close(done)
		m.reconcileRuntimeGovernanceProposals(ctx)
		ticker := time.NewTicker(runtimeGovernanceProposalReconciliationInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.reconcileRuntimeGovernanceProposals(ctx)
			}
		}
	}); err != nil {
		m.reconcileCancel = nil
		m.reconcileDone = nil
		cancel()
		return err
	}
	return nil
}

func (m *Module) stopRuntimeGovernanceProposalReconciliation() {
	if m == nil {
		return
	}
	m.reconcileMu.Lock()
	cancel := m.reconcileCancel
	done := m.reconcileDone
	m.reconcileCancel = nil
	m.reconcileDone = nil
	m.reconcileMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Module) reconcileRuntimeGovernanceProposals(ctx context.Context) {
	if m == nil || m.tickets == nil || m.bizPtr == nil {
		return
	}
	tickets, err := m.tickets.List(ctx)
	if err != nil {
		return
	}
	for _, ticket := range tickets {
		if ctx.Err() != nil || ticket == nil || ticket.DecisionKind != "runtime_v2_governance" ||
			ticket.State != common.DecisionTicketStateOpen || ticket.RuntimeGovernanceProposal == nil ||
			ticket.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalPending {
			continue
		}
		m.reconcileRuntimeGovernanceProposal(ctx, ticket)
	}
}

func (m *Module) reconcileRuntimeGovernanceProposal(ctx context.Context, ticket *decisionmodel.Ticket) {
	if m == nil || ticket == nil || ticket.RuntimeGovernanceProposal == nil {
		return
	}
	proposal := ticket.RuntimeGovernanceProposal.Clone()
	command := decisioncontracts.ResolveTicketCommand{
		ResolutionID: proposal.ResolutionID, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
		RunID: ticket.RunID, StepID: ticket.StepID, Type: proposal.ResolutionType,
		Payload: proposal.Payload.Clone(), ContinuationAction: proposal.ContinuationAction,
		ResumeRunID: proposal.ResumeRunID, SuccessorRunID: proposal.SuccessorRunID,
	}
	commit, err := m.commitRuntimeV2GovernanceTicketProposal(ctx, ticket, &proposal)
	if err != nil {
		if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrInvalidArgument) {
			_ = m.bizPtr.RejectRuntimeGovernanceResolutionProposal(ctx, command, "aggregate_commit_rejected", err.Error())
		}
		return
	}
	switch commit.Disposition {
	case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
		orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
		_, _, _ = m.bizPtr.FinalizeRuntimeGovernanceResolutionProposal(ctx, command)
	case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
		_ = m.bizPtr.RejectRuntimeGovernanceResolutionProposal(ctx, command, "aggregate_commit_stale", "runtime governance proposal belongs to a stale aggregate authority")
	}
}
