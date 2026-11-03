package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

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
	if wire.PromptCacheOptions == nil {
		return
	}
	budget.WireCacheMode = wire.PromptCacheOptions.Mode
	var messages []responsesInputMessage
	if json.Unmarshal(wire.Input, &messages) != nil {
		return
	}
	lastMessage, lastBlock := -1, -1
	for i, message := range messages {
		for j, block := range message.Content {
			if block.PromptCacheBreakpoint != nil {
				budget.WireCacheBreakpoints++
				lastMessage, lastBlock = i, j
			}
		}
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
	if lastMessage < 0 {
		return
	}
	prefix := append([]responsesInputMessage(nil), messages[:lastMessage+1]...)
	prefix[lastMessage].Content = prefix[lastMessage].Content[:lastBlock+1]
	budget.WireCachePrefixHash = hash(struct {
		Model     string
		Reasoning *responsesReasoning
		Text      *responsesTextConfig
		Key       string
		Options   *responsesCacheOptions
		Messages  []responsesInputMessage
	}{wire.Model, wire.Reasoning, wire.Text, wire.PromptCacheKey, wire.PromptCacheOptions, prefix})
}
