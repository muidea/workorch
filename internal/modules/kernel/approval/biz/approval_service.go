package approval

import (
	"context"
	"errors"
	"reflect"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalModel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
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

func (s *Service) CreateApproval(ctx context.Context, cmd approvalcontracts.CreateApprovalCommand) error {
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
	return events.PublishByContract(ctx, s.eventHub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        approvalevents.TopicApprovalCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.ApprovalPayload(item.Reason, item.Action, item.Context, item.State),
	})
}

func (s *Service) EnsureApproval(ctx context.Context, cmd approvalcontracts.CreateApprovalCommand) error {
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

func (s *Service) Approve(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	return s.resolve(ctx, cmd.ApprovalID, common.ApprovalStateApproved, cmd.Context)
}

func (s *Service) EnsureApproved(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureApprovedAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureApprovedAndReturn(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) (*approvalModel.Approval, error) {
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
	if err := s.resolveItem(ctx, item, common.ApprovalStateApproved, cmd.Context); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Reject(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	return s.resolve(ctx, cmd.ApprovalID, common.ApprovalStateRejected, cmd.Context)
}

func (s *Service) EnsureRejected(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) error {
	_, err := s.EnsureRejectedAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureRejectedAndReturn(ctx context.Context, cmd approvalcontracts.ResolveApprovalCommand) (*approvalModel.Approval, error) {
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
	if err := s.resolveItem(ctx, item, common.ApprovalStateRejected, cmd.Context); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) resolve(ctx context.Context, approvalID common.ApprovalID, target common.ApprovalState, context map[string]any) error {
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

	return s.resolveItem(ctx, item, target, context)
}

func (s *Service) resolveItem(ctx context.Context, item *approvalModel.Approval, target common.ApprovalState, resolutionContext map[string]any) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.State != common.ApprovalStatePending {
		return common.ErrConflict
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.Context = mergeApprovalContext(item.Context, resolutionContext)
	item.State = target
	if err := s.approvals.Update(ctx, item); err != nil {
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return events.PublishByContract(ctx, s.eventHub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        approvalevents.TopicApprovalResolved,
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

func mergeApprovalContext(existing map[string]any, overlay map[string]any) map[string]any {
	ret := cloneApprovalContext(existing)
	if len(overlay) == 0 {
		return ret
	}
	if ret == nil {
		ret = map[string]any{}
	}
	for key, value := range overlay {
		ret[key] = value
	}
	return ret
}

func approvalContextEqual(left map[string]any, right map[string]any) bool {
	return reflect.DeepEqual(left, right)
}
