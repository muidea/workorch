package qualityports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCheckGoalDoDQualityEvaluatesExplicitDraft(t *testing.T) {
	t.Parallel()

	got, err := CheckGoalDoDQuality(context.Background(), events.NewMemoryHub(), qualitycore.Command{
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

	if _, err := CheckGoalDoDQuality(context.Background(), nil, qualitycore.Command{WorkspaceID: "ws"}); err != common.ErrInvalidArgument {
		t.Fatalf("nil hub err = %v, want ErrInvalidArgument", err)
	}
	if _, err := CheckGoalDoDQuality(context.Background(), events.NewMemoryHub(), qualitycore.Command{}); err != common.ErrInvalidArgument {
		t.Fatalf("empty workspace err = %v, want ErrInvalidArgument", err)
	}
}

func TestCheckGoalDoDQualityReadsExplicitDefinitionsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-quality-scope")
	goalID := common.GoalID("goal-quality-scope")
	dodID := common.DoDID("dod-quality-scope")

	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("goal envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("goal payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyGoal: &workspacemodel.Goal{
					ID:          goalID,
					WorkspaceID: workspaceID,
					Text:        "- ship scoped quality check",
				},
			},
		})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("dod envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("dod payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyDoD: &workspacemodel.DoD{
					ID:          dodID,
					WorkspaceID: workspaceID,
					Items:       []string{"scoped quality check is verifiable"},
					State:       common.DoDStateReady,
				},
			},
		})
	})

	got, err := CheckGoalDoDQuality(ctx, hub, qualitycore.Command{
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
