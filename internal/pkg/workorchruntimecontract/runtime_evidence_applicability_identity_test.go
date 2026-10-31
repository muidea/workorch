package workorchruntimecontract

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func applicabilityIdentityObservation(revision uint64, version string) RuntimeEvidenceApplicabilityObservation {
	set := (RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "owner/ws", ResourceKind: "content", ResourceID: "target", Revision: version, DependencyHash: "selector"}}}).WithHash()
	request := (RuntimeEvidenceApplicabilityRequest{ProtocolRef: RuntimeEvidenceApplicabilityObservationV1, IssuedAtStateRevision: revision, EvidenceRevision: "1", SourceEvidenceID: "evidence", SourceEvidenceHash: "fact-hash", SourceReceiptHash: "receipt-hash", DispatchID: "dispatch", OperationID: "operation"}).WithID()
	return RuntimeEvidenceApplicabilityObservation{Request: request, RecordedAtStateRevision: revision + 1, CurrentDependencies: &set}
}

func TestEvidenceApplicabilityIdentityDoesNotGrantNoProgressResetForRepeatedQuery(t *testing.T) {
	original := applicabilityIdentityObservation(10, "original")
	repeated := applicabilityIdentityObservation(20, "original")
	before := original.CloneRaw()
	hash := StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{original})
	if hash == "" || hash != StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{original, repeated}) {
		t.Fatal("identical owner bytes granted fresh equivalent authority")
	}
	changed := applicabilityIdentityObservation(30, "changed")
	changedHash := StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{original, changed})
	if changedHash == hash || changedHash != StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{changed, original, repeated}) {
		t.Fatal("distinct owner result was lost or ordering reset equivalent authority")
	}
	if changedHash != StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{original, changed, repeated}) {
		t.Fatal("restoration erased the stale original source")
	}
	if !reflect.DeepEqual(before, original) || StableRuntimeEvidenceApplicabilitySemanticHash(nil) != "" {
		t.Fatal("identity mutated raw source or changed legacy empty authority")
	}
}

func TestHistoricalEffectObservationDoesNotChangeCurrentResourceAuthority(t *testing.T) {
	ordinary := applicabilityIdentityObservation(10, "ordinary-current")
	historical := applicabilityIdentityObservation(20, "obsolete-postimage")
	historical.Request.PurposeRef = RuntimeHistoricalEffectProofV1
	historical.Request = historical.Request.WithID()
	before := historical.CloneRaw()
	if historical.ValidateRaw() != nil || StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{historical}) != "" ||
		StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{ordinary, historical}) != StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{ordinary}) {
		t.Fatal("audit-only old effect changed current authority/no-progress identity")
	}
	changed := historical.CloneRaw()
	changed.CurrentDependencies.Entries[0].Revision = "later-actual-bytes"
	*changed.CurrentDependencies = changed.CurrentDependencies.WithHash()
	if StableRuntimeEvidenceApplicabilitySemanticHash([]RuntimeEvidenceApplicabilityObservation{historical, changed}) != "" || !reflect.DeepEqual(before, historical) {
		t.Fatal("historical read drift became current authority or rewrote its original sample")
	}
	// A purpose label cannot suppress an ordinary source: independent admission
	// must reject it even after all request identities are recomputed.
	fact := historicalEffectPurposeFixture()
	ordinaryFact := fact
	ordinaryFact.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(fact.ApplicabilityDependencies)
	ordinaryFact.ApplicabilityDependencies.ProtocolRef = RuntimeEvidenceApplicabilityDependenciesProtocol
	ordinaryFact.ApplicabilityDependencies.PurposeRef = ""
	ordinaryFact.ApplicabilityDependencies.OwnerModuleID = ""
	ordinaryFact.ApplicabilityDependencies.OperationKind = ""
	ordinaryFact.ApplicabilityDependencies.OwnerContractRef = ""
	ordinaryFact.DispatchID, ordinaryFact.OperationID, ordinaryFact.Revision = historical.Request.DispatchID, historical.Request.OperationID, historical.Request.EvidenceRevision
	historical.Request.SourceEvidenceID = ordinaryFact.ID
	historical.Request.SourceEvidenceHash = StableRuntimeEvidenceFactHash(ordinaryFact)
	historical.Request = historical.Request.WithID()
	if err := ValidateRuntimeEvidenceApplicabilityObservationSource(historical, RuntimeFacts{Evidence: EvidenceLedger{Accepted: []EvidenceFact{ordinaryFact}}}, nil); err == nil || err.Error() != "evidence applicability request differs from original receipt purpose" {
		t.Fatalf("independent source admission did not reject forged purpose before receipt lookup: %v", err)
	}
}

func TestAssessmentApplicabilityHistoricalPrefixExcludesFutureSameEvidenceRevision(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod", TargetText: "original criterion"}
	first, future := applicabilityIdentityObservation(10, "original"), applicabilityIdentityObservation(30, "changed")
	facts := RuntimeFacts{DefinitionSeed: RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{item}}, DefinitionFocuses: []RuntimeDefinitionFocus{{ID: "definition-focus", SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText, CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)}}}
	dispatch := RuntimeDispatchRecord{ID: "invalidated-assessment", State: RuntimeDispatchStateInvalidated, Transaction: AuthorityTransaction{FocusID: "focus", EvidenceRevision: "1"}, Candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}}}, AssessmentAuthorityInput: &RuntimeAssessmentAuthorityInput{SourceSliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID, SourceText: item.TargetText}}
	for _, withOriginalObservation := range []bool{false, true} {
		original := dispatch.Clone()
		if withOriginalObservation {
			original.AssessmentAuthorityInput.ApplicabilityObservations = []RuntimeEvidenceApplicabilityObservation{first.CloneRaw()}
		}
		original.AssessmentAuthorityInput.AuthorityInput.ApplicabilityObservationHash = StableRuntimeEvidenceApplicabilitySemanticHash(original.AssessmentAuthorityInput.ApplicabilityObservations)
		facts.EvidenceApplicabilityObservations = CloneRuntimeEvidenceApplicabilityObservations(original.AssessmentAuthorityInput.ApplicabilityObservations)
		facts.EvidenceApplicabilityObservations = append(facts.EvidenceApplicabilityObservations, future)
		if err := ValidateRuntimeAssessmentAuthoritySources(facts, nil, []RuntimeDispatchRecord{original}); err != nil {
			t.Fatalf("future observation rewrote historical assessment cutoff: %v", err)
		}
		for _, name := range []string{"include_future", "omit_original", "replace_original"} {
			changed := original.Clone()
			switch name {
			case "include_future":
				changed.AssessmentAuthorityInput.ApplicabilityObservations = CloneRuntimeEvidenceApplicabilityObservations(facts.EvidenceApplicabilityObservations)
			case "omit_original":
				if !withOriginalObservation {
					continue
				}
				changed.AssessmentAuthorityInput.ApplicabilityObservations = nil
			case "replace_original":
				changed.AssessmentAuthorityInput.ApplicabilityObservations = []RuntimeEvidenceApplicabilityObservation{future.CloneRaw()}
			}
			if err := ValidateRuntimeAssessmentAuthoritySources(facts, nil, []RuntimeDispatchRecord{changed}); err == nil {
				t.Fatalf("%s bypassed original sealed authority", name)
			}
		}
	}
}
