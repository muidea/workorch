package web

import (
	"context"
	"testing"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchownercontract"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWebApprovalGetterPublishesScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	approvalID := common.ApprovalID("approval-web-scope")
	runID := common.RunID("run-web-scope")
	var captured events.Envelope
	hub.Subscribe(workorchownercontract.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		captured = envelope
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &workorchownercontract.Approval{
					ID:    approvalID,
					RunID: runID,
					State: common.ApprovalStateApproved,
				},
			},
		})
	})

	item, err := (webApprovalGetter{hub: hub}).GetForRun(ctx, approvalID, runID)
	if err != nil {
		t.Fatalf("GetForRun() error = %v", err)
	}
	if item == nil || item.ID != approvalID {
		t.Fatalf("approval = %+v, want %s", item, approvalID)
	}
	if captured.Topic != workorchownercontract.TopicApprovalCommandGet {
		t.Fatalf("topic = %q, want %q", captured.Topic, workorchownercontract.TopicApprovalCommandGet)
	}
	if captured.SourceModule != ModuleID || captured.SubjectType != "approval" || captured.SubjectID != string(approvalID) {
		t.Fatalf("envelope source/subject = %q %q/%q", captured.SourceModule, captured.SubjectType, captured.SubjectID)
	}
	if captured.RunID != string(runID) || captured.Payload[events.PayloadKeyRunID] != string(runID) {
		t.Fatalf("run scope = envelope %q payload %q, want %q", captured.RunID, captured.Payload[events.PayloadKeyRunID], runID)
	}
}

func TestWebModuleQueuesRuntimeExecution(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMagicHub()
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)

	module := New()
	if err := module.Configure(Dependencies{EventHub: hub, Background: background}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer module.Stop(ctx)

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runtimeevent.RuntimeBlockExecuteTopic(StepKindSearch),
		WorkspaceID: "ws-web-queue",
		RunID:       "run-web-queue",
		StepID:      "run-web-queue/step-1",
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequestID: "web-queue-request",
			events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
				WorkspaceID: "ws-web-queue",
				RunID:       "run-web-queue",
				StepID:      "run-web-queue/step-1",
				Kind:        StepKindSearch,
				Input:       map[string]any{"query": "workorch"},
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("runtime command err = %v, want accepted response", err)
	}
	if _, ok := response.Payload[events.PayloadKeyRuntimeAccepted].(runtimemodel.RuntimeStepExecutionAccepted); !ok {
		t.Fatalf("accepted payload missing: %#v", response.Payload)
	}
}
