package approval

import (
	"context"
	"errors"
	"testing"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type approvalTestResult struct {
	Approval   *approvalmodel.Approval
	Approvals  []*approvalmodel.Approval
	NextCursor string
	Deleted    int
}

func sendApprovalTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (approvalTestResult, error) {
	ev := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return approvalTestResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return approvalTestResult{}, err.Cause
		}
		return approvalTestResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return approvalTestResult{}, err
	}
	switch response := data.(type) {
	case approvalevents.GetApprovalResult:
		return approvalTestResult{Approval: response.Approval}, nil
	case approvalevents.ListApprovalsResult:
		return approvalTestResult{Approvals: response.Approvals, NextCursor: response.NextCursor}, nil
	case approvalevents.ListApprovalsByRunResult:
		return approvalTestResult{Approvals: response.Approvals, NextCursor: response.NextCursor}, nil
	case approvalevents.ListApprovalsByWorkspaceResult:
		return approvalTestResult{Approvals: response.Approvals, NextCursor: response.NextCursor}, nil
	case approvalevents.EnsureApprovedResult:
		return approvalTestResult{Approval: response.Approval}, nil
	case approvalevents.EnsureRejectedResult:
		return approvalTestResult{Approval: response.Approval}, nil
	case approvalevents.PurgeWorkspaceApprovalsResult:
		return approvalTestResult{Deleted: response.Deleted}, nil
	default:
		return approvalTestResult{}, common.ErrInvalidArgument
	}
}

func subscribeApprovalEvidenceAck(rawHub magicevent.Hub) func() {
	observer := magicevent.NewSimpleObserver(approvalevents.EvidenceCommandDestination, rawHub)
	observer.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(approvalevents.RecordApprovalEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(approvalevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID,
			WorkspaceID:      command.WorkspaceID,
			RunID:            command.RunID,
			ApprovalID:       command.ApprovalID,
			ResolutionID:     command.ResolutionID,
			ApprovalState:    command.ApprovalState,
		}, nil)
	})
	resolutionObserver := magicevent.NewSimpleObserver(approvalevents.ResolutionRecordCommandDestination, rawHub)
	resolutionObserver.Subscribe(approvalevents.TopicApprovalCommandRecordResolution, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(approvalevents.RecordApprovalResolutionCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		data := command.Data
		result.Set(approvalevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	return func() {
		observer.Unsubscribe(approvalevents.TopicApprovalCommandRecordEvidence)
		resolutionObserver.Unsubscribe(approvalevents.TopicApprovalCommandRecordResolution)
	}
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/approval" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}

	if err := module.Configure(approvalModuleContractDependencies()); err != nil {
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

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := approvalModuleContractDependencies()
	tests := []struct {
		name string
		deps approvalbiz.Dependencies
	}{
		{name: "missing approvals", deps: func() approvalbiz.Dependencies { d := base; d.Approvals = nil; return d }()},
		{name: "missing event hub", deps: func() approvalbiz.Dependencies { d := base; d.EventHub = nil; return d }()},
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
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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

func approvalModuleContractDependencies() approvalbiz.Dependencies {
	return approvalbiz.Dependencies{
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
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice",
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

}

func TestApprovalMutationCommandWithoutScopeFailsClosed(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer subscribeApprovalEvidenceAck(rawHub)()
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
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped approve err = %v, want invalid argument", err)
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
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
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
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped duplicate approve err = %v, want invalid argument", err)
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

func TestApprovalListCommandsAreTypedBoundedAndCursorValidated(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	approvals := memory.NewApprovalRepository()
	for _, item := range []*approvalmodel.Approval{
		{ID: "approval-page-c", WorkspaceID: "ws-page-b", RunID: "run-page-a", State: common.ApprovalStatePending, Reason: "page test"},
		{ID: "approval-page-a", WorkspaceID: "ws-page-a", RunID: "run-page-a", State: common.ApprovalStatePending, Reason: "page test"},
		{ID: "approval-page-b", WorkspaceID: "ws-page-a", RunID: "run-page-b", State: common.ApprovalStatePending, Reason: "page test"},
	} {
		if err := approvals.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	module := New()
	if err := module.Configure(approvalbiz.Dependencies{Approvals: approvals, EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	first, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandList, approvalevents.ListAllApprovalsCommand{
		Scope: approvalevents.ApprovalListScopeAll, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Approvals) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two items and next cursor", first)
	}
	second, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandList, approvalevents.ListAllApprovalsCommand{
		Scope: approvalevents.ApprovalListScopeAll, Cursor: first.NextCursor, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Approvals) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want final item", second)
	}
	if _, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandList, approvalevents.ListAllApprovalsCommand{
		Scope: approvalevents.ApprovalListScopeAll, Limit: approvalevents.MaxListLimit + 1,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized list err = %v, want invalid argument", err)
	}
	if _, err := sendApprovalTestCommand(ctx, rawHub, approvalevents.TopicApprovalCommandList, approvalevents.ListAllApprovalsCommand{
		Scope: approvalevents.ApprovalListScopeAll, Cursor: "unknown-cursor", Limit: 1,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unknown cursor err = %v, want invalid argument", err)
	}
}

func TestApprovalPublishesRuntimeGovernanceCapabilityOnlyToOrchestrator(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	module := New()
	if err := module.Configure(approvalbiz.Dependencies{Approvals: memory.NewApprovalRepository(), EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	command := approvalevents.AcquireRuntimeCapabilitiesCommand(runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-approval-capability", RunID: "run-approval-capability",
		AuthoritySnapshotID: "authority-approval-capability", PrerequisiteID: "prerequisite-approval-capability",
		OwnerModuleID: runtimecontract.RuntimeModuleApproval, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindApprovalRequest},
	})
	send := func(source string) (approvalevents.AcquireRuntimeCapabilitiesResult, error) {
		event := magicevent.NewEventWithContext(approvalevents.TopicAcquireRuntimeCapabilities, source, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		result := rawHub.Send(event)
		if result == nil {
			return approvalevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument
		}
		if resultErr := result.Error(); resultErr != nil {
			if resultErr.Cause != nil {
				return approvalevents.AcquireRuntimeCapabilitiesResult{}, resultErr.Cause
			}
			return approvalevents.AcquireRuntimeCapabilitiesResult{}, resultErr
		}
		data, resultErr := result.Get()
		if resultErr != nil {
			return approvalevents.AcquireRuntimeCapabilitiesResult{}, resultErr
		}
		response, ok := data.(approvalevents.AcquireRuntimeCapabilitiesResult)
		if !ok {
			return approvalevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument
		}
		return response, nil
	}
	if _, err := send("untrusted/module"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("untrusted capability request err = %v, want invalid argument", err)
	}
	response, err := send(runtimecontract.RuntimeModuleOrchestrator)
	if err != nil {
		t.Fatal(err)
	}
	request := runtimecontract.RuntimeCapabilityAcquisitionRequest(command)
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("approval capability acquisition: %v", err)
	}
}
