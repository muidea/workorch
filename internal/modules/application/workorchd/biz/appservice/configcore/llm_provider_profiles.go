package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func applyLLMProviderInferenceProfileUpdate(next *workorchconfig.LLMProviderConfig, update *UpdateLLMProviderInferenceProfile) {
	if next == nil || update == nil {
		return
	}
	if update.Temperature != nil {
		next.Temperature = cloneFloat64Ptr(update.Temperature)
	}
	if update.MaxOutputTokens != nil {
		next.MaxOutputTokens = *update.MaxOutputTokens
	}
	if update.ReasoningEffort != nil {
		next.ReasoningEffort = strings.TrimSpace(*update.ReasoningEffort)
	}
	if update.ThinkingBudgetTokens != nil {
		next.ThinkingBudgetTokens = *update.ThinkingBudgetTokens
	}
}
