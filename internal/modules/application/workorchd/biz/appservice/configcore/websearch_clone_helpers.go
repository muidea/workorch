package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func cloneWebSearchProviderConfigs(source map[string]workorchconfig.WebSearchProviderConfig) map[string]workorchconfig.WebSearchProviderConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.WebSearchProviderConfig{}
	}
	ret := make(map[string]workorchconfig.WebSearchProviderConfig, len(source))
	for id, provider := range source {
		ret[id] = cloneWebSearchProviderConfig(provider)
	}
	return ret
}

func cloneWebSearchProviderConfig(source workorchconfig.WebSearchProviderConfig) workorchconfig.WebSearchProviderConfig {
	return workorchconfig.WebSearchProviderConfig{
		Provider:       source.Provider,
		SearchEndpoint: source.SearchEndpoint,
		APIKey:         source.APIKey,
		Headers:        cloneStringMap(source.Headers),
		Timeout:        source.Timeout,
		MaxResults:     source.MaxResults,
		MaxFetchBytes:  source.MaxFetchBytes,
		MaxRedirects:   source.MaxRedirects,
		AllowDomains:   append([]string(nil), source.AllowDomains...),
		DenyDomains:    append([]string(nil), source.DenyDomains...),
		Disabled:       cloneBoolPtr(source.Disabled),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
