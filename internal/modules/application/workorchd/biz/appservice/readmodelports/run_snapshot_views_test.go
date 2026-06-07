package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
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
