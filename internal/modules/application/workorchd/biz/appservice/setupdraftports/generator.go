package setupdraftports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const WorkspaceSetupDraftSchemaName = "workorch.workspaceSetupDraft.v1"

type EventHubGenerator struct {
	CommandHub magicevent.Hub
}

func (g EventHubGenerator) GenerateWorkspaceSetupDraft(ctx context.Context, request setupdraftcore.GenerationRequest) (setupdraftcore.Draft, error) {
	if g.CommandHub == nil {
		return setupdraftcore.Draft{}, common.ErrInvalidArgument
	}
	payload := map[string]any{
		"instructions": setupdraftcore.Instructions(),
		"request":      request,
		"outputSchema": setupdraftcore.OutputSchema(),
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProvider, "application/workorchd/appservice", llmevents.CommandDestination, magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{
		Request: llmmodels.ProviderRequest{
			Kind:           "workspace/setup_draft",
			Schema:         WorkspaceSetupDraftSchemaName,
			DialogLanguage: request.DialogLanguage,
			Input:          payload,
			TraceRefs: map[string]string{
				"templateId": request.Template.ID,
			},
		},
	})
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.TopicLLMCommandCompleteProvider, events.EventHubLaneScope{SubjectType: "workspace_setup_draft"}))
	result := g.CommandHub.Send(ev)
	if result == nil {
		return setupdraftcore.Draft{}, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return setupdraftcore.Draft{}, resultErr.Cause
		}
		return setupdraftcore.Draft{}, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return setupdraftcore.Draft{}, resultErr
	}
	response, ok := data.(llmevents.LLMCommandResult)
	if !ok || response.Completion == nil {
		return setupdraftcore.Draft{}, fmt.Errorf("%w: llm completion result type %T", common.ErrInvalidArgument, data)
	}
	return setupdraftcore.ParseCompletion(response.Completion.Payload)
}
