package appservice

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraft"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type fakeWorkspaceSetupDraftGenerator struct {
	request setupdraft.GenerationRequest
	draft   setupdraft.Draft
}

func (f *fakeWorkspaceSetupDraftGenerator) GenerateWorkspaceSetupDraft(_ context.Context, request setupdraft.GenerationRequest) (setupdraft.Draft, error) {
	f.request = request
	return f.draft, nil
}

func TestGenerateWorkspaceSetupDraftWithSessionUsesTemplateAndNormalizesDoD(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(context.Background(), "file", dataRoot, workorchmanagementcontract.WorkspaceTemplate{
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
	generator := &fakeWorkspaceSetupDraftGenerator{draft: setupdraft.Draft{
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

	draft, err := GenerateWorkspaceSetupDraftWithSession(context.Background(), session, setupdraft.GenerationCommand{
		TemplateID: "generic-template",
		Workspace:  setupdraft.Workspace{Title: "Draft workspace"},
		Messages:   []setupdraft.Message{{Role: "user", Content: "Need a clear setup"}},
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

func TestSetupDraftParseCompletionAcceptsEnvelope(t *testing.T) {
	payload := []byte(`{"draft":{"goalText":"Goal","doDItems":["### Section","- [ ] item"],"clarifyingQuestions":["question?"],"confidence":"high"}}`)
	draft, err := setupdraft.ParseCompletion(payload)
	if err != nil {
		t.Fatalf("setupdraft.ParseCompletion: %v", err)
	}
	if draft.GoalText != "Goal" || draft.Confidence != "high" || !draft.NeedsClarification {
		t.Fatalf("draft = %+v, want normalized envelope draft", draft)
	}
	if len(draft.DoDItems) != 1 || draft.DoDItems[0] != "Section： item" {
		t.Fatalf("draft DoD = %#v, want normalized section item", draft.DoDItems)
	}
}

func TestSetupDraftParseCompletionAcceptsCommonAliases(t *testing.T) {
	payload := []byte(`{"draft":{"goal":"Alias goal","definitionOfDone":["first item","second item"],"clarifying_questions":["question?"],"generation_summary":"summary"}}`)
	draft, err := setupdraft.ParseCompletion(payload)
	if err != nil {
		t.Fatalf("setupdraft.ParseCompletion: %v", err)
	}
	if draft.GoalText != "Alias goal" || !reflect.DeepEqual(draft.DoDItems, []string{"first item", "second item"}) {
		t.Fatalf("draft = %+v, want alias goal and DoD", draft)
	}
	if len(draft.ClarifyingQuestions) != 1 || draft.GenerationSummary != "summary" {
		t.Fatalf("draft metadata = %+v, want alias metadata", draft)
	}
}

func TestNormalizeWorkspaceSetupDraftAddsReadableClarificationForEmptyDraft(t *testing.T) {
	draft := setupdraft.NormalizeDraft(setupdraft.Draft{})
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
	generator := &fakeWorkspaceSetupDraftGenerator{draft: setupdraft.Draft{
		ClarifyingQuestions: []string{"Need more details"},
	}}
	draft, err := GenerateWorkspaceSetupDraftWithSession(context.Background(), session, setupdraft.GenerationCommand{
		Workspace: setupdraft.Workspace{Title: "Draft workspace"},
		Messages:  []setupdraft.Message{{Role: "user", Content: "Need a clear setup"}},
		CurrentDraft: setupdraft.Draft{
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
	draft := setupdraft.RevisionBaseline(setupdraft.Draft{
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
