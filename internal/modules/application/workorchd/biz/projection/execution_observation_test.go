package projection

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutionObservationWritesOnlyBizOwnedProjection(t *testing.T) {
	component := New(loaderFixture{}, nil, workspaceviewcore.EventPorts{})
	component.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-execution-observation", State: string(common.WorkspaceStateActive)})
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-execution-observation",
		Intent:      "resume",
		Reason:      "user_requested",
		RequestID:   "request-1",
		Source:      "http",
	}
	snapshot, _, err := component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read initial snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("application-local queued run command = %+v, want none before owner outcome", snapshot.RunCommand)
	}

	component.RecordWorkspaceExecutionOutcome("workspace.run", command, workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-1",
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	})
	snapshot, _, err = component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read accepted snapshot: %v", err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "accepted" || snapshot.RunCommand.RunID != "run-1" {
		t.Fatalf("accepted run command = %+v", snapshot.RunCommand)
	}

	component.RecordWorkspaceExecutionRejected("workspace.run", command, context.Canceled)
	snapshot, _, err = component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read rejected snapshot: %v", err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "rejected" || snapshot.RunCommand.Error == "" {
		t.Fatalf("rejected run command = %+v", snapshot.RunCommand)
	}
}
