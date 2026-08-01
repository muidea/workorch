package llm

import (
	"context"
	"fmt"
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type Provider interface {
	Complete(context.Context, llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error)
}

type StreamingProvider interface {
	StreamComplete(context.Context, llmmodels.ProviderRequest, func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error)
}

type EmbeddingProvider interface {
	Embed(context.Context, llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error)
}

type ProviderConfigLoader func() (string, map[string]llmmodels.ProviderConfig, error)
type ProviderConfigRevision func() uint64

// providerCompletionContent preserves byte-for-byte suffixes for the typed
// Runtime candidate continuation protocol. Ordinary structured requests
// retain the established whitespace normalization.
func providerCompletionContent(request llmmodels.ProviderRequest, content string) string {
	if request.RawTextOutput {
		return content
	}
	return strings.TrimSpace(content)
}

func NewProviderFromConfig(config llmmodels.ProviderConfig) Provider {
	switch strings.TrimSpace(config.Provider) {
	case llmcommon.ProviderOpenAI:
		return NewOpenAIProvider(config)
	case llmcommon.ProviderAnthropic:
		return NewAnthropicProvider(config)
	default:
		return NewHTTPProvider(config)
	}
}

func GenerateEmbedding(ctx context.Context, providerID string, config llmmodels.ProviderConfig, input string) (llmmodels.ProviderEmbeddingResult, error) {
	providerID = strings.TrimSpace(providerID)
	input = strings.TrimSpace(input)
	if providerID == "" || input == "" || config.Unavailable {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if !providerSupportsNonChatOperation(config.Capabilities, llmcommon.ProviderOperationEmbeddings) {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider operation %q is not declared by provider capabilities", common.ErrInvalidArgument, llmcommon.ProviderOperationEmbeddings)
	}
	provider, ok := NewProviderFromConfig(config).(EmbeddingProvider)
	if !ok {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider does not implement embeddings", common.ErrInvalidArgument)
	}
	result, err := provider.Embed(ctx, llmmodels.ProviderEmbeddingRequest{Input: input, Model: config.Model})
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	result.ProviderID = providerID
	if strings.TrimSpace(result.Model) == "" {
		result.Model = config.Model
	}
	return result, nil
}

func providerSupportsNonChatOperation(capabilities llmmodels.ProviderCapabilities, operation string) bool {
	capabilities = effectiveProviderCapabilities(capabilities)
	for _, item := range capabilities.NonChatOperations {
		if strings.TrimSpace(item) == operation {
			return true
		}
	}
	return false
}

func effectiveProviderCapabilities(capabilities llmmodels.ProviderCapabilities) llmmodels.ProviderCapabilities {
	if !capabilities.ChatCompletions && !capabilities.Streaming && len(capabilities.NonChatOperations) == 0 {
		capabilities.ChatCompletions = true
	}
	return capabilities
}
