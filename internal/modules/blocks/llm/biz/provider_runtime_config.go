package llm

import (
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// defaultProviderTimeout is used when a resolved provider client has no explicit timeout.
const defaultProviderTimeout = 60 * time.Second

func normalizedProviderConfigForRuntime(config llmmodels.ProviderConfig) llmmodels.ProviderConfig {
	config.APISurface = llmcommon.ProviderSurfaceOpenAICompatibleHTTP
	return config
}

func providerTemperature(config llmmodels.ProviderConfig) float64 {
	if config.InferenceProfile.Temperature == nil {
		return 0
	}
	return *config.InferenceProfile.Temperature
}

func maxOutputTokens(config llmmodels.ProviderConfig) int64 {
	if config.InferenceProfile.MaxOutputTokens <= 0 {
		return 0
	}
	return int64(config.InferenceProfile.MaxOutputTokens)
}

func maxOutputTokensForRequest(config llmmodels.ProviderConfig, request llmmodels.ProviderRequest) int64 {
	if request.MaxOutputTokens > 0 && (config.InferenceProfile.MaxOutputTokens <= 0 || request.MaxOutputTokens < config.InferenceProfile.MaxOutputTokens) {
		return int64(request.MaxOutputTokens)
	}
	return maxOutputTokens(config)
}
