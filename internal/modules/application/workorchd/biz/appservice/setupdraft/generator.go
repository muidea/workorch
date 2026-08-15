package setupdraft

import (
	"context"
	"encoding/json"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
)

const WorkspaceSetupDraftSchemaName = "workorch.workspaceSetupDraft"

type EventHubGenerator struct {
	CommandHub magicevent.Hub
}

func (g EventHubGenerator) GenerateWorkspaceSetupDraft(ctx context.Context, request GenerationRequest) (Draft, error) {
	if g.CommandHub == nil {
		return Draft{}, common.ErrInvalidArgument
	}
	contractPayload, err := json.Marshal(struct {
		Instructions []string       `json:"instructions"`
		OutputSchema map[string]any `json:"outputSchema"`
	}{
		Instructions: Instructions(),
		OutputSchema: OutputSchema(),
	})
	if err != nil {
		return Draft{}, err
	}
	requestPayload, err := json.Marshal(request)
	if err != nil {
		return Draft{}, err
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProvider, "application/workorchd/appservice", llmevents.CommandDestination, magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{
		Request: llmevents.ProviderRequestFromModel(llmmodels.ProviderRequest{
			Kind:           "workspace/setup_draft",
			Schema:         WorkspaceSetupDraftSchemaName,
			DialogLanguage: request.DialogLanguage,
			PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.workspace_setup_prompt", Sections: []llmmodels.PromptSection{
				{ID: "C0.workspace-setup-contract", Name: "Workspace setup contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: string(contractPayload)},
				{ID: "C3.workspace-setup-request", Name: "Workspace setup request", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: string(requestPayload)},
			}},
			TraceRefs: map[string]string{
				"templateId": request.Template.ID,
			},
		}),
	})
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandCompleteProvider, eventcontract.LaneScope{SubjectType: "workspace_setup_draft"}))
	result := g.CommandHub.Send(ev)
	if result == nil {
		return Draft{}, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return Draft{}, resultErr.Cause
		}
		return Draft{}, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return Draft{}, resultErr
	}
	response, ok := data.(llmevents.LLMCommandResult)
	if !ok || response.Completion == nil {
		return Draft{}, fmt.Errorf("%w: llm completion result type %T", common.ErrInvalidArgument, data)
	}
	return ParseCompletion([]byte(response.Completion.Content))
}
