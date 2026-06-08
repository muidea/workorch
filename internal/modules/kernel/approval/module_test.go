package approval

import (
	"context"
	"errors"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestApprovalCommandGetHonorsScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	approvals := memory.NewApprovalRepository()
	approvalID := common.ApprovalID("approval-scoped-get")
	if err := approvals.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: "ws-approval-owner-a",
		RunID:       "run-approval-owner-a",
		StepID:      "step-approval-owner-a",
		State:       common.ApprovalStatePending,
		Reason:      "approval scope test",
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: hub}); err != nil {
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
		Topic:       approvalevents.TopicApprovalCommandGet,
		WorkspaceID: "ws-approval-owner-b",
		RunID:       "run-approval-owner-a",
		StepID:      "step-approval-owner-a",
		Payload: map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: "ws-approval-owner-b",
			events.PayloadKeyRunID:       "run-approval-owner-a",
			events.PayloadKeyStepID:      "step-approval-owner-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       approvalevents.TopicApprovalCommandGet,
		WorkspaceID: "ws-approval-owner-a",
		RunID:       "run-approval-owner-a",
		StepID:      "step-approval-owner-a",
		Payload: map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: "ws-approval-owner-a",
			events.PayloadKeyRunID:       "run-approval-owner-a",
			events.PayloadKeyStepID:      "step-approval-owner-a",
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil || item.WorkspaceID != "ws-approval-owner-a" || item.ID != approvalID {
		t.Fatalf("response approval = %+v, want scoped approval", response.Payload[events.PayloadKeyApproval])
	}
}

func TestApprovalCommandListByRunHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	approvals := memory.NewApprovalRepository()
	runID := common.RunID("run-shared-approval")
	for _, item := range []*approvalmodel.Approval{
		{
			ID:          "approval-list-a",
			WorkspaceID: "ws-approval-list-a",
			RunID:       runID,
			State:       common.ApprovalStatePending,
			Reason:      "approval list scope test",
		},
		{
			ID:          "approval-list-b",
			WorkspaceID: "ws-approval-list-b",
			RunID:       runID,
			State:       common.ApprovalStatePending,
			Reason:      "approval list scope test",
		},
	} {
		if err := approvals.Create(ctx, item); err != nil {
			t.Fatalf("create approval %s: %v", item.ID, err)
		}
	}

	module := New()
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: hub}); err != nil {
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
		Topic:       approvalevents.TopicApprovalCommandListByRun,
		WorkspaceID: "ws-approval-list-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-approval-list-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list by run err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok || len(items) != 1 || items[0].WorkspaceID != "ws-approval-list-a" {
		t.Fatalf("response approvals = %+v, want one scoped approval", response.Payload[events.PayloadKeyItems])
	}
}

func TestApprovalMutationCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	approvals := memory.NewApprovalRepository()
	approvalID := common.ApprovalID("approval-scoped-mutation")
	if err := approvals.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: "ws-approval-mutate-a",
		RunID:       "run-approval-mutate-a",
		State:       common.ApprovalStatePending,
		Reason:      "approval mutation scope test",
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: hub}); err != nil {
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
		Topic:       approvalevents.TopicApprovalCommandEnsureApproved,
		WorkspaceID: "ws-approval-mutate-b",
		RunID:       "run-approval-mutate-a",
		Payload: map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: "ws-approval-mutate-b",
			events.PayloadKeyRunID:       "run-approval-mutate-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace approve err = %v, want not found", err)
	}
	item, err := approvals.Get(ctx, approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if item.State != common.ApprovalStatePending {
		t.Fatalf("approval state = %s, want pending", item.State)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       approvalevents.TopicApprovalCommandDelete,
		WorkspaceID: "ws-approval-mutate-b",
		RunID:       "run-approval-mutate-a",
		Payload: map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: "ws-approval-mutate-b",
			events.PayloadKeyRunID:       "run-approval-mutate-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := approvals.Get(ctx, approvalID); err != nil {
		t.Fatalf("approval should remain after wrong-scope delete: %v", err)
	}
}
