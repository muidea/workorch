package configcore

import (
	"fmt"
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func validatedDefaultLLMProviderID(existing string, update *string, providers map[string]workorchconfig.LLMProviderConfig) (string, error) {
	defaultProviderID := strings.TrimSpace(existing)
	if update != nil {
		defaultProviderID = strings.TrimSpace(*update)
	}
	if defaultProviderID == "" {
		return "", fmt.Errorf("%w: defaultProviderId is required", common.ErrInvalidArgument)
	}
	if _, ok := providers[defaultProviderID]; !ok {
		return "", fmt.Errorf("%w: defaultProviderId must reference an LLM provider", common.ErrInvalidArgument)
	}
	return defaultProviderID, nil
}

func validatedDefaultEmbeddingProviderID(existing string, update *string, providers map[string]workorchconfig.LLMProviderConfig) (string, error) {
	defaultEmbeddingProviderID := strings.TrimSpace(existing)
	if update != nil {
		defaultEmbeddingProviderID = strings.TrimSpace(*update)
	}
	if defaultEmbeddingProviderID == "" {
		return "", nil
	}
	provider, ok := providers[defaultEmbeddingProviderID]
	if !ok {
		return "", fmt.Errorf("%w: defaultEmbeddingProviderId must reference an LLM provider", common.ErrInvalidArgument)
	}
	if !providerSupportsOperation(provider, llmcommon.ProviderOperationEmbeddings) {
		return "", fmt.Errorf("%w: defaultEmbeddingProviderId must reference a provider with embeddings capability", common.ErrInvalidArgument)
	}
	loadedProvider, err := loadUpdatedProviderConfig(defaultEmbeddingProviderID, provider)
	if err != nil {
		return "", err
	}
	if loadedProvider.Unavailable {
		return "", fmt.Errorf("%w: defaultEmbeddingProviderId references an unavailable provider: %s", common.ErrInvalidArgument, loadedProvider.UnavailableReason)
	}
	return defaultEmbeddingProviderID, nil
}
