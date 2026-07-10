package activationports

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func GenerateEmbedding(ctx context.Context, hub magicevent.Hub, providerID string, input string) (llmmodels.ProviderEmbeddingResult, error) {
	if hub == nil || strings.TrimSpace(providerID) == "" || strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandGenerateEmbedding, modulecommon.ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{
		ProviderID: strings.TrimSpace(providerID),
		Input:      strings.TrimSpace(input),
	})
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.TopicLLMCommandGenerateEmbedding, events.EventHubLaneScope{SubjectType: "llm_embedding", SubjectID: strings.TrimSpace(providerID)}))
	result := hub.Send(ev)
	if result == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return llmmodels.ProviderEmbeddingResult{}, resultErr.Cause
		}
		return llmmodels.ProviderEmbeddingResult{}, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return llmmodels.ProviderEmbeddingResult{}, resultErr
	}
	response, ok := data.(llmevents.LLMCommandResult)
	if !ok || response.Embedding == nil {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm embedding result type %T", common.ErrInvalidArgument, data)
	}
	return *response.Embedding, nil
}
