package run

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRunCommandGetHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-get")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-a",
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicRunCommandGet,
		WorkspaceID: "ws-run-owner-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-owner-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicRunCommandGet,
		WorkspaceID: "ws-run-owner-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-owner-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil || item.WorkspaceID != "ws-run-owner-a" || item.ID != runID {
		t.Fatalf("response run = %+v, want scoped run", response.Payload[events.PayloadKeyRun])
	}
}

func TestRunCommandEnsureResumedHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-resume")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      "ws-run-resume-a",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-scoped-resume",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orchestrator := &fakeRunOrchestrator{}
	module := New()
	if err := module.Configure(Dependencies{Runs: runs, Orchestrator: orchestrator, EventHub: hub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicRunCommandEnsureResumed,
		WorkspaceID: "ws-run-resume-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-resume-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace ensure resumed err = %v, want not found", err)
	}
	if orchestrator.resumed != "" {
		t.Fatalf("orchestrator resumed %q on wrong workspace", orchestrator.resumed)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicRunCommandEnsureResumed,
		WorkspaceID: "ws-run-resume-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-resume-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped ensure resumed err = %v, want response", err)
	}
	if response.RunID != string(runID) || orchestrator.resumed != runID {
		t.Fatalf("response runID=%q resumed=%q, want %s", response.RunID, orchestrator.resumed, runID)
	}
}

type fakeRunOrchestrator struct {
	resumed common.RunID
}

func (o *fakeRunOrchestrator) StartRun(context.Context, common.RunID) error { return nil }
func (o *fakeRunOrchestrator) BlockOnDecision(context.Context, common.RunID, common.TicketID) error {
	return nil
}
func (o *fakeRunOrchestrator) BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error {
	return nil
}
func (o *fakeRunOrchestrator) ResumeRun(_ context.Context, runID common.RunID) error {
	o.resumed = runID
	return nil
}
func (o *fakeRunOrchestrator) ValidateRun(context.Context, common.RunID) error { return nil }
func (o *fakeRunOrchestrator) CompleteRun(context.Context, common.RunID) error { return nil }
func (o *fakeRunOrchestrator) CancelRun(context.Context, common.RunID) error   { return nil }
func (o *fakeRunOrchestrator) SupersedeRun(context.Context, common.RunID) error {
	return nil
}
func (o *fakeRunOrchestrator) FailRun(context.Context, common.RunID, string) error { return nil }
