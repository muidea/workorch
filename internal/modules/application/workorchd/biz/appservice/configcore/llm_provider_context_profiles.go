package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func applyLLMProviderContextProfileUpdate(next *workorchconfig.LLMProviderConfig, update *UpdateLLMProviderContextProfile) {
	if next == nil || update == nil {
		return
	}
	if update.ProfileMode != nil {
		next.ContextProfileMode = strings.TrimSpace(*update.ProfileMode)
	}
	if update.ContextWindowTokens != nil {
		next.ContextWindowTokens = *update.ContextWindowTokens
	}
	if update.MaxInputTokens != nil {
		next.MaxInputTokens = *update.MaxInputTokens
	}
	if update.ReservedOutputTokens != nil {
		next.ReservedOutputTokens = *update.ReservedOutputTokens
	}
	if update.SafetyRatio != nil {
		next.SafetyRatio = *update.SafetyRatio
	}
	if update.ContextUtilizationRatio != nil {
		next.ContextUtilizationRatio = *update.ContextUtilizationRatio
	}
	if update.BytesHardLimit != nil {
		next.BytesHardLimit = *update.BytesHardLimit
	}
	if update.MaxRequestBytes != nil {
		next.MaxRequestBytes = *update.MaxRequestBytes
	}
	if update.EstimatedTokensPerByte != nil {
		next.EstimatedTokensPerByte = *update.EstimatedTokensPerByte
	}
}

func simplifyAutoLLMProviderContextProfile(next *workorchconfig.LLMProviderConfig) {
	if next == nil || strings.EqualFold(strings.TrimSpace(next.ContextProfileMode), "manual") {
		return
	}
	if strings.EqualFold(strings.TrimSpace(next.ContextProfileMode), "auto") {
		next.ContextProfileMode = ""
	}
	next.MaxInputTokens = 0
	next.ReservedOutputTokens = 0
	next.SafetyRatio = 0
	next.ContextUtilizationRatio = 0
	next.BytesHardLimit = 0
	next.MaxRequestBytes = 0
	next.EstimatedTokensPerByte = 0
}
