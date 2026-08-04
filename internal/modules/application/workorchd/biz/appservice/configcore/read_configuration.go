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
	defaultWebProviderID, webProviders, err := webSearchProviderConfigurations(cfg)
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	return workorchconfigurationcontract.ServiceConfiguration{
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
	}, nil
}

func llmConfigurationView(cfg workorchconfig.LLMConfig, catalog workorchmanagementcontract.ProviderCatalogSummary) workorchconfigurationcontract.LLMConfiguration {
	var defaultEmbedding *runtimecontract.ProviderModelTarget
	if cfg.DefaultEmbedding != nil {
		target := cfg.DefaultEmbedding.ToRuntimeTarget()
		defaultEmbedding = &target
	}
	customProviders := make([]workorchconfigurationcontract.CustomProviderConfigurationView, 0, len(cfg.CustomProviders))
	for id, provider := range cfg.CustomProviders {
		models := make([]workorchconfigurationcontract.CustomProviderModelView, 0, len(provider.Models))
		for modelID, model := range provider.Models {
			view := workorchconfigurationcontract.CustomProviderModelView{
				ID:                   modelID,
				ContextWindowTokens:  model.ContextWindowTokens,
				MaxOutputTokens:      model.MaxOutputTokens,
				Operations:           append([]string(nil), model.Operations...),
				ReasoningEffort:      model.ReasoningEffort,
				ThinkingBudgetTokens: model.ThinkingBudgetTokens,
				CacheMode:            model.Cache.Mode,
				CacheUsageFields:     append([]string(nil), model.Cache.UsageFields...),
			}
			if model.Temperature != nil {
				value := *model.Temperature
				view.Temperature = &value
			}
			if model.Cache.PromptCacheKey != nil {
				value := *model.Cache.PromptCacheKey
				view.PromptCacheKey = &value
			}
			if model.Cache.MessageCacheControl != nil {
				value := *model.Cache.MessageCacheControl
				view.MessageCacheControl = &value
			}
			models = append(models, view)
		}
		customProviders = append(customProviders, workorchconfigurationcontract.CustomProviderConfigurationView{
			ID:               id,
			Provider:         provider.Provider,
			APISurface:       provider.APISurface,
			BaseURL:          provider.BaseURL,
			Timeout:          provider.Timeout,
			APIKeyConfigured: strings.TrimSpace(provider.APIKey) != "",
			Models:           models,
		})
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
		CustomProviders:  customProviders,
		Catalog:          catalog,
	}
}
