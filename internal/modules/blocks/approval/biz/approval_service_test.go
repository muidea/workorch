package approval

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
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
	evidenceObserver := magicevent.NewSimpleObserver(approvalevents.EvidenceCommandDestination, rawHub)
	evidenceObserver.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(approvalevents.RecordApprovalEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(approvalevents.RecordApprovalEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	t.Cleanup(func() {
		evidenceObserver.Unsubscribe(approvalevents.TopicApprovalCommandRecordEvidence)
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

func (r *countingApprovalRepository) List(ctx context.Context) ([]*approvalModel.Approval, error) {
	return r.inner.List(ctx)
}

func (r *countingApprovalRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalModel.Approval, error) {
	return r.inner.ListByRunID(ctx, runID)
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

func TestService_CreateApproval_SeedsPendingState(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := NewService(repo)

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

func TestService_CreateApproval_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewApprovalRepository())

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

func TestService_CreateApproval_RejectsDuplicateApprovalID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := NewService(repo)

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
		RunID:       common.RunID("run-2"),
		StepID:      common.StepID("run-2/step-1"),
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
	service := NewService(repo)

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
	service := NewService(repo)

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
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
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
		resolve   func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState common.ApprovalState
	}{
		{
			name:      "approve",
			resolve:   (*Service).Approve,
			wantState: common.ApprovalStateApproved,
		},
		{
			name:      "reject",
			resolve:   (*Service).Reject,
			wantState: common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := NewService(repo)

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
				ApprovalID: cmd.ApprovalID,
				Context:    mustApprovalPayload(t, map[string]any{"actor": "operator"}),
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
		resolve    func(*Service, context.Context, approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error)
		wantState  common.ApprovalState
	}{
		{
			name:       "approved",
			approvalID: common.ApprovalID("approval-evidence-approved"),
			resolve:    (*Service).EnsureApprovedAndReturn,
			wantState:  common.ApprovalStateApproved,
		},
		{
			name:       "rejected",
			approvalID: common.ApprovalID("approval-evidence-rejected"),
			resolve:    (*Service).EnsureRejectedAndReturn,
			wantState:  common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			rawHub := magicevent.NewHub(16)
			defer rawHub.Terminate(ctx)
			var evidenceCommand approvalevents.RecordApprovalEvidenceCommand
			observer := magicevent.NewSimpleObserver(approvalevents.EvidenceCommandDestination, rawHub)
			observer.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
				command, ok := ev.Data().(approvalevents.RecordApprovalEvidenceCommand)
				if !ok {
					result.Set(nil, nil)
					return
				}
				evidenceCommand = command
				result.Set(approvalevents.RecordApprovalEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
			})
			defer observer.Unsubscribe(approvalevents.TopicApprovalCommandRecordEvidence)
			owner, err := New(rawHub, nil, Dependencies{Approvals: repo, EventHub: rawHub})
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
				ApprovalID: cmd.ApprovalID,
				Context:    mustApprovalPayload(t, map[string]any{"resolution": tc.name}),
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
			if len(evidenceCommand.Context) != 2 {
				t.Fatalf("evidence context = %#v, want original and resolution fields", evidenceCommand.Context)
			}
		})
	}
}

func TestService_ResolveApproval_RejectsWhenAlreadyResolved(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		firstResolve  func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
		secondResolve func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState     common.ApprovalState
	}{
		{
			name:          "approved cannot reject",
			firstResolve:  (*Service).Approve,
			secondResolve: (*Service).Reject,
			wantState:     common.ApprovalStateApproved,
		},
		{
			name:          "rejected cannot approve",
			firstResolve:  (*Service).Reject,
			secondResolve: (*Service).Approve,
			wantState:     common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := NewService(repo)
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

			if err := tc.firstResolve(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID}); err != nil {
				t.Fatalf("first resolve: %v", err)
			}
			if err := tc.secondResolve(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID}); !errors.Is(err, common.ErrConflict) {
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
		firstEnsure  func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
		secondEnsure func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
		wantState    common.ApprovalState
		wantErr      error
	}{
		{
			name:         "approved idempotent",
			firstEnsure:  (*Service).EnsureApproved,
			secondEnsure: (*Service).EnsureApproved,
			wantState:    common.ApprovalStateApproved,
			wantErr:      nil,
		},
		{
			name:         "rejected idempotent",
			firstEnsure:  (*Service).EnsureRejected,
			secondEnsure: (*Service).EnsureRejected,
			wantState:    common.ApprovalStateRejected,
			wantErr:      nil,
		},
		{
			name:         "approved cannot ensure rejected",
			firstEnsure:  (*Service).EnsureApproved,
			secondEnsure: (*Service).EnsureRejected,
			wantState:    common.ApprovalStateApproved,
			wantErr:      common.ErrConflict,
		},
		{
			name:         "rejected cannot ensure approved",
			firstEnsure:  (*Service).EnsureRejected,
			secondEnsure: (*Service).EnsureApproved,
			wantState:    common.ApprovalStateRejected,
			wantErr:      common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := NewService(repo)
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

			cmd := approvalservice.ResolveApprovalCommand{ApprovalID: approvalID}
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
		resolve   func(*Service, context.Context, approvalservice.ResolveApprovalCommand) (*approvalModel.Approval, error)
		wantState common.ApprovalState
	}{
		{
			name:      "approved",
			resolve:   (*Service).EnsureApprovedAndReturn,
			wantState: common.ApprovalStateApproved,
		},
		{
			name:      "rejected",
			resolve:   (*Service).EnsureRejectedAndReturn,
			wantState: common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newCountingApprovalRepository()
			service := NewService(repo)
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

			item, err := tc.resolve(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: approvalID})
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
	service := NewService(memory.NewApprovalRepository())

	for _, resolve := range []struct {
		name string
		fn   func(*Service, context.Context, approvalservice.ResolveApprovalCommand) error
	}{
		{name: "approve", fn: (*Service).Approve},
		{name: "reject", fn: (*Service).Reject},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			if err := resolve.fn(service, ctx, approvalservice.ResolveApprovalCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("empty approval id err = %v, want invalid argument", err)
			}
			if err := resolve.fn(service, ctx, approvalservice.ResolveApprovalCommand{ApprovalID: common.ApprovalID("missing")}); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("missing approval err = %v, want not found", err)
			}
		})
	}
}

func TestService_CreateAndResolveApproval_PublishesEvents(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	hub := newRecordingApprovalHub(t)
	owner, err := New(hub, nil, Dependencies{Approvals: repo, EventHub: hub})
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
	if err := service.Approve(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: cmd.ApprovalID}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}
	item, err = repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get resolved approval: %v", err)
	}
	owner.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, item)

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

func TestService_ResolveApprovalMergesResolutionContext(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := NewService(repo)

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
		ApprovalID: approvalID,
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
	semantic, ok := item.Context.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	source, _ := semantic.String("source")
	if !ok || intent != "approve" || source != "tui" {
		t.Fatalf("governance semantic = %#v, want approve from tui", item.Context)
	}
}

func TestService_PublishesConcreteApprovalNotificationsThroughEventHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	hub := newRecordingApprovalHub(t)
	owner, err := New(hub, nil, Dependencies{Approvals: repo, EventHub: hub})
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
	if err := service.Approve(ctx, approvalservice.ResolveApprovalCommand{ApprovalID: cmd.ApprovalID}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}
	item, err = repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get resolved approval: %v", err)
	}
	owner.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, item)

	published := hub.Posted()
	if len(published) != 1 || published[0].ID() != approvalevents.TopicApprovalResolved {
		t.Fatalf("published = %+v, want approval resolved notification", published)
	}
	if _, ok := published[0].Data().(approvalevents.ApprovalChangedData); !ok {
		t.Fatalf("resolved data type = %T", published[0].Data())
	}
}
