package readmodelprojection

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestWorkspaceWithoutRunReportsAuthoringSeparately(t *testing.T) {
	for _, test := range []struct {
		state string
		want  string
	}{
		{state: "failed", want: "创作失败"},
		{state: "preparation_failed", want: "创作准备失败"},
		{state: "eligible", want: "已具备执行准入"},
		{state: "outcome_unresolved", want: "创作结果未决"},
	} {
		t.Run(test.state, func(t *testing.T) {
			snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{
				ID: "ws", State: "active", AuthoringSummary: &appmodelcore.AuthoringSummary{
					State: test.state, FailurePhase: "proposal", FailureCode: "invalid_proposal",
				},
			}}
			progress := mapLocalizedWorkspaceTaskProgress(snapshot)
			stale := taskProgressSummary(projectioncontract.TaskProgress{Runtime: &projectioncontract.RuntimeProgress{StateRevision: 1, ConvergenceState: "settled"}}, &readmodelcore.RunSnapshotState{Workspace: snapshot.Workspace})
			if !strings.Contains(stale, "执行未启动") {
				t.Fatalf("无 Run 的旧 Runtime 展示掩盖创作: %q", stale)
			}
			if !strings.Contains(progress.Summary, test.want) || !strings.Contains(progress.Summary, "执行未启动") || !strings.Contains(progress.Summary, "Result 未验收") {
				t.Fatalf("summary=%q", progress.Summary)
			}
			if progress.RunID != "" || progress.State != "active" || projectWorkspaceRuntimeView(snapshot, 10, "").RunState != "" {
				t.Fatalf("创作展示改变了 owner 状态: %+v", progress)
			}
			if test.state == "failed" && (!strings.Contains(progress.Summary, "proposal") || !strings.Contains(progress.Summary, "invalid_proposal")) {
				t.Fatalf("丢失持久失败身份: %q", progress.Summary)
			}
		})
	}
}

func TestActualRunProgressIsNotReplacedByAuthoringFailure(t *testing.T) {
	snapshot := readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws", AuthoringSummary: &appmodelcore.AuthoringSummary{State: "failed"}},
		Run:       appmodelcore.Run{ID: "run", WorkspaceID: "ws", State: "running"},
	}
	summary := taskProgressSummary(projectioncontract.TaskProgress{}, &snapshot)
	if !strings.Contains(summary, "run running") || strings.Contains(summary, "执行未启动") {
		t.Fatalf("summary=%q", summary)
	}
}
