package approval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct{ approvals repository.ApprovalRepository }

func NewService(approvals repository.ApprovalRepository) *Service {
	return &Service{approvals: approvals}
}

func (s *Service) CreateApproval(ctx context.Context, cmd approvalcontracts.CreateApprovalCommand) error {
	if s == nil || s.approvals == nil || cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || strings.TrimSpace(cmd.Reason) == "" {
		return common.ErrInvalidArgument
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &approvalmodel.Approval{
		ID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID,
		Reason: strings.TrimSpace(cmd.Reason), Action: strings.TrimSpace(cmd.Action), Context: cmd.Context.Clone(),
		State: common.ApprovalStatePending, AutoGovernance: cmd.AutoGovernance.Clone(), CreatedAt: now, UpdatedAt: now,
	}
	if cmd.RuntimeGovernance != nil {
		binding := cmd.RuntimeGovernance.Clone()
		item.RuntimeGovernance = &binding
	}
	return s.approvals.Create(ctx, item)
}

func (s *Service) EnsureApproval(ctx context.Context, cmd approvalcontracts.CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || strings.TrimSpace(cmd.Reason) == "" {
		return common.ErrInvalidArgument
	}
	item, err := s.approvals.GetScoped(ctx, cmd.WorkspaceID, cmd.RunID, cmd.ApprovalID)
	if err == nil {
		if item.StepID != cmd.StepID || item.Reason != strings.TrimSpace(cmd.Reason) || item.Action != strings.TrimSpace(cmd.Action) ||
			!item.Context.Equal(cmd.Context) || !sameRuntimeApprovalBinding(item.RuntimeGovernance, cmd.RuntimeGovernance) {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateApproval(ctx, cmd)
}

func (s *Service) EnsureResolutionProposal(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand, target common.ApprovalState) (*approvalmodel.Approval, *approvalmodel.ResolutionProposal, error) {
	if s == nil || cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" ||
		(target != common.ApprovalStateApproved && target != common.ApprovalStateRejected) || cmd.Principal.Validate() != nil {
		return nil, nil, common.ErrInvalidArgument
	}
	if cmd.ResolutionID == "" {
		cmd.ResolutionID = stableApprovalResolutionID(cmd.WorkspaceID, cmd.RunID, cmd.ApprovalID, target, cmd.Principal)
	}
	want := approvalmodel.ResolutionProposal{
		ResolutionID: cmd.ResolutionID, TargetState: target, Principal: cmd.Principal.Normalized(),
		Context: cmd.Context.Clone(), State: approvalmodel.ResolutionProposalPending,
	}
	item, err := s.approvals.GetScoped(ctx, cmd.WorkspaceID, cmd.RunID, cmd.ApprovalID)
	if err != nil {
		return nil, nil, err
	}
	if item.DeletionRequestedAt != "" {
		return nil, nil, common.ErrConflict
	}
	if item.State != common.ApprovalStatePending {
		if item.State == target && item.ResolutionID == want.ResolutionID && item.ResolutionPrincipal.Normalized() == want.Principal && item.ResolutionContext.Equal(want.Context) {
			return item, item.ResolutionProposal, nil
		}
		return nil, nil, common.ErrConflict
	}
	if item.ResolutionProposal != nil {
		if sameApprovalResolutionProposal(*item.ResolutionProposal, want) && item.ResolutionProposal.State != approvalmodel.ResolutionProposalRejected {
			proposal := item.ResolutionProposal.Clone()
			return item, &proposal, nil
		}
		return nil, nil, common.ErrConflict
	}
	updated, err := s.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
		Kind: lifecycleInternalEventResolutionProposed, Proposal: &want,
	})
	if err != nil {
		return nil, nil, err
	}
	proposal := updated.ResolutionProposal.Clone()
	return updated, &proposal, nil
}

func (s *Service) FinalizeResolutionProposal(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand, target common.ApprovalState) (*approvalmodel.Approval, bool, error) {
	item, proposal, err := s.EnsureResolutionProposal(ctx, cmd, target)
	if err != nil {
		return nil, false, err
	}
	if item.State == target {
		return item, false, nil
	}
	updated, err := s.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
		Kind: lifecycleInternalEventResolutionCommitted, Proposal: proposal,
	})
	return updated, err == nil, err
}

func (s *Service) RejectResolutionProposal(ctx context.Context, item *approvalmodel.Approval, failureClass, failureReason string) error {
	if item == nil || item.ResolutionProposal == nil {
		return common.ErrInvalidArgument
	}
	proposal := item.ResolutionProposal.Clone()
	proposal.FailureClass = strings.TrimSpace(failureClass)
	proposal.FailureReason = strings.TrimSpace(failureReason)
	_, err := s.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
		Kind: lifecycleInternalEventResolutionProposalRejected, Proposal: &proposal,
	})
	return err
}

func (s *Service) MarkEvidenceRecorded(ctx context.Context, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	if item.EvidenceRecordedAt != "" {
		return item, nil
	}
	return s.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, Kind: lifecycleInternalEventEvidenceRecorded})
}

func (s *Service) MarkNotificationPublished(ctx context.Context, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	if item.ResolutionNotificationPublishedAt != "" {
		return item, nil
	}
	return s.applyLifecycleInternalEvent(ctx, item, lifecycleInternalEvent{ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, Kind: lifecycleInternalEventNotificationPublished})
}

func (s *Service) Approve(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureApprovedAndReturn(ctx, cmd)
	return err
}
func (s *Service) EnsureApproved(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureApprovedAndReturn(ctx, cmd)
	return err
}
func (s *Service) Reject(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureRejectedAndReturn(ctx, cmd)
	return err
}
func (s *Service) EnsureRejected(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureRejectedAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureApprovedAndReturn(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) (*approvalmodel.Approval, error) {
	cmd = normalizeOwnerLocalResolutionCommand(cmd)
	item, _, err := s.FinalizeResolutionProposal(ctx, cmd, common.ApprovalStateApproved)
	return item, err
}

func (s *Service) EnsureRejectedAndReturn(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) (*approvalmodel.Approval, error) {
	cmd = normalizeOwnerLocalResolutionCommand(cmd)
	item, _, err := s.FinalizeResolutionProposal(ctx, cmd, common.ApprovalStateRejected)
	return item, err
}

func normalizeOwnerLocalResolutionCommand(cmd approvalcontracts.ResolveApprovalCommand) approvalcontracts.ResolveApprovalCommand {
	if cmd.Principal.Validate() != nil {
		cmd.Principal = approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "approval-owner-local"}
	}
	return cmd
}

func stableApprovalResolutionID(workspaceID common.WorkspaceID, runID common.RunID, approvalID common.ApprovalID, state common.ApprovalState, principal approvalmodel.ApprovalResolutionPrincipal) common.ResolutionID {
	principal = principal.Normalized()
	return common.ResolutionID(fmt.Sprintf("approval-resolution/%s/%s/%s/%s/%s/%s", workspaceID, runID, approvalID, state, principal.Kind, principal.SubjectID))
}

func sameRuntimeApprovalBinding(left, right *approvalmodel.RuntimeGovernanceBinding) bool {
	if left == nil || right == nil {
		return left == right
	}
	l := left.Clone()
	r := right.Clone()
	return l == r
}
