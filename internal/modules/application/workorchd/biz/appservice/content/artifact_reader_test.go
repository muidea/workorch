package content

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactFallbackListsArtifactsWithURIWorkspaceScope(t *testing.T) {
	rawHub := magicevent.NewHub(8)
	defer rawHub.Terminate(context.Background())
	var captured resultevents.ListArtifactsByRunCommand
	var capturedWorkspace common.WorkspaceID
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, rawHub)
	observer.Subscribe(resultevents.TopicArtifactCommandListByRun, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(resultevents.ListArtifactsByRunCommand)
		capturedWorkspace = common.RuntimeWorkspaceIDFromContext(event.Context())
		result.Set(resultevents.ListArtifactsByRunResult{Artifacts: []*resultmodel.Artifact{}}, nil)
	})
	defer observer.Unsubscribe(resultevents.TopicArtifactCommandListByRun)

	reader := NewReader(rawHub, "file", t.TempDir())
	_, err := reader.ReadArtifact(context.Background(), "workorch://workspaces/ws-artifact-scope/artifacts/run-001/result-summary.txt", nil)
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadArtifact err = %v, want not found after scoped owner list", err)
	}
	if capturedWorkspace != "ws-artifact-scope" || captured.WorkspaceID != "ws-artifact-scope" || captured.RunID != "run-001" {
		t.Fatalf("captured scope = workspace:%q run:%q, want URI workspace/run", capturedWorkspace, captured.RunID)
	}
}
