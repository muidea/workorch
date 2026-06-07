package activationports

import (
	"context"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func GenerateEmbedding(ctx context.Context, hub events.Hub, providerID string, input string) (llmmodels.ProviderEmbeddingResult, error) {
	if hub == nil || strings.TrimSpace(providerID) == "" || strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        events.TopicLLMCommandGenerateEmbedding,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "llm_embedding",
		SubjectID:    strings.TrimSpace(providerID),
		Payload: map[string]any{
			events.PayloadKeyProviderID: strings.TrimSpace(providerID),
			events.PayloadKeyInput:      strings.TrimSpace(input),
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		embedding, ok := response.Payload[events.PayloadKeyProviderEmbedding].(llmmodels.ProviderEmbeddingResult)
		if !ok {
			return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
		}
		return embedding, nil
	}
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	return llmmodels.ProviderEmbeddingResult{}, common.ErrNotFound
}
