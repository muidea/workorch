package governance

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEventWorkspaceReaderCarriesRuntimeWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	workspaceID := common.WorkspaceID("ws-governance-definition-scope")
	goalID := common.GoalID("goal-governance-definition-scope")
	dodID := common.DoDID("dod-governance-definition-scope")
	var goalCommand workspaceevents.GetGoalCommand
	var dodCommand workspaceevents.GetDoDCommand
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub)
	defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGetGoal)
	defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGetDoD)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(workspaceevents.GetGoalCommand)
		if !ok {
			t.Fatalf("goal command type = %T", ev.Data())
		}
		goalCommand = cmd
		result.Set(workspaceevents.GoalResult{Goal: &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "goal"}}, nil)
	})
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(workspaceevents.GetDoDCommand)
		if !ok {
			t.Fatalf("dod command type = %T", ev.Data())
		}
		dodCommand = cmd
		result.Set(workspaceevents.DoDResult{DoD: &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"dod"}, State: common.DoDStateReady}}, nil)
	})
	reader := eventWorkspaceReader{send: rawHub.Send}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

	if _, err := reader.GetGoal(scopedCtx, goalID); err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if _, err := reader.GetDoD(scopedCtx, dodID); err != nil {
		t.Fatalf("GetDoD: %v", err)
	}
	if goalCommand.WorkspaceID != workspaceID || goalCommand.GoalID != goalID {
		t.Fatalf("goal command = %+v, want %s/%s", goalCommand, workspaceID, goalID)
	}
	if dodCommand.WorkspaceID != workspaceID || dodCommand.DoDID != dodID {
		t.Fatalf("dod command = %+v, want %s/%s", dodCommand, workspaceID, dodID)
	}
}
