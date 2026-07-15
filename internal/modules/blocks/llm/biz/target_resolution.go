package llm

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ResolveProviderModelTarget resolves an exact target into a complete profile.
// No pattern matching, no conservative defaults, no map fallback.
// operation must be chat_completions or embeddings; empty defaults to chat_completions.
func ResolveProviderModelTarget(cfg workorchconfig.LLMConfig, catalog *llmmodels.ProviderModelCatalog, target runtimecontract.ProviderModelTarget, operation string) (llmmodels.ResolvedProviderModelProfile, error) {
	cfg, err := workorchconfig.NormalizeLLMConfig(cfg)
	if err != nil {
		return llmmodels.ResolvedProviderModelProfile{}, err
	}
	target = target.Normalized()
	if err := target.Validate(); err != nil {
		return llmmodels.ResolvedProviderModelProfile{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	operation = normalizeRequestedOperation(operation)
	configRevision := TargetConfigurationRevision(cfg, target)

	if target.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
		return resolveAIProxyTarget(cfg, catalog, target, operation, configRevision)
	}
	return resolveCustomProviderTarget(cfg, target, operation, configRevision)
}

func resolveAIProxyTarget(cfg workorchconfig.LLMConfig, catalog *llmmodels.ProviderModelCatalog, target runtimecontract.ProviderModelTarget, operation string, configRevision string) (llmmodels.ResolvedProviderModelProfile, error) {
	if override, ok := cfg.AIProxy.ModelOverrides[target.ModelID]; ok {
		if override.ContextWindowTokens <= 0 || override.MaxOutputTokens <= 0 || len(override.Operations) == 0 {
			return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
				Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
				Message: "ai-proxy exact override is incomplete",
				Target:  target,
			}
		}
		// Override may correct capacity only; operations remain authoritative and must include requested op.
		if !containsOperation(override.Operations, operation) {
			return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
				Code:    llmmodels.ProviderModelFailureOperationUnsupported,
				Message: fmt.Sprintf("model %q override does not support operation %q", target.ModelID, operation),
				Target:  target,
			}
		}
		// When catalog is present, override cannot invent operations the catalog model does not declare.
		if catalog != nil {
			for _, model := range catalog.Models {
				if !model.Target.Same(target) {
					continue
				}
				if len(model.Operations) > 0 && !containsOperation(model.Operations, operation) {
					return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
						Code:    llmmodels.ProviderModelFailureOperationUnsupported,
						Message: fmt.Sprintf("model %q does not support operation %q", target.ModelID, operation),
						Target:  target,
					}
				}
				break
			}
		}
		profile, err := deriveBudgetProfile(target, operation, llmcommon.ProviderSurfaceOpenAICompatibleHTTP, override.ContextWindowTokens, override.MaxOutputTokens, 0, cfg.Budget, llmmodels.CapabilitySourceAIProxyExactOverride, configRevision, overrideRevision(override))
		return profile, err
	}
	if catalog == nil {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCatalogUnavailable,
			Message: "ai-proxy catalog is unavailable",
			Target:  target,
		}
	}
	for _, model := range catalog.Models {
		if !model.Target.Same(target) {
			continue
		}
		if !model.CapabilityReady {
			return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
				Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
				Message: firstNonEmpty(model.UnavailableReason, "model capability is incomplete"),
				Target:  target,
			}
		}
		if !containsOperation(model.Operations, operation) {
			return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
				Code:    llmmodels.ProviderModelFailureOperationUnsupported,
				Message: fmt.Sprintf("model %q does not support operation %q", target.ModelID, operation),
				Target:  target,
			}
		}
		return deriveBudgetProfile(target, operation, llmcommon.ProviderSurfaceOpenAICompatibleHTTP, model.ContextWindowTokens, model.MaxOutputTokens, 0, cfg.Budget, llmmodels.CapabilitySourceAIProxyCatalog, configRevision, model.CapabilityRevision)
	}
	return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
		Code:    llmmodels.ProviderModelFailureModelNotFound,
		Message: fmt.Sprintf("model %q was not found in ai-proxy catalog", target.ModelID),
		Target:  target,
	}
}

func resolveCustomProviderTarget(cfg workorchconfig.LLMConfig, target runtimecontract.ProviderModelTarget, operation string, configRevision string) (llmmodels.ResolvedProviderModelProfile, error) {
	// Exact map lookup only. NormalizeLLMConfig rejects trim/case-fold duplicates and
	// rewrites keys to trimmed form; readiness/client must not reintroduce fuzzy lookup.
	provider, ok := cfg.CustomProviders[target.ProviderID]
	if !ok {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureProviderNotFound,
			Message: fmt.Sprintf("custom provider %q was not found", target.ProviderID),
			Target:  target,
		}
	}
	model, ok := provider.Models[target.ModelID]
	if !ok {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureModelNotFound,
			Message: fmt.Sprintf("model %q was not found on custom provider %q", target.ModelID, target.ProviderID),
			Target:  target,
		}
	}
	if model.ContextWindowTokens <= 0 || model.MaxOutputTokens <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
			Message: "custom provider model capability is incomplete",
			Target:  target,
		}
	}
	if !containsOperation(model.Operations, operation) {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureOperationUnsupported,
			Message: fmt.Sprintf("custom model %q does not support operation %q", target.ModelID, operation),
			Target:  target,
		}
	}
	apiSurface := strings.TrimSpace(provider.APISurface)
	if apiSurface == "" {
		apiSurface = defaultProviderAPISurface(provider.Provider)
	}
	return deriveBudgetProfile(target, operation, apiSurface, model.ContextWindowTokens, model.MaxOutputTokens, model.MaxOutputTokens, cfg.Budget, llmmodels.CapabilitySourceCustomProviderExplicit, configRevision, "")
}

func deriveBudgetProfile(
	target runtimecontract.ProviderModelTarget,
	operation string,
	apiSurface string,
	contextWindowTokens int,
	providerMaxOutputTokens int,
	modelConfiguredMaxOutput int,
	budget workorchconfig.LLMContextBudgetPolicyConfig,
	capabilitySource string,
	configurationRevision string,
	existingCapabilityRevision string,
) (llmmodels.ResolvedProviderModelProfile, error) {
	if contextWindowTokens <= 0 || providerMaxOutputTokens <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
			Message: "context window and provider max output must be positive",
			Target:  target,
		}
	}
	if budget.ContextUtilizationRatio <= 0 || budget.ContextUtilizationRatio > 1 || budget.EstimatedTokensPerByte <= 0 || budget.DefaultMaxOutputTokens <= 0 || budget.MaxRequestBytes <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureConfigurationInvalid,
			Message: "budget policy is invalid",
			Target:  target,
		}
	}
	requestedOutputTokens := budget.DefaultMaxOutputTokens
	if modelConfiguredMaxOutput > 0 {
		requestedOutputTokens = modelConfiguredMaxOutput
	}
	effectiveMaxOutputTokens := minPositive(requestedOutputTokens, providerMaxOutputTokens, contextWindowTokens-1)
	if effectiveMaxOutputTokens <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
			Message: "effective max output tokens is not positive",
			Target:  target,
		}
	}
	inputWindowTokens := contextWindowTokens - effectiveMaxOutputTokens
	maxInputTokens := int(math.Floor(float64(inputWindowTokens) * budget.ContextUtilizationRatio))
	if maxInputTokens <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
			Message: "max input tokens is not positive",
			Target:  target,
		}
	}
	estimatedInputBytes := int(math.Floor(float64(maxInputTokens) / budget.EstimatedTokensPerByte))
	maxBytes := estimatedInputBytes
	if budget.MaxRequestBytes < maxBytes {
		maxBytes = budget.MaxRequestBytes
	}
	if maxBytes <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureCapabilityIncomplete,
			Message: "max request bytes is not positive",
			Target:  target,
		}
	}
	capabilityRev := existingCapabilityRevision
	if capabilityRev == "" {
		capabilityRev = capabilityRevision(target, contextWindowTokens, providerMaxOutputTokens, capabilitySource+"|"+operation)
	}
	return llmmodels.ResolvedProviderModelProfile{
		Target:                   target,
		Operation:                operation,
		APISurface:               apiSurface,
		ContextWindowTokens:      contextWindowTokens,
		ProviderMaxOutputTokens:  providerMaxOutputTokens,
		EffectiveMaxOutputTokens: effectiveMaxOutputTokens,
		ReservedOutputTokens:     effectiveMaxOutputTokens,
		MaxInputTokens:           maxInputTokens,
		EstimatedTokensPerByte:   budget.EstimatedTokensPerByte,
		MaxRequestBytes:          maxBytes,
		CapabilitySource:         capabilitySource,
		CapabilityRevision:       capabilityRev,
		ConfigurationRevision:    configurationRevision,
	}, nil
}

func ProfileToProviderCeiling(profile llmmodels.ResolvedProviderModelProfile) runtimecontract.ProviderCeiling {
	return runtimecontract.ProviderCeiling{
		Target:                   profile.Target,
		ContextWindowTokens:      profile.ContextWindowTokens,
		ProviderMaxOutputTokens:  profile.ProviderMaxOutputTokens,
		EffectiveMaxOutputTokens: profile.EffectiveMaxOutputTokens,
		MaxInputTokens:           profile.MaxInputTokens,
		MaxBytes:                 profile.MaxRequestBytes,
		CapabilityRevision:       profile.CapabilityRevision,
		ConfigurationRevision:    profile.ConfigurationRevision,
	}
}

// ResolveProviderClient builds a concrete provider client config from an explicit target.
func ResolveProviderClient(cfg workorchconfig.LLMConfig, target runtimecontract.ProviderModelTarget, profile llmmodels.ResolvedProviderModelProfile) (llmmodels.ProviderConfig, error) {
	target = target.Normalized()
	if err := target.Validate(); err != nil {
		return llmmodels.ProviderConfig{}, err
	}
	if !profile.Target.Same(target) {
		return llmmodels.ProviderConfig{}, fmt.Errorf("%w: resolved profile target mismatch", common.ErrInvalidArgument)
	}
	if target.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
		timeout, err := parseProviderTimeout(cfg.AIProxy.Timeout)
		if err != nil {
			return llmmodels.ProviderConfig{}, err
		}
		return withAvailability(llmmodels.ProviderConfig{
			ID:         target.ProviderID,
			Provider:   llmcommon.ProviderCompatible,
			APISurface: llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			BaseURL:    strings.TrimSpace(cfg.AIProxy.BaseURL),
			APIKey:     strings.TrimSpace(cfg.AIProxy.APIKey),
			Model:      target.ModelID,
			Timeout:    timeout,
			Capabilities: llmmodels.ProviderCapabilities{
				ChatCompletions:   true,
				NonChatOperations: []string{workorchconfig.LLMOperationEmbeddings},
			},
			InferenceProfile: llmmodels.ProviderInferenceProfile{
				MaxOutputTokens: profile.EffectiveMaxOutputTokens,
			},
			ContextProfile: llmmodels.ProviderContextProfile{
				CapabilitySource:        profile.CapabilitySource,
				ContextWindowTokens:     profile.ContextWindowTokens,
				MaxInputTokens:          profile.MaxInputTokens,
				ReservedOutputTokens:    profile.EffectiveMaxOutputTokens,
				ContextUtilizationRatio: cfg.Budget.ContextUtilizationRatio,
				MaxRequestBytes:         profile.MaxRequestBytes,
				EstimatedTokensPerByte:  profile.EstimatedTokensPerByte,
			},
		}), nil
	}
	provider, ok := cfg.CustomProviders[target.ProviderID]
	if !ok {
		return llmmodels.ProviderConfig{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureProviderNotFound,
			Message: fmt.Sprintf("custom provider %q was not found", target.ProviderID),
			Target:  target,
		}
	}
	model, ok := provider.Models[target.ModelID]
	if !ok {
		return llmmodels.ProviderConfig{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureModelNotFound,
			Message: fmt.Sprintf("model %q was not found on custom provider %q", target.ModelID, target.ProviderID),
			Target:  target,
		}
	}
	timeout, err := parseProviderTimeout(provider.Timeout)
	if err != nil {
		return llmmodels.ProviderConfig{}, err
	}
	apiSurface := strings.TrimSpace(provider.APISurface)
	if apiSurface == "" {
		apiSurface = defaultProviderAPISurface(provider.Provider)
	}
	return withAvailability(llmmodels.ProviderConfig{
		ID:         target.ProviderID,
		Provider:   strings.TrimSpace(provider.Provider),
		APISurface: apiSurface,
		BaseURL:    strings.TrimSpace(provider.BaseURL),
		APIKey:     strings.TrimSpace(provider.APIKey),
		Model:      target.ModelID,
		Timeout:    timeout,
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions:   containsOperation(model.Operations, workorchconfig.LLMOperationChatCompletions),
			NonChatOperations: nonChatOperations(model.Operations),
		},
		InferenceProfile: llmmodels.ProviderInferenceProfile{
			Temperature:          model.Temperature,
			MaxOutputTokens:      profile.EffectiveMaxOutputTokens,
			ReasoningEffort:      model.ReasoningEffort,
			ThinkingBudgetTokens: model.ThinkingBudgetTokens,
		},
		ContextProfile: llmmodels.ProviderContextProfile{
			CapabilitySource:        profile.CapabilitySource,
			ContextWindowTokens:     profile.ContextWindowTokens,
			MaxInputTokens:          profile.MaxInputTokens,
			ReservedOutputTokens:    profile.EffectiveMaxOutputTokens,
			ContextUtilizationRatio: cfg.Budget.ContextUtilizationRatio,
			MaxRequestBytes:         profile.MaxRequestBytes,
			EstimatedTokensPerByte:  profile.EstimatedTokensPerByte,
		},
		CacheCapability: llmmodels.ProviderCacheCapability{
			Surface:             apiSurface,
			Mode:                model.Cache.Mode,
			PromptCacheKey:      boolValue(model.Cache.PromptCacheKey),
			MessageCacheControl: boolValue(model.Cache.MessageCacheControl),
			UsageFields:         append([]string(nil), model.Cache.UsageFields...),
		},
	}), nil
}

func parseProviderTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 180 * time.Second, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%w: invalid provider timeout", common.ErrInvalidArgument)
	}
	return value, nil
}

func overrideRevision(override workorchconfig.AIProxyModelOverrideConfig) string {
	ops := append([]string(nil), override.Operations...)
	sort.Strings(ops)
	return capabilityRevision(runtimecontract.ProviderModelTarget{}, override.ContextWindowTokens, override.MaxOutputTokens, "override|"+strings.Join(ops, ","))
}

func minPositive(values ...int) int {
	result := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if result == 0 || value < result {
			result = value
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsOperation(operations []string, want string) bool {
	for _, operation := range operations {
		if strings.TrimSpace(operation) == want {
			return true
		}
	}
	return false
}

func nonChatOperations(operations []string) []string {
	ret := make([]string, 0, len(operations))
	for _, operation := range operations {
		operation = strings.TrimSpace(operation)
		if operation == "" || operation == workorchconfig.LLMOperationChatCompletions {
			continue
		}
		ret = append(ret, operation)
	}
	return ret
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func withAvailability(cfg llmmodels.ProviderConfig) llmmodels.ProviderConfig {
	if cfg.Disabled {
		cfg.Unavailable = true
		cfg.UnavailableReason = "provider is disabled"
		return cfg
	}
	if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.APIKey) == "" && strings.TrimSpace(cfg.Model) == "" {
		cfg.Unavailable = true
		cfg.UnavailableReason = "provider endpoint is incomplete"
	}
	return cfg
}

// ProfileFromProviderCeiling reconstructs a request profile from a sealed ceiling.
// It does not consult catalog or re-derive budget from current config.
func ProfileFromProviderCeiling(ceiling runtimecontract.ProviderCeiling) (llmmodels.ResolvedProviderModelProfile, error) {
	ceiling = ceiling.Normalized()
	if err := ceiling.Validate(); err != nil {
		return llmmodels.ResolvedProviderModelProfile{}, err
	}
	tokensPerByte := 0.25
	if ceiling.MaxInputTokens > 0 && ceiling.MaxBytes > 0 {
		tokensPerByte = float64(ceiling.MaxInputTokens) / float64(ceiling.MaxBytes)
	}
	return llmmodels.ResolvedProviderModelProfile{
		Target:                   ceiling.Target,
		APISurface:               llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
		ContextWindowTokens:      ceiling.ContextWindowTokens,
		ProviderMaxOutputTokens:  ceiling.ProviderMaxOutputTokens,
		EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
		ReservedOutputTokens:     ceiling.EffectiveMaxOutputTokens,
		MaxInputTokens:           ceiling.MaxInputTokens,
		EstimatedTokensPerByte:   tokensPerByte,
		MaxRequestBytes:          ceiling.MaxBytes,
		CapabilitySource:         "sealed_ceiling",
		CapabilityRevision:       ceiling.CapabilityRevision,
		ConfigurationRevision:    ceiling.ConfigurationRevision,
	}, nil
}

// ResolveProviderClientForSealedCeiling builds a provider client for a sealed ceiling.
// Current config supplies endpoint/secret only; sealed configuration revision must match.
func ResolveProviderClientForSealedCeiling(cfg workorchconfig.LLMConfig, ceiling runtimecontract.ProviderCeiling) (llmmodels.ProviderConfig, llmmodels.ResolvedProviderModelProfile, error) {
	cfg, err := workorchconfig.NormalizeLLMConfig(cfg)
	if err != nil {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, err
	}
	ceiling = ceiling.Normalized()
	if err := ceiling.Validate(); err != nil {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, err
	}
	currentRevision := TargetConfigurationRevision(cfg, ceiling.Target)
	if currentRevision != ceiling.ConfigurationRevision {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureConfigurationDrift,
			Message: "sealed target configuration revision drifted; create a new run to use the updated provider configuration",
			Target:  ceiling.Target,
		}
	}
	profile, err := ProfileFromProviderCeiling(ceiling)
	if err != nil {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, err
	}
	// Preserve sealed budget fields; ResolveProviderClient may fill surface-specific client details.
	client, err := ResolveProviderClient(cfg, ceiling.Target, profile)
	if err != nil {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, err
	}
	// Force sealed budget onto client context profile so request assembly cannot expand beyond ceiling.
	client.ContextProfile.ContextWindowTokens = ceiling.ContextWindowTokens
	client.ContextProfile.MaxInputTokens = ceiling.MaxInputTokens
	client.ContextProfile.ReservedOutputTokens = ceiling.EffectiveMaxOutputTokens
	client.ContextProfile.MaxRequestBytes = ceiling.MaxBytes
	client.InferenceProfile.MaxOutputTokens = ceiling.EffectiveMaxOutputTokens
	return client, profile, nil
}

func normalizeRequestedOperation(operation string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return workorchconfig.LLMOperationChatCompletions
	}
	// Strict: only canonical operation names; aliases are not accepted.
	return operation
}
