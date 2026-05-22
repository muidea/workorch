package git

import (
	"context"

	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

type Module struct {
	runtimeAdapter *gitbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return err
	}
	if !workorchconfig.GitRuntimeEnabled(cfg.Runtime.Git) {
		m.runtimeAdapter = nil
		return nil
	}
	adapter, err := gitbiz.NewRuntimeAdapterFromTimeoutString(
		workorchconfig.GitRuntimeWriteEnabled(cfg.Runtime.Git),
		cfg.Runtime.Git.DefaultTimeout,
	)
	if err != nil {
		return err
	}
	m.runtimeAdapter = adapter
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *gitbiz.RuntimeAdapter {
	return m.runtimeAdapter
}
