package server

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func TestServiceExecutionObservationDoesNotWriteProjection(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)
	const workspaceID = "ws-transport-diagnostics-only"
	svc.http.state.workspaceProjectionWriter.UpsertWorkspace(appmodelcore.Workspace{ID: workspaceID})

	svc.RecordWorkspaceExecutionTransportOutcome("workspace.run", workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: workspaceID, Intent: "resume", RequestID: "request-1"}, workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: workspaceID, RequestID: "request-1"})
	snapshot, _, err := svc.http.state.workspaceProjection.ReadWorkspaceSummarySnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projection snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("service transport observation wrote run command: %+v", snapshot.RunCommand)
	}

	// Keep teardown explicit because this fixture owns its test session.
	if err := svc.http.state.close(context.Background()); err != nil {
		t.Fatalf("close state: %v", err)
	}
}
