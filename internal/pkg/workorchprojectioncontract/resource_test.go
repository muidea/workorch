package workorchprojectioncontract

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceDetachesNestedProjectionState(t *testing.T) {
	source := Workspace{
		ID:               "ws-clone",
		DefaultLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"},
		SoulBindings:     map[string]string{"execution": "soul-a"},
		ContextHints: map[string]any{
			"nested": map[string]any{"items": []any{"one"}},
		},
		EvidencePolicies: []map[string]any{{"checks": []any{"test"}}},
		Projection:       ProjectionStatus{FailedWorkspaceIds: []string{"ws-failed"}},
		ViewModel: WorkspaceViewModel{
			FailedWorkspaceIds: []string{"ws-degraded"},
			Sections: map[string]WorkspaceViewModelSection{
				"runtime": {Name: "runtime", Revision: 1},
			},
		},
	}

	cloned := CloneWorkspace(source)
	cloned.DefaultLLMTarget.ModelID = "mutated"
	cloned.SoulBindings["execution"] = "mutated"
	cloned.ContextHints["nested"].(map[string]any)["items"].([]any)[0] = "mutated"
	cloned.EvidencePolicies[0]["checks"].([]any)[0] = "mutated"
	cloned.Projection.FailedWorkspaceIds[0] = "mutated"
	cloned.ViewModel.FailedWorkspaceIds[0] = "mutated"
	cloned.ViewModel.Sections["runtime"] = WorkspaceViewModelSection{Name: "runtime", Revision: 2}

	if source.DefaultLLMTarget.ModelID != "model-a" || source.SoulBindings["execution"] != "soul-a" {
		t.Fatal("workspace scalar reference fields share clone storage")
	}
	if source.ContextHints["nested"].(map[string]any)["items"].([]any)[0] != "one" {
		t.Fatal("workspace context hints share clone storage")
	}
	if source.EvidencePolicies[0]["checks"].([]any)[0] != "test" {
		t.Fatal("workspace evidence policies share clone storage")
	}
	if source.Projection.FailedWorkspaceIds[0] != "ws-failed" || source.ViewModel.FailedWorkspaceIds[0] != "ws-degraded" {
		t.Fatal("workspace projection lists share clone storage")
	}
	if source.ViewModel.Sections["runtime"].Revision != 1 {
		t.Fatal("workspace view model sections share clone storage")
	}
}

func TestCloneWorkspacesPreservesNilAndEmptyCollections(t *testing.T) {
	if CloneWorkspaces(nil) != nil {
		t.Fatal("nil workspace collection must remain nil")
	}
	if cloned := CloneWorkspaces([]Workspace{}); cloned == nil || len(cloned) != 0 {
		t.Fatalf("empty workspace collection = %#v", cloned)
	}
}

func TestCloneRunDetachesSealedTargetAndRuntimeActivation(t *testing.T) {
	source := Run{
		ID:                "run-clone",
		SealedLLMTarget:   &runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"},
		RuntimeActivation: &RuntimeActivationStatus{Disposition: "fresh_activation_required"},
	}
	cloned := CloneRun(source)
	cloned.SealedLLMTarget.ModelID = "mutated"
	cloned.RuntimeActivation.Disposition = "mutated"
	if source.SealedLLMTarget.ModelID != "model-a" || source.RuntimeActivation.Disposition != "fresh_activation_required" {
		t.Fatal("run projection clone shares nested storage")
	}
	if CloneRuns(nil) != nil {
		t.Fatal("nil run collection must remain nil")
	}
	if clonedRuns := CloneRuns([]Run{}); clonedRuns == nil || len(clonedRuns) != 0 {
		t.Fatalf("empty run collection = %#v", clonedRuns)
	}
}
