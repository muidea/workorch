package run

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	runs repository.RunRepository
}

func NewService(runs repository.RunRepository) *Service {
	return &Service{runs: runs}
}

func (s *Service) CreateRun(ctx context.Context, cmd contracts.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	_ = ctx
	return nil
}

func (s *Service) StartRun(ctx context.Context, cmd contracts.StartRunCommand) error {
	_ = ctx
	_ = cmd
	return nil
}
func (s *Service) CompleteRun(ctx context.Context, cmd contracts.CompleteRunCommand) error {
	_ = ctx
	_ = cmd
	return nil
}
func (s *Service) FailRun(ctx context.Context, cmd contracts.FailRunCommand) error {
	_ = ctx
	_ = cmd
	return nil
}
