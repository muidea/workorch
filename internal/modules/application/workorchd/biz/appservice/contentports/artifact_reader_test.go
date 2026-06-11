package contentports

import (
	"context"
	"errors"
	"testing"

	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestReadArtifactFallbackListsArtifactsWithURIWorkspaceScope(t *testing.T) {
	hub := events.NewMemoryHub()
	var captured events.Envelope
	hub.Subscribe(resultevents.TopicArtifactCommandListByRun, func(ctx context.Context, envelope events.Envelope) error {
		captured = envelope
		return events.Response(events.Envelope{
			Topic:   envelope.Topic,
			Payload: map[string]any{events.PayloadKeyItems: []*resultmodel.Artifact{}},
		})
	})

	reader := NewReader(hub, "file", t.TempDir())
	_, err := reader.ReadArtifact(context.Background(), "workorch://workspaces/ws-artifact-scope/artifacts/run-001/result-summary.txt", nil)
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadArtifact err = %v, want not found after scoped owner list", err)
	}
	if captured.WorkspaceID != "ws-artifact-scope" || captured.RunID != "run-001" {
		t.Fatalf("captured scope = workspace:%q run:%q, want URI workspace/run", captured.WorkspaceID, captured.RunID)
	}
	if captured.Payload[events.PayloadKeyWorkspaceID] != "ws-artifact-scope" || captured.Payload[events.PayloadKeyRunID] != "run-001" {
		t.Fatalf("captured payload scope = workspace:%v run:%v, want URI workspace/run", captured.Payload[events.PayloadKeyWorkspaceID], captured.Payload[events.PayloadKeyRunID])
	}
}
