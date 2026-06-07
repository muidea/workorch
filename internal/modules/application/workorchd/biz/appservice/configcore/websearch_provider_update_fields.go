package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func applyWebSearchProviderUpdateFields(next *workorchconfig.WebSearchProviderConfig, update UpdateWebSearchProviderConfiguration) {
	if update.Provider != nil {
		next.Provider = strings.TrimSpace(*update.Provider)
	}
	if update.SearchEndpoint != nil {
		next.SearchEndpoint = strings.TrimSpace(*update.SearchEndpoint)
	}
	if update.FetchEndpoint != nil {
		next.FetchEndpoint = strings.TrimSpace(*update.FetchEndpoint)
	}
	if update.APIKey != nil {
		if apiKey := strings.TrimSpace(*update.APIKey); apiKey != "" {
			next.APIKey = apiKey
		}
	}
	if update.Headers != nil {
		next.Headers = cloneStringMap(update.Headers)
	}
	if update.Timeout != nil {
		next.Timeout = strings.TrimSpace(*update.Timeout)
	}
	if update.MaxResults != nil {
		next.MaxResults = *update.MaxResults
	}
	if update.MaxFetchBytes != nil {
		next.MaxFetchBytes = *update.MaxFetchBytes
	}
	if update.MaxRedirects != nil {
		next.MaxRedirects = *update.MaxRedirects
	}
	if update.AllowDomains != nil {
		next.AllowDomains = normalizedProviderOperations(update.AllowDomains)
	}
	if update.DenyDomains != nil {
		next.DenyDomains = normalizedProviderOperations(update.DenyDomains)
	}
	if update.Disabled != nil {
		next.Disabled = boolPtr(*update.Disabled)
	} else if next.Disabled == nil {
		next.Disabled = boolPtr(false)
	}
}
