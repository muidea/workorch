package providerconfig

import (
	"strconv"
	"strings"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
)

const (
	DefaultProviderID    = "default-http"
	DefaultTimeout       = 20 * time.Second
	SystemMaxTimeout     = 60 * time.Second
	DefaultMaxResults    = 8
	SystemMaxResults     = 20
	DefaultMaxFetchBytes = int64(256 * 1024)
	SystemMaxFetchBytes  = int64(1024 * 1024)
	DefaultMaxRedirects  = 3
	SystemMaxRedirects   = 5
)

func LoadProviderConfigs(defaultProviderID string, valuesByID map[string]map[string]string) (string, map[string]webmodels.ProviderConfig, error) {
	providers := map[string]webmodels.ProviderConfig{}
	for id, values := range valuesByID {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		provider := webmodels.ProviderConfig{
			ID:             id,
			Provider:       strings.TrimSpace(values["provider"]),
			SearchEndpoint: strings.TrimSpace(values["searchEndpoint"]),
			FetchEndpoint:  strings.TrimSpace(values["fetchEndpoint"]),
			APIKey:         strings.TrimSpace(values["apiKey"]),
			Headers:        headersFromValues(values),
			Timeout:        boundedDurationValue(values["timeout"], DefaultTimeout, SystemMaxTimeout),
			MaxResults:     boundedIntValue(values["maxResults"], DefaultMaxResults, SystemMaxResults),
			MaxFetchBytes:  boundedInt64Value(values["maxFetchBytes"], DefaultMaxFetchBytes, SystemMaxFetchBytes),
			MaxRedirects:   boundedIntValue(values["maxRedirects"], DefaultMaxRedirects, SystemMaxRedirects),
			AllowDomains:   splitCSV(values["allowDomains"]),
			DenyDomains:    splitCSV(values["denyDomains"]),
			Disabled:       boolValue(values["disabled"]),
		}
		if provider.Provider == "" {
			provider.Provider = webcommon.ProviderHTTP
		}
		if provider.Provider != webcommon.ProviderHTTP {
			provider.Unavailable = true
			provider.UnavailableReason = "unsupported web search provider " + provider.Provider
		}
		if provider.Disabled {
			provider.Unavailable = true
			provider.UnavailableReason = "web search provider is disabled"
		}
		if provider.SearchEndpoint == "" {
			provider.Unavailable = true
			provider.UnavailableReason = "web search provider searchEndpoint is not configured"
		}
		providers[id] = provider
	}
	return strings.TrimSpace(defaultProviderID), NormalizeProviders(providers), nil
}

func NormalizeProviders(providers map[string]webmodels.ProviderConfig) map[string]webmodels.ProviderConfig {
	ret := map[string]webmodels.ProviderConfig{}
	for id, provider := range providers {
		id = strings.TrimSpace(id)
		if id == "" {
			id = strings.TrimSpace(provider.ID)
		}
		if id == "" {
			continue
		}
		provider.ID = id
		if provider.Provider == "" {
			provider.Provider = webcommon.ProviderHTTP
		}
		if provider.Timeout <= 0 {
			provider.Timeout = DefaultTimeout
		} else if provider.Timeout > SystemMaxTimeout {
			provider.Timeout = SystemMaxTimeout
		}
		provider.MaxResults = boundedInt(provider.MaxResults, DefaultMaxResults, SystemMaxResults)
		provider.MaxFetchBytes = boundedInt64(provider.MaxFetchBytes, DefaultMaxFetchBytes, SystemMaxFetchBytes)
		provider.MaxRedirects = boundedInt(provider.MaxRedirects, DefaultMaxRedirects, SystemMaxRedirects)
		if provider.Headers == nil {
			provider.Headers = map[string]string{}
		}
		ret[id] = provider
	}
	return ret
}

func DefaultProviderIDFromProviders(providers map[string]webmodels.ProviderConfig) string {
	if _, ok := providers[DefaultProviderID]; ok {
		return DefaultProviderID
	}
	for id := range providers {
		return id
	}
	return DefaultProviderID
}

func CloneProviders(input map[string]webmodels.ProviderConfig) map[string]webmodels.ProviderConfig {
	ret := map[string]webmodels.ProviderConfig{}
	for id, provider := range input {
		provider.Headers = cloneStringMap(provider.Headers)
		provider.AllowDomains = append([]string(nil), provider.AllowDomains...)
		provider.DenyDomains = append([]string(nil), provider.DenyDomains...)
		ret[id] = provider
	}
	return ret
}

func headersFromValues(values map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range values {
		if strings.HasPrefix(key, "header.") {
			name := strings.TrimPrefix(key, "header.")
			if strings.TrimSpace(name) != "" && strings.TrimSpace(value) != "" {
				ret[name] = strings.TrimSpace(value)
			}
		}
	}
	return ret
}

func durationValue(raw string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boundedDurationValue(raw string, fallback time.Duration, max time.Duration) time.Duration {
	value := durationValue(raw, fallback)
	if max > 0 && value > max {
		return max
	}
	return value
}

func boundedIntValue(raw string, fallback int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return boundedInt(value, fallback, max)
}

func boundedInt64Value(raw string, fallback int64, max int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fallback
	}
	return boundedInt64(value, fallback, max)
}

func boundedInt(value int, fallback int, max int) int {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func boundedInt64(value int64, fallback int64, max int64) int64 {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}

func boolValue(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

func cloneStringMap(input map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
