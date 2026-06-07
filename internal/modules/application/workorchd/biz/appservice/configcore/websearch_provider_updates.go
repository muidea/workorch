package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func updatedWebSearchProviders(existing map[string]workorchconfig.WebSearchProviderConfig, updates []UpdateWebSearchProviderConfiguration) (map[string]workorchconfig.WebSearchProviderConfig, error) {
	ret := make(map[string]workorchconfig.WebSearchProviderConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: Web Search provider id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate Web Search provider id %q", common.ErrInvalidArgument, id)
		}
		next := cloneWebSearchProviderConfig(existing[id])
		applyWebSearchProviderUpdateFields(&next, update)
		ret[id] = next
	}
	return ret, nil
}
