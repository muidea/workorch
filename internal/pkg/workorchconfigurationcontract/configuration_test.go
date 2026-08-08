package workorchconfigurationcontract

import (
	"encoding/json"
	"testing"
)

func TestServiceConfigurationRoundTripPreservesProviderCapabilities(t *testing.T) {
	temperature := 0.2
	enabled := true
	want := ServiceConfiguration{LLM: LLMConfiguration{CustomProviders: []CustomProviderConfigurationView{{
		ID: "provider-1", Models: []CustomProviderModelView{{
			ID: "model-1", ContextWindowTokens: 128000, MaxOutputTokens: 32768,
			Temperature: &temperature, ThinkingBudgetTokens: 8192,
			CacheMode: "provider", PromptCacheKey: &enabled, MessageCacheControl: &enabled,
			CacheUsageFields: []string{"cache_read_input_tokens"},
		}},
	}}}}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ServiceConfiguration
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	model := got.LLM.CustomProviders[0].Models[0]
	if model.Temperature == nil || *model.Temperature != temperature || model.ThinkingBudgetTokens != 8192 || model.CacheMode != "provider" || model.PromptCacheKey == nil || !*model.PromptCacheKey || len(model.CacheUsageFields) != 1 {
		t.Fatalf("provider capability fields were lost: %+v", model)
	}
}
