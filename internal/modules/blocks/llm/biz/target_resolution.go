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
// operation must be responses or embeddings; empty defaults to responses.
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
	if target.ProviderID != runtimecontract.BuiltinAIProxyProviderID {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureProviderNotFound,
			Message: "WorkOrch only supports the ai-proxy provider",
			Target:  target,
		}
	}
	return resolveAIProxyTarget(cfg, catalog, target, operation, configRevision)
}

func resolveAIProxyTarget(cfg workorchconfig.LLMConfig, catalog *llmmodels.ProviderModelCatalog, target runtimecontract.ProviderModelTarget, operation string, configRevision string) (llmmodels.ResolvedProviderModelProfile, error) {
	if operation == workorchconfig.LLMOperationEmbeddings {
		if catalog == nil {
			return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{Code: llmmodels.ProviderModelFailureCatalogUnavailable, Message: "ai-proxy catalog is unavailable", Target: target}
		}
		for _, model := range catalog.Models {
			if !model.Target.Same(target) {
				continue
			}
			if !containsOperation(model.Operations, workorchconfig.LLMOperationEmbeddings) {
				return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{Code: llmmodels.ProviderModelFailureOperationUnsupported, Message: fmt.Sprintf("model %q does not support operation %q", target.ModelID, operation), Target: target}
			}
			if !model.CapabilityReady {
				return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{Code: llmmodels.ProviderModelFailureCapabilityIncomplete, Message: firstNonEmpty(model.UnavailableReason, "embedding model capacity is incomplete"), Target: target}
			}
			return deriveEmbeddingBudgetProfile(target, operation, llmcommon.ProviderSurfaceOpenAICompatibleHTTP, model.ContextWindowTokens, cfg.Budget, llmmodels.CapabilitySourceAIProxyCatalog, configRevision, model.CapabilityRevision)
		}
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{Code: llmmodels.ProviderModelFailureModelNotFound, Message: fmt.Sprintf("model %q was not found in ai-proxy catalog", target.ModelID), Target: target}
	}
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
		profile, err := deriveBudgetProfile(target, operation, llmcommon.ProviderSurfaceOpenAICompatibleHTTP, override.ContextWindowTokens, override.MaxOutputTokens, cfg.Budget, llmmodels.CapabilitySourceAIProxyExactOverride, configRevision, overrideRevision(override))
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
		return deriveBudgetProfile(target, operation, llmcommon.ProviderSurfaceOpenAICompatibleHTTP, model.ContextWindowTokens, model.MaxOutputTokens, cfg.Budget, llmmodels.CapabilitySourceAIProxyCatalog, configRevision, model.CapabilityRevision)
	}
	return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
		Code:    llmmodels.ProviderModelFailureModelNotFound,
		Message: fmt.Sprintf("model %q was not found in ai-proxy catalog", target.ModelID),
		Target:  target,
	}
}

func deriveEmbeddingBudgetProfile(
	target runtimecontract.ProviderModelTarget,
	operation string,
	apiSurface string,
	contextWindowTokens int,
	budget workorchconfig.LLMContextBudgetPolicyConfig,
	capabilitySource string,
	configurationRevision string,
	existingCapabilityRevision string,
) (llmmodels.ResolvedProviderModelProfile, error) {
	if contextWindowTokens <= 0 || budget.ContextUtilizationRatio <= 0 || budget.ContextUtilizationRatio > 1 || budget.EstimatedTokensPerByte <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code: llmmodels.ProviderModelFailureCapabilityIncomplete, Message: "embedding context capacity is incomplete", Target: target,
		}
	}
	maxInputTokens := int(math.Floor(float64(contextWindowTokens) * budget.ContextUtilizationRatio))
	maxBytesFloat := math.Floor(float64(maxInputTokens) / budget.EstimatedTokensPerByte)
	if maxInputTokens <= 0 || maxBytesFloat <= 0 || maxBytesFloat > float64(math.MaxInt) {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code: llmmodels.ProviderModelFailureCapabilityIncomplete, Message: "embedding byte capacity is invalid", Target: target,
		}
	}
	capabilityRev := existingCapabilityRevision
	if capabilityRev == "" {
		capabilityRev = capabilityRevision(target, contextWindowTokens, 0, capabilitySource+"|"+operation)
	}
	return llmmodels.ResolvedProviderModelProfile{
		Target: target, Operation: operation, APISurface: apiSurface,
		ContextWindowTokens: contextWindowTokens, MaxInputTokens: maxInputTokens,
		EstimatedTokensPerByte: budget.EstimatedTokensPerByte, MaxRequestBytes: int(maxBytesFloat),
		CapabilitySource: capabilitySource, CapabilityRevision: capabilityRev, ConfigurationRevision: configurationRevision,
	}, nil
}

func deriveBudgetProfile(
	target runtimecontract.ProviderModelTarget,
	operation string,
	apiSurface string,
	contextWindowTokens int,
	providerMaxOutputTokens int,
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
	if budget.ContextUtilizationRatio <= 0 || budget.ContextUtilizationRatio > 1 || budget.EstimatedTokensPerByte <= 0 {
		return llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureConfigurationInvalid,
			Message: "budget policy is invalid",
			Target:  target,
		}
	}
	// The selected provider/model capability is the single source of truth for
	// the response ceiling. An explicit stop at this boundary may authorize
	// Runtime whole-document recovery; it never authorizes partial continuation.
	effectiveMaxOutputTokens := minPositive(providerMaxOutputTokens, contextWindowTokens-1)
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
	// Providers describe model capacity in tokens, not serialized HTTP bytes.
	// Derive the request-byte ceiling from the selected model's input budget so
	// a process-wide fixed cap cannot silently shrink a large-context model.
	maxBytes := int(math.Floor(float64(maxInputTokens) / budget.EstimatedTokensPerByte))
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
		Selection:                runtimecontract.ProviderInferenceSelection{Target: profile.Target},
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
	if target.ProviderID != runtimecontract.BuiltinAIProxyProviderID {
		return llmmodels.ProviderConfig{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureProviderNotFound,
			Message: "WorkOrch only supports the ai-proxy provider",
			Target:  target,
		}
	}
	timeout, err := parseProviderTimeout(cfg.AIProxy.Timeout)
	if err != nil {
		return llmmodels.ProviderConfig{}, err
	}
	return withAvailability(llmmodels.ProviderConfig{
		ID:          target.ProviderID,
		APISurface:  llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
		BaseURL:     strings.TrimSpace(cfg.AIProxy.BaseURL),
		APIKey:      strings.TrimSpace(cfg.AIProxy.APIKey),
		Model:       target.ModelID,
		Timeout:     timeout,
		HardTimeout: workorchconfig.MaxAIProxyTimeout,
		Capabilities: llmmodels.ProviderCapabilities{
			Responses: true,
			Streaming: true,
			// ai-proxy's Responses integration supports the base text
			// contract only. It does not advertise JSON Schema response
			// formatting to upstream providers.
			JSONSchemaResponse: false,
			NonChatOperations:  []string{workorchconfig.LLMOperationEmbeddings},
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
		Target:                   ceiling.Selection.Target,
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
	currentRevision := TargetConfigurationRevision(cfg, ceiling.Selection.Target)
	if currentRevision != ceiling.ConfigurationRevision {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, llmmodels.ProviderModelFailure{
			Code:    llmmodels.ProviderModelFailureConfigurationDrift,
			Message: "sealed target configuration revision drifted; create a new run to use the updated provider configuration",
			Target:  ceiling.Selection.Target,
		}
	}
	profile, err := ProfileFromProviderCeiling(ceiling)
	if err != nil {
		return llmmodels.ProviderConfig{}, llmmodels.ResolvedProviderModelProfile{}, err
	}
	// Preserve sealed budget fields; ResolveProviderClient may fill surface-specific client details.
	client, err := ResolveProviderClient(cfg, ceiling.Selection.Target, profile)
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
		return workorchconfig.LLMOperationResponses
	}
	// Strict: only canonical operation names; aliases are not accepted.
	return operation
}
