package runtime

import (
	"context"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcceptedAssessmentPurposeIsPreSealedAndReverseProvedAcrossRestartAndDuplicate(t *testing.T) {
	controller, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t)
	base := runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid restarted assessment aggregate: %v", err)
	}
	if base.ActiveDispatch == nil || base.ActiveDispatch.Candidate.Assessment == nil ||
		base.ActiveDispatch.AssessmentAuthorityInput == nil ||
		base.ActiveDispatch.AssessmentAuthorityInput.Purpose != runtimecontract.RuntimeAssessmentPurposeOwnerFactAcquisition {
		t.Fatalf("assessment dispatch did not retain owner-fact purpose: %+v", base.ActiveDispatch)
	}
	var accepted *runtimecontract.RuntimeCandidateAttemptRecord
	for index := range base.CandidateAttempts {
		if base.CandidateAttempts[index].State == runtimecontract.RuntimeCandidateAttemptStateAccepted &&
			base.CandidateAttempts[index].DispatchID == base.ActiveDispatch.ID {
			value := base.CandidateAttempts[index]
			accepted = &value
			break
		}
	}
	if accepted == nil {
		t.Fatal("assessment aggregate has no accepted candidate attempt")
	}
	var manifest *runtimecontract.RuntimeCandidateContractManifest
	for index := range base.CandidateRequestSessions {
		if base.CandidateRequestSessions[index].LogicalRequestID == accepted.LogicalRequestID {
			manifest = base.CandidateRequestSessions[index].CandidateManifest
			break
		}
	}
	if manifest == nil || manifest.AssessmentPurpose != runtimecontract.RuntimeAssessmentPurposeOwnerFactAcquisition ||
		strings.TrimSpace(manifest.AssessmentPurposeManifestHash) == "" {
		t.Fatalf("accepted assessment session has no pre-seal purpose preimage: %+v", manifest)
	}

	beforeRevision := repository.aggregate.StateRevision
	duplicate, err := controller.AcceptCandidateAttempt(
		context.Background(),
		base.Facts.WorkspaceID,
		base.Facts.RunID,
		accepted.Attempt,
		base.ActiveDispatch.Candidate,
	)
	if err != nil {
		t.Fatalf("duplicate assessment delivery: %v", err)
	}
	if duplicate.StateRevision != beforeRevision || duplicate.ActiveDispatch == nil ||
		duplicate.ActiveDispatch.DispatchAuthorityHash != base.ActiveDispatch.DispatchAuthorityHash {
		t.Fatalf("duplicate assessment delivery rewrote the sealed dispatch: before=%d after=%d", beforeRevision, duplicate.StateRevision)
	}

	rawDrift := runtimeTestEvidenceAggregateRoundTrip(t, base)
	for index := range rawDrift.CandidateRequestSessions {
		if rawDrift.CandidateRequestSessions[index].LogicalRequestID != accepted.LogicalRequestID ||
			rawDrift.CandidateRequestSessions[index].CandidateManifest == nil {
			continue
		}
		rawDrift.CandidateRequestSessions[index].CandidateManifest.AssessmentPurpose =
			runtimecontract.RuntimeAssessmentPurpose(" owner_fact_acquisition ")
	}
	if err := rawDrift.Validate(); err == nil || !strings.Contains(err.Error(), "raw-canonical") {
		t.Fatalf("raw assessment purpose laundering was accepted: %v", err)
	}

	mutated := runtimeTestEvidenceAggregateRoundTrip(t, base)
	var mutatedSession *runtimecontract.RuntimeCandidateRequestSession
	for index := range mutated.CandidateRequestSessions {
		if mutated.CandidateRequestSessions[index].LogicalRequestID == accepted.LogicalRequestID {
			mutatedSession = &mutated.CandidateRequestSessions[index]
			break
		}
	}
	if mutatedSession == nil || mutatedSession.CandidateManifest == nil {
		t.Fatal("mutated assessment session is unavailable")
	}
	mutatedSession.CandidateManifest.AssessmentPurpose = runtimecontract.RuntimeAssessmentPurposeDefinitionProof
	mutatedSession.CandidateManifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(*mutatedSession.CandidateManifest)
	mutatedSession.ManifestHash = mutatedSession.CandidateManifest.Hash
	mutatedSession.LogicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
		mutatedSession.DocumentKind,
		mutated.Facts.WorkspaceID,
		mutated.Facts.RunID,
		mutatedSession.Transaction,
		mutatedSession.StablePrefixHash,
		mutatedSession.ManifestHash,
		mutatedSession.CorrectionHash,
	)
	if mutatedSession.ExpectedDocument != nil {
		expected := mutatedSession.ExpectedDocument.Clone()
		expected.ManifestHash = mutatedSession.ManifestHash
		wire, err := runtimecontract.BuildRuntimeCandidateWireContract(*mutatedSession.CandidateManifest)
		if err != nil {
			t.Fatal(err)
		}
		expected.WireContractHash = wire.Hash
		expected.LogicalRequestID = mutatedSession.LogicalRequestID
		expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
		mutatedSession.ExpectedDocument = &expected
		if mutatedSession.LastDocumentFraming != nil {
			framing := *mutatedSession.LastDocumentFraming
			framing.ExpectedDocumentHash = expected.Hash
			mutatedSession.LastDocumentFraming = &framing
		}
	}
	mutatedSession.LastPhysicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(mutatedSession.LogicalRequestID, 1)
	mutated.ActiveDispatch.CandidateManifestHash = mutatedSession.ManifestHash
	mutated.ActiveDispatch.AssessmentAuthorityInput.Purpose = runtimecontract.RuntimeAssessmentPurposeDefinitionProof
	mutated.ActiveDispatch.AssessmentAuthorityInputHash = runtimecontract.StableRuntimeAssessmentAuthorityInputHash(*mutated.ActiveDispatch.AssessmentAuthorityInput)
	mutated.ActiveDispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*mutated.ActiveDispatch))
	for index := range mutated.CandidateAttempts {
		if mutated.CandidateAttempts[index].DispatchID == mutated.ActiveDispatch.ID {
			mutated.CandidateAttempts[index].ManifestHash = mutatedSession.ManifestHash
			mutated.CandidateAttempts[index].LogicalRequestID = mutatedSession.LogicalRequestID
			mutated.CandidateAttempts[index].DispatchAuthorityHash = mutated.ActiveDispatch.DispatchAuthorityHash
		}
	}
	if err := mutated.Validate(); err == nil || !strings.Contains(err.Error(), "immutable semantic preimage") {
		t.Fatalf("coordinated assessment purpose rewrite escaped semantic preimage: %v", err)
	}
}
