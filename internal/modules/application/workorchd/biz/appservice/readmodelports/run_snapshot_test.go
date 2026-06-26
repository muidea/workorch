package readmodelports

import (
	"context"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSnapshotBuildsSnapshotFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-snapshot", Title: "run snapshot", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-snapshot", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	steps := []*runmodel.Step{{ID: "step-snapshot", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateDispatched}}
	events := &fakeRunSnapshotEventReader{}

	got, err := RunSnapshot(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{items: steps},
		ToolCalls:             fakeRunSnapshotToolCallReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Validations:           fakeRunSnapshotValidationReader{},
		Checkpoints:           fakeRunStatusCheckpointReader{},
		Results:               fakeRunSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          events,
		MemoryRecalls:         fakeRunSnapshotMemoryRecallReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
		MCPCapabilities:       fakeRunSnapshotMCPCapabilityReader{},
	}, run.ID)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}
	if got.Run.ID != string(run.ID) || got.Workspace.ID != string(workspace.ID) {
		t.Fatalf("snapshot identity = run:%s workspace:%s, want seeded IDs", got.Run.ID, got.Workspace.ID)
	}
	if len(got.Steps) != 1 || got.Steps[0].ID != string(steps[0].ID) {
		t.Fatalf("steps = %+v, want seeded step", got.Steps)
	}
	if events.calls != 2 {
		t.Fatalf("event reader calls = %d, want run events and llm usage queries", events.calls)
	}
	if len(events.queries) != 2 {
		t.Fatalf("event reader queries = %d, want run events and llm usage queries", len(events.queries))
	}
	if len(events.queries[0].ExcludeTopics) != 1 || events.queries[0].ExcludeTopics[0] != runevents.TopicRunLLMStreamProgress {
		t.Fatalf("snapshot event exclude topics = %+v, want stream progress excluded", events.queries[0].ExcludeTopics)
	}
}

type fakeRunSnapshotWorkspaceReader struct {
	items []*workspacemodel.Workspace
}

func (f fakeRunSnapshotWorkspaceReader) Get(_ context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeRunSnapshotWorkspaceReader) List(context.Context) ([]*workspacemodel.Workspace, error) {
	return append([]*workspacemodel.Workspace(nil), f.items...), nil
}

type fakeRunSnapshotToolCallReader struct{}

func (fakeRunSnapshotToolCallReader) ListByRun(context.Context, common.RunID) ([]*runmodel.ToolCall, error) {
	return nil, nil
}

type fakeRunSnapshotPatchReader struct{}

func (fakeRunSnapshotPatchReader) ListByRun(context.Context, common.RunID) ([]*runmodel.Patch, error) {
	return nil, nil
}

type fakeRunSnapshotValidationReader struct{}

func (fakeRunSnapshotValidationReader) ListByRun(context.Context, common.RunID) ([]*runmodel.ValidationResult, error) {
	return nil, nil
}

type fakeRunSnapshotResultReader struct{}

func (fakeRunSnapshotResultReader) ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error) {
	return nil, nil
}

func (fakeRunSnapshotResultReader) ListByRun(context.Context, common.RunID) ([]*resultmodel.Result, error) {
	return nil, nil
}

type fakeRunSnapshotArtifactReader struct{}

func (fakeRunSnapshotArtifactReader) ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

func (fakeRunSnapshotArtifactReader) ListByRun(context.Context, common.RunID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

func (fakeRunSnapshotArtifactReader) ListByResult(context.Context, common.ResultID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

type fakeRunSnapshotArtifactContinuationReader struct{}

func (fakeRunSnapshotArtifactContinuationReader) ListByRun(context.Context, common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	return nil, nil
}

type fakeRunSnapshotEventReader struct {
	calls   int
	queries []repository.EventRecordQuery
}

func (f *fakeRunSnapshotEventReader) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	f.calls++
	f.queries = append(f.queries, query)
	return nil, nil
}

type fakeRunSnapshotMemoryRecallReader struct{}

func (fakeRunSnapshotMemoryRecallReader) List(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	return nil, nil
}

type fakeRunSnapshotMCPCapabilityReader struct{}

func (fakeRunSnapshotMCPCapabilityReader) List(context.Context) ([]*mcpmodel.Capability, error) {
	return nil, nil
}
