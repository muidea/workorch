package decision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const runtimeGovernanceProposalReconciliationInterval = time.Second

// ReconcileRuntimeGovernanceProposals performs one owner-local recovery pass.
// Lifecycle callers use the scheduled loop; tests and controlled recovery may
// invoke a single pass without introducing a second runtime authority.
func (m *Biz) ReconcileRuntimeGovernanceProposals(ctx context.Context) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	leave, err := m.enterOwner(true)
	if err != nil {
		return err
	}
	defer leave()
	return m.reconcileRuntimeGovernanceProposals(ctx)
}

func (m *Biz) startRuntimeGovernanceProposalReconciliation() error {
	if m == nil || m.BackgroundRoutine() == nil {
		return nil
	}
	m.reconcileMu.Lock()
	if m.reconcileCancel != nil {
		m.reconcileMu.Unlock()
		return nil
	}
	leave, err := m.enterOwner(false)
	if err != nil {
		m.reconcileMu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.reconcileCancel, m.reconcileDone = cancel, done
	m.reconcileMu.Unlock()
	m.shutdownMu.Lock()
	closing := m.closing
	m.shutdownMu.Unlock()
	if closing {
		cancel()
	}
	if err := m.AsyncTask(func() {
		defer close(done)
		defer leave()
		m.logRuntimeGovernanceReconciliationError(m.reconcileRuntimeGovernanceProposals(ctx))
		ticker := time.NewTicker(runtimeGovernanceProposalReconciliationInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.logRuntimeGovernanceReconciliationError(m.reconcileRuntimeGovernanceProposals(ctx))
			}
		}
	}); err != nil {
		cancel()
		leave()
		close(done)
		return err
	}
	return nil
}

func (m *Biz) stopRuntimeGovernanceProposalReconciliation() {
	if m == nil {
		return
	}
	m.reconcileMu.Lock()
	cancel := m.reconcileCancel
	m.reconcileMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Biz) logRuntimeGovernanceReconciliationError(err error) {
	if err != nil {
		slog.Error("decision owner reconciliation failed", "err", err)
	}
}

func (m *Biz) reconcileRuntimeGovernanceProposals(ctx context.Context) error {
	if m == nil || m.tickets == nil || m.service == nil {
		return common.ErrInvalidArgument
	}
	tickets, err := m.tickets.List(ctx)
	if err != nil {
		return err
	}
	var reconcileErrs []error
	for _, ticket := range tickets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ticket == nil {
			continue
		}
		if err := m.reconcileDecisionTicket(ctx, ticket); err != nil {
			reconcileErrs = append(reconcileErrs, err)
		}
	}
	return errors.Join(reconcileErrs...)
}

func (m *Biz) reconcileDecisionTicket(ctx context.Context, observed *decisionmodel.Ticket) error {
	if m == nil || observed == nil || observed.WorkspaceID == "" || observed.ID == "" {
		return common.ErrInvalidArgument
	}
	unlock := m.lockWorkspaceMutation(observed.WorkspaceID)
	defer unlock()

	ticketCtx, err := common.BindRuntimeWorkspaceContext(ctx, observed.WorkspaceID)
	if err != nil {
		return err
	}
	if err := m.rejectFencedWorkspace(ticketCtx, observed.WorkspaceID); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	ticket, err := m.tickets.Get(ticketCtx, observed.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if ticket.WorkspaceID != observed.WorkspaceID {
		return common.ErrConflict
	}

	// Definition clarification has dedicated typed resolution commands because
	// its binding transition needs facts which are intentionally absent from the
	// generic ResolveTicket contract. Never reinterpret an orphan clarification
	// Resolution as an ordinary resolution: the exact Workspace/Run owner
	// command will idempotently re-drive the private clarification reducer and
	// its delivery cut point.
	if ticket.DefinitionClarification != nil || ticket.DecisionKind == "workspace_definition_clarification" || ticket.DecisionKind == "runtime_definition_clarification" {
		return nil
	}

	if ticket.DecisionKind != "runtime_governance" {
		if err := m.reconcileOrdinaryDecisionDelivery(ticketCtx, ticket); err != nil {
			return fmt.Errorf("reconcile decision delivery %s: %w", ticket.ID, err)
		}
		return nil
	}
	if ticket.CreatedNotificationPublishedAt == "" {
		if err := m.ensureTicketCreatedNotification(ticketCtx, ticket); err != nil {
			return fmt.Errorf("deliver runtime governance creation %s: %w", ticket.ID, err)
		}
	}
	if ticket.RuntimeGovernanceProposal == nil {
		return nil
	}
	proposal := ticket.RuntimeGovernanceProposal
	switch {
	case ticket.State == common.DecisionTicketStateOpen && proposal.State == decisionmodel.RuntimeGovernanceResolutionProposalPending:
		if err := m.reconcileRuntimeGovernanceProposal(ticketCtx, ticket); err != nil {
			return fmt.Errorf("reconcile runtime governance proposal %s: %w", ticket.ID, err)
		}
	case ticket.State == common.DecisionTicketStateResolved && proposal.State == decisionmodel.RuntimeGovernanceResolutionProposalCommitted &&
		(proposal.EvidenceRecordedAt == "" || proposal.ResolutionNotificationPublishedAt == ""):
		resolution, err := m.resolutions.GetByTicketID(ticketCtx, ticket.ID)
		if err != nil {
			return fmt.Errorf("load runtime governance resolution %s: %w", ticket.ID, err)
		}
		if err := m.recordAndPublishResolvedTicket(ticketCtx, ticket, resolution, false); err != nil {
			return fmt.Errorf("deliver runtime governance resolution %s: %w", ticket.ID, err)
		}
	}
	return nil
}

func (m *Biz) reconcileOrdinaryDecisionDelivery(ctx context.Context, ticket *decisionmodel.Ticket) error {
	if m == nil || ticket == nil || ticket.DecisionKind == "runtime_governance" {
		return common.ErrInvalidArgument
	}
	switch ticket.State {
	case common.DecisionTicketStateOpen:
		resolution, err := m.resolutions.GetByTicketID(ctx, ticket.ID)
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		resolvedTicket, persisted, transitioned, err := m.service.EnsureResolvedTicketAndReturnWithTransition(ctx, decisioncontracts.ResolveTicketCommand{
			ResolutionID: resolution.ID, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID, RunID: ticket.RunID, StepID: ticket.StepID,
			Type: resolution.Type, Payload: resolution.Payload.Clone(), ContinuationAction: resolution.ContinuationAction,
			ResumeRunID: resolution.ResumeRunID, SuccessorRunID: resolution.SuccessorRunID,
		})
		if err != nil {
			return err
		}
		return m.recordAndPublishResolvedTicket(ctx, resolvedTicket, persisted, transitioned)
	case common.DecisionTicketStateResolved:
		if ticket.ResolutionNotificationPublishedAt != "" {
			return nil
		}
		resolution, err := m.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return err
		}
		return m.recordAndPublishResolvedTicket(ctx, ticket, resolution, false)
	default:
		return common.ErrConflict
	}
}

func (m *Biz) reconcileRuntimeGovernanceProposal(ctx context.Context, ticket *decisionmodel.Ticket) error {
	if m == nil || ticket == nil || ticket.RuntimeGovernanceProposal == nil {
		return common.ErrInvalidArgument
	}
	proposal := ticket.RuntimeGovernanceProposal.Clone()
	command := decisioncontracts.ResolveTicketCommand{
		ResolutionID: proposal.ResolutionID, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
		RunID: ticket.RunID, StepID: ticket.StepID, Type: proposal.ResolutionType,
		Payload: proposal.Payload.Clone(), ContinuationAction: proposal.ContinuationAction,
		ResumeRunID: proposal.ResumeRunID, SuccessorRunID: proposal.SuccessorRunID,
	}
	commit, err := m.commitRuntimeGovernanceTicketProposal(ctx, ticket, &proposal)
	if err != nil {
		return err
	}
	switch commit.Disposition {
	case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
		orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
		resolvedTicket, resolution, transitioned, err := m.service.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, command)
		if err != nil {
			return err
		}
		return m.recordAndPublishResolvedTicket(ctx, resolvedTicket, resolution, transitioned)
	case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
		return m.service.RejectRuntimeGovernanceResolutionProposal(ctx, command, "aggregate_commit_stale", "runtime governance proposal belongs to a stale aggregate authority")
	default:
		return common.ErrConflict
	}
}
