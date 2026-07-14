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

func TestRuntimeCandidateRejectsFileOperationAliasesAndNoOpBeforeDispatch(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileEdit}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	filePath := "main.go"
	oldText := "old"
	newText := "new"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit-1", Kind: StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		Parameters: []OperationParameter{{Name: "filePath", String: &filePath}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
	}}}}}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), `does not accept parameter "filePath"`) {
		t.Fatalf("filePath candidate error = %v", err)
	}

	candidate.StateChange.Operations[0].Operation.Parameters = []OperationParameter{
		{Name: "path", String: &filePath}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &oldText},
	}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "distinct newText") {
		t.Fatalf("no-op candidate error = %v", err)
	}
}

func TestRuntimeCandidateAcceptsConcretePatchApplyContract(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)
	targetRef := "main.go"
	patchType := "text_replace"
	oldText := "old"
	newText := "new"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "patch-1", Kind: RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef},
		Parameters: []OperationParameter{
			{Name: "targetRef", String: &targetRef}, {Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	}}}}}
	if err := candidate.Validate(decision); err != nil {
		t.Fatalf("concrete patch/apply candidate rejected: %v", err)
	}
}
