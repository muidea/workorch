package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func terminalDeliveryTestCommand(identity runtimecontract.RuntimeTerminalDeliveryIdentity, causationID string) runevents.RuntimeTerminalDeliveryFact {
	return runevents.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryIdentity: identity,
		SourceOwner:                     "kernel/orchestrator",
		CausationID:                     causationID,
	}}
}

func newLifecycleTestBiz(t *testing.T) (*Biz, *memory.RunRepository, magicevent.Hub) {
	t.Helper()
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(32)
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := biz.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = biz.Stop(context.Background()) })
	return biz, runs, hub
}

func seedRun(t *testing.T, runs *memory.RunRepository, state common.RunState) *runmodel.Run {
	t.Helper()
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", State: state, CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return run
}

func seedRuntimeTerminalRun(t *testing.T, runs *memory.RunRepository, state common.RunState, action runtimecontract.ConvergenceState, code, summary, authID string) *runmodel.Run {
	t.Helper()
	convergenceState := runtimecontract.ConvergenceStateFailed
	if action == runtimecontract.ConvergenceStateComplete {
		convergenceState = runtimecontract.ConvergenceStateComplete
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", State: state, CreatedAt: "2026-01-01T00:00:00Z",

		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{},
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
			StateRevision: 3,
			Facts: runtimecontract.RuntimeFacts{
				WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1",
			},
			Decision: runtimecontract.RuntimeDecision{
				State:             convergenceState,
				Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: code, Summary: summary},
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: authID},
			},
		},
	}
	run.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3,
		RuntimeStateRef: runevents.RuntimeStateRef("ws-1", "run-1", 3), RuntimeStateHash: "opaque-state", BootstrapHash: "opaque-bootstrap", TerminalFenceID: "td-1",
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	return run
}

func TestReduceActivateRunPendingToActive(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStatePending}
	tr, err := reduceActivateRun(current, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || tr.Noop || tr.Target != common.RunStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceActivateRunDuplicateIsNoop(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive}
	tr, err := reduceActivateRun(current, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceActivateRunRejectsWrongScope(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStatePending}
	_, err := reduceActivateRun(current, activateRunFact{WorkspaceID: "ws-other", RunID: "run-1"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceActivateRunTerminalIsNoop(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted}
	tr, err := reduceActivateRun(current, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || !tr.Noop {
		t.Fatalf("expected terminal noop, got %+v err=%v", tr, err)
	}
}

func schemaIncompatibleFact(workspaceID common.WorkspaceID, runID common.RunID, revision uint64, actualSchema uint32) runevents.RuntimeStateIncompatibleFact {
	stateRef := runevents.RuntimeStateRef(workspaceID, runID, revision)
	return runevents.RuntimeStateIncompatibleFact{
		WorkspaceID:     workspaceID,
		RunID:           runID,
		SourceOwner:     "kernel/orchestrator",
		CausationID:     "runtime-state-incompatible/" + stateRef,
		RuntimeStateRef: stateRef,
		Incompatibility: runtimecontract.RuntimeStateIncompatibility{
			Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported,
			ActualSchemaVersion:    actualSchema,
			SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		},
		Feedback: runtimecontract.RuntimeFeedback{
			Domain:         runtimecontract.RuntimeFeedbackDomainDrive,
			StableCategory: runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported,
			WorkspaceID:    workspaceID,
			RunID:          runID,
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion:    runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
				OwnerModuleID:    "kernel/orchestrator",
				OriginalClass:    "runtime_state_schema",
				OriginalCode:     "unsupported_runtime_state_schema",
				OriginalMessage:  "runtime state schema is unsupported by this binary",
				Signature:        "runtime-state-schema/fixture",
				ObservedRevision: stateRef,
			},
		},
	}
}

func cwiLedgerIncompatibleFact(workspaceID common.WorkspaceID, runID common.RunID, revision uint64) runevents.RuntimeStateIncompatibleFact {
	stateRef := runevents.RuntimeStateRef(workspaceID, runID, revision)
	return runevents.RuntimeStateIncompatibleFact{
		WorkspaceID:     workspaceID,
		RunID:           runID,
		SourceOwner:     "kernel/orchestrator",
		CausationID:     "runtime-state-incompatible/" + stateRef + "/cwi-ledger-invalid",
		RuntimeStateRef: stateRef,
		Incompatibility: runtimecontract.RuntimeStateIncompatibility{
			Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid,
			ActualSchemaVersion:    runtimecontract.RuntimeStateSchemaVersionCurrent,
			SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		},
		Feedback: runtimecontract.RuntimeFeedback{
			Domain:         runtimecontract.RuntimeFeedbackDomainDrive,
			StableCategory: runtimecontract.RuntimeFeedbackCategoryStateCWILedgerInvalid,
			WorkspaceID:    workspaceID,
			RunID:          runID,
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion:    runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
				OwnerModuleID:    "kernel/orchestrator",
				OriginalClass:    "runtime_cwi_ledger",
				OriginalCode:     "invalid_runtime_cwi_ledger",
				OriginalMessage:  "runtime state CWI ledger is ambiguous and cannot be resumed",
				Signature:        "runtime-cwi-ledger/fixture",
				ObservedRevision: stateRef,
			},
		},
	}
}

func TestReduceRuntimeStateIncompatiblePersistsFreshActivationRequirement(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,

		RuntimeState: &runtimecontract.RuntimeStateRecord{SchemaVersion: 2, StateRevision: 7},
	}
	cmd := schemaIncompatibleFact("ws-1", "run-1", 7, 2)
	transition, err := reduceRuntimeStateIncompatible(current, cmd)
	if err != nil || transition.Noop || transition.Target != common.RunStateActive || transition.RuntimeActivationStatus == nil {
		t.Fatalf("transition=%+v err=%v", transition, err)
	}
	next := applyLifecycleTransition(current, transition)
	if next.RuntimeActivationStatus == nil || next.RuntimeActivationStatus.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired ||
		next.RuntimeActivationStatus.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported ||
		next.RuntimeActivationStatus.Incompatibility.ActualSchemaVersion != 2 || next.RuntimeActivationStatus.Feedback == nil ||
		next.RuntimeActivationStatus.Feedback.OriginalDiagnostic == nil || next.RuntimeActivationStatus.Feedback.OriginalDiagnostic.Signature != "runtime-state-schema/fixture" {
		t.Fatalf("fresh-activation status=%+v", next.RuntimeActivationStatus)
	}
	duplicate, err := reduceRuntimeStateIncompatible(next, cmd)
	if err != nil || !duplicate.Noop {
		t.Fatalf("duplicate transition=%+v err=%v", duplicate, err)
	}
	wrongRef := cmd
	wrongRef.RuntimeStateRef = "runtime-state/ws-1/run-1/8"
	if _, err := reduceRuntimeStateIncompatible(current, wrongRef); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched ref error=%v", err)
	}
	wrongSchema := cmd
	wrongSchema.Incompatibility.ActualSchemaVersion = 3
	if _, err := reduceRuntimeStateIncompatible(current, wrongSchema); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched schema error=%v", err)
	}
}

func TestReduceRuntimeStateIncompatiblePreservesCurrentSchemaCWILedgerReason(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
			StateRevision: 7,
		},
	}
	cmd := cwiLedgerIncompatibleFact("ws-1", "run-1", 7)
	transition, err := reduceRuntimeStateIncompatible(current, cmd)
	if err != nil || transition.Noop || transition.Target != common.RunStateActive || transition.RuntimeActivationStatus == nil {
		t.Fatalf("transition=%+v err=%v", transition, err)
	}
	next := applyLifecycleTransition(current, transition)
	if next.RuntimeActivationStatus == nil ||
		next.RuntimeActivationStatus.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired ||
		next.RuntimeActivationStatus.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid ||
		next.RuntimeActivationStatus.Incompatibility.ActualSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		next.RuntimeActivationStatus.Incompatibility.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		next.RuntimeActivationStatus.Feedback == nil ||
		next.RuntimeActivationStatus.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryStateCWILedgerInvalid ||
		next.RuntimeActivationStatus.Feedback.OriginalDiagnostic == nil ||
		next.RuntimeActivationStatus.Feedback.OriginalDiagnostic.Signature != "runtime-cwi-ledger/fixture" {
		t.Fatalf("CWI ledger fresh-activation status=%+v", next.RuntimeActivationStatus)
	}
	duplicate, err := reduceRuntimeStateIncompatible(next, cmd)
	if err != nil || !duplicate.Noop {
		t.Fatalf("duplicate transition=%+v err=%v", duplicate, err)
	}
}

func TestLifecycleIncompatibleStateMakesContinueTypedNoop(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seed := seedRun(t, runs, common.RunStateActive)
	seed.RuntimeState = &runtimecontract.RuntimeStateRecord{SchemaVersion: 2, StateRevision: 4}
	if err := runs.Update(context.Background(), seed); err != nil {
		t.Fatalf("seed incompatible run: %v", err)
	}
	ctx := context.Background()
	response, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunFactRuntimeStateIncompatible, "kernel/orchestrator", runevents.CommandDestination, magicevent.NewHeader(), ctx,
		schemaIncompatibleFact("ws-1", "run-1", 4, 2),
	))
	if err != nil || !response.Transitioned || response.RuntimeActivationDisposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		t.Fatalf("incompatible response=%+v err=%v", response, err)
	}
}

func TestReduceContinuationEligibleRequiresMatchingTicket(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-1",

		RuntimeBootstrap:      &runtimecontract.RuntimeBootstrap{},
		RuntimeState:          &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3, GovernanceFenceID: "runtime-governance/auth-1/dispatch-1", GovernanceResolutionID: "res-1"},
	}
	_, err := reduceContinuationEligible(current, runevents.RuntimeContinuationEligibleFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-other",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	tr, err := reduceContinuationEligible(current, runevents.RuntimeContinuationEligibleFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-1",
	})
	if err != nil || tr.Target != common.RunStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceContinuationEligibleRequiresExactPersistedResolutionBeforeActiveNoop(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{},
		RuntimeState:     &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3,
			GovernanceFenceID: "runtime-governance/auth-1/dispatch-1", GovernanceResolutionID: "resolution-current",
		},
	}
	fact := runevents.RuntimeContinuationEligibleFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-current", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1",
		DispatchID: "dispatch-1", ResolutionID: "resolution-stale", TicketID: "ticket-1",
	}
	if transition, err := reduceContinuationEligible(current, fact); !errors.Is(err, common.ErrConflict) || transition.Noop {
		t.Fatalf("stale resolution transition=%+v err=%v", transition, err)
	}
	fact.ResolutionID = "resolution-current"
	fact.CausationID = "cause-current-exact"
	current.RuntimeLifecycleFence.TerminalFenceID = "terminal-conflict"
	if transition, err := reduceContinuationEligible(current, fact); !errors.Is(err, common.ErrConflict) || transition.Noop {
		t.Fatalf("dual-mode fence transition=%+v err=%v", transition, err)
	}
	current.RuntimeLifecycleFence.TerminalFenceID = ""
	transition, err := reduceContinuationEligible(current, fact)
	if err != nil || !transition.Noop {
		t.Fatalf("exact active duplicate transition=%+v err=%v", transition, err)
	}
}

func TestReduceGovernanceWaitRejectsFenceWithResolution(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{},
		RuntimeState:     &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3,
			GovernanceFenceID: "runtime-governance/auth-1/dispatch-1", GovernanceResolutionID: "resolution-old",
		},
	}
	fact := runevents.RuntimeGovernanceWaitFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator", CausationID: "cause-wait",
		AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
	}
	if transition, err := reduceGovernanceWait(current, fact); !errors.Is(err, common.ErrConflict) || transition.Noop {
		t.Fatalf("resolved fence accepted as wait transition=%+v err=%v", transition, err)
	}
}

func TestReduceApprovalGovernanceWaitAndContinuation(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeBootstrap:      &runtimecontract.RuntimeBootstrap{},
		RuntimeState:          &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3, GovernanceFenceID: "runtime-governance/auth-1/dispatch-1"},
	}
	wait := runevents.RuntimeGovernanceWaitFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-wait", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingApprovalID: "approval-1",
	}
	blocked, err := reduceGovernanceWait(current, wait)
	if err != nil || blocked.Target != common.RunStateBlockedOnApproval || blocked.BlockingApproval != "approval-1" || blocked.BlockingTicketID != "" {
		t.Fatalf("approval wait transition=%+v err=%v", blocked, err)
	}
	current = applyLifecycleTransition(current, blocked)
	// The Orchestrator persists its resolution aggregate and exact opaque
	// lifecycle fence before delivering the continuation fact.
	current.RuntimeLifecycleFence.GovernanceResolutionID = "resolution-1"
	continued, err := reduceContinuationEligible(current, runevents.RuntimeContinuationEligibleFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-resolution", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "resolution-1", ApprovalID: "approval-1",
	})
	if err != nil || continued.Target != common.RunStateActive {
		t.Fatalf("approval continuation transition=%+v err=%v", continued, err)
	}
	resumed := applyLifecycleTransition(current, continued)
	if resumed.State != common.RunStateActive || resumed.BlockingApprovalID != "" || resumed.BlockingTicketID != "" {
		t.Fatalf("approval continuation did not clear blockers: %+v", resumed)
	}
}

func TestReduceGovernanceWaitDuplicateIsNoop(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-1",

		RuntimeBootstrap:      &runtimecontract.RuntimeBootstrap{},
		RuntimeState:          &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 3, GovernanceFenceID: "runtime-governance/auth-1/dispatch-1"},
	}
	tr, err := reduceGovernanceWait(current, runevents.RuntimeGovernanceWaitFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
	})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceTerminalDeliveryIdentityMismatch(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,

		RuntimeState: &runtimecontract.RuntimeStateRecord{
			Decision: runtimecontract.RuntimeDecision{
				State:             runtimecontract.ConvergenceStateComplete,
				Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: "completed", Summary: "done"},
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "auth-1"},
			},
			Facts: runtimecontract.RuntimeFacts{DefinitionRevision: "def-1", EvidenceRevision: "evidence-1"},
		},
	}
	_, err := reduceTerminalDelivery(current, terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-other", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceTerminalDeliveryAfterUnfencedTerminalIsConflict(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted}
	tr, err := reduceTerminalDelivery(current, terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindFailed, Code: "failed", Summary: "no"}}, "terminal-delivery/td-1"))
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected terminal fence conflict, got %+v err=%v", tr, err)
	}
}

func TestTerminalTargetUsesOnlySealedOutcomeKind(t *testing.T) {
	tests := []struct {
		kind runtimecontract.TerminalOutcomeKind
		want common.RunState
	}{
		{runtimecontract.TerminalOutcomeKindCompleted, common.RunStateCompleted},
		{runtimecontract.TerminalOutcomeKindFailed, common.RunStateFailed},
		{runtimecontract.TerminalOutcomeKindCancelled, common.RunStateCancelled},
		{runtimecontract.TerminalOutcomeKindSuperseded, common.RunStateSuperseded},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got, reason, err := terminalTargetFromCommand(terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{Outcome: runtimecontract.TerminalOutcome{Kind: tt.kind, Code: "terminal", Summary: "sealed"}}, "test"))
			if err != nil || got != tt.want {
				t.Fatalf("target=%q err=%v want=%q", got, err, tt.want)
			}
			if tt.kind == runtimecontract.TerminalOutcomeKindCompleted && reason != "" {
				t.Fatalf("completed reason = %q, want empty", reason)
			}
			if tt.kind != runtimecontract.TerminalOutcomeKindCompleted && !strings.Contains(reason, "sealed") {
				t.Fatalf("terminal reason = %q, want sealed outcome summary", reason)
			}
		})
	}
}

func TestLifecycleHandlerActivateDuplicatePublishesOnce(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	seedRun(t, runs, common.RunStatePending)

	received := make(chan runevents.RunStateChangedData, 4)
	observer := magicevent.NewSimpleObserverWithMatchID("/run/consumers/lifecycle-test", "/run/consumers/lifecycle-test", hub)
	observer.Subscribe(runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(runevents.RunStateChangedData)
		if ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunStateChanged) })

	ctx := context.Background()
	first, err := biz.applyRunActivation(ctx, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || !first.Transitioned || first.Run.State != common.RunStateActive {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := biz.applyRunActivation(ctx, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || second.Transitioned {
		t.Fatalf("second should be noop: %+v err=%v", second, err)
	}

	select {
	case data := <-received:
		if data.State != common.RunStateActive || data.RunID != "run-1" {
			t.Fatalf("notification=%+v", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected one RunStateChanged notification after CAS success")
	}
	// Drain optional specific-topic duplicate if any fan-in path posts twice.
	deadline := time.After(50 * time.Millisecond)
	drain := 0
	for {
		select {
		case <-received:
			drain++
			if drain > 1 {
				t.Fatalf("too many extra notifications: %d", drain)
			}
		case <-deadline:
			goto drained
		}
	}
drained:

	stored, err := runs.GetByWorkspaceID(ctx, "ws-1", "run-1")
	if err != nil || stored.State != common.RunStateActive {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func TestLifecycleHandlerRejectsConflictingEventWorkspaceScopeWithoutMutation(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedRun(t, runs, common.RunStatePending)

	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-other")
	_, err := biz.applyRunActivation(ctx, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v, want workspace scope conflict", err)
	}
	item, err := runs.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("get run after rejected command: %v", err)
	}
	if item.State != common.RunStatePending {
		t.Fatalf("run state=%q, want unchanged pending", item.State)
	}
}

// An SM-2/3/4 aggregate save is a Run-owned persistence operation, not an
// SM-1 lifecycle transition. In particular it must not emit an active fact
// after Workspace has already released a terminal Run.
func TestGenericRunUpdateDoesNotPublishLifecycleFactOrChangeSM1(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	seed := seedRun(t, runs, common.RunStateActive)
	bootstrap := runtimecontract.RuntimeBootstrap{}
	state := runtimecontract.RuntimeStateRecord{SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, StateRevision: 1}
	seed.RuntimeBootstrap = &bootstrap
	seed.RuntimeState = &state
	if err := runs.Update(context.Background(), seed); err != nil {
		t.Fatalf("seed Runtime state: %v", err)
	}
	received := make(chan runevents.RunStateChangedData, 1)
	observer := magicevent.NewSimpleObserverWithMatchID("/run/consumers/update-test", "/run/consumers/update-test", hub)
	observer.Subscribe(runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.RunStateChangedData); ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunStateChanged) })

	nextState := state.Clone()
	nextState.StateRevision++
	expected := state.StateRevision
	_, err := biz.executeCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandStoreRuntimeStateRecord, "kernel/orchestrator", runevents.CommandDestination, magicevent.NewHeader(), context.Background(),
		runevents.StoreRuntimeStateRecordCommand{
			RunID: seed.ID, WorkspaceID: seed.WorkspaceID,
			RuntimeState: &nextState, ExpectedRuntimeStateRevision: &expected,
		},
	))
	if err != nil {
		t.Fatalf("aggregate-compatible update: %v", err)
	}
	select {
	case data := <-received:
		t.Fatalf("non-lifecycle update emitted RunStateChanged: %+v", data)
	case <-time.After(50 * time.Millisecond):
	}

	// StoreRuntimeStateRecordCommand intentionally has no State/blocker/failure/lineage fields;
	// this is the regression assertion that generic persistence cannot express an
	// SM-1 transition.
}

// Runtime aggregate writes are semantically fenced by RuntimeStateRevision,
// not by the enclosing Run record revision. This exercises the physical CAS
// rebase used when an unrelated SM-1/model write wins the repository race.
func TestRuntimeAggregatePersistenceRebasesUnrelatedRunModelWrite(t *testing.T) {
	ctx := context.Background()
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "def-v2", GoalID: "goal-v2", DoDID: "dod-v2"}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}}}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: definition, Activation: activation}
	currentState := runtimecontract.RuntimeStateRecord{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		StateRevision: 7, Definition: definition,
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: definition.Revision, Activation: activation},
	}
	base := memory.NewRunRepository()
	if err := base.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}); err != nil {
		t.Fatal(err)
	}
	runs := &runtimeStateCASRaceRepository{RunRepository: base, mutateModelOnly: true}
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(8)}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = biz.Stop(ctx) }()

	nextState := currentState.Clone()
	nextState.StateRevision++
	expected := currentState.StateRevision
	result, err := biz.updateRuntimeState(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}, runevents.StoreRuntimeStateRecordCommand{RunID: "run-v2", WorkspaceID: "ws-v2", RuntimeState: &nextState, ExpectedRuntimeStateRevision: &expected})
	if err != nil {
		t.Fatalf("Runtime aggregate update: %v", err)
	}
	if result.RuntimeStateUpdate == nil || result.RuntimeStateUpdate.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryAccepted {
		t.Fatalf("Runtime aggregate receipt = %+v", result.RuntimeStateUpdate)
	}
	stored, err := base.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || stored.FailureReason != "unrelated-owner-write" || stored.RuntimeState == nil || stored.RuntimeState.StateRevision != nextState.StateRevision {
		t.Fatalf("rebased stored Run = %+v err=%v", stored, err)
	}
	if runs.casCalls != 2 {
		t.Fatalf("physical CAS calls = %d, want 2", runs.casCalls)
	}
}

func TestRuntimeAggregatePersistenceRejectsAggregateRevisionRace(t *testing.T) {
	ctx := context.Background()
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "def-v2", GoalID: "goal-v2", DoDID: "dod-v2"}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}}}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: definition, Activation: activation}
	currentState := runtimecontract.RuntimeStateRecord{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		StateRevision: 7, Definition: definition,
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: definition.Revision, Activation: activation},
	}
	base := memory.NewRunRepository()
	if err := base.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}); err != nil {
		t.Fatal(err)
	}
	runs := &runtimeStateCASRaceRepository{RunRepository: base, mutateAggregate: true}
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(8)}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = biz.Stop(ctx) }()
	nextState := currentState.Clone()
	nextState.StateRevision++
	expected := currentState.StateRevision
	result, err := biz.updateRuntimeState(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}, runevents.StoreRuntimeStateRecordCommand{RunID: "run-v2", WorkspaceID: "ws-v2", RuntimeState: &nextState, ExpectedRuntimeStateRevision: &expected})
	if err != nil || result.RuntimeStateUpdate == nil || result.RuntimeStateUpdate.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch ||
		result.RuntimeStateUpdate.ExpectedRuntimeRevision != expected || result.RuntimeStateUpdate.ObservedRuntimeRevision != expected+1 {
		t.Fatalf("aggregate revision race result=%+v err=%v", result.RuntimeStateUpdate, err)
	}
}

type runtimeStateCASRaceRepository struct {
	*memory.RunRepository
	mutateModelOnly bool
	mutateAggregate bool
	casCalls        int
}

func (r *runtimeStateCASRaceRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	r.casCalls++
	if r.casCalls != 1 {
		return r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
	}
	current, err := r.RunRepository.GetByWorkspaceID(ctx, item.WorkspaceID, item.ID)
	if err != nil {
		return err
	}
	changed := *current
	if r.mutateAggregate {
		next := current.RuntimeState.Clone()
		next.StateRevision++
		changed.RuntimeState = &next
	} else if r.mutateModelOnly {
		changed.FailureReason = "unrelated-owner-write"
	}
	if err := r.RunRepository.CompareAndSwap(ctx, &changed, current.Revision); err != nil {
		return err
	}
	return common.ErrConflict
}

func TestLifecycleHandlerTerminalThenActivateRemainsTerminal(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedRuntimeTerminalRun(t, runs, common.RunStateActive, runtimecontract.ConvergenceStateComplete, "completed", "done", "auth-1")
	ctx := context.Background()

	term, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunFactTerminalDelivery, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"),
	))
	if err != nil || !term.Transitioned || term.Run.State != common.RunStateCompleted {
		t.Fatalf("terminal=%+v err=%v", term, err)
	}
	if term.DeliveryAck == nil || term.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal {
		t.Fatalf("ack=%+v", term.DeliveryAck)
	}

	activate, err := biz.applyRunActivation(ctx, activateRunFact{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || activate.Transitioned || activate.Run.State != common.RunStateCompleted {
		t.Fatalf("activate after terminal=%+v err=%v", activate, err)
	}

	wait, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunFactGovernanceWait, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.RuntimeGovernanceWaitFact{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
		},
	))
	if err != nil || wait.Transitioned || wait.Run.State != common.RunStateCompleted {
		t.Fatalf("wait after terminal=%+v err=%v", wait, err)
	}
}

func TestLifecycleHandlerStaleTerminalDoesNotChangeRun(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedRuntimeTerminalRun(t, runs, common.RunStateActive, runtimecontract.ConvergenceStateComplete, "completed", "done", "auth-1")
	ctx := context.Background()

	_, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunFactTerminalDelivery, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-stale", AuthoritySnapshotID: "auth-stale", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-stale"),
	))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, "ws-1", "run-1")
	if err != nil || stored.State != common.RunStateActive {
		t.Fatalf("stored must remain active: %+v err=%v", stored, err)
	}
}
