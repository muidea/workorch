package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func applyLLMProviderBaseUpdate(next *workorchconfig.LLMProviderConfig, update UpdateLLMProviderConfiguration) {
	if next == nil {
		return
	}
	if update.Provider != nil {
		next.Provider = strings.TrimSpace(*update.Provider)
	}
	if update.BaseURL != nil {
		next.BaseURL = strings.TrimSpace(*update.BaseURL)
	}
	if update.Model != nil {
		next.Model = strings.TrimSpace(*update.Model)
	}
	if update.Timeout != nil {
		next.Timeout = strings.TrimSpace(*update.Timeout)
	}
	if update.APIKey != nil {
		if apiKey := strings.TrimSpace(*update.APIKey); apiKey != "" {
			next.APIKey = apiKey
		}
	}
	if update.Disabled != nil {
		next.Disabled = boolPtr(*update.Disabled)
	} else if next.Disabled == nil {
		next.Disabled = boolPtr(false)
	}
}

func applyLLMProviderCapabilityUpdate(next *workorchconfig.LLMProviderConfig, update UpdateLLMProviderConfiguration) {
	if next == nil {
		return
	}
	if update.ChatCompletions != nil {
		next.ChatCompletions = boolPtr(*update.ChatCompletions)
	} else if next.ChatCompletions == nil {
		next.ChatCompletions = boolPtr(true)
	}
	if update.Streaming != nil {
		next.Streaming = boolPtr(*update.Streaming)
	} else if next.Streaming == nil {
		next.Streaming = boolPtr(false)
	}
	if update.NonChatOperations != nil {
		next.NonChatOperations = normalizedProviderOperations(update.NonChatOperations)
	}
	if update.FailoverEnabled != nil {
		next.FailoverEnabled = boolPtr(*update.FailoverEnabled)
	} else if next.FailoverEnabled == nil {
		next.FailoverEnabled = boolPtr(false)
	}
	if update.FailoverProviders != nil {
		next.FailoverProviders = normalizedProviderOperations(update.FailoverProviders)
	}
}
