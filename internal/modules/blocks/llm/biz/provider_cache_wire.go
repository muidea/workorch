package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/muidea/workorch/internal/pkg/llmaudit"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// Hash the actual adapter input through its last requested breakpoint, including
// non-message configuration that may change the model's hidden prefix. This is
// local wire evidence only: it cannot assert forwarding, a hit or routing.
func observeResponsesCacheWire(budget *models.ProviderContextBudget, wire responsesRequest) {
	if budget == nil {
		return
	}
	budget.WireSchemaHash, budget.WireCachePrefixHash, budget.WireCacheMode, budget.WireCacheBreakpoints = "", "", "", 0
	budget.WireCachePrefixes = nil
	if wire.PromptCacheOptions == nil {
		return
	}
	budget.WireCacheMode = wire.PromptCacheOptions.Mode
	var messages []responsesInputMessage
	if json.Unmarshal(wire.Input, &messages) != nil {
		return
	}
	hash := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		sum := sha256.Sum256(raw)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	if wire.Text != nil && wire.Text.Format != nil {
		budget.WireSchemaHash = hash(wire.Text.Format)
	}
	cachePrefix := func(prefix []responsesInputMessage) any {
		return struct {
			Model     string
			Reasoning *responsesReasoning
			Text      *responsesTextConfig
			Key       string
			Options   *responsesCacheOptions
			Messages  []responsesInputMessage
		}{wire.Model, wire.Reasoning, wire.Text, wire.PromptCacheKey, wire.PromptCacheOptions, prefix}
	}
	for i, message := range messages {
		for j, block := range message.Content {
			if block.PromptCacheBreakpoint == nil {
				continue
			}
			p := append([]responsesInputMessage(nil), messages[:i+1]...)
			p[i].Content = p[i].Content[:j+1]
			raw, _ := json.Marshal(cachePrefix(p))
			sum := sha256.Sum256(raw)
			budget.WireCachePrefixHash = "sha256:" + hex.EncodeToString(sum[:])
			budget.WireCacheBreakpoints++
			budget.WireCachePrefixes = append(budget.WireCachePrefixes, llmaudit.CachePrefixObservation{MessageIndex: i, BlockIndex: j, WireBytes: len(raw), Hash: budget.WireCachePrefixHash})
		}
	}
}
