package approval

import (
	"context"
	"errors"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Service struct {
	approvals  repository.ApprovalRepository
	commandHub magicevent.Hub
}

func NewService(approvals repository.ApprovalRepository, commandHub magicevent.Hub) *Service {
	return &Service{approvals: approvals, commandHub: commandHub}
}

func (s *Service) CreateApproval(ctx context.Context, cmd approvalcontracts.CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &approvalModel.Approval{
		ID:             cmd.ApprovalID,
		WorkspaceID:    cmd.WorkspaceID,
		RunID:          cmd.RunID,
		StepID:         cmd.StepID,
		Reason:         cmd.Reason,
		Action:         cmd.Action,
		Context:        cloneApprovalContext(cmd.Context),
		State:          common.ApprovalStatePending,
		AutoGovernance: cloneApprovalContext(cmd.AutoGovernance),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.approvals.Create(ctx, item); err != nil {
		return err
	}
	return s.publishApprovalChanged(ctx, approvalevents.TopicApprovalCreated, item)
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
		if err := s.recordApprovalGovernanceEvidence(ctx, item); err != nil {
			return nil, err
		}
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
		if err := s.recordApprovalGovernanceEvidence(ctx, item); err != nil {
			return nil, err
		}
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

func (s *Service) resolve(ctx context.Context, approvalID common.ApprovalID, target common.ApprovalState, context workorcheventpayload.Payload) error {
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

func (s *Service) resolveItem(ctx context.Context, item *approvalModel.Approval, target common.ApprovalState, resolutionContext workorcheventpayload.Payload) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.State != common.ApprovalStatePending {
		return common.ErrConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = now
	item.ResolvedAt = now
	item.ResolvedBy = firstNonEmptyApprovalString(approvalStringValue(resolutionContext, "resolvedBy"), approvalStringValue(resolutionContext, "actor"))
	item.Context = mergeApprovalContext(item.Context, resolutionContext)
	item.State = target
	if err := s.approvals.Update(ctx, item); err != nil {
		return err
	}
	if err := s.recordApprovalGovernanceEvidence(ctx, item); err != nil {
		return err
	}
	return s.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, item)
}

func (s *Service) publishApprovalChanged(ctx context.Context, topic string, item *approvalModel.Approval) error {
	if s == nil || s.commandHub == nil || item == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(
		topic,
		modulecommon.ModuleID,
		approvalevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		approvalevents.ApprovalChangedData{
			ApprovalID:  item.ID,
			WorkspaceID: item.WorkspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
			Reason:      item.Reason,
			Action:      item.Action,
			Context:     item.Context.Clone().Fields,
			State:       item.State,
		},
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(topic, workorchevents.EventHubLaneScope{
		SubjectType: "approval",
		SubjectID:   string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
	}))
	s.commandHub.Post(event)
	return nil
}

func cloneApprovalContext(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	return input.Clone()
}

func firstNonEmptyApprovalString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func approvalStringValue(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return value
}

func mergeApprovalContext(existing workorcheventpayload.Payload, overlay workorcheventpayload.Payload) workorcheventpayload.Payload {
	return existing.Merge(overlay)
}

func approvalContextEqual(left workorcheventpayload.Payload, right workorcheventpayload.Payload) bool {
	return left.Equal(right)
}
