package workorchruntimecontract

import (
	"reflect"
	"testing"
)

// Transient provenance is detached just like the original authority view;
// source replay must never expose mutable slices owned by the aggregate.
func TestCompleteAdmissionOriginalSourceLedgersCloneIsDetached(t *testing.T) {
	i := runtimeCompleteAdmissionFixture(t)
	i.EvidenceSequence = 7
	i.ProofLineage = []RuntimeCoverageProofLineage{{SourceProofIdentity: CoverageEvidenceProofIdentity{TargetRefs: []string{"original"}}}}
	i.Governance = []RuntimeOperationGovernanceBinding{{Grant: &RuntimeOperationGovernanceGrant{ResolutionID: "original"}}}
	i.Budget.Policy.Dimensions = []RuntimeBudgetDimensionPolicy{{Unit: "original"}}
	i.Resolutions = []RuntimeGovernanceResolutionRecord{{EvidenceRefs: []string{"original"}}}
	i.Attempts = []RuntimeCandidateAttemptRecord{i.Attempt.Clone()}
	manifest := i.Manifest.Clone()
	i.Sessions = []RuntimeCandidateRequestSession{{CandidateManifest: &manifest}}
	i.Confirmations = []RuntimeRepairSurfaceConfirmationRecord{{Confirmation: RuntimeRepairSurfaceConfirmation{ConfirmedTargetRefs: []string{"original"}, Observation: RuntimeValidationFailureObservation{CandidateTargetRefs: []string{"original"}}}}}
	before := i.Clone()
	c := i.Clone()
	c.ProofLineage[0].SourceProofIdentity.TargetRefs[0] = "changed"
	c.Governance[0].Grant.ResolutionID = "changed"
	c.Budget.Policy.Dimensions[0].Unit = "changed"
	c.Resolutions[0].EvidenceRefs[0] = "changed"
	c.Attempts[0].Proposal.StateChange.Operations[0].Operation.TargetRefs[0] = "changed"
	c.Sessions[0].CandidateManifest.Bindings[0].TargetRefs[0] = "changed"
	c.Confirmations[0].Confirmation.ConfirmedTargetRefs[0] = "changed"
	c.Confirmations[0].Confirmation.Observation.CandidateTargetRefs[0] = "changed"
	c.EvidenceSequence++
	if !reflect.DeepEqual(i.Clone(), before) {
		t.Fatal("transient source ledgers alias original aggregate input")
	}
}
