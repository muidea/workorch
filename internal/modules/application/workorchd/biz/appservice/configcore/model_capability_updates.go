package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func updatedLLMModelCapabilities(updates []LLMModelCapabilityConfiguration) (map[string]workorchconfig.LLMModelCapabilityConfig, error) {
	ret := make(map[string]workorchconfig.LLMModelCapabilityConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: LLM model capability id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate LLM model capability id %q", common.ErrInvalidArgument, id)
		}
		modelPattern := strings.TrimSpace(update.ModelPattern)
		if modelPattern == "" {
			return nil, fmt.Errorf("%w: LLM model capability modelPattern is required", common.ErrInvalidArgument)
		}
		if update.ContextWindowTokens <= 0 {
			return nil, fmt.Errorf("%w: LLM model capability contextWindowTokens must be positive", common.ErrInvalidArgument)
		}
		ret[id] = workorchconfig.LLMModelCapabilityConfig{
			Provider:            strings.TrimSpace(update.Provider),
			ModelPattern:        modelPattern,
			ContextWindowTokens: update.ContextWindowTokens,
		}
	}
	return ret, nil
}
