package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunSnapshotViewsBuildDerivedViewsFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-snapshot-views", Title: "run snapshot views", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-snapshot-views", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	steps := []*runmodel.Step{{ID: "step-snapshot-views", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateCompleted}}
	readers := RunSnapshotReaders{
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
		EventRecords:          &fakeRunSnapshotEventReader{},
		MemoryRecalls:         fakeRunSnapshotMemoryRecallReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}

	progress, err := RunTaskProgress(context.Background(), readers, run.ID)
	if err != nil {
		t.Fatalf("RunTaskProgress: %v", err)
	}
	if progress.RunID != string(run.ID) {
		t.Fatalf("progress run ID = %q, want %q", progress.RunID, run.ID)
	}

	observability, err := RunObservability(context.Background(), readers, run.ID)
	if err != nil {
		t.Fatalf("RunObservability: %v", err)
	}
	if observability.Run.ID != string(run.ID) {
		t.Fatalf("observability run ID = %q, want %q", observability.Run.ID, run.ID)
	}

	bundle, err := RunContextBundle(context.Background(), readers, run.ID)
	if err != nil {
		t.Fatalf("RunContextBundle: %v", err)
	}
	if bundle.RunID != string(run.ID) {
		t.Fatalf("context bundle run ID = %q, want %q", bundle.RunID, run.ID)
	}

	if _, err := RunTaskDecompositions(context.Background(), readers, run.ID); err != nil {
		t.Fatalf("RunTaskDecompositions: %v", err)
	}
}

func TestRunObservabilityUsesCompleteEventTimeline(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-observability-events", Title: "run observability events", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-observability-events", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	events := &fakeRunSnapshotEventReader{}

	_, err := RunObservability(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{},
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
	}, run.ID)
	if err != nil {
		t.Fatalf("RunObservability: %v", err)
	}
	if len(events.queries) < 1 {
		t.Fatalf("event reader queries = %d, want observability event query", len(events.queries))
	}
	if len(events.queries[0].ExcludeTopics) != 0 {
		t.Fatalf("observability exclude topics = %+v, want complete event timeline including %s", events.queries[0].ExcludeTopics, runevents.TopicRunLLMStreamProgress)
	}
	if !events.queries[0].IncludePayload {
		t.Fatalf("observability include payload = false, want true")
	}
}

func TestRunSnapshotProjectionUsesCompactEventTimeline(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-projection-events", Title: "run projection events", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-projection-events", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	events := &fakeRunSnapshotEventReader{}

	_, err := RunSnapshot(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{},
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
	}, run.ID)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}
	if len(events.queries) < 1 {
		t.Fatalf("event reader queries = %d, want projection event query", len(events.queries))
	}
	if events.queries[0].IncludePayload {
		t.Fatalf("projection include payload = true, want compact metadata-only query")
	}
	if len(events.queries[0].ExcludeTopics) != 1 || events.queries[0].ExcludeTopics[0] != runevents.TopicRunLLMStreamProgress {
		t.Fatalf("projection exclude topics = %+v, want stream progress excluded", events.queries[0].ExcludeTopics)
	}
}

func TestRunSnapshotViewHelpersProjectDirectSnapshots(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.RunSnapshot{
		Run: readmodelcore.Run{
			ID:          "run-direct-snapshot-view",
			WorkspaceID: "ws-direct-snapshot-view",
			State:       string(common.RunStateCompleted),
		},
	}
	if got := RunStatusFromSnapshot(snapshot); got.ID != snapshot.Run.ID {
		t.Fatalf("status ID = %q, want %q", got.ID, snapshot.Run.ID)
	}
	if got := RunObservabilityFromSnapshot(snapshot); got.Run.ID != snapshot.Run.ID {
		t.Fatalf("observability run ID = %q, want %q", got.Run.ID, snapshot.Run.ID)
	}
	if got := ProjectRunSnapshotDetail(snapshot); got.Run.ID != snapshot.Run.ID {
		t.Fatalf("projected run ID = %q, want %q", got.Run.ID, snapshot.Run.ID)
	}
}
