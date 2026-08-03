package result

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleSetupRequiresRepositories(t *testing.T) {
	if ModuleID != "blocks/result" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if New().ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", New().ID(), ModuleID)
	}
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

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}

	if err := module.Configure(resultModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleManagesResultsAndArtifacts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
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
	sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandCreate, resultevents.CreateResultCommand{Result: result})
	results := sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{RunID: result.RunID}).Results
	if len(results) != 1 || results[0].ID != result.ID {
		t.Fatalf("results = %+v, want created result", results)
	}
	artifact := validModuleTestArtifact(result.ID)
	sendResultCommand(t, ctx, hub, resultevents.TopicArtifactCommandCreate, resultevents.CreateArtifactCommand{Artifact: artifact})
	artifacts := sendResultCommand(t, ctx, hub, resultevents.TopicArtifactCommandListByResult, resultevents.ListArtifactsByResultCommand{ResultID: result.ID}).Artifacts
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifacts = %+v, want created artifact", artifacts)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("biz after teardown = %v, want nil", module.bizPtr)
	}
}

func TestModuleRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	module := New()
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}); err != nil {
		t.Fatalf("Setup after Teardown: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run after Teardown: %v", err)
	}
}

func TestModuleRejectsInvalidResultCommandWithoutSideEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	resultRepo := memory.NewResultRepository()
	module := New()
	if err := module.Configure(Dependencies{
		Results:   resultRepo,
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result := hub.Send(magicevent.NewEventWithContext(
		resultevents.TopicResultCommandCreate,
		"test",
		resultevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		resultevents.CreateResultCommand{},
	))
	if result == nil || result.Error() == nil {
		t.Fatalf("invalid create result = %v, want an error", result)
	}
	items, err := resultRepo.ListByRun(ctx, "run-result-invalid")
	if err != nil {
		t.Fatalf("ListByRun after invalid command: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want no side effects from invalid command", items)
	}
}

func TestModuleHandlesResultCommandsOverEventHub(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
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
	_ = sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandCreate, resultevents.CreateResultCommand{Result: result})
	response := sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{RunID: result.RunID})
	if len(response.Results) != 1 || response.Results[0].ID != result.ID {
		t.Fatalf("items = %+v, want created result", response.Results)
	}
}

func TestResultOwnerCommandsPublishLifecycleNotifications(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	const observerID = "/result/consumers/blocks/result/owner-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	results := make(chan struct {
		topic string
		data  resultevents.ResultChangedData
	}, 2)
	artifacts := make(chan resultevents.ArtifactCreatedData, 1)
	for _, topic := range []string{resultevents.TopicResultAccepted, resultevents.TopicResultRejected} {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(resultevents.ResultChangedData); ok {
				results <- struct {
					topic string
					data  resultevents.ResultChangedData
				}{topic: topic, data: data}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	observer.Subscribe(resultevents.TopicArtifactCreated, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(resultevents.ArtifactCreatedData); ok {
			artifacts <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(resultevents.TopicResultAccepted)
		observer.Unsubscribe(resultevents.TopicResultRejected)
		observer.Unsubscribe(resultevents.TopicArtifactCreated)
	})

	item := validModuleTestResult()
	createdResult := sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandCreate, resultevents.CreateResultCommand{Result: item})
	if createdResult.Result == nil {
		t.Fatal("create result response is missing owner clone")
	}
	item = createdResult.Result
	created := receiveResultNotification(t, results)
	if created.topic != resultevents.TopicResultAccepted || created.data.ResultID != item.ID || created.data.State != item.State {
		t.Fatalf("created notification = %+v", created)
	}

	item.State = common.ResultStateRejected
	item.Summary = "rejected"
	updatedResult := sendResultCommand(t, ctx, hub, resultevents.TopicResultCommandUpdate, resultevents.UpdateResultCommand{Result: item})
	if updatedResult.Result == nil || updatedResult.Result.Revision <= item.Revision {
		t.Fatalf("update result response = %+v, want advanced owner revision", updatedResult.Result)
	}
	updated := receiveResultNotification(t, results)
	if updated.topic != resultevents.TopicResultRejected || updated.data.ResultID != item.ID || updated.data.Summary != "rejected" {
		t.Fatalf("updated notification = %+v", updated)
	}

	artifact := validModuleTestArtifact(item.ID)
	sendResultCommand(t, ctx, hub, resultevents.TopicArtifactCommandCreate, resultevents.CreateArtifactCommand{Artifact: artifact})
	select {
	case data := <-artifacts:
		if data.ArtifactID != artifact.ID || data.ResultID != item.ID || data.State != artifact.State {
			t.Fatalf("artifact notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed artifact notification not received")
	}
}

func receiveResultNotification(t *testing.T, received <-chan struct {
	topic string
	data  resultevents.ResultChangedData
}) struct {
	topic string
	data  resultevents.ResultChangedData
} {
	t.Helper()
	select {
	case item := <-received:
		return item
	case <-time.After(time.Second):
		t.Fatal("typed result notification not received")
		return struct {
			topic string
			data  resultevents.ResultChangedData
		}{}
	}
}

func resultModuleContractDependencies() Dependencies {
	return Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}
}

func sendResultCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) resultevents.ResultCommandResult {
	t.Helper()
	ev := magicevent.NewEventWithContext(topic, "test", resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(resultevents.ResultCommandResult)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
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
