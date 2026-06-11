package llm

import (
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func normalizedProviderConfigForRuntime(config llmmodels.ProviderConfig) llmmodels.ProviderConfig {
	if strings.TrimSpace(config.APISurface) == "" {
		config.APISurface = defaultProviderAPISurface(config.Provider)
	}
	if strings.TrimSpace(config.CacheCapability.Surface) == "" {
		config.CacheCapability.Surface = config.APISurface
	}
	if strings.TrimSpace(config.CacheCapability.Mode) == "" {
		config.CacheCapability.Mode = defaultRuntimeProviderCacheMode(config.APISurface)
	}
	if config.CacheCapability.Mode == llmcommon.ProviderCacheModePromptCacheKey && !config.CacheCapability.PromptCacheKey {
		config.CacheCapability.PromptCacheKey = true
	}
	if config.CacheCapability.Mode == llmcommon.ProviderCacheModeAnthropicCacheControl && !config.CacheCapability.MessageCacheControl {
		config.CacheCapability.MessageCacheControl = true
	}
	return config
}

func defaultProviderAPISurface(provider string) string {
	switch strings.TrimSpace(provider) {
	case llmcommon.ProviderOpenAI:
		return llmcommon.ProviderSurfaceOpenAISDK
	case llmcommon.ProviderAnthropic:
		return llmcommon.ProviderSurfaceAnthropicSDK
	case llmcommon.ProviderCompatible:
		return llmcommon.ProviderSurfaceOpenAICompatibleHTTP
	default:
		return ""
	}
}

func defaultRuntimeProviderCacheMode(surface string) string {
	switch strings.TrimSpace(surface) {
	case llmcommon.ProviderSurfaceOpenAISDK:
		return llmcommon.ProviderCacheModePromptCacheKey
	case llmcommon.ProviderSurfaceAnthropicSDK:
		return llmcommon.ProviderCacheModeAnthropicCacheControl
	default:
		return llmcommon.ProviderCacheModeNone
	}
}
