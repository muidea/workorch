package web

import (
	"fmt"
	"strings"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (a *RuntimeAdapter) selectProviderID(requestedID string, operationKind string) (webmodels.ProviderConfig, error) {
	defaultID, providers, err := a.providerConfigs()
	if err != nil {
		return webmodels.ProviderConfig{}, err
	}
	providerID := strings.TrimSpace(requestedID)
	if providerID == "" {
		providerID = defaultID
	}
	provider, ok := providers[providerID]
	if !ok {
		return webmodels.ProviderConfig{ID: providerID}, fmt.Errorf("%w: web search provider %q is not configured", common.ErrInvalidArgument, providerID)
	}
	if provider.Unavailable {
		reason := provider.UnavailableReason
		if reason == "" {
			reason = "web search provider is unavailable"
		}
		return provider, fmt.Errorf("%w: %s", common.ErrInvalidArgument, reason)
	}
	if strings.TrimSpace(operationKind) == webcommon.StepKindSearch && strings.TrimSpace(provider.SearchEndpoint) == "" {
		return provider, fmt.Errorf("%w: web search provider searchEndpoint is not configured", common.ErrInvalidArgument)
	}
	return provider, nil
}

func (a *RuntimeAdapter) providerConfigs() (string, map[string]webmodels.ProviderConfig, error) {
	if a.configLoader == nil {
		return a.defaultProviderID, webproviderconfig.NormalizeProviders(a.providers), nil
	}
	revision := uint64(0)
	if a.configRevision != nil {
		revision = a.configRevision()
	}
	a.cacheMu.RLock()
	if a.cache.ready && a.cache.revision == revision {
		defaultID := a.cache.defaultProviderID
		providers := webproviderconfig.CloneProviders(a.cache.providers)
		a.cacheMu.RUnlock()
		return defaultID, providers, nil
	}
	a.cacheMu.RUnlock()

	defaultID, providers, err := a.configLoader()
	if err != nil {
		return "", nil, err
	}
	providers = webproviderconfig.NormalizeProviders(providers)
	if strings.TrimSpace(defaultID) == "" {
		defaultID = webproviderconfig.DefaultProviderIDFromProviders(providers)
	}
	a.cacheMu.Lock()
	a.cache = providerRuntimeCache{
		ready:             true,
		revision:          revision,
		defaultProviderID: defaultID,
		providers:         webproviderconfig.CloneProviders(providers),
	}
	a.cacheMu.Unlock()
	return defaultID, providers, nil
}
