package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const approvalReconciliationInterval = time.Second

func (m *Biz) ReconcileApprovalDeliveries(ctx context.Context) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	leave, err := m.enterOwner(true)
	if err != nil {
		return err
	}
	defer leave()
	return m.reconcileApprovalDeliveries(ctx)
}

func (m *Biz) startApprovalReconciliation() error {
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
		m.runApprovalReconciliation(ctx)
	}); err != nil {
		cancel()
		leave()
		close(done)
		return err
	}
	return nil
}

func (m *Biz) runApprovalReconciliation(ctx context.Context) {
	reconcile := func() {
		if err := m.ReconcileApprovalDeliveries(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("approval owner reconciliation failed", "err", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(approvalReconciliationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (m *Biz) stopApprovalReconciliation() {
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

func (m *Biz) reconcileApprovalDeliveries(ctx context.Context) error {
	if m == nil || m.approvals == nil || m.service == nil {
		return common.ErrInvalidArgument
	}
	items, err := m.approvals.List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item == nil {
			continue
		}
		if err := m.reconcileApprovalDelivery(ctx, item); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Biz) reconcileApprovalDelivery(ctx context.Context, observed *approvalmodel.Approval) error {
	if m == nil || observed == nil || observed.WorkspaceID == "" || observed.RunID == "" || observed.ID == "" {
		return common.ErrInvalidArgument
	}
	// All Approval owner paths use workspace -> lifecycle ordering. This makes
	// deletion seal, ingress mutation and background recovery linearizable while
	// avoiding the previous lifecycle -> workspace inversion.
	unlock := m.lockWorkspaceLifecycleMutation(observed.WorkspaceID)
	defer unlock()

	itemCtx, err := common.BindRuntimeWorkspaceContext(ctx, observed.WorkspaceID)
	if err != nil {
		return err
	}
	if err := m.rejectFencedWorkspace(itemCtx, observed.WorkspaceID); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	item, err := m.approvals.GetScoped(itemCtx, observed.WorkspaceID, observed.RunID, observed.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if item.WorkspaceID != observed.WorkspaceID || item.RunID != observed.RunID {
		return common.ErrConflict
	}
	if item.State == common.ApprovalStatePending && item.ResolutionProposal != nil && item.ResolutionProposal.State == approvalmodel.ResolutionProposalPending {
		if err := m.reconcileApprovalProposal(itemCtx, item); err != nil {
			return fmt.Errorf("reconcile approval %s: %w", item.ID, err)
		}
		return nil
	}
	if (item.State == common.ApprovalStateApproved || item.State == common.ApprovalStateRejected) &&
		(item.EvidenceRecordedAt == "" || item.ResolutionNotificationPublishedAt == "") {
		if err := m.deliverResolvedApproval(itemCtx, item); err != nil {
			return fmt.Errorf("deliver approval %s: %w", item.ID, err)
		}
	}
	return nil
}

func (m *Biz) reconcileApprovalProposal(ctx context.Context, item *approvalmodel.Approval) error {
	proposal := item.ResolutionProposal.Clone()
	if item.RuntimeGovernance != nil {
		commit, err := m.commitRuntimeApprovalProposal(ctx, item, &proposal)
		if err != nil {
			return err
		}
		switch commit.Disposition {
		case orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted, orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate:
		case orchestratorevents.RuntimeGovernanceResolutionDispositionStale:
			return m.service.RejectResolutionProposal(ctx, item, "aggregate_commit_stale", "runtime approval proposal belongs to a stale aggregate authority")
		default:
			return common.ErrConflict
		}
	}
	resolved, _, err := m.service.FinalizeResolutionProposal(ctx, approvalcontracts.ResolveApprovalCommand{
		ResolutionID: proposal.ResolutionID, ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
		Principal: proposal.Principal, Context: proposal.Context.Clone(),
	}, proposal.TargetState)
	if err != nil {
		return err
	}
	return m.deliverResolvedApproval(ctx, resolved)
}
