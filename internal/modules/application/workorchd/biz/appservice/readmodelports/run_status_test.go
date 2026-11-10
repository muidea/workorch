package readmodelports

import (
	"context"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunStatusPrefersBlockingDecisionStep(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:               "run-status",
		WorkspaceID:      "ws-status",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-blocking",
	}
	steps := []*runmodel.Step{
		{ID: "step-stale", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateSucceeded},
		{ID: "step-blocking", RunID: run.ID, Kind: "approval", State: common.StepStatePending},
	}
	checkpoints := []*runmodel.RuntimeCheckpoint{
		{ID: "checkpoint-stale", WorkspaceID: run.WorkspaceID, RunID: run.ID, CurrentStepID: "step-stale"},
	}
	tickets := []*decisionmodel.Ticket{
		{ID: "ticket-blocking", WorkspaceID: run.WorkspaceID, RunID: run.ID, StepID: "step-blocking"},
	}

	got, err := RunStatus(context.Background(), RunStatusReaders{
		Runs:        fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:       fakeRunStatusStepReader{items: steps},
		Checkpoints: fakeRunStatusCheckpointReader{items: checkpoints},
		Tickets:     fakeTicketReader{items: tickets},
		Approvals:   fakeApprovalListReader{},
	}, run.ID)
	if err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if got.CurrentStepID != "step-blocking" {
		t.Fatalf("CurrentStepID = %q, want step-blocking", got.CurrentStepID)
	}
	if got.StepCount != 2 || got.DecisionCount != 1 || got.ApprovalCount != 0 {
		t.Fatalf("counts = steps:%d decisions:%d approvals:%d, want 2/1/0", got.StepCount, got.DecisionCount, got.ApprovalCount)
	}
}

func TestRunStatusUsesLatestCheckpointStep(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{ID: "run-checkpoint-status", WorkspaceID: "ws-checkpoint-status", State: common.RunStateActive}
	steps := []*runmodel.Step{
		{ID: "step-old", RunID: run.ID, Kind: "llm/planning", State: common.StepStateSucceeded},
		{ID: "step-current", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateExecuting},
	}
	checkpoints := []*runmodel.RuntimeCheckpoint{
		{ID: "checkpoint-current", WorkspaceID: run.WorkspaceID, RunID: run.ID, CurrentStepID: "step-current"},
	}

	got, err := RunStatus(context.Background(), RunStatusReaders{
		Runs:        fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:       fakeRunStatusStepReader{items: steps},
		Checkpoints: fakeRunStatusCheckpointReader{items: checkpoints},
		Tickets:     fakeTicketReader{},
		Approvals:   fakeApprovalListReader{},
	}, run.ID)
	if err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if got.CurrentStepID != "step-current" {
		t.Fatalf("CurrentStepID = %q, want step-current", got.CurrentStepID)
	}
	if got.CurrentStepKind != "llm/workspace_execution" {
		t.Fatalf("CurrentStepKind = %q, want llm/workspace_execution", got.CurrentStepKind)
	}
	if got.CurrentStepState != string(common.StepStateExecuting) {
		t.Fatalf("CurrentStepState = %q, want dispatched", got.CurrentStepState)
	}
}

type fakeRunStatusRunReader struct {
	items []*runmodel.Run
}

func (f fakeRunStatusRunReader) Get(_ context.Context, id common.RunID) (*runmodel.Run, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeRunStatusRunReader) GetByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeRunStatusRunReader) List(context.Context) ([]*runmodel.Run, error) {
	return append([]*runmodel.Run(nil), f.items...), nil
}

func (f fakeRunStatusRunReader) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	ret := make([]*runmodel.Run, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

type fakeRunStatusStepReader struct {
	items []*runmodel.Step
}

func (f fakeRunStatusStepReader) ListByRun(_ context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	ret := make([]*runmodel.Step, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

type fakeRunStatusCheckpointReader struct {
	items []*runmodel.RuntimeCheckpoint
}

func (f fakeRunStatusCheckpointReader) ListByRun(_ context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	ret := make([]*runmodel.RuntimeCheckpoint, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (f fakeRunStatusRunReader) ListCompact(ctx context.Context) ([]runmodel.RunInventoryItem, error) {
	items, err := f.List(ctx)
	return testRunInventory(items), err
}

func (f fakeRunStatusRunReader) ListCompactByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]runmodel.RunInventoryItem, error) {
	items, err := f.ListByWorkspaceID(ctx, workspaceID)
	return testRunInventory(items), err
}

type snapshotInventoryOnlyRunReader struct {
	fakeRunStatusRunReader
	calls int
}

func (r *snapshotInventoryOnlyRunReader) ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*runmodel.Run, error) {
	panic("snapshot base must not read historical Runtime aggregates")
}
func (r *snapshotInventoryOnlyRunReader) ListCompactByWorkspaceID(_ context.Context, id common.WorkspaceID) ([]runmodel.RunInventoryItem, error) {
	r.calls++
	return []runmodel.RunInventoryItem{{ID: "selected", WorkspaceID: id, WorkspaceRunGeneration: 3, TerminalResultID: "result", State: common.RunStateFailed}}, nil
}
func TestWorkspaceSnapshotBaseUsesScopedRunInventory(t *testing.T) {
	r := &snapshotInventoryOnlyRunReader{}
	var models workspaceSnapshotModels
	tasks := workspaceSnapshotBaseLoadTasks(context.Background(), WorkspaceSnapshotReaders{Runs: r}, "ws", nil, workspaceSnapshotLoadOptions{}, &models)
	if err := tasks[0](); err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 || len(models.Runs) != 1 || models.Runs[0].WorkspaceID != "ws" || models.Runs[0].TerminalResultID != "result" || models.Runs[0].WorkspaceRunGeneration != 3 {
		t.Fatalf("snapshot inventory: %+v", models.Runs)
	}
}
