package run

import (
	"context"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Orchestrator interface {
	StartRun(context.Context, common.RunID) error
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

func (s *Service) CreateRun(ctx context.Context, cmd contracts.CreateRunCommand) error {
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

func (s *Service) StartRun(ctx context.Context, cmd contracts.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.StartRun(ctx, cmd.RunID)
}

func (s *Service) CompleteRun(ctx context.Context, cmd contracts.CompleteRunCommand) error {
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
	return s.runs.Update(ctx, r)
}

func (s *Service) FailRun(ctx context.Context, cmd contracts.FailRunCommand) error {
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
	return s.runs.Update(ctx, r)
}
