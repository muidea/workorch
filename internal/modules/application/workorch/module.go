package workorch

import (
	"context"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorch/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error { return nil }

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(_ context.Context) error { return nil }
