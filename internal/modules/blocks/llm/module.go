package llm

import (
	"context"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	runtimeAdapter *llmbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	m.runtimeAdapter = llmbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision)
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *llmbiz.RuntimeAdapter {
	return m.runtimeAdapter
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]llmmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return llmbiz.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
}
