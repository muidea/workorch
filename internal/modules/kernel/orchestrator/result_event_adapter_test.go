package orchestrator

import (
	"context"
	"strings"
	"testing"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestResultEventArtifactRepositoryPublishesArtifactScopedCommands(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	var captured events.Envelope
	artifact := &resultmodel.Artifact{
		ID:          "artifact-result-event-adapter",
		WorkspaceID: "ws-result-event-adapter",
		RunID:       "run-result-event-adapter",
		ResultID:    "result-result-event-adapter",
		State:       common.ArtifactStateAvailable,
	}
	hub.Subscribe(events.TopicArtifactCommandCreate, func(_ context.Context, envelope events.Envelope) error {
		captured = envelope
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(resultcontracts.CreateArtifactCommand)
		if !ok || cmd.Artifact != artifact {
			t.Fatalf("payload command = %#v, want CreateArtifactCommand with original artifact", envelope.Payload[events.PayloadKeyCommand])
		}
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/response",
			SourceModule: "result-test-owner",
			SubjectType:  envelope.SubjectType,
			SubjectID:    envelope.SubjectID,
			WorkspaceID:  envelope.WorkspaceID,
			RunID:        envelope.RunID,
		})
	})

	repo := resultEventArtifactRepository{hub: hub}
	if err := repo.Create(ctx, artifact); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if captured.Topic != events.TopicArtifactCommandCreate {
		t.Fatalf("topic = %q, want %q", captured.Topic, events.TopicArtifactCommandCreate)
	}
	if captured.SourceModule != ModuleID {
		t.Fatalf("source module = %q, want %q", captured.SourceModule, ModuleID)
	}
	if captured.SubjectType != "artifact" || captured.SubjectID != string(artifact.ID) {
		t.Fatalf("subject = %q/%q, want artifact/%s", captured.SubjectType, captured.SubjectID, artifact.ID)
	}
	if captured.WorkspaceID != string(artifact.WorkspaceID) || captured.RunID != string(artifact.RunID) {
		t.Fatalf("scope = workspace %q run %q, want %q/%q", captured.WorkspaceID, captured.RunID, artifact.WorkspaceID, artifact.RunID)
	}
}

func TestResultEventRepositoryRequiresOwnerResponse(t *testing.T) {
	ctx := context.Background()
	repo := resultEventResultRepository{hub: events.NewMemoryHub()}
	result := &resultmodel.Result{
		ID:          "result-no-response",
		WorkspaceID: "ws-no-response",
		RunID:       "run-no-response",
		State:       common.ResultStateAccepted,
	}

	err := repo.Create(ctx, result)
	if err == nil || !strings.Contains(err.Error(), "returned no response") {
		t.Fatalf("Create() error = %v, want missing response error", err)
	}
}

func TestResultEventRepositoriesDecodeOwnerResponses(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	result := &resultmodel.Result{
		ID:          "result-decode",
		WorkspaceID: "ws-decode",
		RunID:       "run-decode",
		State:       common.ResultStateAccepted,
	}
	artifact := &resultmodel.Artifact{
		ID:          "artifact-decode",
		WorkspaceID: result.WorkspaceID,
		RunID:       result.RunID,
		ResultID:    result.ID,
		State:       common.ArtifactStateAvailable,
	}
	hub.Subscribe(events.TopicResultCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		if envelope.SubjectType != "workspace" || envelope.SubjectID != string(result.WorkspaceID) {
			t.Fatalf("result list subject = %q/%q, want workspace/%s", envelope.SubjectType, envelope.SubjectID, result.WorkspaceID)
		}
		return events.Response(events.Envelope{
			Topic:   envelope.Topic + "/response",
			Payload: map[string]any{events.PayloadKeyItems: []*resultmodel.Result{result}},
		})
	})
	hub.Subscribe(events.TopicArtifactCommandListByResult, func(_ context.Context, envelope events.Envelope) error {
		if envelope.SubjectType != "result" || envelope.SubjectID != string(result.ID) {
			t.Fatalf("artifact list subject = %q/%q, want result/%s", envelope.SubjectType, envelope.SubjectID, result.ID)
		}
		return events.Response(events.Envelope{
			Topic:   envelope.Topic + "/response",
			Payload: map[string]any{events.PayloadKeyItems: []*resultmodel.Artifact{artifact}},
		})
	})

	results, err := (resultEventResultRepository{hub: hub}).ListByWorkspace(ctx, result.WorkspaceID)
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	if len(results) != 1 || results[0] != result {
		t.Fatalf("results = %#v, want original result", results)
	}
	artifacts, err := (resultEventArtifactRepository{hub: hub}).ListByResult(ctx, result.ID)
	if err != nil {
		t.Fatalf("ListByResult() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0] != artifact {
		t.Fatalf("artifacts = %#v, want original artifact", artifacts)
	}
}
