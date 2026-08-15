package definitionquality

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCheckGoalDoDQualityEvaluatesExplicitDraft(t *testing.T) {
	t.Parallel()

	got, err := CheckGoalDoDQuality(context.Background(), magicevent.NewHub(8), Command{
		WorkspaceID: " ws-quality-port ",
		GoalText:    "实现质量预检端口迁移并保持接口行为",
		DoDItems: []string{
			"执行 go test ./internal/modules/application/workorchd/biz/appservice/... 并记录结果",
			"说明迁移范围和风险限制",
		},
	})
	if err != nil {
		t.Fatalf("CheckGoalDoDQuality: %v", err)
	}
	if got.WorkspaceID != "ws-quality-port" || got.GoalText == "" || len(got.DoDItems) != 2 {
		t.Fatalf("Check = %+v, want normalized explicit draft", got)
	}
	if !got.Executable || !got.Verifiable || got.Status != "ready" {
		t.Fatalf("Check state = executable:%v verifiable:%v status:%s, want ready", got.Executable, got.Verifiable, got.Status)
	}
}

func TestCheckGoalDoDQualityRejectsMissingHubOrWorkspace(t *testing.T) {
	t.Parallel()

	if _, err := CheckGoalDoDQuality(context.Background(), nil, Command{WorkspaceID: "ws"}); err != common.ErrInvalidArgument {
		t.Fatalf("nil hub err = %v, want ErrInvalidArgument", err)
	}
	if _, err := CheckGoalDoDQuality(context.Background(), magicevent.NewHub(8), Command{}); err != common.ErrInvalidArgument {
		t.Fatalf("empty workspace err = %v, want ErrInvalidArgument", err)
	}
}

func TestCheckGoalDoDQualityReadsExplicitDefinitionsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	defer rawHub.Terminate(ctx)
	workspaceID := common.WorkspaceID("ws-quality-scope")
	goalID := common.GoalID("goal-quality-scope")
	dodID := common.DoDID("dod-quality-scope")

	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(event magicevent.Event, result magicevent.Result) {
		cmd, ok := event.Data().(workspaceevents.GetGoalCommand)
		if !ok || cmd.WorkspaceID != workspaceID || cmd.GoalID != goalID || common.RuntimeWorkspaceIDFromContext(event.Context()) != workspaceID {
			t.Fatalf("goal command = %+v context workspace=%q", cmd, common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(workspaceevents.WorkspaceCommandResult{Goal: &workspacemodel.Goal{
			ID:          goalID,
			WorkspaceID: workspaceID,
			Text:        "- ship scoped quality check",
		}}, nil)
	})
	defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGetGoal)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(event magicevent.Event, result magicevent.Result) {
		cmd, ok := event.Data().(workspaceevents.GetDoDCommand)
		if !ok || cmd.WorkspaceID != workspaceID || cmd.DoDID != dodID || common.RuntimeWorkspaceIDFromContext(event.Context()) != workspaceID {
			t.Fatalf("dod command = %+v context workspace=%q", cmd, common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(workspaceevents.WorkspaceCommandResult{DoD: &workspacemodel.DoD{
			ID:          dodID,
			WorkspaceID: workspaceID,
			Items:       []string{"scoped quality check is verifiable"},
			State:       common.DoDStateReady,
		}}, nil)
	})
	defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGetDoD)

	got, err := CheckGoalDoDQuality(ctx, rawHub, Command{
		WorkspaceID: string(workspaceID),
		GoalID:      string(goalID),
		DoDID:       string(dodID),
	})
	if err != nil {
		t.Fatalf("CheckGoalDoDQuality err = %v", err)
	}
	if got.WorkspaceID != string(workspaceID) || got.GoalID != string(goalID) || got.DoDID != string(dodID) {
		t.Fatalf("check = %+v, want scoped goal/dod IDs", got)
	}
}
