package result

import (
	"context"
	"errors"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleSetupRequiresRepositories(t *testing.T) {
	if err := New().Configure(Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup err = %v, want invalid argument", err)
	}
	if err := New().Configure(Dependencies{Results: memory.NewResultRepository()}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup with missing artifacts err = %v, want invalid argument", err)
	}
	if err := New().Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup with missing EventHub err = %v, want invalid argument", err)
	}
}

func TestModuleServiceManagesResultsAndArtifacts(t *testing.T) {
	ctx := context.Background()
	module := New()
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  events.NewMemoryHub(),
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	service := module.Service()
	if service == nil {
		t.Fatalf("Service nil, want configured service")
	}
	result := validModuleTestResult()
	if err := service.CreateResult(ctx, result); err != nil {
		t.Fatalf("CreateResult: %v", err)
	}
	results, err := service.ListResultsByRun(ctx, result.RunID)
	if err != nil {
		t.Fatalf("ListResultsByRun: %v", err)
	}
	if len(results) != 1 || results[0].ID != result.ID {
		t.Fatalf("results = %+v, want created result", results)
	}
	artifact := validModuleTestArtifact(result.ID)
	if err := service.CreateArtifact(ctx, artifact); err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	artifacts, err := service.ListArtifactsByResult(ctx, result.ID)
	if err != nil {
		t.Fatalf("ListArtifactsByResult: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifacts = %+v, want created artifact", artifacts)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if module.Service() != nil {
		t.Fatalf("Service after teardown = %v, want nil", module.Service())
	}
}

func TestModuleRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	module := New()
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  events.NewMemoryHub(),
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	firstSubCount := len(module.unsub)
	if firstSubCount == 0 {
		t.Fatal("first Run did not subscribe")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(module.unsub) != firstSubCount {
		t.Fatalf("subscription count = %d, want %d", len(module.unsub), firstSubCount)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  events.NewMemoryHub(),
	}); err != nil {
		t.Fatalf("Setup after Teardown: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run after Teardown: %v", err)
	}
	if len(module.unsub) != firstSubCount {
		t.Fatalf("subscription count after rerun = %d, want %d", len(module.unsub), firstSubCount)
	}
}

func TestModuleHandlesResultCommandsOverEventHub(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	module := New()
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	result := validModuleTestResult()
	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicResultCommandCreate,
		WorkspaceID: string(result.WorkspaceID),
		RunID:       string(result.RunID),
		Payload: map[string]any{
			events.PayloadKeyResult: result,
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("create command err = %v, want response", err)
	}
	err = hub.Publish(ctx, events.Envelope{
		Topic: events.TopicResultCommandListByRun,
		Payload: map[string]any{
			events.PayloadKeyRunID: string(result.RunID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list command err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok || len(items) != 1 || items[0].ID != result.ID {
		t.Fatalf("items = %+v, want created result", response.Payload[events.PayloadKeyItems])
	}
}

func validModuleTestResult() *resultmodel.Result {
	return &resultmodel.Result{
		ID:          "result-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		GoalID:      "goal-result-test",
		DoDID:       "dod-result-test",
		State:       common.ResultStateAccepted,
		Title:       "accepted result",
		Summary:     "accepted",
	}
}

func validModuleTestArtifact(resultID common.ResultID) *resultmodel.Artifact {
	return &resultmodel.Artifact{
		ID:          "artifact-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		ResultID:    resultID,
		State:       common.ArtifactStateCreated,
		Name:        "artifact",
		Summary:     "artifact",
	}
}
