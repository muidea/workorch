package setupdraftports

import (
	"context"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestEventHubGeneratorUsesLLMCommandEvent(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	var seenRequest llmmodels.ProviderRequest
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, rawHub)
	observer.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(llmevents.CompleteProviderCommand)
		if !ok {
			t.Fatalf("provider command = %#v, want CompleteProviderCommand", ev.Data())
		}
		seenRequest = cmd.Request
		result.Set(llmevents.LLMCommandResult{
			Completion: &llmmodels.ProviderCompletionResult{
				Payload: []byte(`{"draft":{"goalText":"Generated goal","doDItems":["Generated DoD"],"confidence":"high"}}`),
			},
		}, nil)
	})
	defer observer.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
	time.Sleep(10 * time.Millisecond)

	draft, err := EventHubGenerator{CommandHub: rawHub}.GenerateWorkspaceSetupDraft(context.Background(), setupdraftcore.GenerationRequest{
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
