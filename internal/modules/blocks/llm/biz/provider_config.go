package llm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
)

const defaultProviderTimeout = 60 * time.Second
const defaultOpenAIBaseURL = "https://api.openai.com/v1"
const defaultAnthropicBaseURL = "https://api.anthropic.com"
const defaultContextWindowTokens = 32768
const defaultReservedOutputTokens = 4096
const defaultContextSafetyRatio = 0.8
const defaultContextUtilizationRatio = 0.75
const defaultContextBytesHardLimit = 96 * 1024
const defaultContextMaxRequestBytes = 4 * 1024 * 1024

const (
	contextProfileModeAuto   = "auto"
	contextProfileModeManual = "manual"

	contextCapabilitySourceExplicit            = "explicit_config"
	contextCapabilitySourceModelRegistry       = "model_registry"
	contextCapabilitySourceConservativeDefault = "conservative_default"
)

func LoadProviderConfig(providerID string, values map[string]string) (llmmodels.ProviderConfig, error) {
	return llmproviderconfig.LoadProviderConfig(providerID, values)
}

func LoadProviderConfigs(defaultProviderID string, valueSets map[string]map[string]string) (string, map[string]llmmodels.ProviderConfig, error) {
	return llmproviderconfig.LoadProviderConfigs(defaultProviderID, valueSets)
}

func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	return llmproviderconfig.RedactedProviderConfig(cfg)
}

func RedactSecret(value string) string {
	return llmproviderconfig.RedactSecret(value)
}

func providerInferenceProfileFromValues(values map[string]string) (llmmodels.ProviderInferenceProfile, error) {
	var profile llmmodels.ProviderInferenceProfile
	if strings.TrimSpace(values[llmcommon.EnvTemperature]) != "" {
		temperature, err := parseOptionalNonNegativeFloat(values[llmcommon.EnvTemperature], llmcommon.EnvTemperature)
		if err != nil {
			return llmmodels.ProviderInferenceProfile{}, err
		}
		profile.Temperature = &temperature
	}
	maxOutputTokens, err := parseOptionalPositiveInt(values[llmcommon.EnvMaxOutputTokens], llmcommon.EnvMaxOutputTokens)
	if err != nil {
		return llmmodels.ProviderInferenceProfile{}, err
	}
	profile.MaxOutputTokens = maxOutputTokens
	thinkingBudgetTokens, err := parseOptionalPositiveInt(values[llmcommon.EnvThinkingBudget], llmcommon.EnvThinkingBudget)
	if err != nil {
		return llmmodels.ProviderInferenceProfile{}, err
	}
	profile.ThinkingBudgetTokens = thinkingBudgetTokens
	profile.ReasoningEffort = normalizedReasoningEffort(values[llmcommon.EnvReasoningEffort])
	return profile, nil
}

func normalizedReasoningEffort(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func providerContextProfileFromValues(values map[string]string) (llmmodels.ProviderContextProfile, error) {
	profile := llmmodels.ProviderContextProfile{}
	var err error
	profile.ProfileMode = strings.TrimSpace(values[llmcommon.EnvContextProfileMode])
	profile.ContextWindowTokens, err = parseOptionalPositiveInt(values[llmcommon.EnvContextWindow], llmcommon.EnvContextWindow)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.MaxInputTokens, err = parseOptionalPositiveInt(values[llmcommon.EnvMaxInputTokens], llmcommon.EnvMaxInputTokens)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.ReservedOutputTokens, err = parseOptionalPositiveInt(values[llmcommon.EnvReservedOutput], llmcommon.EnvReservedOutput)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.BytesHardLimit, err = parseOptionalPositiveInt(values[llmcommon.EnvBytesHardLimit], llmcommon.EnvBytesHardLimit)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.SafetyRatio, err = parseOptionalPositiveFloat(values[llmcommon.EnvSafetyRatio], llmcommon.EnvSafetyRatio)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.ContextUtilizationRatio, err = parseOptionalPositiveFloat(values[llmcommon.EnvContextUtilization], llmcommon.EnvContextUtilization)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.MaxRequestBytes, err = parseOptionalPositiveInt(values[llmcommon.EnvMaxRequestBytes], llmcommon.EnvMaxRequestBytes)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.EstimatedTokensPerByte, err = parseOptionalPositiveFloat(values[llmcommon.EnvTokensPerByte], llmcommon.EnvTokensPerByte)
	if err != nil {
		return llmmodels.ProviderContextProfile{}, err
	}
	profile.CapabilitySource = strings.TrimSpace(values[llmcommon.EnvContextCapabilitySource])
	return resolveProviderContextProfile(profile, providerContextProfileExplicitFields(values)), nil
}

type providerContextExplicitFields struct {
	ContextWindowTokens     bool
	MaxInputTokens          bool
	ReservedOutputTokens    bool
	BytesHardLimit          bool
	MaxRequestBytes         bool
	ContextUtilizationRatio bool
}

func providerContextProfileExplicitFields(values map[string]string) providerContextExplicitFields {
	has := func(key string) bool { return strings.TrimSpace(values[key]) != "" }
	return providerContextExplicitFields{
		ContextWindowTokens:     has(llmcommon.EnvContextWindow),
		MaxInputTokens:          has(llmcommon.EnvMaxInputTokens),
		ReservedOutputTokens:    has(llmcommon.EnvReservedOutput),
		BytesHardLimit:          has(llmcommon.EnvBytesHardLimit),
		MaxRequestBytes:         has(llmcommon.EnvMaxRequestBytes),
		ContextUtilizationRatio: has(llmcommon.EnvContextUtilization),
	}
}

func resolveProviderContextProfile(profile llmmodels.ProviderContextProfile, explicit providerContextExplicitFields) llmmodels.ProviderContextProfile {
	profile.ProfileMode = normalizedContextProfileMode(profile.ProfileMode)
	registryResolved := profile.ProfileMode == contextProfileModeAuto &&
		strings.TrimSpace(profile.CapabilitySource) == contextCapabilitySourceModelRegistry &&
		explicit.ContextWindowTokens
	if profile.ProfileMode == contextProfileModeAuto {
		resetAutoContextBudgetOverrides(&profile)
	}
	if profile.ContextUtilizationRatio <= 0 || profile.ContextUtilizationRatio > 1 {
		profile.ContextUtilizationRatio = defaultContextUtilizationRatio
	}
	if profile.MaxRequestBytes <= 0 {
		profile.MaxRequestBytes = defaultContextMaxRequestBytes
	}
	if profile.EstimatedTokensPerByte <= 0 {
		profile.EstimatedTokensPerByte = defaultEstimatedTokensPerByte
	}

	switch {
	case registryResolved:
		profile.CapabilitySource = contextCapabilitySourceModelRegistry
	case explicit.ContextWindowTokens || (profile.ProfileMode == contextProfileModeManual && anyExplicitContextProfileField(explicit)):
		profile.CapabilitySource = contextCapabilitySourceExplicit
	default:
		profile.CapabilitySource = contextCapabilitySourceConservativeDefault
	}

	profile = normalizedProviderContextProfile(profile)
	if profile.ProfileMode == contextProfileModeAuto && shouldAutoDeriveContextBudget(explicit, registryResolved) {
		profile.ReservedOutputTokens = autoReservedOutputTokens(profile.ContextWindowTokens)
		inputWindow := profile.ContextWindowTokens - profile.ReservedOutputTokens
		if inputWindow <= 0 {
			inputWindow = profile.ContextWindowTokens
		}
		profile.MaxInputTokens = int(float64(inputWindow) * profile.ContextUtilizationRatio)
		if profile.MaxInputTokens <= 0 {
			profile.MaxInputTokens = inputWindow
		}
		if profile.MaxInputTokens > inputWindow {
			profile.MaxInputTokens = inputWindow
		}
		profile.BytesHardLimit = autoBytesHardLimit(profile.MaxInputTokens, profile.EstimatedTokensPerByte, profile.MaxRequestBytes)
	}
	return normalizedProviderContextProfile(profile)
}

func resetAutoContextBudgetOverrides(profile *llmmodels.ProviderContextProfile) {
	profile.MaxInputTokens = 0
	profile.ReservedOutputTokens = 0
	profile.SafetyRatio = 0
	profile.ContextUtilizationRatio = 0
	profile.BytesHardLimit = 0
	profile.MaxRequestBytes = 0
	profile.EstimatedTokensPerByte = 0
}

func shouldAutoDeriveContextBudget(explicit providerContextExplicitFields, registryResolved bool) bool {
	return registryResolved || explicit.ContextWindowTokens
}

func normalizedProviderContextProfile(profile llmmodels.ProviderContextProfile) llmmodels.ProviderContextProfile {
	profile.ProfileMode = normalizedContextProfileMode(profile.ProfileMode)
	if profile.ContextWindowTokens <= 0 {
		profile.ContextWindowTokens = defaultContextWindowTokens
	}
	if profile.ReservedOutputTokens <= 0 {
		profile.ReservedOutputTokens = defaultReservedOutputTokens
	}
	if profile.ReservedOutputTokens >= profile.ContextWindowTokens {
		profile.ReservedOutputTokens = max(1, profile.ContextWindowTokens/8)
	}
	maxInputByWindow := profile.ContextWindowTokens - profile.ReservedOutputTokens
	if maxInputByWindow <= 0 {
		maxInputByWindow = profile.ContextWindowTokens
	}
	if profile.MaxInputTokens <= 0 || profile.MaxInputTokens > maxInputByWindow {
		profile.MaxInputTokens = maxInputByWindow
	}
	if profile.SafetyRatio <= 0 || profile.SafetyRatio > 1 {
		profile.SafetyRatio = defaultContextSafetyRatio
	}
	if profile.ContextUtilizationRatio <= 0 || profile.ContextUtilizationRatio > 1 {
		profile.ContextUtilizationRatio = defaultContextUtilizationRatio
	}
	if profile.MaxRequestBytes <= 0 {
		profile.MaxRequestBytes = defaultContextMaxRequestBytes
	}
	if profile.BytesHardLimit <= 0 {
		profile.BytesHardLimit = defaultContextBytesHardLimit
	}
	if profile.EstimatedTokensPerByte <= 0 {
		profile.EstimatedTokensPerByte = defaultEstimatedTokensPerByte
	}
	if strings.TrimSpace(profile.CapabilitySource) == "" {
		profile.CapabilitySource = contextCapabilitySourceConservativeDefault
	}
	return profile
}

func normalizedContextProfileMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case contextProfileModeManual:
		return contextProfileModeManual
	default:
		return contextProfileModeAuto
	}
}

func anyExplicitContextProfileField(explicit providerContextExplicitFields) bool {
	return explicit.ContextWindowTokens ||
		explicit.MaxInputTokens ||
		explicit.ReservedOutputTokens ||
		explicit.BytesHardLimit ||
		explicit.MaxRequestBytes ||
		explicit.ContextUtilizationRatio
}

func autoReservedOutputTokens(contextWindowTokens int) int {
	if contextWindowTokens <= 0 {
		return defaultReservedOutputTokens
	}
	reserved := contextWindowTokens / 16
	if reserved < defaultReservedOutputTokens {
		reserved = defaultReservedOutputTokens
	}
	if reserved > 65_536 {
		reserved = 65_536
	}
	if reserved >= contextWindowTokens {
		reserved = max(1, contextWindowTokens/8)
	}
	return reserved
}

func autoBytesHardLimit(maxInputTokens int, tokensPerByte float64, maxRequestBytes int) int {
	if tokensPerByte <= 0 {
		tokensPerByte = defaultEstimatedTokensPerByte
	}
	if maxRequestBytes <= 0 {
		maxRequestBytes = defaultContextMaxRequestBytes
	}
	if maxInputTokens <= 0 {
		return min(defaultContextBytesHardLimit, maxRequestBytes)
	}
	estimated := int(float64(maxInputTokens) / tokensPerByte)
	if estimated <= 0 {
		estimated = defaultContextBytesHardLimit
	}
	if estimated > maxRequestBytes {
		return maxRequestBytes
	}
	return estimated
}

func parseOptionalPositiveInt(raw, key string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, key)
	}
	return parsed, nil
}

func parseOptionalPositiveFloat(raw, key string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, key)
	}
	return parsed, nil
}

func parseOptionalNonNegativeFloat(raw, key string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, key)
	}
	return parsed, nil
}
