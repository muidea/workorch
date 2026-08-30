package decision

import (
	"context"
	"errors"
	"reflect"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceDefinitionClarificationQuestionMatchesRequiredAction(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{
			name:   "definition relation",
			action: decisionevents.DefinitionClarificationActionRewriteAllOf,
			want:   "Revise the Definition into explicit all-of semantics",
		},
		{
			name:   "profile proof contract",
			action: decisionevents.DefinitionClarificationActionFixTemplateProfile,
			want:   "Select or fix the Workspace Template/Profile proof contract",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspaceDefinitionClarificationQuestion(tt.action); got != tt.want {
				t.Fatalf("workspaceDefinitionClarificationQuestion(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestWorkspaceDefinitionClarificationOrphanResolutionWaitsForDedicatedReplay(t *testing.T) {
	ctx := context.Background()
	hub := newRecordingDecisionHub(t)
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatalf("construct Decision owner: %v", err)
	}

	open := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	ticket, err := owner.service.applyLifecycleInternalEvent(ctx, nil, lifecycleInternalEvent{
		Kind:     lifecycleInternalEventOpened,
		TicketID: open.TicketID,
		Ticket:   workspaceDefinitionClarificationTicketForCommand(open),
	})
	if err != nil {
		t.Fatalf("persist clarification ticket: %v", err)
	}
	resolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: open.TicketID, WorkspaceID: open.WorkspaceID,
		OldCandidateID: open.CandidateID, OldCandidateRevision: open.CandidateRevision, OldCandidateHash: open.CandidateHash,
		NewCandidateID: open.CandidateID, NewCandidateRevision: open.CandidateRevision + 1, NewCandidateHash: "candidate-hash-recovered",
		AdmissionResultHash: "admission-hash-recovered", DefinitionRevision: "definition-revision-recovered", StartupInputHash: "startup-input-recovered",
	}.Normalized()
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(resolve.TicketID, resolve.AdmissionResultHash+"\x00"+resolve.StartupInputHash)
	orphan := &decisionmodel.Resolution{
		ID: resolutionID, TicketID: resolve.TicketID, ResolvedAt: newDecisionResolutionTimestamp(),
		Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("definitionRevision", resolve.DefinitionRevision),
			workorcheventpayload.StringField("startupInputHash", resolve.StartupInputHash),
		),
	}
	if err := repositories.Resolutions.Create(ctx, orphan); err != nil {
		t.Fatalf("persist orphan clarification resolution: %v", err)
	}

	if err := owner.reconcileDecisionTicket(ctx, ticket); err != nil {
		t.Fatalf("reconcile clarification orphan: %v", err)
	}
	unchanged, err := repositories.Tickets.Get(ctx, open.TicketID)
	if err != nil {
		t.Fatalf("read ticket after generic reconciliation: %v", err)
	}
	if unchanged.State != common.DecisionTicketStateOpen || unchanged.DefinitionClarification == nil || unchanged.DefinitionClarification.ResolutionID != "" {
		t.Fatalf("generic reconciliation mutated dedicated clarification = %+v", unchanged)
	}

	result, err := owner.resolveWorkspaceDefinitionClarification(ctx, resolve)
	if err != nil {
		t.Fatalf("dedicated clarification replay: %v", err)
	}
	if result.ValidateFor(resolve) != nil || result.Disposition != decisionevents.DefinitionClarificationResolved || result.ResolutionID != resolutionID {
		t.Fatalf("dedicated replay result = %+v", result)
	}
	persisted, err := repositories.Tickets.Get(ctx, open.TicketID)
	if err != nil {
		t.Fatalf("read resolved clarification: %v", err)
	}
	if persisted.State != common.DecisionTicketStateResolved || persisted.DefinitionClarification == nil || persisted.DefinitionClarification.ResolutionID != resolutionID || persisted.ResolvedAt != orphan.ResolvedAt {
		t.Fatalf("dedicated replay did not close exact binding = %+v", persisted)
	}
	if commands := hub.EvidenceCommands(); len(commands) != 0 {
		t.Fatalf("pre-seal dedicated replay emitted run-scoped evidence: %+v", commands)
	}
}

func TestWorkspaceDefinitionClarificationReconcileFencesResolutionOnlySuccessorBeforeOpeningAnotherTicket(t *testing.T) {
	baseCtx := context.Background()
	hub := newRecordingDecisionHub(t)
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatalf("construct Decision owner: %v", err)
	}

	oldRequest := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	ctx := common.ContextWithRuntimeWorkspaceID(baseCtx, oldRequest.WorkspaceID)
	if _, err := owner.openWorkspaceDefinitionClarification(ctx, oldRequest); err != nil {
		t.Fatalf("open old clarification: %v", err)
	}
	selectedRequest := oldRequest
	selectedRequest.CandidateRevision++
	selectedRequest.CandidateHash = "candidate-hash-selected"
	selectedRequest.CanonicalSignature = "one-of:selected"
	selectedRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", selectedRequest.CanonicalSignatureVersion, selectedRequest.CanonicalSignature)
	selectedRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(selectedRequest)
	selectedRequest = selectedRequest.Normalized()
	selectedCommand := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{
		OldRequest: oldRequest, NewRequest: selectedRequest,
	}.Normalized()
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(oldRequest.TicketID, selectedRequest.BindingHash())
	resolution := &decisionmodel.Resolution{
		ID: resolutionID, TicketID: oldRequest.TicketID, ResolvedAt: newDecisionResolutionTimestamp(),
		Type: decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("newTicketId", string(selectedRequest.TicketID)),
			workorcheventpayload.StringField("newCandidateHash", selectedRequest.CandidateHash),
		),
	}
	if err := repositories.Resolutions.Create(ctx, resolution); err != nil {
		t.Fatalf("persist selected resolution cut point: %v", err)
	}

	driftedRequest := selectedRequest
	driftedRequest.CandidateRevision++
	driftedRequest.CandidateHash = "candidate-hash-drifted"
	driftedRequest.CanonicalSignature = "one-of:drifted"
	driftedRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", driftedRequest.CanonicalSignatureVersion, driftedRequest.CanonicalSignature)
	driftedRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(driftedRequest)
	drifted := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{
		OldRequest: oldRequest, NewRequest: driftedRequest,
	}.Normalized()
	if _, err := owner.reconcileWorkspaceDefinitionClarification(ctx, drifted); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution-only drift error = %v, want conflict", err)
	}
	if _, err := repositories.Tickets.Get(ctx, driftedRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("resolution-only drift opened unrelated blocker: %v", err)
	}
	oldTicket, err := repositories.Tickets.Get(ctx, oldRequest.TicketID)
	if err != nil || oldTicket.State != common.DecisionTicketStateOpen || oldTicket.DefinitionClarification == nil || oldTicket.DefinitionClarification.ResolutionID != "" {
		t.Fatalf("resolution-only drift changed old ticket = %+v err=%v", oldTicket, err)
	}

	result, err := owner.reconcileWorkspaceDefinitionClarification(ctx, selectedCommand)
	if err != nil || result.ValidateFor(selectedCommand) != nil || result.NewTicketDisposition != decisionevents.DefinitionClarificationCreated {
		t.Fatalf("exact cut-point replay result = %+v err=%v", result, err)
	}
	oldTicket, err = repositories.Tickets.Get(ctx, oldRequest.TicketID)
	if err != nil || oldTicket.State != common.DecisionTicketStateResolved || oldTicket.DefinitionClarification == nil ||
		oldTicket.DefinitionClarification.ResolutionID != resolutionID ||
		oldTicket.DefinitionClarification.ResolutionBindingHash != selectedCommand.BindingHash(resolutionID) {
		t.Fatalf("exact cut-point replay did not bind selected resolution = %+v err=%v", oldTicket, err)
	}
}

func TestWorkspaceDefinitionClarificationReconcileRejectsCorruptBoundStateBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name              string
		persistSuccessor  bool
		persistResolution bool
	}{
		{name: "missing successor", persistResolution: true},
		{name: "missing resolution", persistSuccessor: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			hub := newRecordingDecisionHub(t)
			repositories := memory.NewDecisionOwnerRepositorySet()
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
			if err != nil {
				t.Fatalf("construct Decision owner: %v", err)
			}
			oldRequest, _, exact := workspaceClarificationFileCutpointCommands()
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), oldRequest.WorkspaceID)
			if _, err := owner.openWorkspaceDefinitionClarification(ctx, oldRequest); err != nil {
				t.Fatalf("open old clarification: %v", err)
			}
			if test.persistSuccessor {
				if _, err := owner.openWorkspaceDefinitionClarification(ctx, exact.NewRequest); err != nil {
					t.Fatalf("persist successor clarification: %v", err)
				}
			}
			resolution := workspaceClarificationSupersededResolution(exact)
			if test.persistResolution {
				if err := repositories.Resolutions.Create(ctx, resolution); err != nil {
					t.Fatalf("persist reconciliation resolution: %v", err)
				}
			}
			oldTicket, err := repositories.Tickets.Get(ctx, oldRequest.TicketID)
			if err != nil {
				t.Fatalf("load old clarification: %v", err)
			}
			oldTicket.State = common.DecisionTicketStateResolved
			oldTicket.ResolvedAt = resolution.ResolvedAt
			oldTicket.DefinitionClarification.ResolutionID = resolution.ID
			oldTicket.DefinitionClarification.ResolutionBindingHash = exact.BindingHash(resolution.ID)
			if err := repositories.Tickets.Update(ctx, oldTicket); err != nil {
				t.Fatalf("persist corrupt bound clarification: %v", err)
			}
			before, err := repositories.Tickets.Get(ctx, oldRequest.TicketID)
			if err != nil {
				t.Fatalf("reload corrupt bound clarification: %v", err)
			}
			postedBefore := len(hub.Posted())

			if _, err := owner.reconcileWorkspaceDefinitionClarification(ctx, exact); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("corrupt bound replay error = %v, want conflict", err)
			}
			after, err := repositories.Tickets.Get(ctx, oldRequest.TicketID)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("corrupt bound replay mutated old ticket: before=%+v after=%+v err=%v", before, after, err)
			}
			if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
				t.Fatalf("corrupt bound replay published notification: before=%d after=%d", postedBefore, postedAfter)
			}
			if !test.persistSuccessor {
				if _, err := repositories.Tickets.Get(ctx, exact.NewRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
					t.Fatalf("corrupt replay reconstructed missing successor: %v", err)
				}
			}
			if !test.persistResolution {
				if _, err := repositories.Resolutions.GetByTicketID(ctx, oldRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
					t.Fatalf("corrupt replay reconstructed missing resolution: %v", err)
				}
			}
		})
	}
}

func TestWorkspaceDefinitionClarificationReconcileRejectsCorruptSuccessorBeforeSelectingResolution(t *testing.T) {
	hub := newRecordingDecisionHub(t)
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatalf("construct Decision owner: %v", err)
	}
	open, _, reconcile := workspaceClarificationFileCutpointCommands()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
	if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); err != nil {
		t.Fatalf("open old clarification: %v", err)
	}
	successor, err := owner.service.applyLifecycleInternalEvent(ctx, nil, lifecycleInternalEvent{
		Kind: lifecycleInternalEventOpened, TicketID: reconcile.NewRequest.TicketID,
		Ticket: workspaceDefinitionClarificationTicketForCommand(reconcile.NewRequest),
	})
	if err != nil {
		t.Fatalf("persist successor creation cut: %v", err)
	}
	// Preserve every immutable opening field while forging a resolved outer
	// lifecycle without its required Decision-owned resolution binding.
	successor.State = common.DecisionTicketStateResolved
	successor.ResolvedAt = "2026-08-30T00:00:00Z"
	if err := repositories.Tickets.Update(ctx, successor); err != nil {
		t.Fatalf("persist corrupt successor lifecycle: %v", err)
	}
	oldBefore, err := repositories.Tickets.Get(ctx, open.TicketID)
	if err != nil {
		t.Fatalf("load old clarification: %v", err)
	}
	successorBefore, err := repositories.Tickets.Get(ctx, reconcile.NewRequest.TicketID)
	if err != nil {
		t.Fatalf("load corrupt successor: %v", err)
	}
	postedBefore := len(hub.Posted())

	if _, err := owner.reconcileWorkspaceDefinitionClarification(ctx, reconcile); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("corrupt successor reconcile error = %v, want conflict", err)
	}
	if _, err := repositories.Resolutions.GetByTicketID(ctx, open.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("corrupt successor selected old-ticket resolution: %v", err)
	}
	oldAfter, err := repositories.Tickets.Get(ctx, open.TicketID)
	if err != nil || !reflect.DeepEqual(oldAfter, oldBefore) {
		t.Fatalf("corrupt successor mutated old ticket: before=%+v after=%+v err=%v", oldBefore, oldAfter, err)
	}
	successorAfter, err := repositories.Tickets.Get(ctx, reconcile.NewRequest.TicketID)
	if err != nil || !reflect.DeepEqual(successorAfter, successorBefore) {
		t.Fatalf("corrupt successor replay mutated successor: before=%+v after=%+v err=%v", successorBefore, successorAfter, err)
	}
	if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
		t.Fatalf("corrupt successor replay published notification: before=%d after=%d", postedBefore, postedAfter)
	}
}

func TestWorkspaceDefinitionClarificationReconcileRejectsResolvedSuccessorWithoutResolutionBeforeMutation(t *testing.T) {
	hub := newRecordingDecisionHub(t)
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatalf("construct Decision owner: %v", err)
	}
	open, _, reconcile := workspaceClarificationFileCutpointCommands()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
	if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); err != nil {
		t.Fatalf("open old clarification: %v", err)
	}
	if _, err := owner.openWorkspaceDefinitionClarification(ctx, reconcile.NewRequest); err != nil {
		t.Fatalf("open successor clarification: %v", err)
	}
	successorResolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: reconcile.NewRequest.TicketID, WorkspaceID: reconcile.NewRequest.WorkspaceID,
		OldCandidateID: reconcile.NewRequest.CandidateID, OldCandidateRevision: reconcile.NewRequest.CandidateRevision, OldCandidateHash: reconcile.NewRequest.CandidateHash,
		NewCandidateID: reconcile.NewRequest.CandidateID, NewCandidateRevision: reconcile.NewRequest.CandidateRevision + 1, NewCandidateHash: "candidate-hash-successor-resolved",
		AdmissionResultHash: "admission-hash-successor-resolved", DefinitionRevision: "definition-successor-resolved", StartupInputHash: "startup-successor-resolved",
	}.Normalized()
	if _, err := owner.resolveWorkspaceDefinitionClarification(ctx, successorResolve); err != nil {
		t.Fatalf("resolve successor clarification: %v", err)
	}
	if err := repositories.Resolutions.DeleteByTicketID(ctx, reconcile.NewRequest.TicketID); err != nil {
		t.Fatalf("remove successor Resolution row: %v", err)
	}
	oldBefore, err := repositories.Tickets.Get(ctx, open.TicketID)
	if err != nil {
		t.Fatalf("load old clarification: %v", err)
	}
	successorBefore, err := repositories.Tickets.Get(ctx, reconcile.NewRequest.TicketID)
	if err != nil {
		t.Fatalf("load resolved successor: %v", err)
	}
	postedBefore := len(hub.Posted())

	if _, err := owner.reconcileWorkspaceDefinitionClarification(ctx, reconcile); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolved successor without Resolution error=%v, want conflict", err)
	}
	if _, err := repositories.Resolutions.GetByTicketID(ctx, open.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("corrupt successor selected old Resolution: %v", err)
	}
	oldAfter, oldErr := repositories.Tickets.Get(ctx, open.TicketID)
	successorAfter, successorErr := repositories.Tickets.Get(ctx, reconcile.NewRequest.TicketID)
	if oldErr != nil || successorErr != nil || !reflect.DeepEqual(oldAfter, oldBefore) || !reflect.DeepEqual(successorAfter, successorBefore) {
		t.Fatalf("corrupt successor preflight mutated tickets: old=%+v/%+v successor=%+v/%+v errors=%v,%v", oldBefore, oldAfter, successorBefore, successorAfter, oldErr, successorErr)
	}
	if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
		t.Fatalf("corrupt successor preflight published notification: before=%d after=%d", postedBefore, postedAfter)
	}
}

func TestDefinitionClarificationResolvedOpenRejectsMissingResolutionRecordBeforeNotification(t *testing.T) {
	t.Run("Workspace", func(t *testing.T) {
		hub := newRecordingDecisionHub(t)
		repositories := memory.NewDecisionOwnerRepositorySet()
		owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
		if err != nil {
			t.Fatalf("construct Decision owner: %v", err)
		}
		open, resolve, _ := workspaceClarificationFileCutpointCommands()
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
		if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); err != nil {
			t.Fatalf("open Workspace clarification: %v", err)
		}
		if _, err := owner.resolveWorkspaceDefinitionClarification(ctx, resolve); err != nil {
			t.Fatalf("resolve Workspace clarification: %v", err)
		}
		if err := repositories.Resolutions.DeleteByTicketID(ctx, open.TicketID); err != nil {
			t.Fatalf("remove Workspace Resolution row: %v", err)
		}
		ticket, err := repositories.Tickets.Get(ctx, open.TicketID)
		if err != nil {
			t.Fatal(err)
		}
		ticket.ResolutionNotificationPublishedAt = ""
		if err := repositories.Tickets.Update(ctx, ticket); err != nil {
			t.Fatal(err)
		}
		before, _ := repositories.Tickets.Get(ctx, open.TicketID)
		postedBefore := len(hub.Posted())
		if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("resolved Workspace duplicate open error=%v, want conflict", err)
		}
		after, loadErr := repositories.Tickets.Get(ctx, open.TicketID)
		if loadErr != nil || !reflect.DeepEqual(after, before) || len(hub.Posted()) != postedBefore {
			t.Fatalf("resolved Workspace duplicate open mutated delivery: before=%+v after=%+v err=%v posts=%d/%d", before, after, loadErr, postedBefore, len(hub.Posted()))
		}
	})

	t.Run("Runtime", func(t *testing.T) {
		hub := newRecordingDecisionHub(t)
		repositories := memory.NewDecisionOwnerRepositorySet()
		owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
		if err != nil {
			t.Fatalf("construct Decision owner: %v", err)
		}
		open := runtimeDefinitionClarificationSourceCommand("missing-resolution")
		resolve := runtimeClarificationResolutionCommand(open)
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
		if _, err := owner.openRuntimeDefinitionClarification(ctx, open); err != nil {
			t.Fatalf("open Runtime clarification: %v", err)
		}
		if _, err := owner.resolveRuntimeDefinitionClarification(ctx, resolve); err != nil {
			t.Fatalf("resolve Runtime clarification: %v", err)
		}
		if err := repositories.Resolutions.DeleteByTicketID(ctx, open.TicketID); err != nil {
			t.Fatalf("remove Runtime Resolution row: %v", err)
		}
		ticket, err := repositories.Tickets.Get(ctx, open.TicketID)
		if err != nil {
			t.Fatal(err)
		}
		ticket.ResolutionNotificationPublishedAt = ""
		if err := repositories.Tickets.Update(ctx, ticket); err != nil {
			t.Fatal(err)
		}
		before, _ := repositories.Tickets.Get(ctx, open.TicketID)
		postedBefore := len(hub.Posted())
		if _, err := owner.openRuntimeDefinitionClarification(ctx, open); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("resolved Runtime duplicate open error=%v, want conflict", err)
		}
		after, loadErr := repositories.Tickets.Get(ctx, open.TicketID)
		if loadErr != nil || !reflect.DeepEqual(after, before) || len(hub.Posted()) != postedBefore {
			t.Fatalf("resolved Runtime duplicate open mutated delivery: before=%+v after=%+v err=%v posts=%d/%d", before, after, loadErr, postedBefore, len(hub.Posted()))
		}
	})
}

func TestDefinitionClarificationResolvedReplayRejectsResolutionTimestampDriftBeforeDelivery(t *testing.T) {
	t.Run("Workspace", func(t *testing.T) {
		hub := newRecordingDecisionHub(t)
		repositories := memory.NewDecisionOwnerRepositorySet()
		owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
		if err != nil {
			t.Fatalf("construct Decision owner: %v", err)
		}
		open, resolve, _ := workspaceClarificationFileCutpointCommands()
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
		if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.resolveWorkspaceDefinitionClarification(ctx, resolve); err != nil {
			t.Fatal(err)
		}
		ticket, _ := repositories.Tickets.Get(ctx, open.TicketID)
		ticket.ResolvedAt = "2026-08-30T12:00:00Z"
		ticket.ResolutionNotificationPublishedAt = ""
		if err := repositories.Tickets.Update(ctx, ticket); err != nil {
			t.Fatal(err)
		}
		before, _ := repositories.Tickets.Get(ctx, open.TicketID)
		resolutionBefore, _ := repositories.Resolutions.GetByTicketID(ctx, open.TicketID)
		postedBefore := len(hub.Posted())
		if _, err := owner.resolveWorkspaceDefinitionClarification(ctx, resolve); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("Workspace timestamp drift replay error=%v, want conflict", err)
		}
		after, ticketErr := repositories.Tickets.Get(ctx, open.TicketID)
		resolutionAfter, resolutionErr := repositories.Resolutions.GetByTicketID(ctx, open.TicketID)
		if ticketErr != nil || resolutionErr != nil || !reflect.DeepEqual(after, before) || !reflect.DeepEqual(resolutionAfter, resolutionBefore) || len(hub.Posted()) != postedBefore {
			t.Fatalf("Workspace timestamp drift replay mutated owner state/delivery: ticket=%+v/%+v resolution=%+v/%+v errors=%v,%v posts=%d/%d", before, after, resolutionBefore, resolutionAfter, ticketErr, resolutionErr, postedBefore, len(hub.Posted()))
		}
	})

	t.Run("Runtime", func(t *testing.T) {
		hub := newRecordingDecisionHub(t)
		repositories := memory.NewDecisionOwnerRepositorySet()
		owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
		if err != nil {
			t.Fatalf("construct Decision owner: %v", err)
		}
		open := runtimeDefinitionClarificationSourceCommand("timestamp-drift")
		resolve := runtimeClarificationResolutionCommand(open)
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
		if _, err := owner.openRuntimeDefinitionClarification(ctx, open); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.resolveRuntimeDefinitionClarification(ctx, resolve); err != nil {
			t.Fatal(err)
		}
		ticket, _ := repositories.Tickets.Get(ctx, open.TicketID)
		ticket.ResolvedAt = "2026-08-30T12:00:00Z"
		ticket.ResolutionNotificationPublishedAt = ""
		if err := repositories.Tickets.Update(ctx, ticket); err != nil {
			t.Fatal(err)
		}
		before, _ := repositories.Tickets.Get(ctx, open.TicketID)
		resolutionBefore, _ := repositories.Resolutions.GetByTicketID(ctx, open.TicketID)
		postedBefore := len(hub.Posted())
		if _, err := owner.resolveRuntimeDefinitionClarification(ctx, resolve); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("Runtime timestamp drift replay error=%v, want conflict", err)
		}
		after, ticketErr := repositories.Tickets.Get(ctx, open.TicketID)
		resolutionAfter, resolutionErr := repositories.Resolutions.GetByTicketID(ctx, open.TicketID)
		if ticketErr != nil || resolutionErr != nil || !reflect.DeepEqual(after, before) || !reflect.DeepEqual(resolutionAfter, resolutionBefore) || len(hub.Posted()) != postedBefore {
			t.Fatalf("Runtime timestamp drift replay mutated owner state/delivery: ticket=%+v/%+v resolution=%+v/%+v errors=%v,%v posts=%d/%d", before, after, resolutionBefore, resolutionAfter, ticketErr, resolutionErr, postedBefore, len(hub.Posted()))
		}
	})
}

func TestWorkspaceDefinitionClarificationMutationIngressRejectsOuterTicketDriftBeforeWrite(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "step", mutate: func(ticket *decisionmodel.Ticket) { ticket.StepID = "forged-step" }},
		{name: "reason", mutate: func(ticket *decisionmodel.Ticket) { ticket.Reason = "forged-reason" }},
		{name: "decision kind", mutate: func(ticket *decisionmodel.Ticket) { ticket.DecisionKind = "ordinary" }},
		{name: "question", mutate: func(ticket *decisionmodel.Ticket) { ticket.Question = "forged question" }},
		{name: "failure class", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureClass = "forged" }},
		{name: "failure signature", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureSignature = "forged" }},
		{name: "recovery actions", mutate: func(ticket *decisionmodel.Ticket) { ticket.AllowedRecoveryActions = []string{"forged"} }},
		{name: "state time", mutate: func(ticket *decisionmodel.Ticket) { ticket.State = common.DecisionTicketStateResolved }},
	}
	for _, operation := range []string{"reconcile", "resolve"} {
		for _, mutation := range mutations {
			t.Run(operation+"/"+mutation.name, func(t *testing.T) {
				hub := newRecordingDecisionHub(t)
				repositories := memory.NewDecisionOwnerRepositorySet()
				owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
				if err != nil {
					t.Fatalf("construct Decision owner: %v", err)
				}
				open, resolve, reconcile := workspaceClarificationFileCutpointCommands()
				ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
				if _, err := owner.openWorkspaceDefinitionClarification(ctx, open); err != nil {
					t.Fatalf("open Workspace clarification: %v", err)
				}
				ticket, err := repositories.Tickets.Get(ctx, open.TicketID)
				if err != nil {
					t.Fatalf("load Workspace clarification: %v", err)
				}
				mutation.mutate(ticket)
				if err := repositories.Tickets.Update(ctx, ticket); err != nil {
					t.Fatalf("persist outer drift: %v", err)
				}
				before, err := repositories.Tickets.Get(ctx, open.TicketID)
				if err != nil {
					t.Fatalf("reload outer drift: %v", err)
				}
				postedBefore := len(hub.Posted())

				switch operation {
				case "reconcile":
					_, err = owner.reconcileWorkspaceDefinitionClarification(ctx, reconcile)
				case "resolve":
					_, err = owner.resolveWorkspaceDefinitionClarification(ctx, resolve)
				}
				if !errors.Is(err, common.ErrConflict) {
					t.Fatalf("%s outer-drift error = %v, want conflict", operation, err)
				}
				after, loadErr := repositories.Tickets.Get(ctx, open.TicketID)
				if loadErr != nil || !reflect.DeepEqual(after, before) {
					t.Fatalf("%s outer-drift mutated ticket: before=%+v after=%+v err=%v", operation, before, after, loadErr)
				}
				if _, loadErr := repositories.Resolutions.GetByTicketID(ctx, open.TicketID); !errors.Is(loadErr, common.ErrNotFound) {
					t.Fatalf("%s outer-drift persisted resolution: %v", operation, loadErr)
				}
				if _, loadErr := repositories.Tickets.Get(ctx, reconcile.NewRequest.TicketID); !errors.Is(loadErr, common.ErrNotFound) {
					t.Fatalf("%s outer-drift opened successor: %v", operation, loadErr)
				}
				if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
					t.Fatalf("%s outer-drift published notification: before=%d after=%d", operation, postedBefore, postedAfter)
				}
			})
		}
	}
}

func TestOpenRuntimeDefinitionClarificationRejectsPersistedSemanticDriftBeforeNotification(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "step", mutate: func(ticket *decisionmodel.Ticket) { ticket.StepID = "forged-step" }},
		{name: "reason", mutate: func(ticket *decisionmodel.Ticket) { ticket.Reason = "forged-reason" }},
		{name: "decision kind", mutate: func(ticket *decisionmodel.Ticket) { ticket.DecisionKind = "ordinary" }},
		{name: "question", mutate: func(ticket *decisionmodel.Ticket) { ticket.Question = "forged question" }},
		{name: "failure class", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureClass = "forged" }},
		{name: "failure signature", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureSignature = "forged" }},
		{name: "recovery actions", mutate: func(ticket *decisionmodel.Ticket) { ticket.AllowedRecoveryActions = []string{"forged"} }},
		{name: "binding refs", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.DefinitionClarification.OffendingRefs = []string{"forged-ref"}
		}},
		{name: "binding focus", mutate: func(ticket *decisionmodel.Ticket) { ticket.DefinitionClarification.FocusRef = "forged-focus" }},
		{name: "binding lifecycle", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.DefinitionClarification.ReplacementIntentID = "forged-intent"
		}},
		{name: "state time", mutate: func(ticket *decisionmodel.Ticket) { ticket.State = common.DecisionTicketStateResolved }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			hub := newRecordingDecisionHub(t)
			repositories := memory.NewDecisionOwnerRepositorySet()
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
			if err != nil {
				t.Fatalf("construct Decision owner: %v", err)
			}
			command := runtimeDefinitionClarificationSourceCommand("semantic-drift")
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), command.WorkspaceID)
			ticket, err := owner.service.applyLifecycleInternalEvent(ctx, nil, lifecycleInternalEvent{
				Kind: lifecycleInternalEventOpened, TicketID: command.TicketID,
				Ticket: runtimeDefinitionClarificationTicketForCommand(command),
			})
			if err != nil {
				t.Fatalf("persist Runtime clarification creation cut: %v", err)
			}
			mutation.mutate(ticket)
			if err := repositories.Tickets.Update(ctx, ticket); err != nil {
				t.Fatalf("persist Runtime clarification drift: %v", err)
			}
			before, err := repositories.Tickets.Get(ctx, command.TicketID)
			if err != nil {
				t.Fatalf("reload Runtime clarification drift: %v", err)
			}

			if _, err := owner.openRuntimeDefinitionClarification(ctx, command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("Runtime duplicate open drift error = %v, want conflict", err)
			}
			after, loadErr := repositories.Tickets.Get(ctx, command.TicketID)
			if loadErr != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("Runtime duplicate open drift mutated ticket: before=%+v after=%+v err=%v", before, after, loadErr)
			}
			if after.CreatedNotificationPublishedAt != "" || len(hub.Posted()) != 0 {
				t.Fatalf("Runtime duplicate open drift published creation: ticket=%+v posted=%d", after, len(hub.Posted()))
			}
		})
	}
}

func TestResolveRuntimeDefinitionClarificationRejectsPersistedSemanticDriftBeforeWrite(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "outer decision kind", mutate: func(ticket *decisionmodel.Ticket) { ticket.DecisionKind = "ordinary" }},
		{name: "opening binding", mutate: func(ticket *decisionmodel.Ticket) { ticket.DefinitionClarification.FocusRef = "forged-focus" }},
		{name: "open lifecycle binding", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.DefinitionClarification.ReplacementIntentID = "forged-intent"
		}},
		{name: "resolved state without binding", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.State = common.DecisionTicketStateResolved
			ticket.ResolvedAt = "2026-08-30T00:00:00Z"
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			hub := newRecordingDecisionHub(t)
			repositories := memory.NewDecisionOwnerRepositorySet()
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
			if err != nil {
				t.Fatalf("construct Decision owner: %v", err)
			}
			open := runtimeDefinitionClarificationSourceCommand("resolve-drift")
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
			if _, err := owner.openRuntimeDefinitionClarification(ctx, open); err != nil {
				t.Fatalf("open Runtime clarification: %v", err)
			}
			ticket, err := repositories.Tickets.Get(ctx, open.TicketID)
			if err != nil {
				t.Fatalf("load Runtime clarification: %v", err)
			}
			mutation.mutate(ticket)
			if err := repositories.Tickets.Update(ctx, ticket); err != nil {
				t.Fatalf("persist Runtime clarification drift: %v", err)
			}
			before, err := repositories.Tickets.Get(ctx, open.TicketID)
			if err != nil {
				t.Fatalf("reload Runtime clarification drift: %v", err)
			}
			postedBefore := len(hub.Posted())
			command := runtimeClarificationResolutionCommand(open)

			if _, err := owner.resolveRuntimeDefinitionClarification(ctx, command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("Runtime resolve drift error = %v, want conflict", err)
			}
			after, loadErr := repositories.Tickets.Get(ctx, open.TicketID)
			if loadErr != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("Runtime resolve drift mutated ticket: before=%+v after=%+v err=%v", before, after, loadErr)
			}
			if _, loadErr := repositories.Resolutions.GetByTicketID(ctx, open.TicketID); !errors.Is(loadErr, common.ErrNotFound) {
				t.Fatalf("Runtime resolve drift persisted resolution: %v", loadErr)
			}
			if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
				t.Fatalf("Runtime resolve drift published notification: before=%d after=%d", postedBefore, postedAfter)
			}
		})
	}
}

func runtimeClarificationResolutionCommand(open decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand {
	return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand{
		TicketID: open.TicketID, WorkspaceID: open.WorkspaceID, PredecessorRunID: open.RunID,
		ReplacementIntentID:           "replacement-intent-semantic-guard",
		PredecessorDefinitionRevision: open.DefinitionRevision, PredecessorStartupInputHash: open.StartupInputHash,
		SuccessorDefinitionRevision: open.DefinitionRevision + "-successor",
		SuccessorStartupInputHash:   open.StartupInputHash + "-successor",
		ReplacementIntentHash:       "replacement-intent-hash-semantic-guard",
	}.Normalized()
}

func workspaceDefinitionClarificationCommandForBizTest() decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand {
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:   "workspace-1", CandidateID: "workspace-definition", CandidateRevision: 1,
		CandidateHash: "candidate-hash-1", ViolationCode: "definition_one_of_unsupported",
		OffendingRefs: []string{"dod:item-1", "goal:item-1"}, CanonicalSignatureVersion: "v1",
		CanonicalSignature: "one-of:dod:item-1", RequiredActionKind: decisionevents.DefinitionClarificationActionRewriteAllOf,
	}
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}

func TestOpenWorkspaceDefinitionClarificationReplayMatchesPersistedFieldsNotOnlyHash(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	for _, test := range []struct {
		name    string
		mutate  func(*decisionmodel.DefinitionClarificationBinding)
		wantErr bool
	}{
		{name: "exact replay"},
		{name: "forged offending refs with unchanged hash", wantErr: true, mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.OffendingRefs = []string{"dod:item-forged"}
		}},
		{name: "forged canonical signature with unchanged hash", wantErr: true, mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.CanonicalSignature = "forged"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repositories := memory.NewDecisionOwnerRepositorySet()
			tickets := repositories.Tickets
			binding := workspaceDefinitionClarificationBindingForCommand(command)
			if test.mutate != nil {
				test.mutate(binding)
			}
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateOpen
			ticket.DefinitionClarification = binding
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatalf("seed ticket: %v", err)
			}
			hub := newRecordingDecisionHub(t)
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
			if err != nil {
				t.Fatalf("construct Decision owner: %v", err)
			}
			result, err := owner.openWorkspaceDefinitionClarification(context.Background(), command)
			if test.wantErr {
				if !errors.Is(err, common.ErrConflict) {
					t.Fatalf("forged replay error = %v, want conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("exact replay: %v", err)
			}
			if result.Disposition != decisionevents.DefinitionClarificationDuplicate {
				t.Fatalf("disposition = %q, want duplicate", result.Disposition)
			}
		})
	}
}

func TestQueryWorkspaceDefinitionClarificationFactReadsExactPersistedTicketAndResolution(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	binding := workspaceDefinitionClarificationBindingForCommand(command)
	resolutionCommand := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID,
		OldCandidateID: command.CandidateID, OldCandidateRevision: command.CandidateRevision, OldCandidateHash: command.CandidateHash,
		NewCandidateID: command.CandidateID, NewCandidateRevision: 2, NewCandidateHash: "candidate-hash-2",
		AdmissionResultHash: "admission-hash-2", DefinitionRevision: "definition-2", StartupInputHash: "startup-2",
	}
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(command.TicketID, resolutionCommand.AdmissionResultHash+"\x00"+resolutionCommand.StartupInputHash)
	binding.ResolutionID = resolutionID
	binding.ResolutionBindingHash = resolutionCommand.BindingHash(resolutionID)
	binding.ResolutionCandidateID = resolutionCommand.NewCandidateID
	binding.ResolutionCandidateRevision = resolutionCommand.NewCandidateRevision
	binding.ResolutionCandidateHash = resolutionCommand.NewCandidateHash
	binding.AdmissionResultHash = resolutionCommand.AdmissionResultHash
	binding.DefinitionRevision = resolutionCommand.DefinitionRevision
	binding.StartupInputHash = resolutionCommand.StartupInputHash
	ticket := workspaceDefinitionClarificationTicketForCommand(command)
	ticket.State = common.DecisionTicketStateResolved
	ticket.ResolvedAt = "2026-08-25T12:00:00Z"
	// Notification delivery is a later idempotent cut point and may already be
	// finalized when another owner queries the durable clarification fact.
	ticket.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z"
	ticket.DefinitionClarification = binding
	if err := tickets.Create(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if err := resolutions.Create(context.Background(), &decisionmodel.Resolution{
		ID: resolutionID, TicketID: command.TicketID, ResolvedAt: ticket.ResolvedAt, Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("definitionRevision", resolutionCommand.DefinitionRevision),
			workorcheventpayload.StringField("startupInputHash", resolutionCommand.StartupInputHash),
		),
	}); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{tickets: tickets, resolutions: resolutions}
	query := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{WorkspaceID: command.WorkspaceID, TicketID: command.TicketID}
	result, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), query)
	if err != nil || result.ValidateFor(query) != nil || !result.Found || result.Ticket == nil || result.Resolution == nil {
		t.Fatalf("persisted fact=%+v err=%v", result, err)
	}
	if !result.Ticket.RequestIdentity.EqualNormalized(definitioncontract.WorkspaceDefinitionClarificationRequestIdentity(command)) || result.Resolution.ResolutionIdentity != definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(resolutionCommand) {
		t.Fatalf("persisted fact identity drift: %+v", result)
	}
}

func TestQueryWorkspaceDefinitionClarificationFactRejectsRetiredOrPartialResolution(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	for _, test := range []struct {
		name   string
		mutate func(*decisionmodel.DefinitionClarificationBinding)
	}{
		{name: "retired superseded ticket", mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.ResolutionID = "resolution-superseded"
			binding.ResolutionBindingHash = "reconciliation-binding"
		}},
		{name: "partial admission resolution", mutate: func(binding *decisionmodel.DefinitionClarificationBinding) {
			binding.ResolutionID = "resolution-partial"
			binding.ResolutionBindingHash = "resolution-binding"
			binding.ResolutionCandidateID = command.CandidateID
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tickets := memory.NewDecisionTicketRepository()
			binding := workspaceDefinitionClarificationBindingForCommand(command)
			test.mutate(binding)
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateResolved
			ticket.ResolvedAt = "2026-08-25T12:00:00Z"
			ticket.DefinitionClarification = binding
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{tickets: tickets, resolutions: memory.NewDecisionResolutionRepository()}
			_, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{
				WorkspaceID: command.WorkspaceID, TicketID: command.TicketID,
			})
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("query error=%v, want conflict", err)
			}
		})
	}
}

func TestQueryWorkspaceDefinitionClarificationFactRejectsOuterTicketSemanticDrift(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	mutations := []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "decision kind", mutate: func(ticket *decisionmodel.Ticket) { ticket.DecisionKind = "other" }},
		{name: "reason", mutate: func(ticket *decisionmodel.Ticket) { ticket.Reason = "other" }},
		{name: "question", mutate: func(ticket *decisionmodel.Ticket) { ticket.Question = "other" }},
		{name: "failure class", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureClass = "other" }},
		{name: "failure signature", mutate: func(ticket *decisionmodel.Ticket) { ticket.FailureSignature = "other" }},
		{name: "allowed recovery actions", mutate: func(ticket *decisionmodel.Ticket) { ticket.AllowedRecoveryActions = []string{"other"} }},
		{name: "step id", mutate: func(ticket *decisionmodel.Ticket) { ticket.StepID = "step-1" }},
		{name: "context", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.Context = workorcheventpayload.New(workorcheventpayload.StringField("forged", "value"))
		}},
		{name: "auto governance", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.AutoGovernance = workorcheventpayload.New(workorcheventpayload.StringField("forged", "value"))
		}},
		{name: "runtime governance proposal", mutate: func(ticket *decisionmodel.Ticket) {
			ticket.RuntimeGovernanceProposal = &decisionmodel.RuntimeGovernanceResolutionProposal{ResolutionID: "resolution-forged"}
		}},
		{name: "open resolved at", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedAt = "2026-08-25T12:00:00Z" }},
		{name: "open resolved by", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedBy = "operator" }},
		{name: "open resolution notification", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z" }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			tickets := memory.NewDecisionTicketRepository()
			ticket := workspaceDefinitionClarificationTicketForCommand(command)
			ticket.State = common.DecisionTicketStateOpen
			test.mutate(ticket)
			if err := tickets.Create(context.Background(), ticket); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{tickets: tickets, resolutions: memory.NewDecisionResolutionRepository()}
			_, err := owner.queryWorkspaceDefinitionClarificationFact(context.Background(), decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{
				WorkspaceID: command.WorkspaceID, TicketID: command.TicketID,
			})
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("query error=%v, want conflict", err)
			}
		})
	}
}

func TestWorkspaceDefinitionClarificationTicketSemanticsRejectsResolvedTimeDrift(t *testing.T) {
	command := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	valid := workspaceDefinitionClarificationTicketForCommand(command)
	valid.State = common.DecisionTicketStateResolved
	valid.ResolvedAt = "2026-08-25T12:00:00Z"
	if !workspaceDefinitionClarificationTicketSemanticsMatch(valid, command) {
		t.Fatal("resolved Definition clarification with owner-stamped time was rejected")
	}
	withNotification := decisionmodel.CloneTicket(valid)
	withNotification.ResolutionNotificationPublishedAt = "2026-08-25T12:00:01Z"
	if !workspaceDefinitionClarificationTicketSemanticsMatch(withNotification, command) {
		t.Fatal("resolved Definition clarification rejected optional published-notification cut point")
	}

	for _, test := range []struct {
		name   string
		mutate func(*decisionmodel.Ticket)
	}{
		{name: "resolved at missing", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedAt = "" }},
		{name: "resolved by forged", mutate: func(ticket *decisionmodel.Ticket) { ticket.ResolvedBy = "operator" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ticket := decisionmodel.CloneTicket(valid)
			test.mutate(ticket)
			if workspaceDefinitionClarificationTicketSemanticsMatch(ticket, command) {
				t.Fatalf("resolved state-time drift was accepted: %+v", ticket)
			}
		})
	}
}
