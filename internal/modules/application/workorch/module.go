package workorch

import (
	"context"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorch/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure() error { return nil }

func (m *Module) Start() error { return nil }

func (m *Module) Stop(_ context.Context) error { return nil }
