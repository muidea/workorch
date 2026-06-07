package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	providers := cloneLLMProviderConfigs(cfg.LLM.Providers)
	if cmd.Providers != nil {
		providers, err = updatedLLMProviders(cfg.LLM.Providers, *cmd.Providers)
		if err != nil {
			return ServiceConfiguration{}, err
		}
	}
	modelCapabilities := cloneLLMModelCapabilityConfigs(cfg.LLM.ModelCapabilities)
	if cmd.ModelCapabilities != nil {
		modelCapabilities, err = updatedLLMModelCapabilities(*cmd.ModelCapabilities)
		if err != nil {
			return ServiceConfiguration{}, err
		}
	}
	defaultProviderID, err := validatedDefaultLLMProviderID(cfg.LLM.DefaultProvider, cmd.DefaultProviderID, providers)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	defaultEmbeddingProviderID, err := validatedDefaultEmbeddingProviderID(cfg.LLM.DefaultEmbeddingProvider, cmd.DefaultEmbeddingProviderID, providers)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	cfg.LLM.DefaultProvider = defaultProviderID
	cfg.LLM.DefaultEmbeddingProvider = defaultEmbeddingProviderID
	cfg.LLM.ModelCapabilities = modelCapabilities
	cfg.LLM.Providers = providers
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}
