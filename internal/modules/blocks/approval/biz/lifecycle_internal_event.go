package approval

import (
	"context"
	"time"

	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// lifecycleInternalEvent is the only Approval-owner lifecycle transition
// input. EventHub commands are copied into this private fact before state is
// selected, reduced, and persisted by the owner.
type lifecycleInternalEvent struct {
	ApprovalID common.ApprovalID
	Kind       lifecycleInternalEventKind
	Context    workorcheventpayload.Payload
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventGrantedObserved  lifecycleInternalEventKind = "granted_observed"
	lifecycleInternalEventRejectedObserved lifecycleInternalEventKind = "rejected_observed"
)

func (s *Service) submitLifecycleInternalEvent(ctx context.Context, event lifecycleInternalEvent) (*approvalModel.Approval, error) {
	if s == nil || event.ApprovalID == "" {
		return nil, common.ErrInvalidArgument
	}
	current, err := s.approvals.Get(ctx, event.ApprovalID)
	if err != nil {
		return nil, err
	}
	return s.applyLifecycleInternalEvent(ctx, current, event)
}

func (s *Service) applyLifecycleInternalEvent(ctx context.Context, current *approvalModel.Approval, event lifecycleInternalEvent) (*approvalModel.Approval, error) {
	if s == nil || event.ApprovalID == "" {
		return nil, common.ErrInvalidArgument
	}
	next, err := reduceApprovalLifecycle(current, event)
	if err != nil {
		return nil, err
	}
	if err := s.approvals.Update(ctx, next); err != nil {
		return nil, err
	}
	return next, nil
}

func reduceApprovalLifecycle(current *approvalModel.Approval, event lifecycleInternalEvent) (*approvalModel.Approval, error) {
	if current == nil || current.ID == "" || current.ID != event.ApprovalID {
		return nil, common.ErrInvalidArgument
	}
	if current.State != common.ApprovalStatePending {
		return nil, common.ErrConflict
	}
	next := approvalModel.CloneApproval(current)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next.UpdatedAt = now
	next.ResolvedAt = now
	next.ResolvedBy = firstNonEmptyApprovalString(approvalStringValue(event.Context, "resolvedBy"), approvalStringValue(event.Context, "actor"))
	next.Context = mergeApprovalContext(next.Context, event.Context)
	switch event.Kind {
	case lifecycleInternalEventGrantedObserved:
		next.State = common.ApprovalStateApproved
	case lifecycleInternalEventRejectedObserved:
		next.State = common.ApprovalStateRejected
	default:
		return nil, common.ErrInvalidArgument
	}
	return next, nil
}
