package orchestrator

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

// testLLMConfigurationSnapshot keeps focused EventHub fixtures independent of
// the process-wide configuration watcher while exercising the real LLM owner
// startup contract.
func testLLMConfigurationSnapshot() (workorchconfig.LLMConfig, error) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8090"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "model-1"}
	return cfg, nil
}
