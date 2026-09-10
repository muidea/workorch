package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type isolatedValidationReadinessHub struct{ magicevent.Hub }

func (h isolatedValidationReadinessHub) Send(event magicevent.Event) magicevent.Result {
	if event.ID() == llmevents.TopicLLMCommandValidateInferenceSelection {
		command := event.Data().(llmevents.ValidateProviderInferenceSelectionCommand)
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		result.Set(llmevents.ValidateProviderInferenceSelectionResult{Selection: command.Selection, CapabilityRevision: "test-provider-readiness"}, nil)
		return result
	}
	return h.Hub.Send(event)
}

// The normal HTTP authoring and production restart compiler must load the
// shipped template through its owners. No test patches a bootstrap/adapter,
// creates a Run, submits a provider request or weakens a user's acceptance text.
func TestSoftwareDevRestartCompilesReviewedDefaultForFullSizeDefinition(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: filepath.Join(t.TempDir(), "data")}))
	handler := newMuxWithState(state)
	request, _ := json.Marshal(map[string]any{"title": "reviewed input activation", "deliverableDirectory": filepath.Join(t.TempDir(), "project"), "templateId": "software-dev"})
	var workspace workspaceResponse
	if err := json.Unmarshal([]byte(postAndExpect(t, handler, "/api/v1/workorch/workspaces", string(request), http.StatusCreated)), &workspace); err != nil {
		t.Fatal(err)
	}
	id := workspace.Workspace.ID
	goal := "Develop the requested multilingual bridge in Go and expose its delivered behavior through MCP."
	goalJSON, _ := json.Marshal(map[string]string{"text": goal})
	postAndExpect(t, handler, "/api/v1/workorch/workspaces/"+id+"/goals", string(goalJSON), http.StatusCreated)
	var criteria []string
	for i := range 24 {
		criteria = append(criteria, fmt.Sprintf("Required delivered behavior %02d is demonstrated by direct executable evidence.", i+1))
	}
	dodJSON, _ := json.Marshal(map[string]any{"items": criteria})
	postAndExpect(t, handler, "/api/v1/workorch/workspaces/"+id+"/dods", string(dodJSON), http.StatusCreated)
	session := testRuntimeSession(state)
	hub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatal(err)
	}
	// Only external provider readiness is scripted; Template/Soul/Workspace
	// admission and application bootstrap compilation are the production path.
	activation := activationports.ActivateWorkspaceExecutionViewRequest{Hub: isolatedValidationReadinessHub{hub}, WorkspaceID: id, DefaultDialogLanguage: "en"}
	first, err := activationports.CompileWorkspaceExecutionRestartBootstrap(context.Background(), activation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := activationports.CompileWorkspaceExecutionRestartBootstrap(context.Background(), activation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Validate() != nil || first.Definition.StartupInputHash == "" || first.Definition.StartupInputHash != second.Definition.StartupInputHash || first.Definition.GoalDefinition != "- "+goal {
		t.Fatalf("normal restart compilation lost sealed definition or idempotency: validation=%v first=%s second=%s goal=%q", first.Validate(), first.Definition.StartupInputHash, second.Definition.StartupInputHash, first.Definition.GoalDefinition)
	}
	for _, criterion := range criteria {
		if !strings.Contains(first.Definition.DoDDefinition, criterion) {
			t.Fatal("default upgrade changed original acceptance")
		}
	}
	found := false
	for _, recipe := range runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(first.Activation.EvidencePolicies) {
		if recipe.AdapterRef != shellcommon.ValidationRecipeAdapterIsolatedEntrypoint {
			t.Fatalf("normal activation retained unreviewed adapter: %+v", recipe)
		}
		if recipe.InputReviewProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocolV2 {
			t.Fatal("normal activation omitted the complete-obligation review protocol")
		}
		found = true
	}
	if !found {
		t.Fatal("normal activation lost reviewed adapter")
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(first.Definition.Revision, first.CoverageItems, first.DefinitionFocuses)
	if err != nil || len(requirements) != 25 || len(first.CoverageItems) != 27 {
		t.Fatalf("normal full-size activation incomplete: proof=%d CWI=%d err=%v", len(requirements), len(first.CoverageItems), err)
	}
}
