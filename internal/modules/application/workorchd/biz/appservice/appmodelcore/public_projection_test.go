package appmodelcore

import (
	"reflect"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectWorkspaceSealsProjectionStatusAndDetachesMutableValues(t *testing.T) {
	target := &runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}
	input := Workspace{
		ID:               "ws-1",
		DefaultLLMTarget: target,
		SoulBindings:     map[string]string{"execution": "soul-1"},
		ContextHints: map[string]any{
			"nested": map[string]any{"target": "source/main.go"},
		},
		EvidencePolicies: []map[string]any{{"kinds": []any{"state_change"}}},
		ViewModel: projectioncontract.WorkspaceViewModel{
			State: "ready", WorkspaceRevision: 7, FailedWorkspaceIds: []string{"ws-failed"},
			Sections: map[string]projectioncontract.WorkspaceViewModelSection{
				"overview": {Name: "overview", State: "ready"},
			},
		},
	}

	got := ProjectWorkspace(input)
	if got.Projection.State != "ready" || got.Projection.WorkspaceRevision != 7 ||
		!reflect.DeepEqual(got.Projection.FailedWorkspaceIds, []string{"ws-failed"}) {
		t.Fatalf("projection = %+v, want status derived from view model", got.Projection)
	}

	got.DefaultLLMTarget.ModelID = "mutated"
	got.SoulBindings["execution"] = "mutated"
	got.ContextHints["nested"].(map[string]any)["target"] = "mutated"
	got.EvidencePolicies[0]["kinds"].([]any)[0] = "mutated"
	got.ViewModel.FailedWorkspaceIds[0] = "mutated"
	got.ViewModel.Sections["overview"] = projectioncontract.WorkspaceViewModelSection{Name: "mutated"}
	got.Projection.FailedWorkspaceIds[0] = "mutated"

	if target.ModelID != "model-1" || input.SoulBindings["execution"] != "soul-1" ||
		input.ContextHints["nested"].(map[string]any)["target"] != "source/main.go" ||
		input.EvidencePolicies[0]["kinds"].([]any)[0] != "state_change" ||
		input.ViewModel.FailedWorkspaceIds[0] != "ws-failed" ||
		input.ViewModel.Sections["overview"].Name != "overview" {
		t.Fatalf("projection aliases application model: input=%+v", input)
	}
}

func TestProjectWorkspaceAndRunCollectionsPreserveNil(t *testing.T) {
	if ProjectWorkspaces(nil) != nil {
		t.Fatal("nil workspace collection became non-nil")
	}
	if ProjectRuns(nil) != nil {
		t.Fatal("nil run collection became non-nil")
	}
}
