package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func TestWorkspaceSnapshotBuildsSnapshotFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-workspace-snapshot", Title: "workspace snapshot", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-workspace-snapshot", WorkspaceID: workspace.ID, State: common.RunStateCompleted}
	events := &fakeRunSnapshotEventReader{}

	got, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          events,
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspace.ID, nil)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	if got.Workspace.ID != string(workspace.ID) {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, workspace.ID)
	}
	if len(got.Runs) != 1 || got.Runs[0].ID != string(run.ID) {
		t.Fatalf("runs = %+v, want seeded run", got.Runs)
	}
	if events.calls != 2 {
		t.Fatalf("event reader calls = %d, want workspace events and llm usage queries", events.calls)
	}
}

func TestWorkspaceSnapshotUsesCachedRunSnapshotForProjectionRun(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-cached-run")
	runID := common.RunID("run-workspace-cached")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	cached := &readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			Title:       "cached workspace",
			State:       string(common.WorkspaceStateRunning),
			ActiveRunID: string(runID),
		},
		Run: appmodelcore.Run{
			ID:          string(runID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateRunning),
		},
		TaskProgress: readmodelcore.TaskProgress{
			RunID:   string(runID),
			Summary: "cached task progress",
		},
	}

	got, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces:            failGetWorkspaceReader{},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          &fakeRunSnapshotEventReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspaceID, cached)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	if got.Run == nil || got.Run.ID != string(runID) {
		t.Fatalf("projected run = %+v, want cached run", got.Run)
	}
	if got.TaskProgress.Summary != "cached task progress" {
		t.Fatalf("task progress summary = %q, want cached task progress", got.TaskProgress.Summary)
	}
}

type failGetWorkspaceReader struct{}

func (failGetWorkspaceReader) Get(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return nil, common.ErrNotFound
}

func (failGetWorkspaceReader) List(context.Context) ([]*workspacemodel.Workspace, error) {
	return nil, nil
}

type fakeWorkspaceSnapshotResultReader struct {
	items []*resultmodel.Result
}

func (f fakeWorkspaceSnapshotResultReader) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	ret := make([]*resultmodel.Result, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (f fakeWorkspaceSnapshotResultReader) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	ret := make([]*resultmodel.Result, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

type recordingWorkspaceEventReader struct {
	queries []repository.EventRecordQuery
}

func (r *recordingWorkspaceEventReader) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	r.queries = append(r.queries, query)
	return nil, nil
}
