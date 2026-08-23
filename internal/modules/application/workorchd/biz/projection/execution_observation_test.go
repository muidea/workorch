package projection

import (
	"encoding/json"
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
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentResume,
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	})
	snapshot, status, err := component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read accepted snapshot: %v", err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "accepted" || snapshot.RunCommand.RunID != "run-1" {
		t.Fatalf("accepted run command = %+v", snapshot.RunCommand)
	}
	if snapshot.RunCommand.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || snapshot.RunCommand.AdmissionReason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("accepted owner admission = %+v", snapshot.RunCommand)
	}
	if snapshot.RunCommand.Revision <= 0 || snapshot.RunCommand.Revision != status.WorkspaceRevision {
		t.Fatalf("accepted run command revision = %d, projection status=%+v", snapshot.RunCommand.Revision, status)
	}
	acceptedRevision := snapshot.RunCommand.Revision
	encoded, err := json.Marshal(snapshot.RunCommand)
	if err != nil {
		t.Fatalf("marshal accepted run command: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal accepted run command: %v", err)
	}
	var encodedRevision int64
	if raw, ok := fields["revision"]; !ok || json.Unmarshal(raw, &encodedRevision) != nil || encodedRevision != acceptedRevision {
		t.Fatalf("production run command JSON revision = %s, want %d", raw, acceptedRevision)
	}
	component.RecordWorkspaceExecutionOutcome("workspace.run", command, workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-mutated",
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentResume,
		Outcome:     runtimecontract.RuntimeExecutionIntentRestartRequired,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
	})
	snapshot, status, err = component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read immutable snapshot: %v", err)
	}
	if snapshot.RunCommand.State != "accepted" || snapshot.RunCommand.RunID != "run-1" || snapshot.RunCommand.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested {
		t.Fatalf("duplicate request rewrote immutable owner admission: %+v", snapshot.RunCommand)
	}
	if snapshot.RunCommand.Revision != acceptedRevision || status.WorkspaceRevision != acceptedRevision {
		t.Fatalf("duplicate request advanced projection revision: command=%+v status=%+v", snapshot.RunCommand, status)
	}

	rejectedCommand := command
	rejectedCommand.RequestID = "request-2"
	component.RecordWorkspaceExecutionOutcome("workspace.run", rejectedCommand, workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: rejectedCommand.WorkspaceID,
		RequestID:   rejectedCommand.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentResume,
		Outcome:     runtimecontract.RuntimeExecutionIntentRestartRequired,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
	})
	snapshot, status, err = component.ReadWorkspaceSummarySnapshot(string(command.WorkspaceID))
	if err != nil {
		t.Fatalf("read rejected snapshot: %v", err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "rejected" || snapshot.RunCommand.Diagnostic == "" {
		t.Fatalf("rejected run command = %+v", snapshot.RunCommand)
	}
	if snapshot.RunCommand.Outcome != runtimecontract.RuntimeExecutionIntentRestartRequired || snapshot.RunCommand.AdmissionReason != runtimecontract.RuntimeExecutionIntentReasonRunNotResumable {
		t.Fatalf("rejected owner admission = %+v", snapshot.RunCommand)
	}
	if snapshot.RunCommand.Revision <= acceptedRevision || snapshot.RunCommand.Revision != status.WorkspaceRevision {
		t.Fatalf("rejected run command revision = %d, previous=%d status=%+v", snapshot.RunCommand.Revision, acceptedRevision, status)
	}
}
