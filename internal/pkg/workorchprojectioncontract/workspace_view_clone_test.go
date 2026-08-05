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
