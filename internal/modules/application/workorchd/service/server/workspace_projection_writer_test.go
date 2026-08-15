package server

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type recordingWorkspaceProjectionWriter struct {
	upserts             int
	deletes             int
	accepted            int
	queued              int
	rejected            int
	fullRebuilds        int
	workspaceRebuilds   []string
	decisionRebuildSets [][]string
	approvalRebuildSets [][]string
}

func (w *recordingWorkspaceProjectionWriter) UpsertWorkspace(appmodelcore.Workspace) {
	w.upserts++
}

func (w *recordingWorkspaceProjectionWriter) UpsertSnapshot(readmodelcore.WorkspaceSnapshotState) {}

func (w *recordingWorkspaceProjectionWriter) DeleteWorkspace(string) {
	w.deletes++
}

func (w *recordingWorkspaceProjectionWriter) ApplyRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate) bool {
	w.accepted++
	return true
}

func (w *recordingWorkspaceProjectionWriter) ApplyRunCommandQueued(workspaceviewcore.RunCommandQueuedUpdate) bool {
	w.queued++
	return true
}

func (w *recordingWorkspaceProjectionWriter) ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate) bool {
	w.rejected++
	return true
}

func (w *recordingWorkspaceProjectionWriter) RebuildAll(context.Context) error {
	w.fullRebuilds++
	return nil
}

func (w *recordingWorkspaceProjectionWriter) RebuildWorkspace(_ context.Context, workspaceID string) error {
	w.workspaceRebuilds = append(w.workspaceRebuilds, workspaceID)
	return nil
}

func (w *recordingWorkspaceProjectionWriter) RebuildWorkspacesForDecisions(_ context.Context, items []projectioncontract.Decision) error {
	set := make([]string, 0, len(items))
	for _, item := range items {
		set = append(set, item.WorkspaceID)
	}
	w.decisionRebuildSets = append(w.decisionRebuildSets, set)
	return nil
}

func (w *recordingWorkspaceProjectionWriter) RebuildWorkspacesForApprovals(_ context.Context, items []projectioncontract.Approval) error {
	set := make([]string, 0, len(items))
	for _, item := range items {
		set = append(set, item.WorkspaceID)
	}
	w.approvalRebuildSets = append(w.approvalRebuildSets, set)
	return nil
}

func TestProductionProjectionHelpersPreferBizOwnedWriter(t *testing.T) {
	writer := &recordingWorkspaceProjectionWriter{}
	state := &serverState{workspaceProjectionWriter: writer}

	state.upsertProjectedWorkspace(appmodelcore.Workspace{ID: "ws-writer"})
	state.deleteProjectedWorkspace("ws-writer")
	state.projectAcceptedSchedulerCommand(workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-writer", Intent: "resume"})
	state.projectQueuedSchedulerCommand(workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-writer", Intent: "resume"})
	state.projectRejectedSchedulerCommand(workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-writer", Intent: "resume"}, context.Canceled)
	state.rebuildProjectedWorkspacesForDecisions(context.Background(), []projectioncontract.Decision{{WorkspaceID: "ws-writer"}})
	state.rebuildProjectedWorkspacesForApprovals(context.Background(), []projectioncontract.Approval{{WorkspaceID: "ws-writer"}})

	if writer.upserts != 1 || writer.deletes != 1 {
		t.Fatalf("workspace writes = upsert %d/delete %d, want 1/1", writer.upserts, writer.deletes)
	}
	if writer.accepted != 1 || writer.queued != 1 || writer.rejected != 1 {
		t.Fatalf("scheduler writes = accepted %d/queued %d/rejected %d, want 1/1/1", writer.accepted, writer.queued, writer.rejected)
	}
	if len(writer.decisionRebuildSets) != 1 || len(writer.approvalRebuildSets) != 1 {
		t.Fatalf("governance rebuild calls = decisions %d/approvals %d, want 1/1", len(writer.decisionRebuildSets), len(writer.approvalRebuildSets))
	}
}
