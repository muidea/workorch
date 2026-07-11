package orchestrator

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEnableEventDrivenDispatchUsesConcreteRunStateNotification(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	const observerID = "/run/consumers/kernel/orchestrator/biz-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	received := make(chan runevents.RunStateChangedData, 1)
	observer.Subscribe(runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.RunStateChangedData); ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunStateChanged) })

	orchestrator := &RuntimeOrchestrator{
		eventer: newEventSubsystem(nil, nil),
	}
	orchestrator.EnableEventDrivenDispatch(rawHub)
	run := &runmodel.Run{
		ID:                 "run-orchestrator-typed",
		WorkspaceID:        "ws-orchestrator-typed",
		State:              common.RunStateBlockedOnDecision,
		BlockingTicketID:   "ticket-orchestrator-typed",
		BlockingApprovalID: "approval-orchestrator-typed",
	}
	if err := orchestrator.eventPublisher().publishRunStateChanged(ctx, run); err != nil {
		t.Fatalf("publish run state: %v", err)
	}

	select {
	case data := <-received:
		if data.RunID != run.ID || data.State != run.State || data.BlockingTicketID != run.BlockingTicketID {
			t.Fatalf("run notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed orchestrator run state notification not received")
	}
}

func TestEnableEventDrivenDispatchUsesConcreteStepAndLLMNotifications(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	const observerID = "/run/consumers/kernel/orchestrator/step-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	steps := make(chan runevents.StepChangedData, 1)
	progress := make(chan runevents.LLMStreamProgressData, 1)
	usage := make(chan runevents.LLMUsageRecordedData, 1)
	observer.Subscribe(runevents.TopicStepStateChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.StepChangedData); ok {
			steps <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunLLMStreamProgress, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.LLMStreamProgressData); ok {
			progress <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunLLMUsageRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.LLMUsageRecordedData); ok {
			usage <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicStepStateChanged)
		observer.Unsubscribe(runevents.TopicRunLLMStreamProgress)
		observer.Unsubscribe(runevents.TopicRunLLMUsageRecorded)
	})

	orchestrator := &RuntimeOrchestrator{
		eventer: newEventSubsystem(nil, nil),
	}
	orchestrator.EnableEventDrivenDispatch(rawHub)
	run := &runmodel.Run{ID: "run-orchestrator-step-typed", WorkspaceID: "ws-orchestrator-step-typed"}
	step := &runmodel.Step{
		ID:    "run-orchestrator-step-typed/llm",
		RunID: run.ID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateSucceeded,
		Input: map[string]any{"stage": "execution"},
		Output: map[string]any{
			"stage":  "execution",
			"status": "completed",
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": "abc"},
			},
			"contextBudget": map[string]any{"providerId": "provider-a", "model": "model-a"},
			"llmUsage": map[string]any{
				"providerId":   "provider-a",
				"model":        "model-a",
				"inputTokens":  10,
				"outputTokens": 2,
				"totalTokens":  12,
				"status":       "completed",
			},
		},
	}
	if err := orchestrator.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepStateChanged); err != nil {
		t.Fatalf("publish typed step state: %v", err)
	}

	select {
	case data := <-steps:
		if data.StepID != step.ID || data.State != step.State || data.Output.Ref == "" {
			t.Fatalf("step notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed step notification not received")
	}
	select {
	case data := <-progress:
		if data.StepID != step.ID || data.ChunkCount != 1 || data.ReceivedBytes != 3 {
			t.Fatalf("progress notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed llm progress notification not received")
	}
	select {
	case data := <-usage:
		if data.StepID != step.ID || data.TotalTokens != 12 || data.ProviderID != "provider-a" {
			t.Fatalf("usage notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed llm usage notification not received")
	}
}

func TestEnableEventDrivenDispatchUsesConcretePatchAndContinuationNotifications(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	const observerID = "/orchestrator/consumers/kernel/orchestrator/detail-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	patches := make(chan orchestratorevents.PatchChangedData, 1)
	continuations := make(chan orchestratorevents.ArtifactContinuationChangedData, 1)
	observer.Subscribe(orchestratorevents.TopicPatchApplied, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(orchestratorevents.PatchChangedData); ok {
			patches <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(orchestratorevents.TopicArtifactContinuationAssembled, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(orchestratorevents.ArtifactContinuationChangedData); ok {
			continuations <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicPatchApplied)
		observer.Unsubscribe(orchestratorevents.TopicArtifactContinuationAssembled)
	})

	orchestrator := &RuntimeOrchestrator{eventer: newEventSubsystem(nil, nil)}
	orchestrator.EnableEventDrivenDispatch(rawHub)
	run := &runmodel.Run{ID: "run-detail-typed", WorkspaceID: "ws-detail-typed"}
	step := &runmodel.Step{ID: "step-detail-typed", RunID: run.ID}
	patch := &runmodel.Patch{ID: "patch-detail-typed", WorkspaceID: run.WorkspaceID, RunID: run.ID, StepID: step.ID, PatchType: "replace", TargetRef: "result.txt", State: common.PatchStateApplied}
	continuation := &runtimecontract.ArtifactContinuation{
		ID: "continuation-detail-typed", WorkspaceID: run.WorkspaceID, RunID: run.ID, ArtifactID: "artifact-001", TargetRef: "result.txt",
		State: common.ArtifactContinuationStateAssembling, ReceivedSequences: []int{0, 1}, CoverageTargets: []string{"dod-001"}, BytesWritten: 42,
	}
	if err := orchestrator.eventPublisher().publishPatchChanged(ctx, run, step, nil, patch, orchestratorevents.TopicPatchApplied, nil); err != nil {
		t.Fatalf("publish typed patch: %v", err)
	}
	if err := orchestrator.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, continuation, orchestratorevents.TopicArtifactContinuationAssembled, nil); err != nil {
		t.Fatalf("publish typed continuation: %v", err)
	}
	select {
	case data := <-patches:
		if data.PatchID != patch.ID || data.TargetRef != patch.TargetRef || data.State != patch.State {
			t.Fatalf("patch notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed patch notification not received")
	}
	select {
	case data := <-continuations:
		if data.ContinuationID != continuation.ID || data.BytesWritten != 42 || len(data.ReceivedSequences) != 2 {
			t.Fatalf("continuation notification = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed continuation notification not received")
	}
}
