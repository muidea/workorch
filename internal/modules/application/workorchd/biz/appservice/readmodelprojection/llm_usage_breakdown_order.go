package readmodelprojection

import (
	"sort"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func sortedLLMUsageProviderModelBreakdowns(items map[string]*projectioncontract.LLMUsageProviderModelBreakdown) []projectioncontract.LLMUsageProviderModelBreakdown {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.LLMUsageProviderModelBreakdown, 0, len(items))
	for _, item := range items {
		ret = append(ret, *item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		left := ret[i]
		right := ret[j]
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.APISurface != right.APISurface {
			return left.APISurface < right.APISurface
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		if left.ProviderMode != right.ProviderMode {
			return left.ProviderMode < right.ProviderMode
		}
		return false
	})
	return ret
}
