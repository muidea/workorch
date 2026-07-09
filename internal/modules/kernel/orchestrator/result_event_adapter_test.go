package orchestrator

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestResultEventArtifactRepositoryPublishesArtifactScopedCommands(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)
	var captured resultevents.CreateArtifactCommand
	artifact := &resultmodel.Artifact{
		ID:          "artifact-result-event-adapter",
		WorkspaceID: "ws-result-event-adapter",
		RunID:       "run-result-event-adapter",
		ResultID:    "result-result-event-adapter",
		State:       common.ArtifactStateAvailable,
	}
	subscribeResultCommand(t, rawHub, resultevents.TopicArtifactCommandCreate, func(cmd resultevents.CreateArtifactCommand, result magicevent.Result) {
		captured = cmd
		result.Set(resultevents.ResultCommandResult{}, nil)
	})

	repo := resultEventArtifactRepository{hub: hub}
	if err := repo.Create(ctx, artifact); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if captured.Artifact != artifact {
		t.Fatalf("command artifact = %#v, want original artifact", captured.Artifact)
	}
}

func TestResultEventRepositoryRequiresOwnerResponse(t *testing.T) {
	ctx := context.Background()
	repo := resultEventResultRepository{hub: events.NewMagicHubWithHub(magicevent.NewHub(16))}
	result := &resultmodel.Result{
		ID:          "result-no-response",
		WorkspaceID: "ws-no-response",
		RunID:       "run-no-response",
		State:       common.ResultStateAccepted,
	}

	err := repo.Create(ctx, result)
	if err == nil || !strings.Contains(err.Error(), "missing observer") {
		t.Fatalf("Create() error = %v, want missing observer error", err)
	}
}

func TestResultEventRepositoriesDecodeOwnerResponses(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)
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
	subscribeResultCommand(t, rawHub, resultevents.TopicResultCommandListByWorkspace, func(cmd resultevents.ListResultsByWorkspaceCommand, response magicevent.Result) {
		if cmd.WorkspaceID != result.WorkspaceID {
			t.Fatalf("workspace = %s, want %s", cmd.WorkspaceID, result.WorkspaceID)
		}
		response.Set(resultevents.ResultCommandResult{Results: []*resultmodel.Result{result}}, nil)
	})
	subscribeResultCommand(t, rawHub, resultevents.TopicArtifactCommandListByResult, func(cmd resultevents.ListArtifactsByResultCommand, response magicevent.Result) {
		if cmd.ResultID != result.ID {
			t.Fatalf("result id = %s, want %s", cmd.ResultID, result.ID)
		}
		response.Set(resultevents.ResultCommandResult{Artifacts: []*resultmodel.Artifact{artifact}}, nil)
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

func subscribeResultCommand[Command any](t *testing.T, hub magicevent.Hub, topic string, handle func(Command, magicevent.Result)) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(Command)
		if !ok {
			t.Fatalf("event %s command type = %T", topic, ev.Data())
		}
		handle(cmd, result)
	})
}
