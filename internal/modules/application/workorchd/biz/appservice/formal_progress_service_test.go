package appservice

import (
	"context"
	"reflect"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestReadWorkspaceProgressDoesNotCompleteActiveEmptyAuthoring(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-unsealed-authoring-progress")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"workspace_defined", "formal_result"} {
		milestone := findFormalProgressMilestone(t, snapshot.TaskProgress, id)
		if milestone.State != "pending" || len(milestone.EvidenceRefs) != 0 {
			t.Fatalf("active 空 Workspace 不能证明正式封存或结果：%+v", milestone)
		}
	}
	if len(snapshot.Results) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("进度读取不能生成业务结果：%+v", snapshot)
	}
	assertWorkspaceFormalContextMatches(t, dataRoot, workspaceID, snapshot.TaskProgress)
}

func TestReadWorkspaceProgressIgnoresHistoricalAcceptedResultForSuccessor(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-successor-formal-progress")
	predecessorID, successorID := common.RunID("run-old-progress"), common.RunID("run-new-progress")
	const seal = "successor-progress-seal"
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: successorID, StartupInput: &workspacemodel.WorkspaceStartupInputContract{StartupInputHash: seal}}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: predecessorID, WorkspaceID: workspaceID, GoalID: "goal-progress", DoDID: "dod-progress", StartupInputHash: seal, State: common.RunStateCompleted, TerminalResultID: "result-old-progress"}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: successorID, WorkspaceID: workspaceID, GoalID: "goal-progress", DoDID: "dod-progress", StartupInputHash: seal, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{ID: "result-old-progress", WorkspaceID: workspaceID, RunID: predecessorID, GoalID: "goal-progress", DoDID: "dod-progress", State: common.ResultStateAccepted, CreatedAt: "2026-10-03T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatal(err)
	}
	milestone := findFormalProgressMilestone(t, snapshot.TaskProgress, "formal_result")
	if milestone.State == "completed" || len(milestone.EvidenceRefs) != 0 {
		t.Fatalf("历史 accepted Result 不能完成 successor：%+v", milestone)
	}
	if snapshot.TaskProgress.RunID != string(successorID) || len(snapshot.Results) != 1 {
		t.Fatalf("展示应保留历史记录但进度绑定当前 Run：%+v", snapshot.TaskProgress)
	}
	assertWorkspaceFormalContextMatches(t, dataRoot, workspaceID, snapshot.TaskProgress)
}

func assertWorkspaceFormalContextMatches(t *testing.T, dataRoot string, workspaceID common.WorkspaceID, progress projectioncontract.TaskProgress) {
	t.Helper()
	bundle, err := ReadWorkspaceContextBundleWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"workspace_defined", "formal_result"} {
		want := findFormalProgressMilestone(t, progress, id)
		got := findFormalProgressMilestone(t, bundle.TaskProgress, id)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("上下文与正式进度的 owner 事实不一致：%s，got=%+v，want=%+v", id, got, want)
		}
		found := false
		for _, milestone := range bundle.CompressedContext.Milestones {
			if milestone.ID == id {
				found = true
				if !reflect.DeepEqual(milestone, want) {
					t.Fatalf("压缩上下文不得重新推测正式完成：%+v，want=%+v", milestone, want)
				}
			}
		}
		if !found {
			t.Fatalf("压缩上下文缺少正式里程碑 %s", id)
		}
	}
}

func findFormalProgressMilestone(t *testing.T, progress projectioncontract.TaskProgress, id string) projectioncontract.ProgressMilestone {
	t.Helper()
	for _, milestone := range progress.Milestones {
		if milestone.ID == id {
			return milestone
		}
	}
	t.Fatalf("进度缺少里程碑 %s：%+v", id, progress)
	return projectioncontract.ProgressMilestone{}
}
