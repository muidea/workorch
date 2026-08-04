package configcore

import (
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
)

func UpdateWebSearchConfiguration(cmd UpdateWebSearchConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	providers := cloneWebSearchProviderConfigs(cfg.WebSearch.Providers)
	if cmd.Providers != nil {
		providers, err = updatedWebSearchProviders(cfg.WebSearch.Providers, *cmd.Providers)
		if err != nil {
			return workorchconfigurationcontract.ServiceConfiguration{}, err
		}
	}
	if len(providers) == 0 {
		cfg.WebSearch.DefaultProvider = ""
		cfg.WebSearch.Providers = map[string]workorchconfig.WebSearchProviderConfig{}
		if err := workorchconfig.Save(cfg); err != nil {
			return workorchconfigurationcontract.ServiceConfiguration{}, err
		}
		if err := workorchconfig.Reload(); err != nil {
			return workorchconfigurationcontract.ServiceConfiguration{}, err
		}
		return ReadServiceConfiguration()
	}
	defaultProviderID, err := validatedDefaultWebSearchProviderID(cfg.WebSearch.DefaultProvider, cmd.DefaultProviderID, providers)
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	cfg.WebSearch.DefaultProvider = defaultProviderID
	cfg.WebSearch.Providers = providers
	if err := workorchconfig.Save(cfg); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}
