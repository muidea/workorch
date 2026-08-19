package approval

import (
	"context"
	"strings"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// lifecycleInternalEvent is the only Approval-owner lifecycle transition
// input. Cross-owner commands are validated and copied here before persistence.
type lifecycleInternalEvent struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Kind        lifecycleInternalEventKind
	Proposal    *approvalmodel.ResolutionProposal
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventResolutionProposed         lifecycleInternalEventKind = "resolution_proposed"
	lifecycleInternalEventResolutionCommitted        lifecycleInternalEventKind = "resolution_committed"
	lifecycleInternalEventResolutionProposalRejected lifecycleInternalEventKind = "resolution_proposal_rejected"
	lifecycleInternalEventEvidenceRecorded           lifecycleInternalEventKind = "evidence_recorded"
	lifecycleInternalEventNotificationPublished      lifecycleInternalEventKind = "notification_published"
	lifecycleInternalEventDeletionRequested          lifecycleInternalEventKind = "deletion_requested"
)

func (s *Service) applyLifecycleInternalEvent(ctx context.Context, current *approvalmodel.Approval, event lifecycleInternalEvent) (*approvalmodel.Approval, error) {
	if s == nil || current == nil || event.ApprovalID == "" {
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

func reduceApprovalLifecycle(current *approvalmodel.Approval, event lifecycleInternalEvent) (*approvalmodel.Approval, error) {
	if current == nil || current.ID == "" || current.ID != event.ApprovalID || current.WorkspaceID != event.WorkspaceID || current.RunID != event.RunID {
		return nil, common.ErrInvalidArgument
	}
	next := approvalmodel.CloneApproval(current)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch event.Kind {
	case lifecycleInternalEventResolutionProposed:
		if current.State != common.ApprovalStatePending || event.Proposal == nil || event.Proposal.ResolutionID == "" || event.Proposal.Principal.Validate() != nil {
			return nil, common.ErrConflict
		}
		proposal := event.Proposal.Clone()
		proposal.State = approvalmodel.ResolutionProposalPending
		if proposal.ProposedAt == "" {
			proposal.ProposedAt = now
		}
		next.ResolutionProposal = &proposal
	case lifecycleInternalEventResolutionCommitted:
		if current.State != common.ApprovalStatePending || current.ResolutionProposal == nil || event.Proposal == nil ||
			!sameApprovalResolutionProposal(*current.ResolutionProposal, *event.Proposal) {
			return nil, common.ErrConflict
		}
		proposal := current.ResolutionProposal.Clone()
		proposal.State = approvalmodel.ResolutionProposalCommitted
		proposal.CommittedAt = now
		next.ResolutionProposal = &proposal
		next.State = proposal.TargetState
		next.ResolutionID = proposal.ResolutionID
		next.ResolutionPrincipal = proposal.Principal.Normalized()
		next.ResolutionContext = proposal.Context.Clone()
		next.ResolvedBy = next.ResolutionPrincipal.SubjectID
		next.ResolvedAt = now
	case lifecycleInternalEventResolutionProposalRejected:
		if current.ResolutionProposal == nil || event.Proposal == nil || !sameApprovalResolutionProposal(*current.ResolutionProposal, *event.Proposal) {
			return nil, common.ErrConflict
		}
		proposal := current.ResolutionProposal.Clone()
		proposal.State = approvalmodel.ResolutionProposalRejected
		proposal.FailureClass = strings.TrimSpace(event.Proposal.FailureClass)
		proposal.FailureReason = strings.TrimSpace(event.Proposal.FailureReason)
		next.ResolutionProposal = &proposal
	case lifecycleInternalEventEvidenceRecorded:
		if current.State != common.ApprovalStateApproved && current.State != common.ApprovalStateRejected {
			return nil, common.ErrConflict
		}
		next.EvidenceRecordedAt = now
		if next.ResolutionProposal != nil {
			next.ResolutionProposal.EvidenceRecordedAt = now
		}
	case lifecycleInternalEventNotificationPublished:
		if current.State != common.ApprovalStateApproved && current.State != common.ApprovalStateRejected {
			return nil, common.ErrConflict
		}
		next.ResolutionNotificationPublishedAt = now
		if next.ResolutionProposal != nil {
			next.ResolutionProposal.ResolutionNotificationPublishedAt = now
		}
	case lifecycleInternalEventDeletionRequested:
		if current.DeletionRequestedAt != "" {
			return nil, common.ErrConflict
		}
		next.DeletionRequestedAt = now
	default:
		return nil, common.ErrInvalidArgument
	}
	next.UpdatedAt = now
	return next, nil
}

func sameApprovalResolutionProposal(left approvalmodel.ResolutionProposal, right approvalmodel.ResolutionProposal) bool {
	left = left.Clone()
	right = right.Clone()
	return left.ResolutionID == right.ResolutionID && left.TargetState == right.TargetState && left.Principal == right.Principal && left.Context.Equal(right.Context)
}
