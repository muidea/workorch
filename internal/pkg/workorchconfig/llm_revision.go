package workorchconfig

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"sort"
	"strconv"
	"strings"
)

// CloneLLMConfig returns an independent copy of the LLM configuration. The
// LLM owner keeps this value for its process lifetime, so caller-owned maps
// and slices must not remain shared with the active snapshot.
func CloneLLMConfig(cfg LLMConfig) LLMConfig {
	ret := cfg
	if cfg.DefaultEmbedding != nil {
		value := *cfg.DefaultEmbedding
		ret.DefaultEmbedding = &value
	}
	ret.AIProxy.ModelOverrides = make(map[string]AIProxyModelOverrideConfig, len(cfg.AIProxy.ModelOverrides))
	for modelID, override := range cfg.AIProxy.ModelOverrides {
		override.Operations = append([]string(nil), override.Operations...)
		ret.AIProxy.ModelOverrides[modelID] = override
	}
	return ret
}

// LLMConfigurationRevision is a secret-safe identity for the complete LLM
// configuration consumed by the active LLM owner. It deliberately includes
// defaults, catalog policy, model overrides, and budget policy so any saved
// LLM change is staged for the next process start.
func LLMConfigurationRevision(input LLMConfig) (string, error) {
	cfg, err := NormalizeLLMConfig(CloneLLMConfig(input))
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writeLLMRevisionString(h, "workorch/llm-configuration-revision/v1")
	writeLLMRevisionString(h, strings.TrimSpace(cfg.AIProxy.BaseURL))
	writeLLMRevisionString(h, llmRevisionSecretFingerprint(cfg.AIProxy.APIKey))
	writeLLMRevisionString(h, strings.TrimSpace(cfg.AIProxy.Timeout))
	writeLLMRevisionString(h, strings.TrimSpace(cfg.AIProxy.CatalogRequestTimeout))
	writeLLMRevisionString(h, strings.TrimSpace(cfg.AIProxy.CatalogTTL))
	writeLLMRevisionString(h, strings.TrimSpace(cfg.AIProxy.CatalogMaxStaleAge))
	_ = binary.Write(h, binary.BigEndian, int64(cfg.AIProxy.MaxCatalogResponseBytes))
	_ = binary.Write(h, binary.BigEndian, int64(cfg.AIProxy.MaxCatalogModels))

	modelIDs := make([]string, 0, len(cfg.AIProxy.ModelOverrides))
	for modelID := range cfg.AIProxy.ModelOverrides {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)
	_ = binary.Write(h, binary.BigEndian, int64(len(modelIDs)))
	for _, modelID := range modelIDs {
		override := cfg.AIProxy.ModelOverrides[modelID]
		writeLLMRevisionString(h, modelID)
		_ = binary.Write(h, binary.BigEndian, int64(override.ContextWindowTokens))
		_ = binary.Write(h, binary.BigEndian, int64(override.MaxOutputTokens))
		operations := append([]string(nil), override.Operations...)
		sort.Strings(operations)
		_ = binary.Write(h, binary.BigEndian, int64(len(operations)))
		for _, operation := range operations {
			writeLLMRevisionString(h, operation)
		}
	}

	defaultChat := cfg.DefaultChat.Normalized()
	writeLLMRevisionString(h, defaultChat.ProviderID)
	writeLLMRevisionString(h, defaultChat.ModelID)
	if cfg.DefaultEmbedding == nil {
		writeLLMRevisionString(h, "default-embedding=none")
	} else {
		defaultEmbedding := cfg.DefaultEmbedding.Normalized()
		writeLLMRevisionString(h, "default-embedding=present")
		writeLLMRevisionString(h, defaultEmbedding.ProviderID)
		writeLLMRevisionString(h, defaultEmbedding.ModelID)
	}
	writeLLMRevisionString(h, strconv.FormatFloat(cfg.Budget.ContextUtilizationRatio, 'g', -1, 64))
	writeLLMRevisionString(h, strconv.FormatFloat(cfg.Budget.EstimatedTokensPerByte, 'g', -1, 64))
	return "llm-config-sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func llmRevisionSecretFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	sum := sha256.Sum256([]byte("workorch/llm-configuration-revision/api-key\x00" + value))
	return hex.EncodeToString(sum[:])
}

func writeLLMRevisionString(w io.Writer, value string) {
	_ = binary.Write(w, binary.BigEndian, int64(len(value)))
	_, _ = io.WriteString(w, value)
}
