package workorchruntimecontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateRequestIdentityIsTotalAndDeterministic(t *testing.T) {
	for _, test := range []struct {
		name     string
		logical  string
		sequence int
		physical string
		wantErr  bool
	}{
		{name: "complete", logical: "candidate-request-1", sequence: 1, physical: "candidate-call-1"},
		{name: "empty", wantErr: true},
		{name: "missing logical", sequence: 1, physical: "candidate-call-1", wantErr: true},
		{name: "zero sequence", logical: "candidate-request-1", physical: "candidate-call-1", wantErr: true},
		{name: "missing physical", logical: "candidate-request-1", sequence: 1, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := (RuntimeCandidateRequestIdentity{LogicalRequestID: test.logical, Sequence: test.sequence, PhysicalCallID: test.physical}).Validate()
			if test.wantErr {
				if !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("RuntimeCandidateRequestIdentity.Validate() error = %v, want invalid argument", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	first := StableRuntimeCandidatePhysicalCallID("candidate-request-1", 1)
	if first == "" || first != StableRuntimeCandidatePhysicalCallID("candidate-request-1", 1) || first == StableRuntimeCandidatePhysicalCallID("candidate-request-1", 2) {
		t.Fatalf("physical call ID is not stable per logical request/sequence: %q", first)
	}
	base := RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "schema-v1", Reason: "volatile reason"}
	baseObservation := &RuntimeCandidateCorrectionObservation{Class: base.Class, Code: base.Code, Signature: base.Signature, Diagnostic: &RuntimeCandidateFailureDiagnostic{Path: "$.operations"}}
	if StableRuntimeCandidateRequestCorrectionHash(nil, "", nil, nil) != "" || StableRuntimeCandidateRequestCorrectionHash(baseObservation, "", nil, nil) == "" {
		t.Fatal("correction hash did not distinguish initial and correction request")
	}
	changedReason := base
	changedReason.Reason = "another volatile reason"
	changedObservation := &RuntimeCandidateCorrectionObservation{Class: changedReason.Class, Code: changedReason.Code, Signature: changedReason.Signature, Diagnostic: &RuntimeCandidateFailureDiagnostic{Path: "$.operations"}}
	if StableRuntimeCandidateRequestCorrectionHash(baseObservation, "", nil, nil) != StableRuntimeCandidateRequestCorrectionHash(changedObservation, "", nil, nil) {
		t.Fatal("correction hash must not depend on unstable reason text")
	}
	directive := runtimeCandidateAttemptCorrectableDirective("manifest-1")
	boundHash := StableRuntimeCandidateRequestCorrectionHash(baseObservation, directive.ManifestHash, directive.Objectives, directive.Witness)
	changedDirective := directive.Clone()
	changedDirective.Witness.AlternativeValue = "different_field_present"
	if boundHash == "" || boundHash == StableRuntimeCandidateRequestCorrectionHash(baseObservation, "", nil, nil) || boundHash == StableRuntimeCandidateRequestCorrectionHash(baseObservation, changedDirective.ManifestHash, changedDirective.Objectives, changedDirective.Witness) {
		t.Fatal("correction hash did not bind the complete normalized witness")
	}
	sealedObservation := baseObservation.Clone()
	sealedObservation.SealedCorrectionHash = StableRuntimeCandidateSealedCorrectionHash(directive.ManifestHash, directive.Objectives, directive.Witness)
	if StableRuntimeCandidateRequestCorrectionHash(&sealedObservation, directive.ManifestHash, directive.Objectives, directive.Witness) == boundHash {
		t.Fatal("correction request hash did not bind the sealed observation authority")
	}
	driftedSealedObservation := sealedObservation.Clone()
	driftedSealedObservation.SealedCorrectionHash = "different-sealed-correction"
	if StableRuntimeCandidateRequestCorrectionHash(&driftedSealedObservation, directive.ManifestHash, directive.Objectives, directive.Witness) == StableRuntimeCandidateRequestCorrectionHash(&sealedObservation, directive.ManifestHash, directive.Objectives, directive.Witness) {
		t.Fatal("correction request hash ignored sealed correction drift")
	}
	normalized := (RuntimeCandidateRequestIdentity{LogicalRequestID: "  candidate-request-1 ", Sequence: 1, PhysicalCallID: " candidate-call-1  "}).Normalized()
	want := RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-1", Sequence: 1, PhysicalCallID: "candidate-call-1"}
	if normalized != want || !SameRuntimeCandidateRequestIdentity(normalized, want) {
		t.Fatalf("normalized request identity = %+v, want %+v", normalized, want)
	}
	changedSequence := want
	changedSequence.Sequence++
	if SameRuntimeCandidateRequestIdentity(want, changedSequence) {
		t.Fatal("request identities with different physical sequences were equal")
	}
}

func TestRuntimeCandidateRequestSessionIDIsRunScoped(t *testing.T) {
	tx := AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "output-1"}
	first := StableRuntimeCandidateRequestSessionID(RuntimeProviderDocumentSemanticPlan, "ws-1", "run-1", tx, "prefix", "manifest", "batch-0")
	secondRun := StableRuntimeCandidateRequestSessionID(RuntimeProviderDocumentSemanticPlan, "ws-1", "run-2", tx, "prefix", "manifest", "batch-0")
	secondBatch := StableRuntimeCandidateRequestSessionID(RuntimeProviderDocumentSemanticPlan, "ws-1", "run-1", tx, "prefix", "manifest", "batch-1")
	if first == "" || first == secondRun || first == secondBatch {
		t.Fatalf("candidate request session identity is not scoped to run/batch lineage: %q %q %q", first, secondRun, secondBatch)
	}
}

func TestRuntimeCandidateRequestSessionBindsPersistedManifestWireContract(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindFileWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-session-wire"},
			TargetRefs:           []string{"go.mod"},
			RequiredOperationSequence: []RuntimeRequiredOperationVariant{{
				RequirementID: "create-module", Kind: StepKindFileWrite, TargetRef: "go.mod",
			}},
		},
	})
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion:    RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:     RuntimeProviderDocumentOperations,
		ManifestHash:     manifest.Hash,
		WireContractHash: wire.Hash,
		LogicalRequestID: "logical-wire-1", StablePrefixHash: "prefix-wire-1",
		MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	session := RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind,
		ExpectedDocument: &expected, Transaction: AuthorityTransaction{AuthoritySnapshotID: "authority-wire-1"},
		StablePrefixHash: expected.StablePrefixHash, ManifestHash: manifest.Hash, CandidateManifest: &manifest,
		State: RuntimeCandidateRequestSessionPrepared, NextSequence: 1,
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("valid manifest-bound request session: %v", err)
	}

	drifted := session.Clone()
	drifted.ExpectedDocument.WireContractHash = "runtime-candidate-wire-drifted"
	drifted.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*drifted.ExpectedDocument)
	if err := drifted.Validate(); err == nil {
		t.Fatal("request session accepted a wire hash that did not derive from its persisted manifest")
	}
}

func TestRuntimeCandidateRequestOutcomeRequiresClosedIdentity(t *testing.T) {
	valid := []RuntimeCandidateRequestOutcome{
		{},
		{Class: RuntimeCandidateRequestOutcomeDocumentComplete},
		{Class: RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: RuntimeProviderDocumentCodeIncompleteJSON, Signature: RuntimeProviderDocumentSignatureIncompleteJSON},
		{Class: RuntimeCandidateRequestOutcomeDocumentConformanceRejected, Code: RuntimeProviderDocumentCodeFinalOutputInvalid, Signature: RuntimeProviderDocumentSignatureOutputInvalid},
	}
	for _, outcome := range valid {
		if err := outcome.Validate(); err != nil {
			t.Fatalf("valid request outcome %+v: %v", outcome, err)
		}
	}
	invalid := []RuntimeCandidateRequestOutcome{
		{Code: "stray"},
		{Class: RuntimeCandidateRequestOutcomeDocumentComplete, Code: "stray", Signature: "stray"},
		{Class: RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: RuntimeProviderDocumentCodeIncompleteJSON},
		{Class: RuntimeCandidateRequestOutcomeDocumentConformanceRejected, Signature: RuntimeProviderDocumentSignatureOutputInvalid},
	}
	for _, outcome := range invalid {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("request outcome with incomplete identity was accepted: %+v", outcome)
		}
	}
}

func TestRuntimeCandidateRequestSessionRecoveryExhaustedRequiresExactIncompleteFraming(t *testing.T) {
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentOperations,
		ManifestHash: "manifest-1", WireContractHash: "wire-1", LogicalRequestID: "logical-1", StablePrefixHash: "prefix-1",
		MaxOutputTokens: 1024, MaxRecoveryAttempts: 1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	framing := RuntimeProviderDocumentFraming{
		Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: expected.Hash,
		ResponseHash: "partial-response", ResponseBytes: 8,
		Code: RuntimeProviderDocumentCodeIncompleteJSON, Signature: RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	session := RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected,
		Transaction: AuthorityTransaction{AuthoritySnapshotID: "authority-1"}, StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: RuntimeCandidateRequestSessionTerminal, NextSequence: 3, IncompleteOutcomes: 2,
		LastDocumentFraming: &framing, LastPhysicalCallID: "physical-2",
		LastOutcome: RuntimeCandidateRequestOutcome{
			Class: RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted,
			Code:  "candidate_document_recovery_exhausted", Signature: "candidate-document-recovery-exhausted",
		},
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("valid recovery-exhausted session: %v", err)
	}
	missingFraming := session.Clone()
	missingFraming.LastDocumentFraming = nil
	if err := missingFraming.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("recovery-exhausted session without framing err=%v, want invalid argument", err)
	}
	premature := session.Clone()
	premature.IncompleteOutcomes = expected.MaxRecoveryAttempts
	premature.NextSequence = premature.IncompleteOutcomes + 1
	if err := premature.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("premature recovery-exhausted session err=%v, want invalid argument", err)
	}
}

func TestRuntimeCandidateDocumentRecoveryPolicyRejectsNegativeBudget(t *testing.T) {
	if err := (RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: -1}).Validate(); err == nil {
		t.Fatal("negative document recovery budget was accepted")
	}
}
