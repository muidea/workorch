package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The fixture is typed source material, not an owner-currentness oracle. These
// tests verify preservation only; no newer snapshot is substituted in the union.
func applicabilityProjectionFixture(t *testing.T) RuntimeStateRecord {
	t.Helper()
	source := terminalSourceObligationFixture(t)
	fact := &source.Facts.Evidence.Accepted[0]
	set := *CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet)
	set.Entries[0].ResourceKind = "workspace_target_content"
	set.Entries[0].Revision = "independent-postcondition"
	set.Entries[0].DependencyHash = "postcondition-selector"
	set = set.WithHash()
	fact.ApplicabilityDependencies = &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: set}
	return source
}

func TestRuntimeApplicabilityProjectionPreservesBothCollections(t *testing.T) {
	source := applicabilityProjectionFixture(t)
	// No actual current observation is present in this preservation fixture.
	// Failed material must still retain both collections; it cannot claim a
	// successful current proof from the owner snapshot alone.
	source.Decision.State = ConvergenceStateFailed
	source.Decision.Terminal.Code = "fixture_failure"
	fact := source.Facts.Evidence.Accepted[0]
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	item := material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0]
	if !reflect.DeepEqual(item.OwnerRevisionSet, fact.OwnerRevisionSet) || !reflect.DeepEqual(item.ApplicabilityDependencies, fact.ApplicabilityDependencies) {
		t.Fatal("formal evidence projection lost original collection")
	}
	for _, closeout := range []RuntimeResultCloseout{material.RuntimeCloseout.CloneRaw(), material.RuntimeCloseout.Normalized()} {
		closeout.RequirementOutcomes[0].EvidenceItems[0].ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if item.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != "independent-postcondition" {
			t.Fatal("closeout clone shares original applicability")
		}
	}
	body, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeTerminalResultMaterial
	if err := json.Unmarshal(body, &reloaded); err != nil || !reflect.DeepEqual(material, reloaded) {
		t.Fatalf("current-schema material reload changed original values: %v", err)
	}
	proof := RuntimeAssessmentAdjudicationProof{CoverageWorkItemID: "cwi", EvidenceKind: fact.Kind, EvidenceFactID: fact.ID, EvidenceFactHash: StableRuntimeEvidenceFactHash(fact), OwnerRevisionSet: CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet), ApplicabilityDependencies: CloneRuntimeEvidenceApplicabilityDependencies(fact.ApplicabilityDependencies)}
	adjudication := RuntimeAssessmentAdjudication{DispatchID: "assessment", AuthorityInputID: "source", Disposition: AssessmentDispositionSatisfied, Proofs: []RuntimeAssessmentAdjudicationProof{proof}}
	for _, copy := range []RuntimeAssessmentAdjudication{adjudication.Normalized(), cloneRuntimeAssessmentAdjudicationRaw(adjudication)} {
		copy.Proofs[0].ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if proof.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != "independent-postcondition" {
			t.Fatal("assessment clone shares original applicability")
		}
	}
	changed := adjudication.Normalized()
	changed.Proofs[0].ApplicabilityDependencies = nil
	if StableRuntimeAssessmentAdjudicationHash(changed) == StableRuntimeAssessmentAdjudicationHash(adjudication) {
		t.Fatal("assessment hash did not bind applicability body")
	}
}

func TestRuntimeApplicabilityProjectionRejectsMalformedOriginal(t *testing.T) {
	for _, name := range []string{"protocol", "definition", "hash", "raw", "missing_reads"} {
		t.Run(name, func(t *testing.T) {
			source := applicabilityProjectionFixture(t)
			fact := &source.Facts.Evidence.Accepted[0]
			switch name {
			case "protocol":
				fact.ApplicabilityDependencies.ProtocolRef += " "
			case "definition":
				fact.ApplicabilityDependencies.OwnerRevisionSet.DefinitionRevision = "other"
				fact.ApplicabilityDependencies.OwnerRevisionSet = fact.ApplicabilityDependencies.OwnerRevisionSet.WithHash()
			case "hash":
				fact.ApplicabilityDependencies.OwnerRevisionSet.SetHash = "forged"
			case "raw":
				fact.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].ResourceID += " "
			case "missing_reads":
				fact.OwnerRevisionSet = nil
			}
			before := source.Clone()
			if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
				t.Fatal("malformed original applicability reached formal result")
			}
			if !reflect.DeepEqual(before, source) {
				t.Fatal("compiler repaired original values")
			}
		})
	}
}
