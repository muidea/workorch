package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDispatchRepairCommitmentProjectionRejectsNormalizationLaundering(t *testing.T) {
	closure := RuntimeRepairClosure{
		GapSignature:                      "gap-1",
		BaselineEvidenceRevision:          InitialRuntimeEvidenceRevision,
		CoverageWorkItemIDs:               []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
		RequiredEvidenceKinds:             []string{EvidenceKindStateChange, EvidenceKindValidation},
		GroundedTargetRefs:                []string{"file/main.go", "file/other.go"},
		RequireValidationAfterStateChange: true,
	}.Normalized()
	closure.ID = StableRuntimeRepairClosureID(closure)
	commitment := RuntimeDispatchRepairCommitment{
		Purpose:                   RuntimeDispatchRepairPurposeConfirmedFileValidation,
		OriginDispatchID:          "dispatch-repair-1",
		FocusID:                   "focus-1",
		Closure:                   closure,
		ConfirmationIdentityHash:  "confirmation-identity-hash",
		ConfirmationHash:          "confirmation-hash",
		SourceDispatchID:          "dispatch-validation-1",
		SourceAuthoritySnapshotID: "authority-validation-1",
		SourceOperationID:         "operation-validation-1",
		SourceOperationHash:       "operation-hash",
		SourceEvidenceFactID:      common.EvidenceRecordID("evidence-validation-1"),
		SourceEvidenceFactHash:    "evidence-hash",
		SourceReceiptExecutionKey: "receipt-execution-key",
		SourceReceiptFence:        1,
		SourceReceiptHash:         "receipt-hash",
	}
	commitment = commitment.Normalized()
	hash := StableRuntimeDispatchRepairCommitmentHash(commitment)
	if err := ValidateRuntimeDispatchRepairCommitmentProjection(hash, &commitment); err != nil {
		t.Fatalf("canonical commitment rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RuntimeDispatchRepairCommitment)
	}{
		{"hash whitespace", func(RuntimeDispatchRepairCommitment *RuntimeDispatchRepairCommitment) {}},
		{"purpose whitespace", func(value *RuntimeDispatchRepairCommitment) {
			value.Purpose = RuntimeDispatchRepairPurpose(" " + string(value.Purpose) + " ")
		}},
		{"origin whitespace", func(value *RuntimeDispatchRepairCommitment) {
			value.OriginDispatchID = " " + value.OriginDispatchID
		}},
		{"evidence id whitespace", func(value *RuntimeDispatchRepairCommitment) {
			value.SourceEvidenceFactID = common.EvidenceRecordID(string(value.SourceEvidenceFactID) + " ")
		}},
		{"closure field whitespace", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.GapSignature += " "
		}},
		{"closure coverage order", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.CoverageWorkItemIDs[0], value.Closure.CoverageWorkItemIDs[1] =
				value.Closure.CoverageWorkItemIDs[1], value.Closure.CoverageWorkItemIDs[0]
		}},
		{"closure coverage duplicate", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.CoverageWorkItemIDs = append(value.Closure.CoverageWorkItemIDs, value.Closure.CoverageWorkItemIDs[0])
		}},
		{"closure evidence order", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.RequiredEvidenceKinds[0], value.Closure.RequiredEvidenceKinds[1] =
				value.Closure.RequiredEvidenceKinds[1], value.Closure.RequiredEvidenceKinds[0]
		}},
		{"closure evidence duplicate", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.RequiredEvidenceKinds = append(value.Closure.RequiredEvidenceKinds, value.Closure.RequiredEvidenceKinds[0])
		}},
		{"closure target order", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.GroundedTargetRefs[0], value.Closure.GroundedTargetRefs[1] =
				value.Closure.GroundedTargetRefs[1], value.Closure.GroundedTargetRefs[0]
		}},
		{"closure target duplicate", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.GroundedTargetRefs = append(value.Closure.GroundedTargetRefs, value.Closure.GroundedTargetRefs[0])
		}},
		{"closure nil empty drift", func(value *RuntimeDispatchRepairCommitment) {
			value.Closure.GroundedTargetRefs = []string{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := commitment
			mutated.Closure.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), commitment.Closure.CoverageWorkItemIDs...)
			mutated.Closure.RequiredEvidenceKinds = append([]string(nil), commitment.Closure.RequiredEvidenceKinds...)
			mutated.Closure.GroundedTargetRefs = append([]string(nil), commitment.Closure.GroundedTargetRefs...)
			test.mutate(&mutated)
			mutatedHash := StableRuntimeDispatchRepairCommitmentHash(mutated)
			if test.name == "hash whitespace" {
				mutatedHash = " " + mutatedHash
			}
			if err := ValidateRuntimeDispatchRepairCommitmentProjection(mutatedHash, &mutated); err == nil {
				t.Fatal("projection accepted normalization laundering")
			}
		})
	}

	nilTargetClosure := closure
	nilTargetClosure.GroundedTargetRefs = nil
	nilTargetClosure.ID = StableRuntimeRepairClosureID(nilTargetClosure)
	nilTargetCommitment := commitment
	nilTargetCommitment.Closure = nilTargetClosure
	nilTargetHash := StableRuntimeDispatchRepairCommitmentHash(nilTargetCommitment)
	if err := ValidateRuntimeDispatchRepairCommitmentProjection(nilTargetHash, &nilTargetCommitment); err != nil {
		t.Fatalf("canonical nil-target commitment rejected: %v", err)
	}
	nilTargetCommitment.Closure.GroundedTargetRefs = []string{}
	nilTargetHash = StableRuntimeDispatchRepairCommitmentHash(nilTargetCommitment)
	if err := ValidateRuntimeDispatchRepairCommitmentProjection(nilTargetHash, &nilTargetCommitment); err == nil {
		t.Fatal("projection accepted nil/empty closure slice laundering")
	}

	t.Run("persisted session attempt and dispatch", func(t *testing.T) {
		drifted := commitment
		drifted.Closure.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), commitment.Closure.CoverageWorkItemIDs...)
		drifted.Closure.RequiredEvidenceKinds = append([]string(nil), commitment.Closure.RequiredEvidenceKinds...)
		drifted.Closure.GroundedTargetRefs = append([]string(nil), commitment.Closure.GroundedTargetRefs...)
		drifted.Closure.RequiredEvidenceKinds[0], drifted.Closure.RequiredEvidenceKinds[1] =
			drifted.Closure.RequiredEvidenceKinds[1], drifted.Closure.RequiredEvidenceKinds[0]
		driftedHash := StableRuntimeDispatchRepairCommitmentHash(drifted)

		session := RuntimeCandidateRequestSession{
			LogicalRequestID: "request-1", DocumentKind: RuntimeProviderDocumentOperations,
			Transaction:      AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			StablePrefixHash: "prefix-1", ManifestHash: "manifest-1",
			State: RuntimeCandidateRequestSessionPrepared, NextSequence: 1,
			RepairCommitment: &drifted, RepairCommitmentHash: driftedHash,
		}
		if err := session.Validate(); err == nil {
			t.Fatal("request session normalized a noncanonical persisted repair commitment")
		}
		if err := ValidateRuntimeCandidateRequestSessions([]RuntimeCandidateRequestSession{session}); err == nil {
			t.Fatal("request-session ledger normalized a noncanonical persisted repair commitment")
		}

		facts := testRuntimeFacts()
		transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
		targetProjection := []RuntimeDispatchOperationTargetAuthorization{{
			OperationID: "op-1", OperationHash: "operation-hash-1", RequireTargets: true,
			Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
		}}
		attempt := RuntimeCandidateAttemptRecord{
			AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
			Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-1",
			State: RuntimeCandidateAttemptStateAccepted, CandidateHash: "candidate-1", DispatchID: "dispatch-1",
			AdmissionTargetAuthorizations: targetProjection,
			TargetAuthorizationHash:       StableRuntimeDispatchOperationTargetAuthorizationsHash(targetProjection),
			AdmissionCoverageBindingHash:  StableRuntimeCoverageEvidenceBindingSnapshotHash(nil),
			AdmissionRepairCommitment:     &drifted, RepairCommitmentHash: driftedHash,
			DispatchAuthorityHash: "dispatch-authority-1",
		}
		if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{attempt}); err == nil {
			t.Fatal("accepted attempt normalized a noncanonical persisted repair commitment")
		}

		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.RepairCommitment = &drifted
		dispatch.RepairCommitmentHash = driftedHash
		dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
		if err := dispatch.ValidateIntrinsic(); err == nil {
			t.Fatal("sealed dispatch normalized a noncanonical persisted repair commitment")
		}
	})
}
