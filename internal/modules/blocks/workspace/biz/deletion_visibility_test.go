package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestCompletedRetainDeletionHidesWorkspaceGoalAndDoDWithoutPhysicalRemoval(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-retained-tombstone")
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	goalRepo := memoryrepo.NewGoalRepository()
	dodRepo := memoryrepo.NewDoDRepository()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaceRepo, workspaceRepo, goalRepo, dodRepo, memoryrepo.NewWorkspaceTemplateRepository()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	deletion := completedRetainWorkspaceDeletionForTest(workspaceID, now)
	item := &workspacemodel.Workspace{ID: workspaceID, Title: "retained", State: common.WorkspaceStateActive, DeliverableDirectory: "/tmp/ws-retained", Deletion: &deletion}
	if err := workspaceRepo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	goal := &workspacemodel.Goal{ID: "goal-retained", WorkspaceID: workspaceID, Text: "retained goal"}
	dod := &workspacemodel.DoD{ID: "dod-retained", WorkspaceID: workspaceID, Items: []string{"retained dod"}, State: common.DoDStateReady}
	if err := goalRepo.Create(ctx, goal); err != nil {
		t.Fatal(err)
	}
	if err := dodRepo.Create(ctx, dod); err != nil {
		t.Fatal(err)
	}

	workspaceGet := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	if _, err := owner.getWorkspace(ctx, workspaceGet); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Workspace Get err=%v, want not found", err)
	}
	workspaceList := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspacesCommand{})
	listed, err := owner.listWorkspaces(ctx, workspaceList)
	if err != nil || len(listed.Workspaces) != 0 {
		t.Fatalf("Workspace List=%+v err=%v, want hidden", listed.Workspaces, err)
	}
	goalGet := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetGoalCommand{WorkspaceID: workspaceID, GoalID: goal.ID})
	if _, err := owner.executeGoalCommand(ctx, goalGet); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Goal Get err=%v, want not found", err)
	}
	goalList := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListGoalsByWS, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID})
	goals, err := owner.executeGoalCommand(ctx, goalList)
	if err != nil || len(goals.Goals) != 0 {
		t.Fatalf("Goal List=%+v err=%v, want hidden", goals.Goals, err)
	}
	dodGet := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetDoD, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetDoDCommand{WorkspaceID: workspaceID, DoDID: dod.ID})
	if _, err := owner.executeDoDCommand(ctx, dodGet); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("DoD Get err=%v, want not found", err)
	}
	dodList := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListDoDsByWS, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListDoDsByWorkspaceCommand{WorkspaceID: workspaceID})
	dods, err := owner.executeDoDCommand(ctx, dodList)
	if err != nil || len(dods.DoDs) != 0 {
		t.Fatalf("DoD List=%+v err=%v, want hidden", dods.DoDs, err)
	}
	if _, err := workspaceRepo.Get(ctx, workspaceID); err != nil {
		t.Fatalf("retained Workspace tombstone was physically removed: %v", err)
	}
	if _, err := goalRepo.Get(ctx, goal.ID); err != nil {
		t.Fatalf("retained Goal was physically removed: %v", err)
	}
	if _, err := dodRepo.Get(ctx, dod.ID); err != nil {
		t.Fatalf("retained DoD was physically removed: %v", err)
	}
}

func completedRetainWorkspaceDeletionForTest(workspaceID common.WorkspaceID, now time.Time) workspacemodel.WorkspaceDeletionRecord {
	record := workspacemodel.WorkspaceDeletionRecord{
		DeletionID: "deletion-retain", DeletionGeneration: 1, RequestID: "request-retain", Mode: deletioncontract.ModeRetain,
		Phase:    workspacemodel.WorkspaceDeletionPhaseCompleted,
		Revision: 8, RequestedAt: now, UpdatedAt: now, CompletedAt: now, Completed: true,
	}
	record.IntentHash = workspacemodel.StableWorkspaceDeletionIntentHash(workspaceID, record)
	record.RunDrainingAck = &workspacemodel.WorkspaceDeletionRunDrainingAck{FenceRevision: 1, WorkspaceRunGeneration: 2}
	token := workspaceDeletionQuiescenceTokenForTest(&record, workspaceID, 2)
	record.RuntimeQuiescenceAck = &workspacemodel.WorkspaceDeletionRuntimeQuiescenceAck{Token: token}
	record.RunRetiredAck = &workspacemodel.WorkspaceDeletionRunRetiredAck{FenceRevision: 2, WorkspaceRunGeneration: 2, QuiescenceID: token.QuiescenceID, RuntimeInventoryHash: token.RuntimeInventoryHash}
	for _, owner := range workspacemodel.WorkspaceDeletionRequiredSealOwners() {
		record.OwnerSealAcks = append(record.OwnerSealAcks, workspacemodel.WorkspaceDeletionOwnerSealAck{OwnerModuleID: owner, FenceID: owner + "-fence", Revision: 1, AckHash: owner + "-ack", SealedAt: now})
	}
	return record
}
