package readmodelprojection

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPublicSnapshotsExcludePrivateStateAndDetachCollections(t *testing.T) {
	state := readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:                "workspace-1",
			InternalDirectory: "/private/workorch",
			SoulBindings:      map[string]string{"execution": "soul-1"},
		},
		Run: appmodelcore.Run{
			ID:           "run-1",
			WorkspaceID:  "workspace-1",
			RuntimeState: &runtimecontract.RuntimeStateRecord{},
		},
		CoverageWorkItems:       []map[string]any{{"id": "cwi-1"}},
		CoverageWorkItemContext: map[string]any{"scope": "selected"},
		Steps:                   []projectioncontract.StepView{{ID: "step-1"}},
	}

	public := PublicRunSnapshot(state)
	if public.Workspace.ID != state.Workspace.ID || public.Run.ID != state.Run.ID {
		t.Fatalf("public resources not preserved: %+v", public)
	}
	if _, ok := reflect.TypeOf(public.Workspace).FieldByName("InternalDirectory"); ok {
		t.Fatal("public workspace exposes InternalDirectory")
	}
	if _, ok := reflect.TypeOf(public.Run).FieldByName("RuntimeState"); ok {
		t.Fatal("public run exposes RuntimeState")
	}

	public.Workspace.SoulBindings["execution"] = "changed"
	public.CoverageWorkItems[0]["id"] = "changed"
	public.CoverageWorkItemContext["scope"] = "changed"
	public.Steps[0].ID = "changed"
	if state.Workspace.SoulBindings["execution"] != "soul-1" ||
		state.CoverageWorkItems[0]["id"] != "cwi-1" ||
		state.CoverageWorkItemContext["scope"] != "selected" ||
		state.Steps[0].ID != "step-1" {
		t.Fatalf("public projection shares mutable top-level collections with private state: %+v", state)
	}
}

func TestPublicWorkspaceSnapshotAndObservabilityPreserveNilAndRunCollections(t *testing.T) {
	workspaceState := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "workspace-1"},
		Runs: []appmodelcore.Run{
			{ID: "run-1", WorkspaceID: "workspace-1", RuntimeState: &runtimecontract.RuntimeStateRecord{}},
		},
	}
	workspace := PublicWorkspaceSnapshot(workspaceState)
	if workspace.Run != nil {
		t.Fatalf("nil active run changed to %+v", workspace.Run)
	}
	if len(workspace.Runs) != 1 || workspace.Runs[0].ID != "run-1" {
		t.Fatalf("workspace runs not preserved: %+v", workspace.Runs)
	}
	if _, ok := reflect.TypeOf(workspace.Runs[0]).FieldByName("RuntimeState"); ok {
		t.Fatal("public workspace run exposes RuntimeState")
	}

	observability := PublicRunObservability(readmodelcore.RunObservabilityState{
		Run:      appmodelcore.Run{ID: "run-1", WorkspaceID: "workspace-1", RuntimeState: &runtimecontract.RuntimeStateRecord{}},
		Timeline: []projectioncontract.RuntimeTimelineItem{{ID: "timeline-1"}},
	})
	observability.Timeline[0].ID = "changed"
	if observability.Run.ID != "run-1" {
		t.Fatalf("observability run not preserved: %+v", observability.Run)
	}
}

func TestPublicSnapshotContractsStayFieldCompleteAgainstPrivateState(t *testing.T) {
	assertSameFieldNames(t, reflect.TypeOf(readmodelcore.RunSnapshotState{}), reflect.TypeOf(projectioncontract.RunSnapshot{}))
	assertSameFieldNames(t, reflect.TypeOf(readmodelcore.WorkspaceSnapshotState{}), reflect.TypeOf(projectioncontract.WorkspaceSnapshot{}))
	assertSameFieldNames(t, reflect.TypeOf(readmodelcore.RunObservabilityState{}), reflect.TypeOf(projectioncontract.RunObservability{}))
}

func assertSameFieldNames(t *testing.T, private reflect.Type, public reflect.Type) {
	t.Helper()
	privateFields := make(map[string]struct{}, private.NumField())
	for index := 0; index < private.NumField(); index++ {
		privateFields[private.Field(index).Name] = struct{}{}
	}
	publicFields := make(map[string]struct{}, public.NumField())
	for index := 0; index < public.NumField(); index++ {
		publicFields[public.Field(index).Name] = struct{}{}
	}
	if !reflect.DeepEqual(privateFields, publicFields) {
		t.Fatalf("private/public fields diverged\nprivate=%v\n public=%v", privateFields, publicFields)
	}
}
