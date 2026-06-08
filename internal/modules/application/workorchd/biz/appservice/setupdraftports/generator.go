package setupdraftports

import (
	"context"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const WorkspaceSetupDraftSchemaName = "workorch.workspaceSetupDraft.v1"

type EventHubGenerator struct {
	EventHub events.Hub
}

func (g EventHubGenerator) GenerateWorkspaceSetupDraft(ctx context.Context, request setupdraftcore.GenerationRequest) (setupdraftcore.Draft, error) {
	if g.EventHub == nil {
		return setupdraftcore.Draft{}, common.ErrInvalidArgument
	}
	payload := map[string]any{
		"instructions": setupdraftcore.Instructions(),
		"request":      request,
		"outputSchema": setupdraftcore.OutputSchema(),
	}
	err := g.EventHub.Publish(ctx, events.Envelope{
		Topic:        llmevents.TopicLLMCommandCompleteProvider,
		SourceModule: "application/workorchd/appservice",
		SubjectType:  "workspace_setup_draft",
		Payload: map[string]any{
			events.PayloadKeyProviderRequest: llmmodels.ProviderRequest{
				Kind:           "workspace/setup_draft",
				Schema:         WorkspaceSetupDraftSchemaName,
				DialogLanguage: request.DialogLanguage,
				Input:          payload,
				TraceRefs: map[string]string{
					"templateId": request.Template.ID,
				},
			},
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		completion, ok := response.Payload[events.PayloadKeyProviderCompletion].(llmmodels.ProviderCompletionResult)
		if !ok {
			return setupdraftcore.Draft{}, common.ErrInvalidArgument
		}
		return setupdraftcore.ParseCompletion(completion.Payload)
	}
	if err != nil {
		return setupdraftcore.Draft{}, err
	}
	return setupdraftcore.Draft{}, common.ErrNotFound
}
