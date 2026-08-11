package workorchconfigurationcontract

import (
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestServiceConfigurationRoundTripPreservesAIProxyConfiguration(t *testing.T) {
	want := ServiceConfiguration{LLM: LLMConfiguration{
		AIProxy: AIProxyConfigurationView{
			BaseURL:          "http://127.0.0.1:8090",
			APIKeyConfigured: true,
			Timeout:          "180s",
		},
		DefaultChat: workorchconfig.ProviderModelTargetConfig{
			ProviderID: "ai-proxy",
			ModelID:    "model-1",
		}.ToRuntimeTarget(),
		Budget: workorchconfig.LLMContextBudgetPolicyConfig{
			ContextUtilizationRatio: 0.75,
			EstimatedTokensPerByte:  0.25,
		},
	}}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ServiceConfiguration
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.LLM.AIProxy.BaseURL != want.LLM.AIProxy.BaseURL || !got.LLM.AIProxy.APIKeyConfigured || got.LLM.DefaultChat.ProviderID != "ai-proxy" || got.LLM.DefaultChat.ModelID != "model-1" || got.LLM.Budget.ContextUtilizationRatio != 0.75 {
		t.Fatalf("ai-proxy configuration fields were lost: %+v", got.LLM)
	}
}
