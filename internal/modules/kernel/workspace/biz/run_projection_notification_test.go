package workspace

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func subscribeWorkspaceStateChangedForProjectionTest(t *testing.T, hub magicevent.Hub) <-chan workspaceevents.WorkspaceChangedData {
	t.Helper()
	const observerID = "/workspace/consumers/blocks/workspace/run-projection-notification-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan workspaceevents.WorkspaceChangedData, 16)
	observer.Subscribe(workspaceevents.TopicWorkspaceStateChanged, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(workspaceevents.WorkspaceChangedData)
		if ok {
			notifications <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceStateChanged) })
	return notifications
}

func expectWorkspaceStateChangedForProjectionTest(
	t *testing.T,
	notifications <-chan workspaceevents.WorkspaceChangedData,
	wantState common.WorkspaceState,
	wantTitle string,
) workspaceevents.WorkspaceChangedData {
	t.Helper()
	select {
	case data := <-notifications:
		if data.State != wantState || data.Title != wantTitle {
			t.Fatalf("Workspace notification=%+v, want state=%q title=%q", data, wantState, wantTitle)
		}
		return data
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for Workspace notification state=%q title=%q", wantState, wantTitle)
		return workspaceevents.WorkspaceChangedData{}
	}
}

func TestWorkspaceRunProjectionPersistsCursorWithoutRepublishingUnchangedPublicState(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	notifications := subscribeWorkspaceStateChangedForProjectionTest(t, hub)
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", Title: "workspace", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(
		hub, nil, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository(),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })

	apply := func(runRevision, runtimeRevision uint64, state common.RunState) {
		t.Helper()
		if err := owner.applyRunStateChangedData(ctx, workspaceRunStateChangedTestData("run-1", runRevision, runtimeRevision, state)); err != nil {
			t.Fatalf("apply Run projection %d/%d/%s: %v", runRevision, runtimeRevision, state, err)
		}
	}

	apply(2, 1, common.RunStateActive)
	expectWorkspaceStateChangedForProjectionTest(t, notifications, common.WorkspaceStateActive, "workspace")

	// These accepted/stale facts only move (or replay) the owner-private cursor.
	// The later public transition is a same-lane barrier: if either fact were
	// republished, it would be observed before the expected blocked notification.
	apply(3, 2, common.RunStateActive)
	apply(3, 2, common.RunStateActive)
	apply(2, 1, common.RunStateActive)
	apply(4, 3, common.RunStateBlockedOnApproval)
	expectWorkspaceStateChangedForProjectionTest(t, notifications, common.WorkspaceStateBlockedOnApproval, "workspace")

	apply(5, 4, common.RunStateBlockedOnApproval)
	apply(6, 5, common.RunStateBlockedOnDecision)
	expectWorkspaceStateChangedForProjectionTest(t, notifications, common.WorkspaceStateActive, "workspace")

	// Duplicate and stale inputs remain notification-free as well. A final real
	// public transition again fences all preceding posts on the Workspace lane.
	apply(6, 5, common.RunStateBlockedOnDecision)
	apply(5, 4, common.RunStateBlockedOnApproval)
	apply(7, 6, common.RunStateBlockedOnApproval)
	expectWorkspaceStateChangedForProjectionTest(t, notifications, common.WorkspaceStateBlockedOnApproval, "workspace")

	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 7 || stored.RunProjectionCursor.RunRevision != 7 || stored.RunProjectionCursor.RuntimeStateRevision != 6 ||
		stored.State != common.WorkspaceStateBlockedOnApproval || stored.ActiveRunID != "run-1" {
		t.Fatalf("stored Workspace=%+v, want six accepted owner-private/public CAS transitions", stored)
	}
}

func TestWorkspaceRunProjectionFreshOwnerAdvancesCursorWithoutPublicStateNotification(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	notifications := subscribeWorkspaceStateChangedForProjectionTest(t, hub)

	workspaces := memory.NewWorkspaceRepository()
	persistedFact := workspaceRunProjectionTestFact("run-1", 2, 1, common.RunStateActive)
	persisted := applyWorkspaceProjection(&workspacemodel.Workspace{
		ID: "ws-1", Title: "persisted", State: common.WorkspaceStateActive,
	}, workspaceProjectionTransition{
		ActiveRunID: "run-1",
		State:       common.WorkspaceStateActive,
		RunProjection: &workspacemodel.WorkspaceRunProjectionCursor{
			WorkspaceRunGeneration: persistedFact.WorkspaceRunGeneration,
			RunID:                  persistedFact.RunID,
			RunRevision:            persistedFact.RunRevision,
			RuntimeStateRevision:   persistedFact.RuntimeStateRevision,
			RuntimeStateRef:        persistedFact.RuntimeStateRef,
			CausationID:            persistedFact.CausationID,
		},
	})
	if err := workspaces.Create(ctx, persisted); err != nil {
		t.Fatal(err)
	}

	runs := memory.NewRunRepository()
	currentRun := &runmodel.Run{
		Revision: 3, ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 2},
	}
	if err := runs.Create(ctx, currentRun); err != nil {
		t.Fatal(err)
	}
	inventory := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	inventory.Subscribe(runevents.TopicRunCommandLifecycleFactInventory, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.QueryRunLifecycleFactInventoryCommand)
		if !ok {
			result.Set(runevents.QueryRunLifecycleFactInventoryResult{}, nil)
			return
		}
		items, err := runs.List(event.Context())
		if err != nil {
			result.Set(nil, nil)
			return
		}
		result.Set(testRunLifecycleFactInventory(items, command), nil)
	})
	t.Cleanup(func() { inventory.Unsubscribe(runevents.TopicRunCommandLifecycleFactInventory) })

	owner, err := New(hub, nil, workspaceOwnerTestDependencies(
		hub, nil, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository(),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}

	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 2 || stored.RunProjectionCursor.RunRevision != currentRun.Revision ||
		stored.RunProjectionCursor.RuntimeStateRevision != currentRun.RuntimeState.StateRevision || stored.ActiveRunID != currentRun.ID {
		t.Fatalf("fresh owner Workspace=%+v, want persisted current Run cursor", stored)
	}

	// Publish a same-lane marker only after startup reconciliation. It must be
	// the first public event; an accidental cursor-only notification would carry
	// the old title and be delivered ahead of this marker.
	marker := *stored
	marker.Title = "notification-barrier"
	owner.publishWorkspaceUpdated(ctx, &marker)
	expectWorkspaceStateChangedForProjectionTest(t, notifications, common.WorkspaceStateActive, "notification-barrier")
}
