package workorchprojectioncontract

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceSnapshotDetachesNestedValues(t *testing.T) {
	input := WorkspaceSnapshot{
		Workspace: Workspace{
			SoulBindings:     map[string]string{"execution": "soul-1"},
			DefaultLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"},
			Projection:       ProjectionStatus{FailedWorkspaceIds: []string{"workspace-1"}},
		},
		Run: &Run{
			SealedLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"},
		},
		SchedulerDiagnostics: &SchedulerDiagnostics{
			RecentContinuations: []ContinuationDiagnostic{{RecommendedActions: []string{"resume"}}},
		},
		CoverageSummary: map[string]any{"open": []any{"cwi-1"}},
	}

	got := CloneWorkspaceSnapshot(input)
	got.Workspace.SoulBindings["execution"] = "changed"
	got.Workspace.DefaultLLMTarget.ModelID = "changed"
	got.Workspace.Projection.FailedWorkspaceIds[0] = "changed"
	got.Run.SealedLLMTarget.ModelID = "changed"
	got.SchedulerDiagnostics.RecentContinuations[0].RecommendedActions[0] = "changed"
	got.CoverageSummary["open"].([]any)[0] = "changed"

	if input.Workspace.SoulBindings["execution"] != "soul-1" ||
		input.Workspace.DefaultLLMTarget.ModelID != "model-1" ||
		input.Workspace.Projection.FailedWorkspaceIds[0] != "workspace-1" ||
		input.Run.SealedLLMTarget.ModelID != "model-1" ||
		input.SchedulerDiagnostics.RecentContinuations[0].RecommendedActions[0] != "resume" ||
		input.CoverageSummary["open"].([]any)[0] != "cwi-1" {
		t.Fatalf("workspace snapshot clone shares source storage: %+v", input)
	}
}
