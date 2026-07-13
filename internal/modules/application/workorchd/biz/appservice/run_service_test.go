package appservice

import (
	"context"
	"errors"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateRunWithSession(context.Background(), nil, "ws-1", "run-1", "", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateRunWithBackendReturnsCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run created workspace", "", "workspaces/run created workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "created goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", []string{"created dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run-created", goal.ID, dod.ID)
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if item.ID != "run-created" || item.WorkspaceID != "ws-run-created" {
		t.Fatalf("run = %+v, want created run", item)
	}
	if item.State != string(common.RunStatePending) || item.GoalID != goal.ID || item.DoDID != dod.ID {
		t.Fatalf("run fields = %+v, want pending state with goal/dod", item)
	}
	if item.DefinitionRevision != "goal:"+goal.ID+"|dod:"+dod.ID {
		t.Fatalf("run definition revision = %q, want typed definition revision", item.DefinitionRevision)
	}
}

func TestStartRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := StartRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestResumeRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ResumeRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestStartRunWithBackendRequiresWorkspaceScope(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-start-scope", "run start scope workspace", "", "workspaces/run start scope workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-scope", "run-start-scope", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-scope"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("StartRunWithBackend err = %v, want invalid argument for missing workspace scope", err)
	}
}

func TestResumeRunWithBackendRequiresWorkspaceScope(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-scope", "run resume scope workspace", "", "workspaces/run resume scope workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-scope", "run-resume-scope", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, "run-resume-scope"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ResumeRunWithBackend err = %v, want invalid argument for missing workspace scope", err)
	}
}

func TestWorkspaceScopedRunLookupDisambiguatesDuplicateRunIDs(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	for _, workspaceID := range []string{"ws-run-lookup-a", "ws-run-lookup-b"} {
		if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, workspaceID, workspaceID, "", "workspaces/"+workspaceID); err != nil {
			t.Fatalf("CreateWorkspaceWithBackend(%s): %v", workspaceID, err)
		}
		if _, err := CreateRunWithBackend(ctx, "file", dataRoot, workspaceID, "run-001", "", ""); err != nil {
			t.Fatalf("CreateRunWithBackend(%s): %v", workspaceID, err)
		}
	}
	if _, err := ReadRunWithBackend(ctx, "file", dataRoot, "run-001"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped ReadRunWithBackend err = %v, want conflict", err)
	}

	session, sessionCtx := newTestRuntimeSession(t, "file", dataRoot)
	scopedCtx := common.ContextWithRuntimeWorkspaceID(sessionCtx, "ws-run-lookup-b")
	run, err := ReadRunWithSession(scopedCtx, session.Borrow(), "run-001")
	if err != nil {
		t.Fatalf("ReadRunWithSession: %v", err)
	}
	if run.WorkspaceID != "ws-run-lookup-b" {
		t.Fatalf("run workspace = %q, want ws-run-lookup-b", run.WorkspaceID)
	}
	status, err := ReadRunStatusWithSession(scopedCtx, session.Borrow(), "run-001")
	if err != nil {
		t.Fatalf("ReadRunStatusWithSession: %v", err)
	}
	if status.WorkspaceID != "ws-run-lookup-b" {
		t.Fatalf("status workspace = %q, want ws-run-lookup-b", status.WorkspaceID)
	}
	snapshot, err := ReadRunSnapshotWithSession(scopedCtx, session.Borrow(), "run-001")
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithSession: %v", err)
	}
	if snapshot.Run.WorkspaceID != "ws-run-lookup-b" || snapshot.Workspace.ID != "ws-run-lookup-b" {
		t.Fatalf("snapshot run/workspace = %q/%q, want ws-run-lookup-b", snapshot.Run.WorkspaceID, snapshot.Workspace.ID)
	}
}

func TestCreateRunWithBackendReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	if _, err := CreateRunWithBackend(context.Background(), "file", t.TempDir(), "ws-missing", "run-created", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestReadRunWithBackendReturnsPersistedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run read workspace", "", "workspaces/run read workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run-read", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := ReadRunWithBackend(context.Background(), "file", dataRoot, "run-read")
	if err != nil {
		t.Fatalf("ReadRunWithBackend: %v", err)
	}
	if item.ID != "run-read" || item.WorkspaceID != "ws-run-read" || item.State != string(common.RunStatePending) {
		t.Fatalf("run = %+v, want persisted run", item)
	}
}

func TestListRunsWithBackendReturnsFilteredRuns(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run workspace a", "", "workspaces/run workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-a: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run workspace b", "", "workspaces/run workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run-b", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run-a", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-a: %v", err)
	}

	items, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "")
	if err != nil {
		t.Fatalf("ListRunsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "run-a" || items[1].ID != "run-b" {
		t.Fatalf("run ids = %q/%q, want run-a/run-b", items[0].ID, items[1].ID)
	}

	filtered, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "ws-run-b")
	if err != nil {
		t.Fatalf("ListRunsWithBackend filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-b" {
		t.Fatalf("filtered runs = %+v, want run-b only", filtered)
	}
}

func TestStartRunWithBackendReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	item := prepareDecisionRun(t, dataRoot, "ws-run-start-blocked-decision", "run-start-blocked-decision")
	if item.State != "blocked_on_decision" || item.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", item)
	}
}

func TestReadRunWithSessionReturnsFailureReason(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-run-failure", Title: "run failure workspace", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: "run-failure", WorkspaceID: "ws-run-failure", State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := sendAppserviceTestCommand[orchestratorevents.FailRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, exports.Infrastructure.CommandHub, orchestratorevents.TopicOrchestratorCommandFailRun, orchestratorevents.CommandDestination, orchestratorevents.FailRunCommand{
		RunID:  "run-failure",
		Reason: "manual fail",
	}); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	item, err := ReadRunWithSession(ctx, session, "run-failure")
	if err != nil {
		t.Fatalf("ReadRunWithSession: %v", err)
	}
	if item.State != "failed" || item.FailureReason != "manual fail" {
		t.Fatalf("run = %+v, want failed/manual fail", item)
	}
}

func TestListRunsWithSessionHydratesActiveCurrentStepOnly(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-run-list-current-step")
	activeRunID := common.RunID("run-list-active-current-step")
	completedRunID := common.RunID("run-list-completed-current-step")
	activeStepID := common.StepID("run-list-active-current-step/step-1")
	completedStepID := common.StepID("run-list-completed-current-step/step-1")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: activeRunID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, item := range []*runmodel.Run{
		{ID: completedRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted},
		{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateActive},
	} {
		if err := exports.Repositories.Runs.Create(ctx, item); err != nil {
			t.Fatalf("create run %s: %v", item.ID, err)
		}
	}
	for _, item := range []*runmodel.Step{
		{ID: completedStepID, RunID: completedRunID, Kind: "llm/workspace_execution", State: common.StepStateSucceeded},
		{ID: activeStepID, RunID: activeRunID, Kind: "llm/workspace_execution", State: common.StepStateExecuting},
	} {
		if err := exports.Repositories.Steps.Create(ctx, item); err != nil {
			t.Fatalf("create step %s: %v", item.ID, err)
		}
	}

	items, err := ListRunsWithSession(ctx, session, string(workspaceID))
	if err != nil {
		t.Fatalf("ListRunsWithSession: %v", err)
	}
	byID := map[string]Run{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if got := byID[string(activeRunID)]; got.CurrentStepID != string(activeStepID) ||
		got.CurrentStepKind != "llm/workspace_execution" ||
		got.CurrentStepState != string(common.StepStateExecuting) {
		t.Fatalf("active run current step = %+v, want %s/llm/workspace_execution/dispatched", got, activeStepID)
	}
	if got := byID[string(completedRunID)]; got.CurrentStepID != "" {
		t.Fatalf("completed run current step = %+v, want list hydration skipped for terminal run", got)
	}
}
