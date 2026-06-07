package configcore

import (
	"fmt"
	"strings"

	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func validatedDefaultWebSearchProviderID(existing string, update *string, providers map[string]workorchconfig.WebSearchProviderConfig) (string, error) {
	defaultProviderID := strings.TrimSpace(existing)
	if update != nil {
		defaultProviderID = strings.TrimSpace(*update)
	}
	if defaultProviderID == "" {
		return "", fmt.Errorf("%w: defaultProviderId is required", common.ErrInvalidArgument)
	}
	if _, ok := providers[defaultProviderID]; !ok {
		return "", fmt.Errorf("%w: defaultProviderId must reference a Web Search provider", common.ErrInvalidArgument)
	}
	if _, loaded, err := webproviderconfig.LoadProviderConfigs(defaultProviderID, workorchconfig.Config{
		WebSearch: workorchconfig.WebSearchConfig{
			DefaultProvider: defaultProviderID,
			Providers:       providers,
		},
	}.WebSearchProviderValueSets()); err != nil {
		return "", err
	} else if provider := loaded[defaultProviderID]; provider.Unavailable {
		return "", fmt.Errorf("%w: defaultProviderId references an unavailable Web Search provider: %s", common.ErrInvalidArgument, provider.UnavailableReason)
	}
	return defaultProviderID, nil
}
