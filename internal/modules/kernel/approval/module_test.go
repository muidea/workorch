package approval

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func sendApprovalTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (approvalevents.ApprovalCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "approval/module_test", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return approvalevents.ApprovalCommandResult{}, err.Cause
		}
		return approvalevents.ApprovalCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return approvalevents.ApprovalCommandResult{}, err
	}
	response, ok := data.(approvalevents.ApprovalCommandResult)
	if !ok {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.Service() != nil {
		t.Fatal("new module service is non-nil before Configure")
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}

	if err := module.Configure(approvalModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure")
	}
	if module.bizPtr == nil ||
		module.servicePtr == nil ||
		module.eventHub == nil ||
		module.approvals == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe approval commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		module.approvals != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := approvalModuleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing approvals", deps: func() Dependencies { d := base; d.Approvals = nil; return d }()},
		{name: "missing event hub", deps: func() Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestApprovalCommandGetHonorsScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

	_, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandGet, approvalevents.GetApprovalCommand{
		WorkspaceID: "ws-approval-owner-b",
		RunID:       "run-approval-owner-a",
		ApprovalID:  approvalID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	response, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandGet, approvalevents.GetApprovalCommand{
		WorkspaceID: "ws-approval-owner-a",
		RunID:       "run-approval-owner-a",
		ApprovalID:  approvalID,
	})
	if err != nil {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	if response.Approval == nil || response.Approval.WorkspaceID != "ws-approval-owner-a" || response.Approval.ID != approvalID {
		t.Fatalf("response approval = %+v, want scoped approval", response.Approval)
	}
}

func approvalModuleContractDependencies() Dependencies {
	return Dependencies{
		Approvals: memory.NewApprovalRepository(),
		EventHub:  magicevent.NewHub(16),
	}
}

func TestApprovalCommandListByRunHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

	response, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandListByRun, approvalevents.ListApprovalsByRunCommand{
		WorkspaceID: "ws-approval-list-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("list by run err = %v, want response", err)
	}
	if len(response.Approvals) != 1 || response.Approvals[0].WorkspaceID != "ws-approval-list-a" {
		t.Fatalf("response approvals = %+v, want one scoped approval", response.Approvals)
	}
}

func TestApprovalMutationCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

	_, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.EnsureApprovedCommand{
		ApprovalID:  approvalID,
		WorkspaceID: "ws-approval-mutate-b",
		RunID:       "run-approval-mutate-a",
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

	_, err = sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandDelete, approvalevents.DeleteApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: "ws-approval-mutate-b",
		RunID:       "run-approval-mutate-a",
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := approvals.Get(ctx, approvalID); err != nil {
		t.Fatalf("approval should remain after wrong-scope delete: %v", err)
	}
}

func TestApprovalMutationCommandWithoutScopeResolvesUniquePendingApproval(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	approvals := memory.NewApprovalRepository()
	approvalID := common.ApprovalID("approval-unscoped-unique")
	if err := approvals.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: "ws-approval-unscoped-unique",
		RunID:       "run-approval-unscoped-unique",
		State:       common.ApprovalStatePending,
		Reason:      "approval unscoped unique test",
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

	response, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.EnsureApprovedCommand{
		ApprovalID: approvalID,
	})
	if err != nil {
		t.Fatalf("unscoped approve err = %v, want response", err)
	}
	if response.Approval == nil || response.Approval.ID != approvalID || response.Approval.State != common.ApprovalStateApproved {
		t.Fatalf("response approval = %+v, want approved unique approval", response.Approval)
	}
}

func TestApprovalMutationCommandWithoutScopeConflictsOnDuplicatePendingApprovals(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	approvals := memory.NewApprovalRepository()
	approvalID := common.ApprovalID("approval-unscoped-duplicate")
	for _, item := range []*approvalmodel.Approval{
		{
			ID:          approvalID,
			WorkspaceID: "ws-approval-unscoped-duplicate-a",
			RunID:       "run-approval-unscoped-duplicate-a",
			State:       common.ApprovalStatePending,
			Reason:      "approval unscoped duplicate test",
		},
		{
			ID:          approvalID,
			WorkspaceID: "ws-approval-unscoped-duplicate-b",
			RunID:       "run-approval-unscoped-duplicate-b",
			State:       common.ApprovalStatePending,
			Reason:      "approval unscoped duplicate test",
		},
	} {
		if err := approvals.Create(ctx, item); err != nil {
			t.Fatalf("create approval %s/%s: %v", item.WorkspaceID, item.ID, err)
		}
	}

	module := New()
	if err := module.Configure(Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

	_, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.EnsureApprovedCommand{
		ApprovalID: approvalID,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate approve err = %v, want conflict", err)
	}
	items, err := approvals.List(ctx)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	for _, item := range items {
		if item.ID == approvalID && item.State != common.ApprovalStatePending {
			t.Fatalf("approval %s state = %s, want pending after duplicate conflict", item.WorkspaceID, item.State)
		}
	}
}
