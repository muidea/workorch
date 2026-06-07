package configcore

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func modelCapabilityConfigurations(source map[string]workorchconfig.LLMModelCapabilityConfig) []LLMModelCapabilityConfiguration {
	ret := make([]LLMModelCapabilityConfiguration, 0, len(source))
	for id, capability := range source {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ret = append(ret, LLMModelCapabilityConfiguration{
			ID:                  id,
			Provider:            strings.TrimSpace(capability.Provider),
			ModelPattern:        strings.TrimSpace(capability.ModelPattern),
			ContextWindowTokens: capability.ContextWindowTokens,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}
