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
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func GenerateEmbedding(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, target runtimecontract.ProviderModelTarget, input string) (llmmodels.ProviderEmbeddingResult, error) {
	target = target.Normalized()
	if hub == nil || workspaceID == "" || target.Validate() != nil || strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	command := llmevents.GenerateEmbeddingCommand{
		WorkspaceID: workspaceID,
		Target:      target,
		Input:       strings.TrimSpace(input),
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandGenerateEmbedding, modulecommon.ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandGenerateEmbedding, eventcontract.LaneScope{SubjectType: "llm_embedding", SubjectID: target.ProviderID + "/" + target.ModelID, WorkspaceID: string(workspaceID)}))
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
	response, ok := data.(llmevents.GenerateEmbeddingResult)
	if !ok {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm embedding result type %T", common.ErrInvalidArgument, data)
	}
	if err := response.ValidateFor(command); err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	embedding := response.Embedding
	embedding.Vector = append([]float64(nil), response.Embedding.Vector...)
	if response.Embedding.Usage != nil {
		usage := *response.Embedding.Usage
		embedding.Usage = &usage
	}
	return embedding, nil
}
