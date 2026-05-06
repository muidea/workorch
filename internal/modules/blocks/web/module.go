package web

import (
	"context"

	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Dependencies struct {
	Approvals repository.ApprovalRepository
}

type Module struct {
	runtimeAdapter *webbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	m.runtimeAdapter = webbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision, deps.Approvals)
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *webbiz.RuntimeAdapter {
	return m.runtimeAdapter
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]webbiz.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return webbiz.LoadProviderConfigs(cfg.WebSearch.DefaultProvider, cfg.WebSearchProviderValueSets())
}
