package run

import (
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBuildRunLifecycleFactInventoryIsBoundedCurrentOwnerFact(t *testing.T) {
	items := []*runmodel.Run{
		{Revision: 3, WorkspaceRunGeneration: 2, ID: "run-b", WorkspaceID: "ws-2", State: common.RunStateBlockedOnDecision, RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 9}},
		{Revision: 2, WorkspaceRunGeneration: 1, ID: "run-a", WorkspaceID: "ws-1", State: common.RunStateActive, RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 7}},
		{Revision: 4, WorkspaceRunGeneration: 1, ID: "run-terminal", WorkspaceID: "ws-3", State: common.RunStateCompleted, RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 10}},
	}
	first, err := buildRunLifecycleFactInventory(items, runevents.QueryRunLifecycleFactInventoryCommand{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Items[0].WorkspaceID != "ws-1" || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	if first.Items[0].RunRevision != 2 || first.Items[0].RuntimeStateRevision != 7 || first.Items[0].CausationID != "run-state/run-a/1/2/7" {
		t.Fatalf("first fact=%+v", first.Items[0])
	}
	second, err := buildRunLifecycleFactInventory(items, runevents.QueryRunLifecycleFactInventoryCommand{Cursor: first.NextCursor, Limit: 1})
	if err != nil || len(second.Items) != 1 || second.Items[0].WorkspaceID != "ws-2" || second.NextCursor != "" {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
}

func TestBuildRunLifecycleFactInventoryRejectsMultipleCurrentRunsInWorkspace(t *testing.T) {
	_, err := buildRunLifecycleFactInventory([]*runmodel.Run{
		{Revision: 1, WorkspaceRunGeneration: 1, ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive},
		{Revision: 1, WorkspaceRunGeneration: 2, ID: "run-2", WorkspaceID: "ws-1", State: common.RunStatePending},
	}, runevents.QueryRunLifecycleFactInventoryCommand{})
	if err == nil {
		t.Fatal("multiple current Runs in one Workspace must fail closed")
	}
}

func TestBuildRunLifecycleFactInventoryRejectsMalformedCurrentRun(t *testing.T) {
	_, err := buildRunLifecycleFactInventory([]*runmodel.Run{{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
	}}, runevents.QueryRunLifecycleFactInventoryCommand{})
	if err == nil {
		t.Fatal("Run without owner persistence revision must fail closed")
	}
}
