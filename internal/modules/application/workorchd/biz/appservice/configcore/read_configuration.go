package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func ReadServiceConfiguration() (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	defaultProviderID, providers, err := llmProviderConfigurations(cfg)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	defaultWebProviderID, webProviders, err := webSearchProviderConfigurations(cfg)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	return ServiceConfiguration{
		Server:     serverConfiguration(cfg),
		Repository: repositoryConfiguration(cfg),
		Runtime:    runtimeConfiguration(cfg),
		LLM: LLMConfiguration{
			DefaultProviderID:          defaultProviderID,
			DefaultEmbeddingProviderID: defaultEmbeddingProviderID(cfg),
			ModelCapabilities:          modelCapabilityConfigurations(cfg.LLM.ModelCapabilities),
			Providers:                  providers,
			ProviderTemplates:          DefaultLLMProviderTemplates(),
		},
		WebSearch: WebSearchConfiguration{
			DefaultProviderID: defaultWebProviderID,
			Providers:         webProviders,
			ProviderTemplates: DefaultWebSearchProviderTemplates(),
		},
		Governance: governanceConfiguration(cfg),
	}, nil
}
