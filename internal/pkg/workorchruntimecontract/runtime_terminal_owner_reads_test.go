package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeTerminalOwnerReadUnionOriginalCurrentProofAndHistory(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	original := source.Clone()
	want := *CloneRuntimeOwnerRevisionSet(source.Facts.Evidence.Accepted[0].OwnerRevisionSet)
	got, err := CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(source, original) {
		t.Fatalf("original exact union changed: %+v / %v", got, err)
	}
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil || !reflect.DeepEqual(material.RuntimeCloseout.OwnerRevisionSet, want) {
		t.Fatal("material replaced actual read set with synthetic evidence references")
	}
	historical := cloneEvidenceFactRaw(source.Facts.Evidence.Accepted[0])
	historical.ID, historical.BindingRefs = "historical", []string{"retired-binding"}
	historical.OwnerRevisionSet.Entries[0].Revision = "historical-bytes"
	*historical.OwnerRevisionSet = historical.OwnerRevisionSet.WithHash()
	source.Facts.Evidence.Accepted = append(source.Facts.Evidence.Accepted, historical)
	source.Facts.Evidence.Rejected = []EvidenceFact{historical}
	got, err = CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("historical/rejected reads entered current union")
	}
	historical.BindingRefs = source.Facts.Evidence.Accepted[0].BindingRefs
	source.Facts.Evidence.Accepted[1] = historical
	if _, err := CompileRuntimeTerminalOwnerReadDependencies(source); err == nil {
		t.Fatal("conflicting current original reads were arbitrarily selected")
	}
	body, _ := json.Marshal(original)
	var reload RuntimeStateRecord
	if json.Unmarshal(body, &reload) != nil {
		t.Fatal("source reload failed")
	}
	got, err = CompileRuntimeTerminalOwnerReadDependencies(reload)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("independent reload changed original read union")
	}
	got.Entries[0].Revision = "caller"
	if original.Facts.Evidence.Accepted[0].OwnerRevisionSet.Entries[0].Revision == "caller" {
		t.Fatal("union aliases original source")
	}
}

func TestRuntimeTerminalOwnerReadUnionDoesNotInventUnknownRead(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	source.Facts.Evidence.Accepted[0].OwnerRevisionSet = nil
	got, err := CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || len(got.Entries) != 0 {
		t.Fatal("missing original sample became a fake owner dependency")
	}
	source.Facts.Coverage[0].CurrentBindingRef.BindingHash = "retired"
	got, err = CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || len(got.Entries) != 0 {
		t.Fatal("retired proof contributed dependencies")
	}
}

func TestRuntimeTerminalOwnerReadUnionUsesObservedApplicabilityNotHistoricalMutationInputs(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	fact := &source.Facts.Evidence.Accepted[0]
	source.Facts.EvidenceRevision = fact.Revision
	historical := CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet)
	current := CloneRuntimeOwnerRevisionSet(historical)
	current.Entries[0].ResourceKind = "workspace_target_content"
	current.Entries[0].Revision, current.Entries[0].DependencyHash = "actual-postcondition", "content-selector"
	*current = current.WithHash()
	fact.ApplicabilityDependencies = &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: *current}
	got, err := CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || len(got.Entries) != 0 {
		t.Fatal("unobserved mutation postcondition was promoted to current union")
	}
	observation := RuntimeEvidenceApplicabilityObservation{Request: RuntimeEvidenceApplicabilityRequest{SourceEvidenceID: fact.ID, SourceEvidenceHash: StableRuntimeEvidenceFactHash(*fact), EvidenceRevision: fact.Revision}, CurrentDependencies: CloneRuntimeOwnerRevisionSet(current)}
	source.Facts.EvidenceApplicabilityObservations = []RuntimeEvidenceApplicabilityObservation{observation}
	original := source.Clone()
	got, err = CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || !reflect.DeepEqual(got, *current) || !reflect.DeepEqual(source, original) {
		t.Fatalf("SM-2 current set was replaced by historical mutation input: %+v %v", got, err)
	}
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSet, historical) || !reflect.DeepEqual(material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].ApplicabilityDependencies, fact.ApplicabilityDependencies) {
		t.Fatal("formal item lost historical/applicability pair")
	}
	// Each individually matching source must still agree in the terminal union.
	// Never choose a latest version to hide conflicting current proof.
	second := cloneEvidenceFactRaw(*fact)
	second.ID = "second-current-source"
	second.OwnerRevisionSet.Entries[0].Revision = "other-historical-input"
	*second.OwnerRevisionSet = second.OwnerRevisionSet.WithHash()
	second.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "different-postcondition"
	second.ApplicabilityDependencies.OwnerRevisionSet = second.ApplicabilityDependencies.OwnerRevisionSet.WithHash()
	source.Facts.Evidence.Accepted = append(source.Facts.Evidence.Accepted, second)
	secondObservation := observation.CloneRaw()
	secondObservation.Request.SourceEvidenceID, secondObservation.Request.SourceEvidenceHash = second.ID, StableRuntimeEvidenceFactHash(second)
	secondObservation.CurrentDependencies = CloneRuntimeOwnerRevisionSet(&second.ApplicabilityDependencies.OwnerRevisionSet)
	source.Facts.EvidenceApplicabilityObservations = append(source.Facts.EvidenceApplicabilityObservations, secondObservation)
	if _, err := CompileRuntimeTerminalOwnerReadDependencies(source); err == nil {
		t.Fatal("conflicting current resource versions were silently selected")
	}
	second.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(fact.ApplicabilityDependencies)
	source.Facts.Evidence.Accepted[1] = second
	source.Facts.EvidenceApplicabilityObservations[1].Request.SourceEvidenceHash = StableRuntimeEvidenceFactHash(second)
	source.Facts.EvidenceApplicabilityObservations[1].CurrentDependencies = CloneRuntimeOwnerRevisionSet(current)
	got, err = CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || !reflect.DeepEqual(got, *current) {
		t.Fatal("different historical versions conflicted despite equal actual current applicability")
	}
}

func TestRuntimeTerminalApplicabilityRequiresExactRevisionAndResumeGeneration(t *testing.T) {
	for _, name := range []string{"matching", "old_revision", "old_generation", "missing", "stale_then_restored"} {
		t.Run(name, func(t *testing.T) {
			source := terminalSourceObligationFixture(t)
			fact := &source.Facts.Evidence.Accepted[0]
			source.Facts.EvidenceRevision = fact.Revision
			source.Reconciliation.RequestHash = "current-resume-generation"
			fact.ApplicabilityDependencies = &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: *CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet)}
			observation := RuntimeEvidenceApplicabilityObservation{Request: RuntimeEvidenceApplicabilityRequest{SourceEvidenceID: fact.ID, SourceEvidenceHash: StableRuntimeEvidenceFactHash(*fact), EvidenceRevision: fact.Revision, ReconciliationRequestHash: source.Reconciliation.RequestHash}, CurrentDependencies: CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet)}
			switch name {
			case "old_revision":
				source.Facts.EvidenceRevision = fact.Revision + "1"
			case "old_generation":
				observation.Request.ReconciliationRequestHash = "old-resume-generation"
			case "missing":
				observation.Missing, observation.CurrentDependencies = true, nil
			case "stale_then_restored":
				stale := observation.CloneRaw()
				stale.CurrentDependencies.Entries[0].Revision = "intervening-change"
				*stale.CurrentDependencies = stale.CurrentDependencies.WithHash()
				source.Facts.EvidenceApplicabilityObservations = append(source.Facts.EvidenceApplicabilityObservations, stale)
			}
			source.Facts.EvidenceApplicabilityObservations = append(source.Facts.EvidenceApplicabilityObservations, observation)
			original := source.Clone()
			union, err := CompileRuntimeTerminalOwnerReadDependencies(source)
			if err != nil {
				t.Fatal(err)
			}
			material, materialErr := DeriveRuntimeTerminalResultMaterialFromState(source)
			if name == "matching" {
				if materialErr != nil || !reflect.DeepEqual(union, *fact.OwnerRevisionSet) || material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].State != RuntimeRequirementEvidenceAccepted {
					t.Fatalf("exact current source rejected: %v", materialErr)
				}
			} else if len(union.Entries) != 0 || materialErr == nil {
				t.Fatal("historical/failed observation became a successful current material proof")
			}
			body, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeStateRecord
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			_, reloadErr := DeriveRuntimeTerminalResultMaterialFromState(reloaded)
			if (reloadErr == nil) != (materialErr == nil) || !reflect.DeepEqual(source, original) {
				t.Fatal("reload changed applicability or material compiler mutated history")
			}
		})
	}
}
