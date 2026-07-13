package run

import (
	"context"

	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateRunCommand = runcontracts.CreateRunCommand

type Service interface {
	CreateRun(context.Context, CreateRunCommand) error
	EnsureRun(context.Context, CreateRunCommand) error
}

type Module struct {
	createRun func(context.Context, CreateRunCommand) error
	ensureRun func(context.Context, CreateRunCommand) error
}

type Dependencies struct {
	CreateRun func(context.Context, CreateRunCommand) error
	EnsureRun func(context.Context, CreateRunCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createRun: deps.CreateRun,
		ensureRun: deps.EnsureRun,
	}
}

func (m *Module) CreateRun(ctx context.Context, cmd CreateRunCommand) error {
	if m == nil || m.createRun == nil {
		return common.ErrInvalidArgument
	}
	return m.createRun(ctx, cmd)
}

func (m *Module) EnsureRun(ctx context.Context, cmd CreateRunCommand) error {
	if m == nil || m.ensureRun == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureRun(ctx, cmd)
}
