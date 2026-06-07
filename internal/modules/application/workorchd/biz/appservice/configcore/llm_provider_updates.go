package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func updatedLLMProviders(existing map[string]workorchconfig.LLMProviderConfig, updates []UpdateLLMProviderConfiguration) (map[string]workorchconfig.LLMProviderConfig, error) {
	if len(updates) == 0 {
		return nil, fmt.Errorf("%w: at least one LLM provider is required", common.ErrInvalidArgument)
	}
	ret := make(map[string]workorchconfig.LLMProviderConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: LLM provider id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate LLM provider id %q", common.ErrInvalidArgument, id)
		}
		next := cloneLLMProviderConfig(existing[id])
		applyLLMProviderBaseUpdate(&next, update)
		applyLLMProviderCapabilityUpdate(&next, update)
		if update.InferenceProfile != nil {
			applyLLMProviderInferenceProfileUpdate(&next, update.InferenceProfile)
		}
		if update.ContextProfile != nil {
			applyLLMProviderContextProfileUpdate(&next, update.ContextProfile)
		}
		simplifyAutoLLMProviderContextProfile(&next)
		ret[id] = next
	}
	return ret, nil
}
