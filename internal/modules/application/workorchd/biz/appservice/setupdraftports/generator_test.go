package setupdraftports

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubGeneratorUsesLLMCommandEvent(t *testing.T) {
	hub := events.NewMemoryHub()
	var seenRequest llmmodels.ProviderRequest
	unsubscribe := hub.Subscribe(events.TopicLLMCommandCompleteProvider, func(_ context.Context, envelope events.Envelope) error {
		request, ok := envelope.Payload[events.PayloadKeyProviderRequest].(llmmodels.ProviderRequest)
		if !ok {
			t.Fatalf("provider request payload = %#v, want ProviderRequest", envelope.Payload[events.PayloadKeyProviderRequest])
		}
		seenRequest = request
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/response",
			SourceModule: "blocks/llm",
			Payload: map[string]any{
				events.PayloadKeyProviderCompletion: llmmodels.ProviderCompletionResult{
					Payload: []byte(`{"draft":{"goalText":"Generated goal","doDItems":["Generated DoD"],"confidence":"high"}}`),
				},
			},
		})
	})
	defer unsubscribe()

	draft, err := EventHubGenerator{EventHub: hub}.GenerateWorkspaceSetupDraft(context.Background(), setupdraftcore.GenerationRequest{
		Template:       workspacecore.WorkspaceTemplate{ID: "template-001"},
		DialogLanguage: "zh-CN",
		Messages:       []setupdraftcore.Message{{Role: "user", Content: "Create a workspace"}},
	})
	if err != nil {
		t.Fatalf("GenerateWorkspaceSetupDraft: %v", err)
	}
	if draft.GoalText != "Generated goal" || !reflect.DeepEqual(draft.DoDItems, []string{"Generated DoD"}) {
		t.Fatalf("draft = %+v, want provider response parsed", draft)
	}
	if seenRequest.Kind != "workspace/setup_draft" || seenRequest.Schema != WorkspaceSetupDraftSchemaName || seenRequest.DialogLanguage != "zh-CN" {
		t.Fatalf("provider request = %+v, want workspace setup draft request", seenRequest)
	}
	if seenRequest.TraceRefs["templateId"] != "template-001" {
		t.Fatalf("trace refs = %+v, want template id", seenRequest.TraceRefs)
	}
}
