package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestStableRuntimeEvidenceFactHashBindsExactRejectedProjection(t *testing.T) {
	exitCode := int64(1)
	baseline := EvidenceFact{
		ID: "runtime-receipt|receipt-hash", Kind: EvidenceKindValidation,
		FocusID:    "focus-1",
		SubjectRef: "operation-1", SourceRef: "runtime-receipt/exec-1", AuthoritySnapshotID: "authority-1",
		DispatchID: "dispatch-1", OperationID: "operation-1", Revision: "evidence-7",
		FailureClass: FailureClassBuildFailure, FailureCode: "build_failed", FailureSignature: "signature-1", FailureMessage: "build failed",
		Summary: "validation rejected", ContentHash: strings.Repeat("a", 64), ExitCode: &exitCode,
		EvidenceRefs: []string{"evidence/ref-1"}, TargetRefs: []string{"target.go"}, BindingRefs: []string{"binding-1"},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Quality: common.EvidenceQualityValid,
		EvidenceReason: "owner rejected validation", EvidenceTags: []string{"rejected"},
		Validation: &RuntimeValidationObservation{RecipeID: "recipe-1", ExecutedChecks: []string{"check-1"}, FailedChecks: []string{"check-1"}},
	}
	baselineHash := StableRuntimeEvidenceFactHash(baseline)
	if baselineHash == "" {
		t.Fatal("stable evidence fact hash is empty")
	}
	tests := []struct {
		name   string
		mutate func(*EvidenceFact)
	}{
		{name: "kind", mutate: func(value *EvidenceFact) { value.Kind = EvidenceKindStateChange }},
		{name: "focus id", mutate: func(value *EvidenceFact) { value.FocusID = "focus-2" }},
		{name: "revision", mutate: func(value *EvidenceFact) { value.Revision = "evidence-8" }},
		{name: "binding refs", mutate: func(value *EvidenceFact) { value.BindingRefs = []string{"binding-2"} }},
		{name: "coverage work item ids", mutate: func(value *EvidenceFact) { value.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-2"} }},
		{name: "evidence reason", mutate: func(value *EvidenceFact) { value.EvidenceReason = "rewritten reason" }},
		{name: "validation recipe", mutate: func(value *EvidenceFact) {
			cloned := value.Validation.Clone()
			cloned.RecipeID = "recipe-2"
			value.Validation = &cloned
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := baseline
			test.mutate(&changed)
			if got := StableRuntimeEvidenceFactHash(changed); got == baselineHash {
				t.Fatalf("evidence fact hash did not bind %s", test.name)
			}
		})
	}
}

func TestRuntimeOperationExecutionEvidenceCommitRequiresIndependentFactIdentity(t *testing.T) {
	record := RuntimeOperationExecutionRecord{
		OperationID: "operation-1", OperationHash: "operation-hash-1", ExecutionKey: "exec/ws/run/dispatch/operation-1",
		ExpectedEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		State: OperationExecutionOwnerCommitted, Fence: 1, OwnerReceiptRef: "runtime-receipt/exec/ws/run/dispatch/operation-1",
		OwnerReceiptHash: "receipt-hash-1", ClaimedAt: RuntimeOperationClaimMarkerInitial,
	}
	record.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(record)
	if err := record.Validate(); err != nil {
		t.Fatalf("owner-committed fixture: %v", err)
	}
	immutableAuthorityHash := record.AuthorityHash

	committed := record
	committed.State = OperationExecutionEvidenceCommitted
	if err := committed.Validate(); err == nil || !strings.Contains(err.Error(), "requires evidence fact id/hash") {
		t.Fatalf("missing evidence fact identity error = %v", err)
	}
	committed.EvidenceFactID = "evidence-1"
	committed.EvidenceFactHash = "evidence-hash-1"
	if err := committed.Validate(); err != nil {
		t.Fatalf("evidence-committed fixture: %v", err)
	}
	if committed.AuthorityHash != immutableAuthorityHash || StableRuntimeOperationExecutionAuthorityHash(committed) != immutableAuthorityHash {
		t.Fatal("mutable evidence commit identity changed immutable execution authority")
	}

	invalidOwnerCommit := record
	invalidOwnerCommit.EvidenceFactID = committed.EvidenceFactID
	invalidOwnerCommit.EvidenceFactHash = committed.EvidenceFactHash
	if err := invalidOwnerCommit.Validate(); err == nil || !strings.Contains(err.Error(), "owner_committed") {
		t.Fatalf("owner-committed evidence identity error = %v", err)
	}
}

func TestStableRuntimeOperationReceiptHashBindsValidationRecipeIdentity(t *testing.T) {
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation-1", Fence: 1, OperationID: "operation-1",
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectNone,
		Evidence: RuntimeOperationEvidence{Validation: &RuntimeValidationObservation{RecipeID: "recipe-1", ExecutedChecks: []string{"check-1"}, PassedChecks: []string{"check-1"}}},
	}
	baseline := StableRuntimeOperationReceiptHash(receipt)
	changed := receipt.Clone()
	changed.Evidence.Validation.RecipeID = "recipe-2"
	if StableRuntimeOperationReceiptHash(changed) == baseline {
		t.Fatal("operation receipt hash did not bind validation recipe identity")
	}
}
