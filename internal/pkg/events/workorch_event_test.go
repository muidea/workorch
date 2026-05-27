package events

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecodeWorkorchEventReturnsTypedRunEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicRunCreated,
		RunID:   "run-1",
		Payload: map[string]any{
			PayloadKeyGoalID:             "goal-1",
			PayloadKeyDoDID:              "dod-1",
			PayloadKeyState:              string(common.RunStateRunning),
			PayloadKeyBlockingTicketID:   "",
			PayloadKeyBlockingApprovalID: "",
		},
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}

	runEvent, ok := event.(RunEvent)
	if !ok {
		t.Fatalf("event type = %T, want RunEvent", event)
	}
	if runEvent.Data.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want %s", runEvent.Data.State, common.RunStateRunning)
	}
	if runEvent.Data.GoalID != "goal-1" || runEvent.Data.DoDID != "dod-1" {
		t.Fatalf("run goal/dod = %s/%s, want goal-1/dod-1", runEvent.Data.GoalID, runEvent.Data.DoDID)
	}
	if runEvent.Envelope().Version != WorkorchEventVersion {
		t.Fatalf("normalized version = %s, want %s", runEvent.Envelope().Version, WorkorchEventVersion)
	}
	if runEvent.Envelope().Payload[PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("normalized payload state = %v, want %s", runEvent.Envelope().Payload[PayloadKeyState], common.RunStateRunning)
	}
}

func TestDecodeWorkorchEventReturnsTypedPatchEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicPatchApplied,
		RunID:   "run-1",
		StepID:  "run-1/step-1",
		Payload: PatchPayload(
			common.PatchID("patch-1"),
			common.StepID("run-1/step-1"),
			common.ToolCallID("run-1/step-1/tool-call"),
			"file_write",
			"result.txt",
			"write result",
			common.PatchStateApplied,
			1,
			0,
			"content-sha",
			"",
			"",
			"",
			"",
		),
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}
	patchEvent, ok := event.(PatchEvent)
	if !ok {
		t.Fatalf("event type = %T, want PatchEvent", event)
	}
	if patchEvent.Data.PatchID != "patch-1" || patchEvent.Data.TargetRef != "result.txt" || patchEvent.Data.State != common.PatchStateApplied {
		t.Fatalf("patch data = %+v, want applied patch data", patchEvent.Data)
	}
	if patchEvent.Envelope().Payload[PayloadKeyPatchID] != "patch-1" {
		t.Fatalf("normalized payload = %+v, want patch id", patchEvent.Envelope().Payload)
	}
}

func TestDecodeWorkorchEventReturnsTypedRejectedResultEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicResultRejected,
		RunID:   "run-1",
		Payload: ResultPayload(
			common.GoalID("goal-1"),
			common.DoDID("dod-1"),
			common.ResultStateRejected,
			"Rejected Result",
			"missing evidence",
			"dod_assessment",
			"run-1/step-1",
			[]string{"run-1/step-1"},
			nil,
		),
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}
	resultEvent, ok := event.(ResultEvent)
	if !ok {
		t.Fatalf("event type = %T, want ResultEvent", event)
	}
	if resultEvent.Envelope().Topic != TopicResultRejected || resultEvent.Data.State != common.ResultStateRejected {
		t.Fatalf("result event = %+v topic=%s, want rejected result topic/state", resultEvent.Data, resultEvent.Envelope().Topic)
	}
}

func TestDecodeWorkorchEventReturnsTypedLLMStreamProgressEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicRunLLMStreamProgress,
		RunID:   "run-1",
		StepID:  "run-1/step-1",
		Payload: LLMStreamProgressPayload(
			common.WorkspaceID("ws-1"),
			common.RunID("run-1"),
			common.StepID("run-1/step-1"),
			"provider-a",
			"model-a",
			4,
			512,
			120,
			"completed",
			"llm_execution_graph_plan - completed",
		),
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}
	streamEvent, ok := event.(LLMStreamProgressEvent)
	if !ok {
		t.Fatalf("event type = %T, want LLMStreamProgressEvent", event)
	}
	if streamEvent.Data.RunID != "run-1" ||
		streamEvent.Data.StepID != "run-1/step-1" ||
		streamEvent.Data.ProviderID != "provider-a" ||
		streamEvent.Data.Model != "model-a" ||
		streamEvent.Data.ChunkCount != 4 ||
		streamEvent.Data.ReceivedBytes != 512 ||
		streamEvent.Data.Status != "completed" {
		t.Fatalf("stream data = %+v, want typed progress payload", streamEvent.Data)
	}
}

func TestDecodeWorkorchEventReturnsTypedLLMUsageEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicRunLLMUsageRecorded,
		RunID:   "run-1",
		StepID:  "run-1/step-1",
		Payload: LLMUsagePayload(LLMUsagePayloadData{
			WorkspaceID:   common.WorkspaceID("ws-1"),
			RunID:         common.RunID("run-1"),
			StepID:        common.StepID("run-1/step-1"),
			ProviderID:    "provider-a",
			Provider:      "compatible",
			Model:         "model-a",
			Operation:     "chat_completions",
			InputTokens:   100,
			OutputTokens:  20,
			TotalTokens:   120,
			Estimated:     true,
			UsageSource:   "estimated",
			RequestBytes:  1024,
			ResponseBytes: 256,
			Streaming:     true,
			Status:        "completed",
		}),
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}
	usageEvent, ok := event.(LLMUsageEvent)
	if !ok {
		t.Fatalf("event type = %T, want LLMUsageEvent", event)
	}
	if usageEvent.Data.RunID != "run-1" || usageEvent.Data.StepID != "run-1/step-1" || usageEvent.Data.TotalTokens != 120 || usageEvent.Data.Status != "completed" || usageEvent.Data.UsageSource != "estimated" {
		t.Fatalf("usage event data = %+v, want typed usage payload", usageEvent.Data)
	}
}

func TestDecodeWorkorchEventRejectsIncompatibleVersion(t *testing.T) {
	_, ok := DecodeWorkorchEvent(Envelope{
		Version: "workorch.event.v2",
		Topic:   TopicRunCreated,
		RunID:   "run-1",
		Payload: RunPayload("goal-1", "dod-1", common.RunStateRunning, "", "", ""),
	})
	if ok {
		t.Fatalf("DecodeWorkorchEvent should reject incompatible version")
	}
}

func TestSubscribeWorkorchSkipsInvalidPayload(t *testing.T) {
	hub := NewMemoryHub()
	var calls int
	unsubscribers := SubscribeWorkorch(hub, func(_ context.Context, event WorkorchEvent) error {
		calls++
		return nil
	})
	defer func() {
		for _, unsubscribe := range unsubscribers {
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}()

	if err := hub.Publish(context.Background(), Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicStepStateChanged,
		StepID:  "run-1/step-1",
		Payload: map[string]any{PayloadKeyState: "completed"},
	}); err != nil {
		t.Fatalf("publish invalid step event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{
		Version:   WorkorchEventVersion,
		Topic:     TopicApprovalResolved,
		RunID:     "run-1",
		SubjectID: "approval-1",
		Payload:   ApprovalPayload("gate", "deploy", map[string]any{"risk": "external"}, common.ApprovalStateApproved),
	}); err != nil {
		t.Fatalf("publish valid approval event: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestNormalizeWorkorchEnvelopeReturnsSanitizedCopy(t *testing.T) {
	envelope, ok := NormalizeWorkorchEnvelope(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "workorch"},
		},
	})
	if !ok {
		t.Fatalf("NormalizeWorkorchEnvelope returned !ok")
	}
	if envelope.Version != WorkorchEventVersion {
		t.Fatalf("normalized version = %s, want %s", envelope.Version, WorkorchEventVersion)
	}

	payload, ok := envelope.Payload[PayloadKeyPayload].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", envelope.Payload[PayloadKeyPayload])
	}
	payload["operator"] = "mutated"

	envelope2, ok := NormalizeWorkorchEnvelope(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "workorch"},
		},
	})
	if !ok {
		t.Fatalf("second NormalizeWorkorchEnvelope returned !ok")
	}
	payload2 := envelope2.Payload[PayloadKeyPayload].(map[string]any)
	if payload2["operator"] != "workorch" {
		t.Fatalf("sanitized payload operator = %v, want workorch", payload2["operator"])
	}
}
