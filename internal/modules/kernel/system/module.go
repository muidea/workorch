package system

import (
	"context"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/system/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Souls           repository.SoulRepository
	Templates       repository.WorkspaceTemplateRepository
	MCPServers      repository.MCPServerRepository
	MCPCapabilities repository.MCPCapabilityRepository
}

type Module struct {
	deps Dependencies
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Souls == nil || deps.Templates == nil {
		return common.ErrInvalidArgument
	}
	m.deps = deps
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m == nil || m.deps.Souls == nil || m.deps.Templates == nil {
		return common.ErrInvalidArgument
	}
	return workorchdefaults.EnsureBuiltinDefaults(ctx, m.deps.Souls, m.deps.Templates, m.deps.MCPServers, m.deps.MCPCapabilities)
}

func (m *Module) Stop(_ context.Context) error {
	m.deps = Dependencies{}
	return nil
}
