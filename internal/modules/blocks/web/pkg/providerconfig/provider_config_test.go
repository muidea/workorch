package providerconfig

import (
	"testing"
	"time"

	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
)

func TestProviderTimeoutAndResponseLimitsAreSystemBounded(t *testing.T) {
	_, providers, err := LoadProviderConfigs("provider-a", map[string]map[string]string{
		"provider-a": {
			"searchEndpoint": "https://provider.invalid/search",
			"timeout":        "24h",
			"maxResults":     "999",
			"maxFetchBytes":  "999999999",
			"maxRedirects":   "999",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := providers["provider-a"]
	if provider.Timeout != SystemMaxTimeout || provider.MaxResults != SystemMaxResults || provider.MaxFetchBytes != SystemMaxFetchBytes || provider.MaxRedirects != SystemMaxRedirects {
		t.Fatalf("provider limits = %+v", provider)
	}

	normalized := NormalizeProviders(map[string]webmodels.ProviderConfig{
		"provider-b": {ID: "provider-b", Timeout: 2 * SystemMaxTimeout, MaxResults: 200, MaxFetchBytes: 2 * SystemMaxFetchBytes, MaxRedirects: 20},
	})["provider-b"]
	if normalized.Timeout != SystemMaxTimeout || normalized.Timeout > time.Minute || normalized.MaxResults != SystemMaxResults || normalized.MaxFetchBytes != SystemMaxFetchBytes || normalized.MaxRedirects != SystemMaxRedirects {
		t.Fatalf("normalized limits = %+v", normalized)
	}
}
