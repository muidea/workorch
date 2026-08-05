package workorchprojectioncontract

import "testing"

func TestCloneWorkspaceViewModelDetachesCollections(t *testing.T) {
	input := WorkspaceViewModel{
		FailedWorkspaceIds: []string{"workspace-1"},
		Sections: map[string]WorkspaceViewModelSection{
			"runtime": {Name: "runtime", Revision: 1},
		},
	}
	got := CloneWorkspaceViewModel(input)
	input.FailedWorkspaceIds[0] = "mutated"
	input.Sections["runtime"] = WorkspaceViewModelSection{Name: "runtime", Revision: 2}
	if got.FailedWorkspaceIds[0] != "workspace-1" || got.Sections["runtime"].Revision != 1 {
		t.Fatalf("workspace view clone shares source storage: %+v", got)
	}
}

func TestCloneWorkspaceRuntimeViewDetachesNestedValues(t *testing.T) {
	input := WorkspaceRuntimeView{
		Projection: ProjectionStatus{FailedWorkspaceIds: []string{"workspace-1"}},
		Activity: WorkspaceActivityFeed{
			Items: []WorkspaceActivityItem{{ID: "activity-1"}},
		},
		BlockingDecision: &Decision{AllowedRecoveryActions: []string{"continue"}},
	}

	got := CloneWorkspaceRuntimeView(input)
	got.Projection.FailedWorkspaceIds[0] = "changed"
	got.Activity.Items[0].ID = "changed"
	got.BlockingDecision.AllowedRecoveryActions[0] = "changed"

	if input.Projection.FailedWorkspaceIds[0] != "workspace-1" ||
		input.Activity.Items[0].ID != "activity-1" ||
		input.BlockingDecision.AllowedRecoveryActions[0] != "continue" {
		t.Fatalf("runtime view clone shares source storage: %+v", input)
	}
}

func TestCloneWorkspaceProjectionChangedDetachesSections(t *testing.T) {
	input := WorkspaceProjectionChanged{
		ChangedSections: []WorkspaceProjectionSection{WorkspaceProjectionSectionActivity},
	}
	got := CloneWorkspaceProjectionChanged(input)
	got.ChangedSections[0] = WorkspaceProjectionSectionOverview
	if input.ChangedSections[0] != WorkspaceProjectionSectionActivity {
		t.Fatalf("projection notice clone shares source storage: %+v", input)
	}
}
