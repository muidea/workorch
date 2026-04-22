package run

import (
	"context"
	"errors"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Orchestrator interface {
	StartRun(context.Context, common.RunID) error
	BlockOnDecision(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error
	ResumeRun(context.Context, common.RunID) error
	CompleteRun(context.Context, common.RunID) error
	FailRun(context.Context, common.RunID, string) error
}

type Service struct {
	runs         repository.RunRepository
	workspaces   repository.WorkspaceRepository
	orchestrator Orchestrator
}

func NewService(runs repository.RunRepository, workspaces repository.WorkspaceRepository, orchestrator Orchestrator) *Service {
	return &Service{runs: runs, workspaces: workspaces, orchestrator: orchestrator}
}

func (s *Service) CreateRun(ctx context.Context, cmd runservice.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := s.workspaces.Get(ctx, cmd.WorkspaceID); err != nil {
		return err
	}

	run := &runModel.Run{
		ID:          cmd.RunID,
		WorkspaceID: cmd.WorkspaceID,
		GoalID:      cmd.GoalID,
		DoDID:       cmd.DoDID,
		State:       common.RunStateCreated,
	}
	return s.runs.Create(ctx, run)
}

func (s *Service) EnsureRun(ctx context.Context, cmd runservice.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}

	run, err := s.runs.Get(ctx, cmd.RunID)
	if err == nil {
		if run.WorkspaceID != cmd.WorkspaceID {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateRun(ctx, cmd)
}

func (s *Service) StartRun(ctx context.Context, cmd runservice.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.StartRun(ctx, cmd.RunID)
}

func (s *Service) EnsureStarted(ctx context.Context, cmd runservice.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateCompleted {
		return nil
	}
	if r.State == common.RunStateFailed || r.State == common.RunStateBlockedOnDecision || r.State == common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return s.StartRun(ctx, cmd)
}

func (s *Service) ResumeRun(ctx context.Context, cmd runservice.ResumeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateBlockedOnDecision && r.State != common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}

	return s.orchestrator.ResumeRun(ctx, cmd.RunID)
}

func (s *Service) EnsureResumed(ctx context.Context, cmd runservice.ResumeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateCompleted || r.State == common.RunStateFailed {
		return nil
	}
	if r.State != common.RunStateBlockedOnDecision && r.State != common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return s.ResumeRun(ctx, cmd)
}

func (s *Service) BlockOnDecision(ctx context.Context, cmd runservice.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
}

func (s *Service) EnsureBlockedOnDecision(ctx context.Context, cmd runservice.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateBlockedOnDecision && r.BlockingTicketID == cmd.TicketID {
		return nil
	}
	if r.State == common.RunStateCompleted || r.State == common.RunStateFailed {
		return nil
	}
	return s.BlockOnDecision(ctx, cmd)
}

func (s *Service) BlockOnApproval(ctx context.Context, cmd runservice.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
}

func (s *Service) EnsureBlockedOnApproval(ctx context.Context, cmd runservice.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateBlockedOnApproval && r.BlockingApprovalID == cmd.ApprovalID {
		return nil
	}
	if r.State == common.RunStateCompleted || r.State == common.RunStateFailed {
		return nil
	}
	return s.BlockOnApproval(ctx, cmd)
}

func (s *Service) CompleteRun(ctx context.Context, cmd runservice.CompleteRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.CompleteRun(ctx, cmd.RunID)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	r.State = common.RunStateCompleted
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	return s.runs.Update(ctx, r)
}

func (s *Service) FailRun(ctx context.Context, cmd runservice.FailRunCommand) error {
	if cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.FailRun(ctx, cmd.RunID, cmd.Reason)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	r.State = common.RunStateFailed
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	return s.runs.Update(ctx, r)
}
