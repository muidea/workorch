package llm

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	apiKeyFingerprintDomainCatalog = "workorch/llm/catalog-api-key"
	apiKeyFingerprintDomainTarget  = "workorch/llm/target-api-key"
)

// CatalogConnectionRevision identifies the non-secret catalog connection identity.
// It is used as the catalog cache key together with providerID + baseURL.
func CatalogConnectionRevision(cfg workorchconfig.LLMConfig) string {
	h := sha256.New()
	writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.BaseURL))
	writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.Timeout))
	writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.CatalogRequestTimeout))
	writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.CatalogTTL))
	writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.CatalogMaxStaleAge))
	_ = binary.Write(h, binary.BigEndian, int64(cfg.AIProxy.MaxCatalogResponseBytes))
	_ = binary.Write(h, binary.BigEndian, int64(cfg.AIProxy.MaxCatalogModels))
	writeLenPrefixed(h, apiKeyFingerprint(apiKeyFingerprintDomainCatalog, cfg.AIProxy.APIKey))
	// Exact model overrides are target-specific authority and must NOT affect catalog connection identity.
	return "catalog-connection-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// TargetConfigurationRevision is the complete sealed configuration identity for one target.
// Changing endpoint, secret, budget, override, or custom model parameters must change this revision.
func TargetConfigurationRevision(cfg workorchconfig.LLMConfig, target runtimecontract.ProviderModelTarget) string {
	target = target.Normalized()
	h := sha256.New()
	writeLenPrefixed(h, target.ProviderID)
	writeLenPrefixed(h, target.ModelID)
	writeLenPrefixed(h, fmt.Sprintf("%.8f", cfg.Budget.ContextUtilizationRatio))
	writeLenPrefixed(h, fmt.Sprintf("%.8f", cfg.Budget.EstimatedTokensPerByte))

	if target.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
		writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.BaseURL))
		writeLenPrefixed(h, strings.TrimSpace(cfg.AIProxy.Timeout))
		writeLenPrefixed(h, apiKeyFingerprint(apiKeyFingerprintDomainTarget, cfg.AIProxy.APIKey))
		if override, ok := cfg.AIProxy.ModelOverrides[target.ModelID]; ok {
			_ = binary.Write(h, binary.BigEndian, int64(override.ContextWindowTokens))
			_ = binary.Write(h, binary.BigEndian, int64(override.MaxOutputTokens))
			ops := append([]string(nil), override.Operations...)
			sort.Strings(ops)
			for _, op := range ops {
				writeLenPrefixed(h, strings.TrimSpace(op))
			}
			writeLenPrefixed(h, "override=1")
		} else {
			writeLenPrefixed(h, "override=0")
		}
	} else if provider, ok := cfg.CustomProviders[target.ProviderID]; ok {
		writeLenPrefixed(h, strings.TrimSpace(provider.Provider))
		writeLenPrefixed(h, strings.TrimSpace(provider.APISurface))
		writeLenPrefixed(h, strings.TrimSpace(provider.BaseURL))
		writeLenPrefixed(h, strings.TrimSpace(provider.Timeout))
		writeLenPrefixed(h, apiKeyFingerprint(apiKeyFingerprintDomainTarget, provider.APIKey))
		if model, ok := provider.Models[target.ModelID]; ok {
			_ = binary.Write(h, binary.BigEndian, int64(model.ContextWindowTokens))
			_ = binary.Write(h, binary.BigEndian, int64(model.MaxOutputTokens))
			ops := append([]string(nil), model.Operations...)
			sort.Strings(ops)
			for _, op := range ops {
				writeLenPrefixed(h, strings.TrimSpace(op))
			}
			if model.Temperature != nil {
				writeLenPrefixed(h, fmt.Sprintf("temp=%.8f", *model.Temperature))
			}
			_ = binary.Write(h, binary.BigEndian, int64(model.ThinkingBudgetTokens))
			writeLenPrefixed(h, strings.TrimSpace(model.Cache.Mode))
			if model.Cache.PromptCacheKey != nil {
				writeLenPrefixed(h, fmt.Sprintf("promptCache=%t", *model.Cache.PromptCacheKey))
			}
			if model.Cache.MessageCacheControl != nil {
				writeLenPrefixed(h, fmt.Sprintf("msgCache=%t", *model.Cache.MessageCacheControl))
			}
			fields := append([]string(nil), model.Cache.UsageFields...)
			sort.Strings(fields)
			for _, field := range fields {
				writeLenPrefixed(h, field)
			}
		}
	}
	return "target-config-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func apiKeyFingerprint(domain string, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "apiKeyFingerprint=none"
	}
	sum := sha256.Sum256([]byte(domain + "\x00" + apiKey))
	return "apiKeyFingerprint=" + hex.EncodeToString(sum[:])
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Re-export writeLenPrefixed for package-local use if catalog_service defines its own.
// Keep a local alias to avoid circular helper issues; catalog_service already has writeLenPrefixed.
