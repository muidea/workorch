package approval

import (
	"context"
	"errors"
	"reflect"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/common"
	approvalModel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/kernel/approval/service"
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
	if cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &approvalModel.Approval{
		ID:          cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		Action:      cmd.Action,
		Context:     cloneApprovalContext(cmd.Context),
		State:       common.ApprovalStatePending,
		CreatedAt:   now,
		UpdatedAt:   now,
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
		Payload:      events.ApprovalPayload(item.Reason, item.Action, item.Context, item.State),
	})
}

func (s *Service) EnsureApproval(ctx context.Context, cmd approvalservice.CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID ||
			item.RunID != cmd.RunID ||
			item.StepID != cmd.StepID ||
			item.Reason != cmd.Reason ||
			item.Action != cmd.Action ||
			!approvalContextEqual(item.Context, cmd.Context) {
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
	_, err := s.EnsureApprovedAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureApprovedAndReturn(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error) {
	if cmd.ApprovalID == "" {
		return nil, common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err != nil {
		return nil, err
	}
	if item.State == common.ApprovalStateApproved {
		return item, nil
	}
	if item.State == common.ApprovalStateRejected {
		return nil, common.ErrConflict
	}
	if err := s.resolveItem(ctx, item, common.ApprovalStateApproved); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Reject(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	return s.resolve(ctx, cmd.ApprovalID, common.ApprovalStateRejected)
}

func (s *Service) EnsureRejected(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
	_, err := s.EnsureRejectedAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureRejectedAndReturn(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error) {
	if cmd.ApprovalID == "" {
		return nil, common.ErrInvalidArgument
	}

	item, err := s.approvals.Get(ctx, cmd.ApprovalID)
	if err != nil {
		return nil, err
	}
	if item.State == common.ApprovalStateRejected {
		return item, nil
	}
	if item.State == common.ApprovalStateApproved {
		return nil, common.ErrConflict
	}
	if err := s.resolveItem(ctx, item, common.ApprovalStateRejected); err != nil {
		return nil, err
	}
	return item, nil
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

	return s.resolveItem(ctx, item, target)
}

func (s *Service) resolveItem(ctx context.Context, item *approvalModel.Approval, target common.ApprovalState) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.State != common.ApprovalStatePending {
		return common.ErrConflict
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
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
		Payload:      events.ApprovalPayload(item.Reason, item.Action, item.Context, item.State),
	})
}

func cloneApprovalContext(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func approvalContextEqual(left map[string]any, right map[string]any) bool {
	return reflect.DeepEqual(left, right)
}
