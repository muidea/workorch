package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	cfg, err := workorchconfig.Persisted()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	next := cfg.LLM
	if cmd.ClearAPIKey && cmd.APIKey != nil && strings.TrimSpace(*cmd.APIKey) != "" {
		return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: clearApiKey cannot be combined with a non-empty apiKey", common.ErrInvalidArgument)
	}
	if cmd.ClearAPIKey {
		next.AIProxy.APIKey = ""
	} else if cmd.APIKey != nil && strings.TrimSpace(*cmd.APIKey) != "" {
		next.AIProxy.APIKey = strings.TrimSpace(*cmd.APIKey)
	}
	if cmd.DefaultChatModelID != nil {
		modelID := strings.TrimSpace(*cmd.DefaultChatModelID)
		if modelID == "" {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: defaultChatModelId is required when supplied", common.ErrInvalidArgument)
		}
		next.DefaultChat = workorchconfig.ProviderModelTargetConfig{
			ProviderID: runtimecontract.BuiltinAIProxyProviderID,
			ModelID:    modelID,
		}
	}
	if cmd.ClearDefaultEmbedding {
		if cmd.DefaultEmbeddingModelID != nil && strings.TrimSpace(*cmd.DefaultEmbeddingModelID) != "" {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: clearDefaultEmbedding cannot be combined with defaultEmbeddingModelId", common.ErrInvalidArgument)
		}
		next.DefaultEmbedding = nil
	} else if cmd.DefaultEmbeddingModelID != nil {
		modelID := strings.TrimSpace(*cmd.DefaultEmbeddingModelID)
		if modelID == "" {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: defaultEmbeddingModelId is required when supplied", common.ErrInvalidArgument)
		}
		target := workorchconfig.ProviderModelTargetConfig{
			ProviderID: runtimecontract.BuiltinAIProxyProviderID,
			ModelID:    modelID,
		}
		next.DefaultEmbedding = &target
	}
	if cmd.Budget != nil {
		if cmd.Budget.ContextUtilizationRatio != nil {
			next.Budget.ContextUtilizationRatio = *cmd.Budget.ContextUtilizationRatio
		}
		if cmd.Budget.EstimatedTokensPerByte != nil {
			next.Budget.EstimatedTokensPerByte = *cmd.Budget.EstimatedTokensPerByte
		}
	}
	normalized, err := workorchconfig.NormalizeLLMConfig(next)
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	cfg.LLM = normalized
	if err := workorchconfig.Save(cfg); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	item, err := ReadServiceConfiguration()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	// The active LLM owner has intentionally retained its startup snapshot.
	// This write therefore always stages the persisted values for the next
	// WorkOrch process, even when the framework configuration watcher reloads.
	item.LLM.RestartRequired = true
	return item, nil
}
