package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateRequiresAcceptedOperationWhenDeclared(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{"git/write"}, AcceptedOperations: []string{"commit"},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	add := "add"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "git-add", Kind: "git/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Parameters: []OperationParameter{{Name: "operation", String: &add}},
	}}}}}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "requires one accepted operation") {
		t.Fatalf("candidate error = %v, want accepted operation requirement", err)
	}

	commit := "commit"
	candidate.StateChange.Operations = append(candidate.StateChange.Operations, StateChangingOperation{Operation: RuntimeOperation{
		ID: "git-commit", Kind: "git/write", DependsOn: []string{"git-add"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []OperationParameter{{Name: "operation", String: &commit}},
	}})
	if err := candidate.Validate(decision); err != nil {
		t.Fatalf("candidate with commit was rejected: %v", err)
	}
}
