package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunSnapshotViewsBuildDerivedViewsFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-snapshot-views", Title: "run snapshot views", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-snapshot-views", WorkspaceID: workspace.ID, State: common.RunStateActive}
	steps := []*runmodel.Step{{ID: "step-snapshot-views", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateSucceeded}}
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

	workspace := &workspacemodel.Workspace{ID: "ws-run-observability-events", Title: "run observability events", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-observability-events", WorkspaceID: workspace.ID, State: common.RunStateActive}
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
		t.Fatalf("observability exclude topics = %+v, want complete event timeline including %s", events.queries[0].ExcludeTopics, "/run/llm_stream/progress")
	}
	if !events.queries[0].IncludePayload {
		t.Fatalf("observability include payload = false, want true")
	}
}

func TestRunSnapshotProjectionUsesCompactEventTimeline(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-projection-events", Title: "run projection events", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-projection-events", WorkspaceID: workspace.ID, State: common.RunStateActive}
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
	if len(events.queries) < 2 {
		t.Fatalf("event reader queries = %d, want metadata timeline + llm usage queries", len(events.queries))
	}
	var sawMetadata, sawUsage bool
	for _, query := range events.queries {
		if query.Topic == "/run/llm_usage/recorded" {
			sawUsage = true
			if !query.IncludePayload {
				t.Fatalf("usage query = %+v, want IncludePayload", query)
			}
			continue
		}
		sawMetadata = true
		if query.IncludePayload {
			t.Fatalf("projection include payload = true, want compact metadata-only query: %+v", query)
		}
		if len(query.ExcludeTopics) != 1 || query.ExcludeTopics[0] != "/run/llm_stream/progress" {
			t.Fatalf("projection exclude topics = %+v, want stream progress excluded", query.ExcludeTopics)
		}
	}
	if !sawMetadata || !sawUsage {
		t.Fatalf("queries = %+v, want metadata timeline + llm usage payload query", events.queries)
	}
}

func TestRunSnapshotViewHelpersProjectDirectSnapshots(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.RunSnapshotState{
		Run: appmodelcore.Run{
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
