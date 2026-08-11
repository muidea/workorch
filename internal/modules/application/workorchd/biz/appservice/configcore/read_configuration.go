package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ReadServiceConfiguration() (workorchconfigurationcontract.ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	persisted, err := workorchconfig.Persisted()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	cfg.LLM = persisted.LLM
	defaultWebProviderID, webProviders, err := webSearchProviderConfigurations(cfg)
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	item := workorchconfigurationcontract.ServiceConfiguration{
		Server:     serverConfiguration(cfg),
		Repository: repositoryConfiguration(cfg),
		Runtime:    runtimeConfiguration(cfg),
		LLM:        llmConfigurationView(cfg.LLM, workorchmanagementcontract.ProviderCatalogSummary{ProviderID: runtimecontract.BuiltinAIProxyProviderID, State: "empty"}),
		WebSearch: workorchconfigurationcontract.WebSearchConfiguration{
			DefaultProviderID: defaultWebProviderID,
			Providers:         webProviders,
			ProviderTemplates: DefaultWebSearchProviderTemplates(),
		},
		Governance: governanceConfiguration(cfg),
	}
	return item, nil
}

func llmConfigurationView(cfg workorchconfig.LLMConfig, catalog workorchmanagementcontract.ProviderCatalogSummary) workorchconfigurationcontract.LLMConfiguration {
	var defaultEmbedding *runtimecontract.ProviderModelTarget
	if cfg.DefaultEmbedding != nil {
		target := cfg.DefaultEmbedding.ToRuntimeTarget()
		defaultEmbedding = &target
	}
	return workorchconfigurationcontract.LLMConfiguration{
		AIProxy: workorchconfigurationcontract.AIProxyConfigurationView{
			BaseURL:                 cfg.AIProxy.BaseURL,
			APIKeyConfigured:        strings.TrimSpace(cfg.AIProxy.APIKey) != "",
			Timeout:                 cfg.AIProxy.Timeout,
			CatalogRequestTimeout:   cfg.AIProxy.CatalogRequestTimeout,
			CatalogTTL:              cfg.AIProxy.CatalogTTL,
			CatalogMaxStaleAge:      cfg.AIProxy.CatalogMaxStaleAge,
			MaxCatalogResponseBytes: cfg.AIProxy.MaxCatalogResponseBytes,
			MaxCatalogModels:        cfg.AIProxy.MaxCatalogModels,
		},
		DefaultChat:      cfg.DefaultChat.ToRuntimeTarget(),
		DefaultEmbedding: defaultEmbedding,
		Budget:           cfg.Budget,
		Catalog:          catalog,
	}
}
