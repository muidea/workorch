package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func currentAssessmentPurposeFixture(t testing.TB, ledger RuntimeSemanticPlanningLedger, bindings []CoverageEvidenceBinding) RuntimeFacts {
	t.Helper()
	binding := bindings[0]
	ref := RuntimeCWIBindingRefForCoverageBinding(binding, ledger.Manifest.Hash)
	return RuntimeFacts{
		WorkspaceID: ledger.Manifest.WorkspaceID, RunID: ledger.Manifest.RunID,
		DefinitionRevision: ledger.Manifest.DefinitionRevision, DefinitionSeed: ledger.DefinitionSeed,
		Focus:    CoverageFocus{ID: binding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, CoverageEvidenceBindings: bindings},
		Coverage: []RuntimeCoverageItemFact{{ID: binding.CoverageWorkItemID, FocusID: binding.FocusID, CurrentBindingRef: &ref}},
	}
}

func TestCurrentAssessmentPurposeConsumesExactOwnerSourceAcrossReload(t *testing.T) {
	active, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "current-request", false)
	facts := currentAssessmentPurposeFixture(t, active, bindings)
	// All three generations have the same manifest hash. Only the original
	// accepted request/proposal identity selects the current source.
	active = assessmentPreimageSessionFixture(t)
	want, err := ResolveRuntimeAssessmentPurposePreimage(active, bindings)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(active)
	var reloaded RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	for _, ledger := range []RuntimeSemanticPlanningLedger{active, reloaded} {
		for range 2 {
			got, err := RuntimeAssessmentPurposeForCurrentFocus(facts, ledger, facts.Focus)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("current purpose differs from original admitted source: %+v err=%v", got, err)
			}
		}
	}
	after, _ := json.Marshal(active)
	if string(before) != string(after) {
		t.Fatal("current-purpose consumption mutated original sources")
	}
	// No successful lookup is cached across calls or current CWI changes.
	facts.Coverage[0].CurrentBindingRef.BindingHash = "changed"
	if _, err := RuntimeAssessmentPurposeForCurrentFocus(facts, active, facts.Focus); err == nil {
		t.Fatal("later owner binding drift reused an earlier success")
	}
}

func TestCurrentAssessmentPurposeReadsRetiredSourceWithoutReadmittingUnrelatedHistory(t *testing.T) {
	definition, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	facts := currentAssessmentPurposeFixture(t, definition, bindings)
	repair, _ := assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	repair.History = []RuntimeSemanticPlanningGeneration{{
		RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{"changed"}, PrerequisiteIDs: []string{definition.Manifest.OwnerCatalogs[0].PrerequisiteID}, Snapshot: definition.Clone(),
	}}
	want, err := ResolveRuntimeAssessmentPurposePreimage(repair, bindings)
	if err != nil {
		t.Fatal(err)
	}
	// Reading a formal current result is not a raw-ledger admission. An
	// unrelated invalid source must still be rejected at raw admission/audit.
	repair.Manifest.OwnerCatalogs[0].ObservedRevision = "unrelated-raw-corruption"
	got, err := RuntimeAssessmentPurposeForCurrentFocus(facts, repair, facts.Focus)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated source was recompiled during current consumption: %+v err=%v", got, err)
	}
	if _, err := ResolveRuntimeAssessmentPurposePreimage(repair, bindings); err == nil {
		t.Fatal("raw admission/audit accepted corrupt unrelated source")
	}
}

func TestCurrentAssessmentPurposeSelectsRetiredRequestWithSameManifest(t *testing.T) {
	ledger := assessmentPreimageSessionFixture(t)
	source, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "history-request-a", false)
	if source.Manifest.Hash != ledger.Manifest.Hash || source.Manifest.Hash != ledger.History[1].Snapshot.Manifest.Hash {
		t.Fatal("fixture does not exercise equal manifest identities")
	}
	facts := currentAssessmentPurposeFixture(t, source, bindings)
	want, err := ResolveRuntimeAssessmentPurposePreimage(ledger, bindings)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RuntimeAssessmentPurposeForCurrentFocus(facts, ledger, facts.Focus)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("retired request was confused with another same-manifest generation: %+v err=%v", got, err)
	}
	ledger.History[0].Snapshot.Batches[0].RequestLogicalID = "changed"
	if _, err := RuntimeAssessmentPurposeForCurrentFocus(facts, ledger, facts.Focus); err == nil {
		t.Fatal("stale current reference accepted a different retired request")
	}
}

func TestCurrentAssessmentPurposeRejectsMissingAmbiguousAndForeignSources(t *testing.T) {
	base, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	for _, scenario := range []string{"no_row", "duplicate_row", "no_ref", "row_focus", "binding", "no_source", "unaccepted", "no_document", "document", "request", "sequence", "duplicate_document", "duplicate_generation", "workspace", "run", "definition", "seed", "manifest_scope", "no_obligation", "duplicate_obligation"} {
		t.Run(scenario, func(t *testing.T) {
			ledger := base.Clone()
			facts := currentAssessmentPurposeFixture(t, ledger, CloneCoverageEvidenceBindings(bindings))
			switch scenario {
			case "no_row":
				facts.Coverage = nil
			case "duplicate_row":
				facts.Coverage = append(facts.Coverage, facts.Coverage[0])
			case "no_ref":
				facts.Coverage[0].CurrentBindingRef = nil
			case "row_focus":
				facts.Coverage[0].FocusID = "other"
			case "binding":
				facts.Focus.CoverageEvidenceBindings[0].Hash = "other"
			case "no_source":
				ledger.Manifest = nil
			case "unaccepted":
				ledger.Batches = nil
			case "no_document":
				ledger.BatchDocuments = nil
			case "document":
				ledger.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = []string{"changed.txt"}
			case "request":
				ledger.Batches[0].RequestLogicalID = "different-generation"
			case "sequence":
				ledger.BatchDocuments[0].BatchSequence++
			case "duplicate_document":
				ledger.BatchDocuments = append(ledger.BatchDocuments, ledger.BatchDocuments[0])
			case "duplicate_generation":
				ledger.History = []RuntimeSemanticPlanningGeneration{{Snapshot: base.Clone()}}
			case "workspace":
				facts.WorkspaceID = "foreign"
			case "run":
				facts.RunID = "foreign"
			case "definition":
				facts.DefinitionRevision = "foreign"
			case "seed":
				facts.DefinitionSeed.Hash = "foreign"
			case "manifest_scope":
				ledger.Manifest.RunID = "foreign"
			case "no_obligation":
				ledger.Manifest.ProofObligations = nil
			case "duplicate_obligation":
				ledger.Manifest.ProofObligations = append(ledger.Manifest.ProofObligations, ledger.Manifest.ProofObligations[0])
			}
			if _, err := RuntimeAssessmentPurposeForCurrentFocus(facts, ledger, facts.Focus); err == nil {
				t.Fatal("missing, ambiguous, stale or foreign current source accepted")
			}
		})
	}
	repair, repairBindings := assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	facts := currentAssessmentPurposeFixture(t, repair, repairBindings)
	if _, err := RuntimeAssessmentPurposeForCurrentFocus(facts, repair, facts.Focus); err == nil {
		t.Fatal("repair-purpose binding authorized a terminal assessment")
	}
}

// Same immutable three-generation fixture and returned purpose, but distinct
// responsibilities: raw full-source audit versus formal current consumption.
// This is not an end-to-end throughput or acceptance benchmark.
func BenchmarkCurrentAssessmentPurpose(b *testing.B) {
	ledger := assessmentPreimageSessionFixture(b)
	source, bindings := assessmentPurposeGenerationForTest(b, "cwi-source", "current-request", false)
	facts := currentAssessmentPurposeFixture(b, source, bindings)
	for _, current := range []bool{false, true} {
		name := "full_source_audit"
		if current {
			name = "formal_current_consumption"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if current {
					_, err = RuntimeAssessmentPurposeForCurrentFocus(facts, ledger, facts.Focus)
				} else {
					_, err = ResolveRuntimeAssessmentPurposePreimage(ledger, bindings)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
