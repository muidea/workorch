package proof

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMatchProspectiveCoverageReceiptSetRequiresCompleteAllOfOwnerEvidence(t *testing.T) {
	binding := prospectiveReceiptSetBinding(t)
	first := prospectiveReceiptSetMember(binding, "write-go-mod", "go.mod")
	second := prospectiveReceiptSetMember(binding, "write-test", "workorch_test.go")

	tests := []struct {
		name       string
		members    []ProspectiveCoverageReceiptMember
		complete   bool
		accepted   []string
		missing    []string
		diagnostic bool
	}{
		{name: "partial", members: []ProspectiveCoverageReceiptMember{first}, accepted: []string{"go.mod"}, missing: []string{"workorch_test.go"}},
		{name: "rejected", members: []ProspectiveCoverageReceiptMember{first, mutateProspectiveReceiptSetMember(second, func(member *ProspectiveCoverageReceiptMember) {
			member.Receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
			member.Execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(member.Receipt)
		})}, accepted: []string{"go.mod"}, missing: []string{"workorch_test.go"}},
		{name: "stale_uncommitted", members: []ProspectiveCoverageReceiptMember{first, mutateProspectiveReceiptSetMember(second, func(member *ProspectiveCoverageReceiptMember) {
			member.Execution.State = runtimecontract.OperationExecutionClaimed
		})}, accepted: []string{"go.mod"}, missing: []string{"workorch_test.go"}},
		{name: "receipt_without_accepted_evidence", members: []ProspectiveCoverageReceiptMember{first, mutateProspectiveReceiptSetMember(second, func(member *ProspectiveCoverageReceiptMember) {
			member.AcceptedEvidence = nil
		})}, accepted: []string{"go.mod"}, missing: []string{"workorch_test.go"}},
		{name: "duplicate_is_idempotent", members: []ProspectiveCoverageReceiptMember{first, first, second}, complete: true, accepted: []string{"go.mod", "workorch_test.go"}},
		{name: "all_accepted", members: []ProspectiveCoverageReceiptMember{second, first}, complete: true, accepted: []string{"go.mod", "workorch_test.go"}},
		{name: "conflicting_duplicate_fails_closed", members: []ProspectiveCoverageReceiptMember{first, second, mutateProspectiveReceiptSetMember(first, func(member *ProspectiveCoverageReceiptMember) {
			member.Receipt.CompletedAt = "different-receipt"
			member.Execution.ExecutionKey = "exec/ws/run/dispatch/write-go-mod-duplicate"
			member.Receipt.ExecutionKey = member.Execution.ExecutionKey
			member.Execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(member.Receipt)
		})}, accepted: []string{"go.mod", "workorch_test.go"}, diagnostic: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			finding := MatchProspectiveCoverageReceiptSet(binding, runtimecontract.EvidenceKindStateChange, testCase.members)
			if finding.Complete() != testCase.complete || !reflect.DeepEqual(finding.AcceptedTargetRefs, testCase.accepted) ||
				!reflect.DeepEqual(finding.MissingTargetRefs, testCase.missing) || (finding.Diagnostic != "") != testCase.diagnostic {
				t.Fatalf("collective finding = %+v, want complete=%t accepted=%v missing=%v diagnostic=%t", finding, testCase.complete, testCase.accepted, testCase.missing, testCase.diagnostic)
			}
		})
	}
}

func prospectiveReceiptSetBinding(t *testing.T) runtimecontract.CoverageEvidenceBinding {
	t.Helper()
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/prospective/all-of", CoverageWorkItemID: "cwi-prospective", FocusID: "focus-prospective",
		DefinitionRevision: "definition-prospective", DeclarationRef: "test/prospective/all-of",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind:          runtimecontract.EvidenceKindStateChange,
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		}},
		TargetSelectorRef: "workspace-relative-new-file-target", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile,
		TargetSelectorSubjectRef: "workspace_deliverable", TargetSelectorCandidateRefs: []string{"go.mod", "workorch_test.go"},
		ProspectiveTargetRefs: []string{"go.mod", "workorch_test.go"},
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	return binding
}

func prospectiveReceiptSetMember(binding runtimecontract.CoverageEvidenceBinding, operationID, targetRef string) ProspectiveCoverageReceiptMember {
	operation := runtimecontract.RuntimeOperation{
		ID: operationID, Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{binding.ID}, TargetRefs: []string{targetRef},
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/" + operationID, Fence: 1, OperationID: operationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{targetRef}, Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid},
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence,
		State:           runtimecontract.OperationExecutionEvidenceCommitted,
		OwnerReceiptRef: "runtime-receipt/" + receipt.ExecutionKey, OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
	}
	evidence := runtimecontract.EvidenceFact{
		ID: "evidence-" + common.EvidenceRecordID(operationID), Kind: runtimecontract.EvidenceKindStateChange,
		DispatchID: "dispatch", OperationID: operationID, TargetRefs: []string{targetRef},
		BindingRefs: []string{binding.ID}, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID},
		Quality: common.EvidenceQualityValid,
	}
	return ProspectiveCoverageReceiptMember{
		DispatchID: "dispatch", Operation: operation, Execution: execution, Receipt: receipt, AcceptedEvidence: &evidence,
	}
}

func mutateProspectiveReceiptSetMember(source ProspectiveCoverageReceiptMember, mutate func(*ProspectiveCoverageReceiptMember)) ProspectiveCoverageReceiptMember {
	value := source
	value.Operation = source.Operation.Normalized()
	value.Execution = source.Execution.Normalized()
	value.Receipt = source.Receipt.Normalized()
	if source.AcceptedEvidence != nil {
		evidence := *source.AcceptedEvidence
		evidence.TargetRefs = append([]string(nil), source.AcceptedEvidence.TargetRefs...)
		evidence.BindingRefs = append([]string(nil), source.AcceptedEvidence.BindingRefs...)
		evidence.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), source.AcceptedEvidence.CoverageWorkItemIDs...)
		value.AcceptedEvidence = &evidence
	}
	mutate(&value)
	return value
}
