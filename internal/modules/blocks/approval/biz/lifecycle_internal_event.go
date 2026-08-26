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
	Approval    *approvalmodel.Approval
	Proposal    *approvalmodel.ResolutionProposal
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventOpened                     lifecycleInternalEventKind = "opened"
	lifecycleInternalEventResolutionProposed         lifecycleInternalEventKind = "resolution_proposed"
	lifecycleInternalEventResolutionCommitted        lifecycleInternalEventKind = "resolution_committed"
	lifecycleInternalEventResolutionProposalRejected lifecycleInternalEventKind = "resolution_proposal_rejected"
	lifecycleInternalEventEvidenceRecorded           lifecycleInternalEventKind = "evidence_recorded"
	lifecycleInternalEventNotificationPublished      lifecycleInternalEventKind = "notification_published"
)

func (s *mutationService) applyLifecycleInternalEvent(ctx context.Context, current *approvalmodel.Approval, event lifecycleInternalEvent) (*approvalmodel.Approval, error) {
	if s == nil || s.approvals == nil || event.ApprovalID == "" || event.WorkspaceID == "" || event.RunID == "" {
		return nil, common.ErrInvalidArgument
	}
	if current == nil {
		if event.Kind != lifecycleInternalEventOpened || event.Approval == nil {
			return nil, common.ErrInvalidArgument
		}
	} else if event.Kind == lifecycleInternalEventOpened || event.Approval != nil {
		return nil, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return nil, err
	}
	next, err := reduceApprovalLifecycle(current, event)
	if err != nil {
		return nil, err
	}
	if event.Kind == lifecycleInternalEventOpened {
		if err := s.approvals.Create(bound, next); err != nil {
			return nil, err
		}
		return next, nil
	}
	if err := s.approvals.Update(bound, next); err != nil {
		return nil, err
	}
	return next, nil
}

func reduceApprovalLifecycle(current *approvalmodel.Approval, event lifecycleInternalEvent) (*approvalmodel.Approval, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.Kind == lifecycleInternalEventOpened {
		seed := event.Approval
		if current != nil || seed == nil || seed.ID == "" || seed.ID != event.ApprovalID || seed.WorkspaceID != event.WorkspaceID || seed.RunID != event.RunID || strings.TrimSpace(seed.Reason) == "" ||
			seed.State != "" || seed.ResolutionID != "" || seed.ResolutionProposal != nil || seed.EvidenceRecordedAt != "" || seed.ResolutionNotificationPublishedAt != "" || seed.ResolvedAt != "" || seed.ResolvedBy != "" || seed.CreatedAt != "" || seed.UpdatedAt != "" || event.Proposal != nil {
			return nil, common.ErrInvalidArgument
		}
		next := approvalmodel.CloneApproval(seed)
		next.State = common.ApprovalStatePending
		next.CreatedAt = now
		next.UpdatedAt = now
		return next, nil
	}
	if current == nil || current.ID == "" || current.ID != event.ApprovalID || current.WorkspaceID != event.WorkspaceID || current.RunID != event.RunID {
		return nil, common.ErrInvalidArgument
	}
	next := approvalmodel.CloneApproval(current)
	switch event.Kind {
	case lifecycleInternalEventResolutionProposed:
		if current.State != common.ApprovalStatePending || event.Proposal == nil || event.Proposal.ResolutionID == "" || event.Proposal.Principal.Validate() != nil ||
			(event.Proposal.TargetState != common.ApprovalStateApproved && event.Proposal.TargetState != common.ApprovalStateRejected) {
			return nil, common.ErrConflict
		}
		if event.Proposal.State != "" || strings.TrimSpace(event.Proposal.FailureClass) != "" || strings.TrimSpace(event.Proposal.FailureReason) != "" ||
			strings.TrimSpace(event.Proposal.ProposedAt) != "" || strings.TrimSpace(event.Proposal.CommittedAt) != "" ||
			strings.TrimSpace(event.Proposal.EvidenceRecordedAt) != "" || strings.TrimSpace(event.Proposal.ResolutionNotificationPublishedAt) != "" {
			return nil, common.ErrInvalidArgument
		}
		proposal := event.Proposal.Clone()
		proposal.State = approvalmodel.ResolutionProposalPending
		proposal.ProposedAt = now
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
