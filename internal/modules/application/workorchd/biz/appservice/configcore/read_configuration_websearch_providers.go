package configcore

import (
	"sort"

	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func webSearchProviderConfigurations(cfg workorchconfig.Config) (string, []WebSearchProviderConfiguration, error) {
	defaultProviderID, providerConfigs, err := webproviderconfig.LoadProviderConfigs(cfg.WebSearch.DefaultProvider, cfg.WebSearchProviderValueSets())
	if err != nil {
		return "", nil, err
	}
	providers := make([]WebSearchProviderConfiguration, 0, len(providerConfigs))
	for id, providerConfig := range providerConfigs {
		rawProvider := cfg.WebSearch.Providers[id]
		providers = append(providers, WebSearchProviderConfiguration{
			ID:               id,
			Provider:         providerConfig.Provider,
			SearchEndpoint:   providerConfig.SearchEndpoint,
			FetchEndpoint:    providerConfig.FetchEndpoint,
			Headers:          cloneStringMap(rawProvider.Headers),
			Timeout:          rawProvider.Timeout,
			MaxResults:       rawProvider.MaxResults,
			MaxFetchBytes:    rawProvider.MaxFetchBytes,
			MaxRedirects:     rawProvider.MaxRedirects,
			AllowDomains:     append([]string(nil), rawProvider.AllowDomains...),
			DenyDomains:      append([]string(nil), rawProvider.DenyDomains...),
			Disabled:         rawProvider.Disabled != nil && *rawProvider.Disabled,
			Ready:            !providerConfig.Unavailable,
			Reason:           providerConfig.UnavailableReason,
			Default:          id == defaultProviderID,
			APIKeyConfigured: rawProvider.APIKey != "",
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].Default != providers[j].Default {
			return providers[i].Default
		}
		return providers[i].ID < providers[j].ID
	})
	return defaultProviderID, providers, nil
}
