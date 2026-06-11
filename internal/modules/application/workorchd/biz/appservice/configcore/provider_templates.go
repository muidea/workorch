package configcore

import (
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func DefaultLLMProviderTemplates() []LLMProviderTemplate {
	return []LLMProviderTemplate{
		{
			TemplateID:      llmcommon.ProviderOpenAI,
			Name:            "OpenAI API",
			Description:     "使用 OpenAI 官方 SDK 调用 Chat Completions；DeepSeek、MiniMax 等可直接使用 OpenAI SDK 的服务应选择该模式并配置各自 baseURL。",
			ID:              "openai-main",
			Provider:        llmcommon.ProviderOpenAI,
			APISurface:      llmcommon.ProviderSurfaceOpenAISDK,
			BaseURL:         "https://api.openai.com/v1",
			Model:           "gpt-4.1-mini",
			Timeout:         "60s",
			ChatCompletions: true,
		},
		{
			TemplateID:      llmcommon.ProviderAnthropic,
			Name:            "Anthropic API",
			Description:     "使用 Anthropic 官方 SDK 调用 Messages API；当前不作为 Embedding Provider。",
			ID:              "anthropic-main",
			Provider:        llmcommon.ProviderAnthropic,
			APISurface:      llmcommon.ProviderSurfaceAnthropicSDK,
			BaseURL:         "https://api.anthropic.com",
			Model:           "claude-sonnet-4-5-20250929",
			Timeout:         "60s",
			ChatCompletions: true,
		},
		{
			TemplateID:      llmcommon.ProviderCompatible,
			Name:            "OpenAI Compatible API",
			Description:     "用于未接入官方 SDK、但暴露 OpenAI-compatible HTTP 接口的本地或自定义服务，继续走轻量 HTTP 兼容实现。",
			ID:              "compatible-main",
			Provider:        llmcommon.ProviderCompatible,
			APISurface:      llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			BaseURL:         "http://127.0.0.1:11434/v1",
			Model:           "deepseek-chat",
			Timeout:         "60s",
			ChatCompletions: true,
		},
	}
}
