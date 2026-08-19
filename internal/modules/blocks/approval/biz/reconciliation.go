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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.reconcileApprovalDeliveries(ctx)
}

func (m *Biz) startApprovalReconciliation() error {
	if m == nil || m.BackgroundRoutine() == nil {
		return nil
	}
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	if m.reconcileCancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.reconcileCancel, m.reconcileDone = cancel, done
	if err := m.BackgroundRoutine().AsyncFunction(func() {
		defer close(done)
		m.runApprovalReconciliation(ctx)
	}); err != nil {
		m.reconcileCancel, m.reconcileDone = nil, nil
		cancel()
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
	cancel, done := m.reconcileCancel, m.reconcileDone
	m.reconcileCancel, m.reconcileDone = nil, nil
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
		itemCtx, bindErr := common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
		if bindErr != nil {
			errs = append(errs, bindErr)
			continue
		}
		if item.DeletionRequestedAt != "" {
			if err := m.approvals.DeleteScoped(itemCtx, item.WorkspaceID, item.RunID, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				errs = append(errs, err)
			}
			continue
		}
		if item.State == common.ApprovalStatePending && item.ResolutionProposal != nil && item.ResolutionProposal.State == approvalmodel.ResolutionProposalPending {
			if err := m.reconcileApprovalProposal(itemCtx, item); err != nil {
				errs = append(errs, fmt.Errorf("reconcile approval %s: %w", item.ID, err))
			}
			continue
		}
		if (item.State == common.ApprovalStateApproved || item.State == common.ApprovalStateRejected) &&
			(item.EvidenceRecordedAt == "" || item.ResolutionNotificationPublishedAt == "") {
			if err := m.deliverResolvedApproval(itemCtx, item); err != nil {
				errs = append(errs, fmt.Errorf("deliver approval %s: %w", item.ID, err))
			}
		}
	}
	return errors.Join(errs...)
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
