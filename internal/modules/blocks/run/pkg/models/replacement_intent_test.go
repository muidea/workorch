package run

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReplacementIntentValidForPredecessorAndCloneDetachesBootstrap(t *testing.T) {
	if got := StableReplacementIntentHash(ReplacementIntent{}); got != "" {
		t.Fatalf("nil-bootstrap replacement hash = %q, want empty", got)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", GoalID: "goal-1", DoDID: "dod-1", Revision: "def-1"}}
	predecessor := &Run{ID: "run-001", WorkspaceID: "ws-1", ReplacementIntent: &ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: ReplacementIntentPending,
	}}
	if !predecessor.ReplacementIntent.ValidFor(predecessor) {
		t.Fatalf("valid predecessor intent rejected: %+v", predecessor.ReplacementIntent)
	}
	cloned := CloneRun(predecessor)
	cloned.ReplacementIntent.Bootstrap.Definition.Revision = "mutated"
	if predecessor.ReplacementIntent.Bootstrap.Definition.Revision != "def-1" {
		t.Fatal("replacement bootstrap clone leaked mutation")
	}
	invalid := *predecessor.ReplacementIntent
	invalid.WorkspaceID = "ws-other"
	if invalid.ValidFor(predecessor) {
		t.Fatal("cross-workspace replacement intent accepted")
	}
	retired := CloneReplacementIntent(*predecessor.ReplacementIntent)
	predecessor.RetiredReplacementIntents = []ReplacementIntent{retired}
	cloned = CloneRun(predecessor)
	cloned.RetiredReplacementIntents[0].Bootstrap.Definition.Revision = "retired-mutated"
	if predecessor.RetiredReplacementIntents[0].Bootstrap.Definition.Revision != "def-1" {
		t.Fatal("retired replacement bootstrap clone leaked mutation")
	}
}

func TestClarificationReplacementIntentRequiresResolutionThroughTerminalStates(t *testing.T) {
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-new", GoalID: "goal-1", DoDID: "dod-1", Revision: "def-2"}}
	predecessor := &Run{ID: "run-001", WorkspaceID: "ws-1", DefinitionRevision: "def-1", State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-1"}
	pending := ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-2", StartupInputHash: "startup-new",
		ReplacementIntentID: "runtime-definition-replacement/restart-1", ClarificationTicketID: "runtime-definition-clarification:ticket-1",
		PredecessorStartupInputHash: "startup-old", SuccessorStartupInputHash: "startup-new", Bootstrap: &bootstrap,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: ReplacementIntentPendingClarificationResolution,
	}
	pending.ReplacementIntentHash = StableReplacementIntentHash(pending)
	if !pending.ValidFor(predecessor) {
		t.Fatalf("valid pending clarification intent rejected: %+v", pending)
	}

	forgedCompleted := pending
	forgedCompleted.State = ReplacementIntentCompleted
	forgedCompleted.SuccessorRunID = "run-002"
	forgedCompleted.TerminalDeliveryID = "terminal-1"
	forgedCompleted.RuntimeStateRevision = 7
	forgedCompleted.AcknowledgedAt = "2026-08-21T00:00:00Z"
	if forgedCompleted.ValidFor(predecessor) {
		t.Fatal("clarification intent reached completed without a bound resolution")
	}

	bound := pending
	bound.State = ReplacementIntentClarificationResolutionBound
	bound.ClarificationResolutionID = "resolution-1"
	bound.ClarificationResolutionBindingHash = "resolution-binding-1"
	if !bound.ValidFor(predecessor) {
		t.Fatalf("valid bound clarification intent rejected: %+v", bound)
	}
	completed := bound
	completed.State = ReplacementIntentCompleted
	completed.SuccessorRunID = "run-002"
	completed.TerminalDeliveryID = "terminal-1"
	completed.RuntimeStateRevision = 7
	completed.AcknowledgedAt = "2026-08-21T00:00:00Z"
	if !completed.ValidFor(predecessor) {
		t.Fatalf("valid completed clarification intent rejected: %+v", completed)
	}
	completed.ClarificationResolutionBindingHash = ""
	if completed.ValidFor(predecessor) {
		t.Fatal("completed clarification intent lost its resolution binding")
	}
	rejected := pending
	rejected.State = ReplacementIntentClarificationResolutionRejected
	rejected.ClarificationResolutionBindingHash = "stale-binding"
	rejected.ClarificationConflictKind = "ticket_already_bound"
	if rejected.ValidFor(predecessor) {
		t.Fatal("ticket-already-bound rejection omitted conflicting intent identity")
	}
	rejected.ConflictingReplacementIntentID = "runtime-definition-replacement/restart-current"
	rejected.ConflictingReplacementIntentHash = "replacement-hash-current"
	if !rejected.ValidFor(predecessor) {
		t.Fatalf("valid ticket-already-bound rejection was rejected: %+v", rejected)
	}
}
