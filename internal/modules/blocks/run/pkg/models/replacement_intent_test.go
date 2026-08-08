package run

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReplacementIntentValidForPredecessorAndCloneDetachesBootstrap(t *testing.T) {
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "def-1"}}
	predecessor := &Run{ID: "run-001", WorkspaceID: "ws-1", ReplacementIntent: &ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", Bootstrap: &bootstrap,
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
}
