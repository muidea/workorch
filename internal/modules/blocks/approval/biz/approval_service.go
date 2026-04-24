package approval

import (
	"context"
	"errors"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	approvals repository.ApprovalRepository
	eventHub  events.Hub
}

func NewService(approvals repository.ApprovalRepository, eventHub events.Hub) *Service {
	return &Service{approvals: approvals, eventHub: eventHub}
}

func (s *Service) CreateApproval(ctx context.Context, cmd approvalservice.CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item := &approvalModel.Approval{
		ID:          cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		State:       common.ApprovalStatePending,
	}
	if err := s.approvals.Create(ctx, item); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicApprovalCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.ApprovalPayload(item.Reason, item.State),
	})
}

func (s *Service) EnsureApproval(ctx context.Context, cmd approvalservice.CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID || item.RunID != cmd.RunID || item.StepID != cmd.StepID || item.Reason != cmd.Reason {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateApproval(ctx, cmd)
}

func (s *Service) Approve(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	return s.resolve(ctx, cmd.ApprovalID, common.ApprovalStateApproved)
}

func (s *Service) EnsureApproved(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	if cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err != nil {
		return err
	}
	if item.State == common.ApprovalStateApproved {
		return nil
	}
	if item.State == common.ApprovalStateRejected {
		return common.ErrConflict
	}
	return s.Approve(ctx, cmd)
}

func (s *Service) Reject(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	return s.resolve(ctx, cmd.ApprovalID, common.ApprovalStateRejected)
}

func (s *Service) EnsureRejected(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	if cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err != nil {
		return err
	}
	if item.State == common.ApprovalStateRejected {
		return nil
	}
	if item.State == common.ApprovalStateApproved {
		return common.ErrConflict
	}
	return s.Reject(ctx, cmd)
}

func (s *Service) resolve(ctx context.Context, approvalID common.ApprovalID, target common.ApprovalState) error {
	if approvalID == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		return err
	}
	if item.State != common.ApprovalStatePending {
		return common.ErrConflict
	}

	item.State = target
	if err := s.approvals.Update(ctx, item); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicApprovalResolved,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.ApprovalPayload(item.Reason, item.State),
	})
}
