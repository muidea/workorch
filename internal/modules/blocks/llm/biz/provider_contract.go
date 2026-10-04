package llm

import (
	"context"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
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

// providerCompletionContent preserves Runtime candidate provider bytes until
// the LLM owner classifies complete versus incomplete framing. Partial output
// is never used as a continuation prefix. Ordinary structured requests retain
// the established whitespace normalization.
func providerCompletionContent(request llmmodels.ProviderRequest, content string) string {
	if request.RawTextOutput || request.CaptureDocumentObservation {
		return content
	}
	return strings.TrimSpace(content)
}

func NewProviderFromConfig(config llmmodels.ProviderConfig) Provider {
	return NewHTTPProvider(config)
}

func GenerateEmbedding(ctx context.Context, providerID string, config llmmodels.ProviderConfig, input string) (llmmodels.ProviderEmbeddingResult, error) {
	providerID = strings.TrimSpace(providerID)
	input = strings.TrimSpace(input)
	if providerID == "" || input == "" || config.Unavailable {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if !providerSupportsNonChatOperation(config.Capabilities, "embeddings") {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider operation %q is not declared by provider capabilities", common.ErrInvalidArgument, "embeddings")
	}
	provider, ok := NewProviderFromConfig(config).(EmbeddingProvider)
	if !ok {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider does not implement embeddings", common.ErrInvalidArgument)
	}
	result, err := provider.Embed(ctx, llmmodels.ProviderEmbeddingRequest{Input: input, Model: config.Model})
	if modeErr := validatePhysicalResponseMode(result.PhysicalResponseMode, llmaudit.PhysicalResponseModeNonStreaming); modeErr != nil {
		return result, modeErr
	}
	if err != nil {
		return result, err
	}
	result.ProviderID = providerID
	if strings.TrimSpace(result.Model) == "" {
		result.Model = config.Model
	}
	return result, nil
}

func validatePhysicalResponseMode(actual, expected llmaudit.PhysicalResponseMode) error {
	actual = actual.Normalized()
	expected = expected.Normalized()
	if err := actual.Validate(); err != nil {
		return fmt.Errorf("%w: provider returned invalid physical response mode: %v", common.ErrInvalidArgument, err)
	}
	if err := expected.Validate(); err != nil || actual != expected {
		return fmt.Errorf("%w: provider physical response mode %q does not match dispatched mode %q", common.ErrInvalidArgument, actual, expected)
	}
	return nil
}

func providerSupportsNonChatOperation(capabilities llmmodels.ProviderCapabilities, operation string) bool {
	for _, item := range capabilities.NonChatOperations {
		if strings.TrimSpace(item) == operation {
			return true
		}
	}
	return false
}
