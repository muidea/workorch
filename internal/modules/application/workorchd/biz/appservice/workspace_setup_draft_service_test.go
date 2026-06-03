package appservice

import (
	"context"
	"reflect"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

type fakeWorkspaceSetupDraftGenerator struct {
	request WorkspaceSetupDraftGenerationRequest
	draft   WorkspaceSetupDraft
}

func (f *fakeWorkspaceSetupDraftGenerator) GenerateWorkspaceSetupDraft(_ context.Context, request WorkspaceSetupDraftGenerationRequest) (WorkspaceSetupDraft, error) {
	f.request = request
	return f.draft, nil
}

func TestGenerateWorkspaceSetupDraftWithSessionUsesTemplateAndNormalizesDoD(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(context.Background(), "file", dataRoot, WorkspaceTemplate{
		ID:          "generic-template",
		Name:        "Generic Template",
		Description: "Template description",
		GoalText:    "Template goal",
		DoDItems:    []string{"Template DoD"},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	generator := &fakeWorkspaceSetupDraftGenerator{draft: WorkspaceSetupDraft{
		GoalText: "Generated goal",
		DoDItems: []string{`
### 1. Acceptance
- [ ] **Group**:
- [ ] first item
- [ ] second item
`},
		ClarifyingQuestions: []string{"  "},
		Confidence:          "unexpected",
	}}

	draft, err := GenerateWorkspaceSetupDraftWithSession(context.Background(), session, WorkspaceSetupDraftGenerationCommand{
		TemplateID: "generic-template",
		Workspace:  WorkspaceSetupDraftWorkspace{Title: "Draft workspace"},
		Messages:   []WorkspaceSetupDraftMessage{{Role: "user", Content: "Need a clear setup"}},
	}, generator)
	if err != nil {
		t.Fatalf("GenerateWorkspaceSetupDraftWithSession: %v", err)
	}
	if generator.request.Template.ID != "generic-template" || generator.request.Template.GoalText != "Template goal" {
		t.Fatalf("generation request template = %+v, want selected template", generator.request.Template)
	}
	wantItems := []string{
		"Acceptance / Group： first item",
		"Acceptance / Group： second item",
	}
	if !reflect.DeepEqual(draft.DoDItems, wantItems) {
		t.Fatalf("draft items = %#v, want %#v", draft.DoDItems, wantItems)
	}
	if draft.NeedsClarification || draft.Confidence != "medium" {
		t.Fatalf("draft flags = %+v, want no clarification and default medium confidence", draft)
	}
}

func TestParseWorkspaceSetupDraftCompletionAcceptsEnvelope(t *testing.T) {
	payload := []byte(`{"draft":{"goalText":"Goal","doDItems":["### Section","- [ ] item"],"clarifyingQuestions":["question?"],"confidence":"high"}}`)
	draft, err := ParseWorkspaceSetupDraftCompletion(payload)
	if err != nil {
		t.Fatalf("ParseWorkspaceSetupDraftCompletion: %v", err)
	}
	if draft.GoalText != "Goal" || draft.Confidence != "high" || !draft.NeedsClarification {
		t.Fatalf("draft = %+v, want normalized envelope draft", draft)
	}
	if len(draft.DoDItems) != 1 || draft.DoDItems[0] != "Section： item" {
		t.Fatalf("draft DoD = %#v, want normalized section item", draft.DoDItems)
	}
}

func TestParseWorkspaceSetupDraftCompletionAcceptsCommonAliases(t *testing.T) {
	payload := []byte(`{"draft":{"goal":"Alias goal","definitionOfDone":["first item","second item"],"clarifying_questions":["question?"],"generation_summary":"summary"}}`)
	draft, err := ParseWorkspaceSetupDraftCompletion(payload)
	if err != nil {
		t.Fatalf("ParseWorkspaceSetupDraftCompletion: %v", err)
	}
	if draft.GoalText != "Alias goal" || !reflect.DeepEqual(draft.DoDItems, []string{"first item", "second item"}) {
		t.Fatalf("draft = %+v, want alias goal and DoD", draft)
	}
	if len(draft.ClarifyingQuestions) != 1 || draft.GenerationSummary != "summary" {
		t.Fatalf("draft metadata = %+v, want alias metadata", draft)
	}
}

func TestDefaultWorkspaceSetupDraftGeneratorUsesLLMCommandEvent(t *testing.T) {
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

	draft, err := defaultWorkspaceSetupDraftGenerator{eventHub: hub}.GenerateWorkspaceSetupDraft(context.Background(), WorkspaceSetupDraftGenerationRequest{
		Template:       WorkspaceTemplate{ID: "template-001"},
		DialogLanguage: "zh-CN",
		Messages:       []WorkspaceSetupDraftMessage{{Role: "user", Content: "Create a workspace"}},
	})
	if err != nil {
		t.Fatalf("GenerateWorkspaceSetupDraft: %v", err)
	}
	if draft.GoalText != "Generated goal" || !reflect.DeepEqual(draft.DoDItems, []string{"Generated DoD"}) {
		t.Fatalf("draft = %+v, want provider response parsed", draft)
	}
	if seenRequest.Kind != "workspace/setup_draft" || seenRequest.Schema != workspaceSetupDraftSchemaName || seenRequest.DialogLanguage != "zh-CN" {
		t.Fatalf("provider request = %+v, want workspace setup draft request", seenRequest)
	}
	if seenRequest.TraceRefs["templateId"] != "template-001" {
		t.Fatalf("trace refs = %+v, want template id", seenRequest.TraceRefs)
	}
}

func TestNormalizeWorkspaceSetupDraftAddsReadableClarificationForEmptyDraft(t *testing.T) {
	draft := NormalizeWorkspaceSetupDraft(WorkspaceSetupDraft{})
	if !draft.NeedsClarification || len(draft.ClarifyingQuestions) == 0 {
		t.Fatalf("draft = %+v, want readable clarification", draft)
	}
}

func TestGenerateWorkspaceSetupDraftWithSessionPreservesCurrentDraftOnPartialOutput(t *testing.T) {
	dataRoot := t.TempDir()
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	generator := &fakeWorkspaceSetupDraftGenerator{draft: WorkspaceSetupDraft{
		ClarifyingQuestions: []string{"Need more details"},
	}}
	draft, err := GenerateWorkspaceSetupDraftWithSession(context.Background(), session, WorkspaceSetupDraftGenerationCommand{
		Workspace: WorkspaceSetupDraftWorkspace{Title: "Draft workspace"},
		Messages:  []WorkspaceSetupDraftMessage{{Role: "user", Content: "Need a clear setup"}},
		CurrentDraft: WorkspaceSetupDraft{
			GoalText: "Existing goal",
			DoDItems: []string{"Existing DoD"},
		},
	}, generator)
	if err != nil {
		t.Fatalf("GenerateWorkspaceSetupDraftWithSession: %v", err)
	}
	if draft.GoalText != "Existing goal" || !reflect.DeepEqual(draft.DoDItems, []string{"Existing DoD"}) {
		t.Fatalf("draft = %+v, want current Goal/DoD preserved", draft)
	}
	if len(draft.ClarifyingQuestions) != 1 || !draft.NeedsClarification {
		t.Fatalf("draft clarification = %+v, want clarification retained", draft)
	}
}

func TestWorkspaceSetupDraftRevisionBaselineClearsStaleClarifications(t *testing.T) {
	draft := WorkspaceSetupDraftRevisionBaseline(WorkspaceSetupDraft{
		GoalText:            "Existing goal",
		DoDItems:            []string{"Existing DoD"},
		ClarifyingQuestions: []string{"Old question"},
		TemplateContextUsed: true,
	})
	if draft.GoalText != "Existing goal" || !reflect.DeepEqual(draft.DoDItems, []string{"Existing DoD"}) {
		t.Fatalf("draft = %+v, want Goal/DoD preserved", draft)
	}
	if len(draft.ClarifyingQuestions) != 0 || draft.NeedsClarification {
		t.Fatalf("draft = %+v, want stale clarifications cleared", draft)
	}
	if !draft.TemplateContextUsed {
		t.Fatalf("draft = %+v, want metadata preserved", draft)
	}
}
