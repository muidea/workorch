package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func sortedLLMUsageProviderModelBreakdowns(items map[string]*readmodelcore.LLMUsageProviderModelBreakdown) []readmodelcore.LLMUsageProviderModelBreakdown {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.LLMUsageProviderModelBreakdown, 0, len(items))
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
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		return false
	})
	return ret
}
