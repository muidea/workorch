package approval

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustApprovalPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type recordingApprovalHub struct {
	magicevent.Hub
	mu     sync.Mutex
	posted []magicevent.Event
}

func newRecordingApprovalHub(t *testing.T) *recordingApprovalHub {
	t.Helper()
	rawHub := magicevent.NewHub(32)
	hub := &recordingApprovalHub{Hub: rawHub}
	evidenceObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	evidenceObserver.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.RecordApprovalEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID,
			WorkspaceID:      command.WorkspaceID,
			RunID:            command.RunID,
			ApprovalID:       command.ApprovalID,
			ResolutionID:     command.ResolutionID,
			ApprovalState:    command.ApprovalState,
		}, nil)
	})
	resolutionObserver := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, rawHub)
	resolutionObserver.Subscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(eventstreamevents.RecordApprovalResolutionCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		data := command.Data
		result.Set(eventstreamevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	t.Cleanup(func() {
		evidenceObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence)
		resolutionObserver.Unsubscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution)
		rawHub.Terminate(context.Background())
	})
	return hub
}

func (h *recordingApprovalHub) Post(event magicevent.Event) {
	h.mu.Lock()
	h.posted = append(h.posted, event)
	h.mu.Unlock()
	h.Hub.Post(event)
}

func (h *recordingApprovalHub) Posted() []magicevent.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]magicevent.Event(nil), h.posted...)
}

type countingApprovalRepository struct {
	inner    *memory.ApprovalRepository
	getCount atomic.Int64
}

func newCountingApprovalRepository() *countingApprovalRepository {
	return &countingApprovalRepository{inner: memory.NewApprovalRepository()}
}

func (r *countingApprovalRepository) Create(ctx context.Context, item *approvalModel.Approval) error {
	return r.inner.Create(ctx, item)
}

func (r *countingApprovalRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalModel.Approval, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingApprovalRepository) GetScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) (*approvalModel.Approval, error) {
	r.getCount.Add(1)
	return r.inner.GetScoped(ctx, workspaceID, runID, id)
}

func (r *countingApprovalRepository) List(ctx context.Context) ([]*approvalModel.Approval, error) {
	return r.inner.List(ctx)
}

func (r *countingApprovalRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalModel.Approval, error) {
	return r.inner.ListByRunID(ctx, runID)
}

func (r *countingApprovalRepository) ListByRunScope(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) ([]*approvalModel.Approval, error) {
	return r.inner.ListByRunScope(ctx, workspaceID, runID)
}

func (r *countingApprovalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalModel.Approval, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingApprovalRepository) Update(ctx context.Context, item *approvalModel.Approval) error {
	return r.inner.Update(ctx, item)
}

func (r *countingApprovalRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	return r.inner.Delete(ctx, id)
}

func (r *countingApprovalRepository) DeleteScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) error {
	return r.inner.DeleteScoped(ctx, workspaceID, runID, id)
}

func TestService_CreateApproval_SeedsPendingState(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)

	cmd := approvalservice.CreateApprovalCommand{
		ApprovalID:     common.ApprovalID("approval-1"),
		WorkspaceID:    common.WorkspaceID("ws-1"),
		RunID:          common.RunID("run-1"),
		StepID:         common.StepID("run-1/step-1"),
		Reason:         "need human approval",
		AutoGovernance: mustApprovalPayload(t, map[string]any{"policy": "manual"}),
	}
	if err := service.CreateApproval(ctx, cmd); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	approval, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.State != common.ApprovalStatePending {
		t.Fatalf("approval state = %q, want pending", approval.State)
	}
	policy, _ := approval.AutoGovernance.String("policy")
	if policy != "manual" {
		t.Fatalf("approval auto governance = %+v", approval.AutoGovernance)
	}
}

func TestApprovalLifecycleSelectsInitialResolutionProposalState(t *testing.T) {
	current := &approvalModel.Approval{
		ID:          "approval-proposal-owner",
		WorkspaceID: "ws-proposal-owner",
		RunID:       "run-proposal-owner",
		Reason:      "review runtime action",
		State:       common.ApprovalStatePending,
	}
	seed := &approvalModel.ResolutionProposal{
		ResolutionID: "resolution-proposal-owner",
		TargetState:  common.ApprovalStateApproved,
		Principal: approvalModel.ApprovalResolutionPrincipal{
			Kind: approvalModel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "operator",
		},
	}

	next, err := reduceApprovalLifecycle(current, lifecycleInternalEvent{
		ApprovalID: current.ID, WorkspaceID: current.WorkspaceID, RunID: current.RunID,
		Kind: lifecycleInternalEventResolutionProposed, Proposal: seed,
	})
	if err != nil {
		t.Fatalf("prepare proposal: %v", err)
	}
	if next.ResolutionProposal == nil || next.ResolutionProposal.State != approvalModel.ResolutionProposalPending || next.ResolutionProposal.ProposedAt == "" {
		t.Fatalf("proposal lifecycle fields = %+v", next.ResolutionProposal)
	}
	if seed.State != "" || seed.ProposedAt != "" {
		t.Fatalf("proposal seed was mutated by reducer: %+v", seed)
	}

	callerSelected := seed.Clone()
	callerSelected.State = approvalModel.ResolutionProposalPending
	if rejected, err := reduceApprovalLifecycle(current, lifecycleInternalEvent{
		ApprovalID: current.ID, WorkspaceID: current.WorkspaceID, RunID: current.RunID,
		Kind: lifecycleInternalEventResolutionProposed, Proposal: &callerSelected,
	}); !errors.Is(err, common.ErrInvalidArgument) || rejected != nil {
		t.Fatalf("caller-selected proposal state was accepted: next=%+v err=%v", rejected, err)
	}
}

func TestService_CreateApproval_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	service := newMutationService(memory.NewApprovalRepository())

	tests := []approvalservice.CreateApprovalCommand{
		{ApprovalID: "", RunID: common.RunID("run-1"), Reason: "need human approval"},
		{ApprovalID: common.ApprovalID("approval-1"), WorkspaceID: "", RunID: common.RunID("run-1"), Reason: "need human approval"},
		{ApprovalID: common.ApprovalID("approval-1"), RunID: "", Reason: "need human approval"},
		{ApprovalID: common.ApprovalID("approval-1"), RunID: common.RunID("run-1"), Reason: ""},
	}

	for _, tt := range tests {
		if err := service.CreateApproval(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}
}

func TestApprovalLifecycleRejectsUnknownFactKind(t *testing.T) {
	current := &approvalModel.Approval{ID: "approval-1", State: common.ApprovalStatePending}
	if next, err := reduceApprovalLifecycle(current, lifecycleInternalEvent{ApprovalID: current.ID, Kind: "approved"}); !errors.Is(err, common.ErrInvalidArgument) || next != nil {
		t.Fatalf("unknown approval fact kind produced state: next=%+v err=%v", next, err)
	}
}

func TestApprovalLifecycleDuplicateObservedFactDoesNotMutateOwnerState(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)
	cmd := approvalservice.CreateApprovalCommand{ApprovalID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", Reason: "review"}
	if err := service.CreateApproval(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	resolve := approvalservice.ResolveApprovalCommand{ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
	if err := service.Approve(ctx, resolve); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Approve(ctx, resolve); err != nil {
		t.Fatalf("duplicate granted observation err=%v, want idempotent success", err)
	}
	after, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("duplicate granted observation mutated approval: before=%+v after=%+v", before, after)
	}
}

func TestService_CreateApproval_RejectsDuplicateApprovalID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)

	first := approvalservice.CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-dup"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need human approval",
	}
	if err := service.CreateApproval(ctx, first); err != nil {
		t.Fatalf("first create approval: %v", err)
	}

	second := approvalservice.CreateApprovalCommand{
		ApprovalID:  first.ApprovalID,
		WorkspaceID: first.WorkspaceID,
		RunID:       first.RunID,
		StepID:      common.StepID("run-1/other-step"),
		Reason:      "other approval",
	}
	if err := service.CreateApproval(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, first.ApprovalID)
	if err != nil {
		t.Fatalf("get approval after duplicate create: %v", err)
	}
	if got.RunID != first.RunID || got.Reason != first.Reason {
		t.Fatalf("approval mutated after duplicate create: %+v", got)
	}
}

func TestService_CreateApproval_AllowsDuplicateApprovalIDAcrossWorkspaces(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)

	first := approvalservice.CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-workspace-local"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need human approval",
	}
	second := approvalservice.CreateApprovalCommand{
		ApprovalID:  first.ApprovalID,
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       common.RunID("run-2"),
		StepID:      common.StepID("run-2/step-1"),
		Reason:      "other approval",
	}
	if err := service.CreateApproval(ctx, first); err != nil {
		t.Fatalf("first create approval: %v", err)
	}
	if err := service.CreateApproval(ctx, second); err != nil {
		t.Fatalf("second workspace-local create approval: %v", err)
	}
	if _, err := repo.Get(ctx, first.ApprovalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped get err = %v, want conflict for duplicate workspace-local approval id", err)
	}
	gotFirst, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, first.WorkspaceID), first.ApprovalID)
	if err != nil {
		t.Fatalf("get first scoped approval: %v", err)
	}
	gotSecond, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, second.WorkspaceID), second.ApprovalID)
	if err != nil {
		t.Fatalf("get second scoped approval: %v", err)
	}
	if gotFirst.RunID != first.RunID || gotSecond.RunID != second.RunID {
		t.Fatalf("scoped approvals = %+v/%+v, want distinct workspace-local records", gotFirst, gotSecond)
	}
}

func TestService_EnsureApproval_IsIdempotentAndRejectsBindingMismatch(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)

	cmd := approvalservice.CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-ensure"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need human approval",
	}
	if err := service.EnsureApproval(ctx, cmd); err != nil {
		t.Fatalf("first ensure approval: %v", err)
	}
	if err := service.EnsureApproval(ctx, cmd); err != nil {
		t.Fatalf("second ensure approval: %v", err)
	}

	if err := service.EnsureApproval(ctx, approvalservice.CreateApprovalCommand{
		ApprovalID:  cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      common.StepID("run-1/other-step"),
		Reason:      cmd.Reason,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("binding mismatch err = %v, want conflict", err)
	}

	if err := service.EnsureApproval(ctx, approvalservice.CreateApprovalCommand{
		ApprovalID:  cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      "other approval reason",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("reason mismatch err = %v, want conflict", err)
	}
}

func TestService_ResolveApproval_PendingTransitions(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		resolve   func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState common.ApprovalState
	}{
		{
			name:      "approve",
			resolve:   (*mutationService).Approve,
			wantState: common.ApprovalStateApproved,
		},
		{
			name:      "reject",
			resolve:   (*mutationService).Reject,
			wantState: common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := newMutationService(repo)

			cmd := approvalservice.CreateApprovalCommand{
				ApprovalID:  common.ApprovalID("approval-" + tc.name),
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}
			if err := service.CreateApproval(ctx, cmd); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			if err := tc.resolve(service, ctx, approvalservice.ResolveApprovalCommand{
				ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID,
				Principal: approvalModel.ApprovalResolutionPrincipal{Kind: approvalModel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "operator"},
				Context:   mustApprovalPayload(t, map[string]any{"actor": "operator"}),
			}); err != nil {
				t.Fatalf("resolve approval: %v", err)
			}

			approval, err := repo.Get(ctx, cmd.ApprovalID)
			if err != nil {
				t.Fatalf("get approval: %v", err)
			}
			if approval.State != tc.wantState {
				t.Fatalf("approval state = %q, want %q", approval.State, tc.wantState)
			}
			if approval.ResolvedAt == "" || approval.ResolvedBy != "operator" {
				t.Fatalf("approval resolution audit fields = %+v", approval)
			}
		})
	}
}

func TestService_ResolveApproval_RecordsGovernanceEvidence(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		approvalID common.ApprovalID
		resolve    func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error)
		wantState  common.ApprovalState
	}{
		{
			name:       "approved",
			approvalID: common.ApprovalID("approval-evidence-approved"),
			resolve:    (*mutationService).EnsureApprovedAndReturn,
			wantState:  common.ApprovalStateApproved,
		},
		{
			name:       "rejected",
			approvalID: common.ApprovalID("approval-evidence-rejected"),
			resolve:    (*mutationService).EnsureRejectedAndReturn,
			wantState:  common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			rawHub := magicevent.NewHub(16)
			defer rawHub.Terminate(ctx)
			var evidenceCommand orchestratorevents.RecordApprovalEvidenceCommand
			observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, func(ev magicevent.Event, result magicevent.Result) {
				command, ok := ev.Data().(orchestratorevents.RecordApprovalEvidenceCommand)
				if !ok {
					result.Set(nil, nil)
					return
				}
				evidenceCommand = command
				result.Set(orchestratorevents.RecordApprovalEvidenceResult{
					EvidenceRecordID: command.EvidenceRecordID,
					WorkspaceID:      command.WorkspaceID,
					RunID:            command.RunID,
					ApprovalID:       command.ApprovalID,
					ResolutionID:     command.ResolutionID,
					ApprovalState:    command.ApprovalState,
				}, nil)
			})
			defer observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence)
			owner, err := New(rawHub, nil, Dependencies{OwnerRepositories: approvalBizTestOwnerRepositories(repo, memory.NewApprovalWorkspaceDeletionRepository()), EventHub: rawHub})
			if err != nil {
				t.Fatalf("new owner: %v", err)
			}
			service := owner.service

			cmd := approvalservice.CreateApprovalCommand{
				ApprovalID:  tc.approvalID,
				WorkspaceID: common.WorkspaceID("ws-evidence"),
				RunID:       common.RunID("run-evidence"),
				StepID:      common.StepID("run-evidence/approval"),
				Reason:      "need human approval",
				Action:      "apply pending change",
				Context:     mustApprovalPayload(t, map[string]any{"scope": "runtime governance"}),
			}
			if err := service.CreateApproval(ctx, cmd); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			resolveCmd := approvalservice.ResolveApprovalCommand{
				ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID,
				Context: mustApprovalPayload(t, map[string]any{"resolution": tc.name}),
			}
			item, err := tc.resolve(service, ctx, resolveCmd)
			if err != nil {
				t.Fatalf("resolve approval: %v", err)
			}
			if err := owner.recordApprovalGovernanceEvidence(ctx, item); err != nil {
				t.Fatalf("record approval evidence: %v", err)
			}
			item, err = tc.resolve(service, ctx, resolveCmd)
			if err != nil {
				t.Fatalf("second resolve approval: %v", err)
			}
			if err := owner.recordApprovalGovernanceEvidence(ctx, item); err != nil {
				t.Fatalf("record repeated approval evidence: %v", err)
			}

			if evidenceCommand.EvidenceRecordID == "" || strings.ContainsAny(string(evidenceCommand.EvidenceRecordID), `/\`) {
				t.Fatalf("evidence record id = %q, want non-empty file-safe id", evidenceCommand.EvidenceRecordID)
			}
			if evidenceCommand.WorkspaceID != cmd.WorkspaceID || evidenceCommand.RunID != cmd.RunID || evidenceCommand.StepID != cmd.StepID ||
				evidenceCommand.ApprovalID != cmd.ApprovalID || evidenceCommand.ApprovalState != tc.wantState || evidenceCommand.Action != cmd.Action {
				t.Fatalf("evidence command = %#v, want scoped approval contract", evidenceCommand)
			}
			if len(evidenceCommand.Context) != 1 || len(evidenceCommand.ResolutionContext) != 1 {
				t.Fatalf("evidence contexts = original:%#v resolution:%#v, want immutable target and separate resolution metadata", evidenceCommand.Context, evidenceCommand.ResolutionContext)
			}
		})
	}
}

func TestService_ResolveApproval_RejectsWhenAlreadyResolved(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		firstResolve  func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
		secondResolve func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState     common.ApprovalState
	}{
		{
			name:          "approved cannot reject",
			firstResolve:  (*mutationService).Approve,
			secondResolve: (*mutationService).Reject,
			wantState:     common.ApprovalStateApproved,
		},
		{
			name:          "rejected cannot approve",
			firstResolve:  (*mutationService).Reject,
			secondResolve: (*mutationService).Approve,
			wantState:     common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := newMutationService(repo)
			approvalID := common.ApprovalID("approval-1")

			if err := service.CreateApproval(ctx, approvalservice.CreateApprovalCommand{
				ApprovalID:  approvalID,
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			resolve := approvalservice.ResolveApprovalCommand{ApprovalID: approvalID, WorkspaceID: "ws-1", RunID: "run-1"}
			if err := tc.firstResolve(service, ctx, resolve); err != nil {
				t.Fatalf("first resolve: %v", err)
			}
			if err := tc.secondResolve(service, ctx, resolve); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("second resolve err = %v, want conflict", err)
			}

			approval, err := repo.Get(ctx, approvalID)
			if err != nil {
				t.Fatalf("get approval: %v", err)
			}
			if approval.State != tc.wantState {
				t.Fatalf("approval state = %q, want %q", approval.State, tc.wantState)
			}
		})
	}
}

func TestService_EnsureApprovalResolution_IsIdempotentAndRejectsOppositeTerminalState(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name         string
		firstEnsure  func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
		secondEnsure func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState    common.ApprovalState
		wantErr      error
	}{
		{
			name:         "approved idempotent",
			firstEnsure:  (*mutationService).EnsureApproved,
			secondEnsure: (*mutationService).EnsureApproved,
			wantState:    common.ApprovalStateApproved,
			wantErr:      nil,
		},
		{
			name:         "rejected idempotent",
			firstEnsure:  (*mutationService).EnsureRejected,
			secondEnsure: (*mutationService).EnsureRejected,
			wantState:    common.ApprovalStateRejected,
			wantErr:      nil,
		},
		{
			name:         "approved cannot ensure rejected",
			firstEnsure:  (*mutationService).EnsureApproved,
			secondEnsure: (*mutationService).EnsureRejected,
			wantState:    common.ApprovalStateApproved,
			wantErr:      common.ErrConflict,
		},
		{
			name:         "rejected cannot ensure approved",
			firstEnsure:  (*mutationService).EnsureRejected,
			secondEnsure: (*mutationService).EnsureApproved,
			wantState:    common.ApprovalStateRejected,
			wantErr:      common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := newMutationService(repo)
			approvalID := common.ApprovalID("approval-" + tc.name)

			if err := service.CreateApproval(ctx, approvalservice.CreateApprovalCommand{
				ApprovalID:  approvalID,
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			cmd := approvalservice.ResolveApprovalCommand{ApprovalID: approvalID, WorkspaceID: "ws-1", RunID: "run-1"}
			if err := tc.firstEnsure(service, ctx, cmd); err != nil {
				t.Fatalf("first ensure resolve: %v", err)
			}
			err := tc.secondEnsure(service, ctx, cmd)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("second ensure resolve err = %v, want %v", err, tc.wantErr)
			}

			approval, err := repo.Get(ctx, approvalID)
			if err != nil {
				t.Fatalf("get approval: %v", err)
			}
			if approval.State != tc.wantState {
				t.Fatalf("approval state = %q, want %q", approval.State, tc.wantState)
			}
		})
	}
}

func TestService_EnsureApprovalResolutionReturnsLoadedApproval(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		resolve   func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error)
		wantState common.ApprovalState
	}{
		{
			name:      "approved",
			resolve:   (*mutationService).EnsureApprovedAndReturn,
			wantState: common.ApprovalStateApproved,
		},
		{
			name:      "rejected",
			resolve:   (*mutationService).EnsureRejectedAndReturn,
			wantState: common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newCountingApprovalRepository()
			service := newMutationService(repo)
			approvalID := common.ApprovalID("approval-ensure-loaded-" + tc.name)
			if err := service.CreateApproval(ctx, approvalservice.CreateApprovalCommand{
				ApprovalID:  approvalID,
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			item, err := tc.resolve(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID, WorkspaceID: "ws-1", RunID: "run-1"})
			if err != nil {
				t.Fatalf("ensure %s: %v", tc.name, err)
			}
			if item == nil || item.ID != approvalID || item.State != tc.wantState {
				t.Fatalf("returned approval = %+v, want %s", item, tc.wantState)
			}
			if got := repo.getCount.Load(); got != 1 {
				t.Fatalf("approval Get count = %d, want one read reused for transition and response", got)
			}
		})
	}
}

func TestService_ResolveApproval_ValidateArgumentsAndNotFound(t *testing.T) {
	ctx := context.Background()
	service := newMutationService(memory.NewApprovalRepository())

	for _, resolve := range []struct {
		name string
		fn   func(*mutationService, context.Context, approvalservice.ResolveApprovalCommand) error
	}{
		{name: "approve", fn: (*mutationService).Approve},
		{name: "reject", fn: (*mutationService).Reject},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			if err := resolve.fn(service, ctx, approvalservice.ResolveApprovalCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("empty approval id err = %v, want invalid argument", err)
			}
			if err := resolve.fn(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: common.ApprovalID("missing"), WorkspaceID: "ws-missing", RunID: "run-missing"}); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("missing approval err = %v, want not found", err)
			}
		})
	}
}

func TestService_CreateAndResolveApproval_PublishesEvents(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	hub := newRecordingApprovalHub(t)
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: approvalBizTestOwnerRepositories(repo, memory.NewApprovalWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatalf("new owner: %v", err)
	}
	service := owner.service

	cmd := approvalservice.CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-event"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/manual-approval"),
		Reason:      "need human approval",
	}
	if err := service.CreateApproval(ctx, cmd); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	item, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get created approval: %v", err)
	}
	if err := service.Approve(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}
	item, err = repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get resolved approval: %v", err)
	}
	data, err := approvalResolutionNotification(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, data); err != nil {
		t.Fatal(err)
	}

	published := hub.Posted()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].ID() != approvalevents.TopicApprovalResolved {
		t.Fatalf("topic = %s, want %s", published[0].ID(), approvalevents.TopicApprovalResolved)
	}
	resolved, ok := published[0].Data().(approvalevents.ApprovalChangedData)
	if !ok || resolved.State != common.ApprovalStateApproved {
		t.Fatalf("resolved data = %+v, want approved", resolved)
	}
}

func TestService_ResolveApprovalKeepsAuthorizationContextImmutable(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := newMutationService(repo)

	approvalID := common.ApprovalID("approval-context")
	if err := service.CreateApproval(ctx, approvalservice.CreateApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: "ws-context",
		RunID:       "run-context",
		Reason:      "need human approval",
		Context:     mustApprovalPayload(t, map[string]any{"existing": "kept"}),
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := service.Approve(ctx, approvalservice.ResolveApprovalCommand{
		ApprovalID: approvalID, WorkspaceID: "ws-context", RunID: "run-context",
		Context: mustApprovalPayload(t, map[string]any{
			"governanceSemantic": map[string]any{
				"intent": "approve",
				"source": "tui",
			},
		}),
	}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}

	item, err := repo.Get(ctx, approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	existing, _ := item.Context.String("existing")
	if existing != "kept" {
		t.Fatalf("existing context = %v, want kept", existing)
	}
	if _, exists := item.Context.Object("governanceSemantic"); exists {
		t.Fatalf("authorization context was overwritten by resolution metadata: %#v", item.Context)
	}
	semantic, ok := item.ResolutionContext.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	source, _ := semantic.String("source")
	if !ok || intent != "approve" || source != "tui" {
		t.Fatalf("governance semantic = %#v, want approve from tui", item.ResolutionContext)
	}
}

func TestService_PublishesConcreteApprovalNotificationsThroughEventHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	hub := newRecordingApprovalHub(t)
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: approvalBizTestOwnerRepositories(repo, memory.NewApprovalWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatalf("new owner: %v", err)
	}
	service := owner.service

	cmd := approvalservice.CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-async"),
		WorkspaceID: common.WorkspaceID("ws-async"),
		RunID:       common.RunID("run-async"),
		StepID:      common.StepID("run-async/manual-approval"),
		Reason:      "need human approval",
	}
	if err := service.CreateApproval(ctx, cmd); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	item, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get created approval: %v", err)
	}
	if err := service.Approve(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}
	item, err = repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get resolved approval: %v", err)
	}
	data, err := approvalResolutionNotification(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, data); err != nil {
		t.Fatal(err)
	}

	published := hub.Posted()
	if len(published) != 1 || published[0].ID() != approvalevents.TopicApprovalResolved {
		t.Fatalf("published = %+v, want approval resolved notification", published)
	}
	if _, ok := published[0].Data().(approvalevents.ApprovalChangedData); !ok {
		t.Fatalf("resolved data type = %T", published[0].Data())
	}
}
