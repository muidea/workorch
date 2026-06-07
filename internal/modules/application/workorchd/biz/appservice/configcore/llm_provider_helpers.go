package configcore

import (
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func loadUpdatedProviderConfig(providerID string, provider workorchconfig.LLMProviderConfig) (llmmodels.ProviderConfig, error) {
	values := workorchconfig.Config{
		LLM: workorchconfig.LLMConfig{
			Providers: map[string]workorchconfig.LLMProviderConfig{
				providerID: provider,
			},
		},
	}.LLMProviderValueSets()[providerID]
	return llmproviderconfig.LoadProviderConfig(providerID, values)
}

func providerSupportsOperation(provider workorchconfig.LLMProviderConfig, operation string) bool {
	for _, item := range provider.NonChatOperations {
		if strings.TrimSpace(item) == operation {
			return true
		}
	}
	return false
}

func normalizedProviderOperations(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	return ret
}
