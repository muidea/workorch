package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDecisionActionStateMatrix(t *testing.T) {
	tests := []struct {
		name          string
		state         ConvergenceState
		action        RuntimeAction
		output        RequiredOutputContract
		prerequisites []RuntimePrerequisite
		terminal      *TerminalOutcome
	}{
		{name: "prerequisite", state: ConvergenceStateNeedPrerequisite, action: RuntimeActionAcquirePrerequisite, output: RequiredOutputContract{Kind: RequiredOutputKindPrerequisite, PrerequisiteIDs: []string{"snapshot"}}, prerequisites: []RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}},
		{name: "state change", state: ConvergenceStateNeedStateChange, action: RuntimeActionProduceStateChange, output: RequiredOutputContract{Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{"patch/apply"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
		{name: "validation", state: ConvergenceStateNeedValidation, action: RuntimeActionProduceValidation, output: RequiredOutputContract{Kind: RequiredOutputKindValidation, AcceptedKinds: []string{"validation/managed_session"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
		{name: "governance", state: ConvergenceStateNeedGovernance, action: RuntimeActionRequestGovernance, output: RequiredOutputContract{Kind: RequiredOutputKindGovernance, AcceptedKinds: []string{"decision/resolution"}}},
		{name: "complete", state: ConvergenceStateComplete, action: RuntimeActionComplete, terminal: &TerminalOutcome{Code: "satisfied"}},
		{name: "fail", state: ConvergenceStateFailed, action: RuntimeActionFail, terminal: &TerminalOutcome{Code: "unrecoverable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := SealRuntimeDecision(testRuntimeFacts(), RuntimeDecision{
				State:          tt.state,
				Action:         tt.action,
				Focus:          testRuntimeFacts().Focus,
				RequiredOutput: tt.output,
				Prerequisites:  tt.prerequisites,
				Terminal:       tt.terminal,
			}, 0)
			if err != nil {
				t.Fatalf("SealRuntimeDecision() error = %v", err)
			}
			if err := decision.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			transaction := decision.AuthorityTransaction()
			if transaction.AuthoritySnapshotID == "" || transaction.RequiredOutputHash == "" {
				t.Fatalf("transaction is not sealed: %+v", transaction)
			}
		})
	}
}

func TestRuntimeDecisionRejectsEveryMismatchedAction(t *testing.T) {
	states := []ConvergenceState{
		ConvergenceStateNeedPrerequisite,
		ConvergenceStateNeedStateChange,
		ConvergenceStateNeedValidation,
		ConvergenceStateNeedGovernance,
		ConvergenceStateComplete,
		ConvergenceStateFailed,
	}
	actions := []RuntimeAction{
		RuntimeActionAcquirePrerequisite,
		RuntimeActionProduceStateChange,
		RuntimeActionProduceValidation,
		RuntimeActionRequestGovernance,
		RuntimeActionComplete,
		RuntimeActionFail,
	}
	for _, state := range states {
		want, _, _ := convergenceStateContract(state)
		for _, action := range actions {
			if action == want {
				continue
			}
			d := validDecisionForState(state)
			d.Action = action
			d.AuthoritySnapshot.Action = action
			d.AuthoritySnapshot.ID = stableAuthoritySnapshotID(d.AuthoritySnapshot)
			if err := d.Validate(); err == nil {
				t.Fatalf("state %s accepted invalid action %s", state, action)
			}
		}
	}
}

func TestRuntimeDecisionRejectsOutputAuthorityMismatch(t *testing.T) {
	d := validDecisionForState(ConvergenceStateNeedValidation)
	d.RequiredOutput = RequiredOutputContract{
		Kind:                RequiredOutputKindStateChange,
		AcceptedKinds:       []string{"patch/apply"},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	d.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(d.RequiredOutput)
	d.AuthoritySnapshot.ID = stableAuthoritySnapshotID(d.AuthoritySnapshot)
	if err := d.Validate(); err == nil {
		t.Fatal("validation state accepted state-changing required output")
	}
}

func TestStableRequiredOutputHashNormalizesOrderAndDetectsSemanticChange(t *testing.T) {
	left := RequiredOutputContract{
		Kind:                  RequiredOutputKindStateChange,
		AcceptedKinds:         []string{"git/write", "patch/apply", "patch/apply"},
		AcceptedOperations:    []string{"commit", "add", "commit"},
		RequiredEvidenceKinds: []string{"validation", "state_change"},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-2", "cwi-1"},
		TargetRefs:            []string{"b.go", "a.go"},
	}
	right := RequiredOutputContract{
		Kind:                  RequiredOutputKindStateChange,
		AcceptedKinds:         []string{"patch/apply", "git/write"},
		AcceptedOperations:    []string{"add", "commit"},
		RequiredEvidenceKinds: []string{"state_change", "validation"},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
		TargetRefs:            []string{"a.go", "b.go"},
	}
	if StableRequiredOutputHash(left) != StableRequiredOutputHash(right) {
		t.Fatal("hash changed with set ordering")
	}
	right.AcceptedOperations = []string{"push"}
	if StableRequiredOutputHash(left) == StableRequiredOutputHash(right) {
		t.Fatal("hash did not change with accepted operation")
	}
	right.AcceptedOperations = []string{"add", "commit"}
	right.TargetRefs = []string{"a.go", "c.go"}
	if StableRequiredOutputHash(left) == StableRequiredOutputHash(right) {
		t.Fatal("hash did not change with semantic target change")
	}
}

func TestSealedRuntimeDecisionRejectsSnapshotMutation(t *testing.T) {
	d := validDecisionForState(ConvergenceStateNeedStateChange)
	d.AuthoritySnapshot.EvidenceRevision = "evidence-2"
	if err := d.Validate(); err == nil {
		t.Fatal("decision accepted mutated authority snapshot")
	}
}

func validDecisionForState(state ConvergenceState) RuntimeDecision {
	action, outputKind, terminal := convergenceStateContract(state)
	output := RequiredOutputContract{Kind: outputKind}
	var prerequisites []RuntimePrerequisite
	switch outputKind {
	case RequiredOutputKindPrerequisite:
		output.PrerequisiteIDs = []string{"snapshot"}
		prerequisites = []RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}
	case RequiredOutputKindStateChange:
		output.AcceptedKinds = []string{"patch/apply"}
		output.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	case RequiredOutputKindValidation:
		output.AcceptedKinds = []string{"validation/managed_session"}
		output.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	case RequiredOutputKindGovernance:
		output.AcceptedKinds = []string{"decision/resolution"}
	}
	var outcome *TerminalOutcome
	if terminal {
		outcome = &TerminalOutcome{Code: "terminal"}
	}
	d, err := SealRuntimeDecision(testRuntimeFacts(), RuntimeDecision{
		State:          state,
		Action:         action,
		Focus:          testRuntimeFacts().Focus,
		RequiredOutput: output,
		Prerequisites:  prerequisites,
		Terminal:       outcome,
	}, 0)
	if err != nil {
		panic(err)
	}
	return d
}

func testRuntimeFacts() RuntimeFacts {
	return RuntimeFacts{
		WorkspaceID:        common.WorkspaceID("ws-1"),
		RunID:              common.RunID("run-1"),
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: CoverageFocus{
			ID:                  "focus-1",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		},
	}
}
