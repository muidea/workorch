package qualityports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
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
