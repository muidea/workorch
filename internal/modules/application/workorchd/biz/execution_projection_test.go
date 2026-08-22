package biz

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	projectioncomponent "github.com/muidea/workorch/internal/modules/application/workorchd/biz/projection"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestExecutionTransportFailureDoesNotProjectRunCommand(t *testing.T) {
	component := projectioncomponent.New(nil, nil, workspaceviewcore.EventPorts{})
	component.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-transport-failure", State: string(common.WorkspaceStateActive)})
	observation := executionProjection{projection: component}
	observation.RecordWorkspaceExecutionFailure(
		workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: "ws-transport-failure",
			Intent:      "resume",
			RequestID:   "request-stable",
			Source:      "http",
		},
		errors.New("transport unavailable"),
	)

	snapshot, _, err := component.ReadWorkspaceSummarySnapshot("ws-transport-failure")
	if err != nil {
		t.Fatalf("read workspace snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("transport failure fabricated owner outcome: %+v", snapshot.RunCommand)
	}

	// Closing is not required for a non-started component, but keep the test
	// lifecycle explicit if the projection later gains background resources.
	if err := component.CloseContext(context.Background()); err != nil {
		t.Fatalf("close projection: %v", err)
	}
}
