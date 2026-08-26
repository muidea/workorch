package content

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactUsesResultOwnerContentCommandWithURIWorkspaceScope(t *testing.T) {
	rawHub := magicevent.NewHub(8)
	defer rawHub.Terminate(context.Background())
	var captured resultevents.ReadArtifactContentCommand
	var capturedWorkspace common.WorkspaceID
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, rawHub)
	observer.Subscribe(resultevents.TopicArtifactCommandReadContent, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(resultevents.ReadArtifactContentCommand)
		capturedWorkspace = common.RuntimeWorkspaceIDFromContext(event.Context())
		result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
	})
	defer observer.Unsubscribe(resultevents.TopicArtifactCommandReadContent)

	reader := NewReader(rawHub)
	_, err := reader.ReadArtifact(context.Background(), "workorch://workspaces/ws-artifact-scope/artifacts/run-001/result-summary.txt")
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadArtifact err = %v, want not found after scoped owner list", err)
	}
	if capturedWorkspace != "ws-artifact-scope" || captured.WorkspaceID != "ws-artifact-scope" || captured.RunID != "run-001" || captured.URI == "" {
		t.Fatalf("captured scope = workspace:%q run:%q, want URI workspace/run", capturedWorkspace, captured.RunID)
	}
}
