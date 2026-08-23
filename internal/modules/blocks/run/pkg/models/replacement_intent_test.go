package run

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReplacementIntentPreservesValidatedLegacyIdentitySlotWithoutRestoringPriority(t *testing.T) {
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-new", GoalID: "goal-1", DoDID: "dod-1", Revision: "def-2"}}
	predecessor := &Run{ID: "run-001", WorkspaceID: "ws-1", DefinitionRevision: "def-1", State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-1"}
	legacy := ReplacementIntent{
		RequestID: "restart-legacy", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-2", StartupInputHash: "startup-new",
		ReplacementIntentID: "runtime-definition-replacement/restart-legacy", ClarificationTicketID: "runtime-definition-clarification:ticket-1",
		PredecessorStartupInputHash: "startup-old", SuccessorStartupInputHash: "startup-new", Bootstrap: &bootstrap,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: ReplacementIntentPendingClarificationResolution,
		legacyIdentityHashSlot: "7",
	}
	legacy.ReplacementIntentHash = StableReplacementIntentHash(legacy)
	legacy.Recovery = replacementRecoveryFixture(legacy, ReplacementRecoveryClarificationResolution, ReplacementRecoveryScheduled)
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"Priority":7`) {
		t.Fatalf("legacy identity slot was not preserved in persistence JSON: %s", encoded)
	}

	var decoded ReplacementIntent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.ValidFor(predecessor) || StableReplacementIntentHash(decoded) != legacy.ReplacementIntentHash {
		t.Fatalf("decoded legacy intent lost its sealed identity: %+v", decoded)
	}
	canonical := CloneReplacementIntent(decoded)
	canonical.legacyIdentityHashSlot = ""
	canonical.ReplacementIntentHash = StableReplacementIntentHash(canonical)
	if !EquivalentReplacementIntentHash(decoded, canonical) {
		t.Fatal("canonical duplicate did not match validated persisted v1 identity")
	}
	forgedCanonical := canonical
	forgedCanonical.ReplacementIntentHash = "replacement-intent-sha256:forged"
	if EquivalentReplacementIntentHash(decoded, forgedCanonical) {
		t.Fatal("forged canonical duplicate matched validated persisted v1 identity")
	}
	tamperedLegacy := decoded
	tamperedLegacy.Reason = "tampered after decode"
	if _, err := json.Marshal(tamperedLegacy); err == nil {
		t.Fatal("tampered decoded legacy identity was persisted")
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(canonicalJSON), `"Priority"`) {
		t.Fatalf("new canonical intent persisted retired priority: %s", canonicalJSON)
	}

	var tampered map[string]any
	if err := json.Unmarshal(encoded, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered["Priority"] = float64(8)
	tamperedJSON, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tamperedJSON, &decoded); err == nil {
		t.Fatal("tampered persisted legacy identity slot was accepted")
	}
}

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
	predecessor.ReplacementIntent.Recovery = replacementRecoveryFixture(*predecessor.ReplacementIntent, ReplacementRecoveryAdministrativeTermination, ReplacementRecoveryScheduled)
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
	pending.Recovery = replacementRecoveryFixture(pending, ReplacementRecoveryClarificationResolution, ReplacementRecoveryScheduled)
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
	bound.Recovery = replacementRecoveryFixture(bound, ReplacementRecoveryAdministrativeTermination, ReplacementRecoveryScheduled)
	if !bound.ValidFor(predecessor) {
		t.Fatalf("valid bound clarification intent rejected: %+v", bound)
	}
	completed := bound
	completed.State = ReplacementIntentCompleted
	completed.SuccessorRunID = "run-002"
	completed.TerminalDeliveryID = "terminal-1"
	completed.RuntimeStateRevision = 7
	completed.AcknowledgedAt = "2026-08-21T00:00:00Z"
	completed.Recovery = replacementRecoveryFixture(completed, ReplacementRecoverySuccessorMaterialization, ReplacementRecoveryCompleted)
	predecessor.State = common.RunStateSuperseded
	predecessor.TerminalDelivery = &runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID, TerminalDeliveryID: completed.TerminalDeliveryID}
	predecessor.Lineage.SuccessorRunID = completed.SuccessorRunID
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
	rejected.Recovery = replacementRecoveryFixture(rejected, ReplacementRecoveryClarificationResolution, ReplacementRecoveryCompleted)
	if rejected.ValidFor(predecessor) {
		t.Fatal("ticket-already-bound rejection omitted conflicting intent identity")
	}
	rejected.ConflictingReplacementIntentID = "runtime-definition-replacement/restart-current"
	rejected.ConflictingReplacementIntentHash = "replacement-hash-current"
	if !rejected.ValidFor(predecessor) {
		t.Fatalf("valid ticket-already-bound rejection was rejected: %+v", rejected)
	}
}

func replacementRecoveryFixture(intent ReplacementIntent, stage ReplacementRecoveryStage, state ReplacementRecoveryState) ReplacementRecoveryLease {
	dueAt := ""
	if state == ReplacementRecoveryScheduled {
		dueAt = "2026-08-23T00:00:00Z"
	}
	return ReplacementRecoveryLease{
		RequestID: intent.RequestID, IntentHash: StableReplacementIntentHash(intent), Stage: stage, State: state,
		Generation: 1, MaxAttempts: 8, DueAt: dueAt,
	}
}
