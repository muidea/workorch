package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceDeletionWaitingBlocksConfigurationGoalAndDoDMutations(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	goalRepo := memoryrepo.NewGoalRepository()
	dodRepo := memoryrepo.NewDoDRepository()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner, err := New(hub, nil, Dependencies{Workspaces: workspaceRepo, DeletionMaterialCleaner: workspaceRepo, Goals: goalRepo, DoDs: dodRepo, Templates: memoryrepo.NewWorkspaceTemplateRepository(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-waiting-ingress-fence")
	waiting := workspaceWithWaitingDeletionForTest(t, &workspacemodel.Workspace{ID: workspaceID, Title: "before", State: common.WorkspaceStateActive}, "delete-ingress")
	if err := workspaceRepo.Create(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	before, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	update := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdate, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceCommand{WorkspaceID: workspaceID, Title: "after"})
	if _, err := owner.updateWorkspace(ctx, update); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Workspace config update err=%v, want conflict", err)
	}
	llm := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, ExpectedRevision: before.Revision})
	if _, err := owner.updateWorkspaceLLMSelection(ctx, llm); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Workspace LLM config update err=%v, want conflict", err)
	}
	if err := owner.manager.DefineGoal(ctx, workspaceevents.DefineGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-blocked", Text: "blocked"}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Goal define err=%v, want conflict", err)
	}
	if err := owner.manager.DefineDoD(ctx, workspaceevents.DefineDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-blocked", Items: []string{"blocked"}}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("DoD define err=%v, want conflict", err)
	}
	after, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Deletion.Revision != before.Deletion.Revision || after.Title != before.Title {
		t.Fatalf("waiting deletion config mutated: before=%+v after=%+v", before, after)
	}
	if goals, _ := goalRepo.ListByWorkspaceID(ctx, workspaceID); len(goals) != 0 {
		t.Fatalf("Goal writes=%v, want none", goals)
	}
	if dods, _ := dodRepo.ListByWorkspaceID(ctx, workspaceID); len(dods) != 0 {
		t.Fatalf("DoD writes=%v, want none", dods)
	}
}
