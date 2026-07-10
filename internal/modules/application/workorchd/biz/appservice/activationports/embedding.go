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

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func GenerateEmbedding(ctx context.Context, hub events.Hub, providerID string, input string) (llmmodels.ProviderEmbeddingResult, error) {
	if hub == nil || strings.TrimSpace(providerID) == "" || strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	rawHub, err := activationRawMagicHub(hub)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandGenerateEmbedding, modulecommon.ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{
		ProviderID: strings.TrimSpace(providerID),
		Input:      strings.TrimSpace(input),
	})
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{Topic: llmevents.TopicLLMCommandGenerateEmbedding, SubjectType: "llm_embedding", SubjectID: strings.TrimSpace(providerID)}))
	result := rawHub.Send(ev)
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

func activationRawMagicHub(hub events.Hub) (magicevent.Hub, error) {
	provider, ok := hub.(rawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return nil, common.ErrInvalidArgument
	}
	return provider.RawHub(), nil
}
