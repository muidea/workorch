package proof

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Frozen original Runtime focus traversal. The projected single-member
// predicate is the actual pre-batch path, not a full-Facts benchmark substitute.
func referenceAnyFocusBeforeBatch(facts rc.RuntimeFacts, fact rc.EvidenceFact, lineage []rc.RuntimeCoverageProofLineage) bool {
	focuses := append([]rc.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	focuses = append(focuses, facts.CompletedFocuses...)
	for _, focus := range focuses {
		probe := facts
		probe.Focus = focus
		for _, id := range focus.CoverageWorkItemIDs {
			if EvidenceAppliesToCoverageWorkItemWithLineage(probe, fact, id, lineage) {
				return true
			}
		}
	}
	return false
}

// Independent original complete-input predicate also checks that common-field
// normalization is independent of Focus. No test reference is a production lane.
func referenceAnyFocusCompleteFacts(facts rc.RuntimeFacts, fact rc.EvidenceFact, lineage []rc.RuntimeCoverageProofLineage) bool {
	focuses := append([]rc.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	focuses = append(focuses, facts.CompletedFocuses...)
	for _, focus := range focuses {
		probe := facts
		probe.Focus = focus
		for _, id := range focus.CoverageWorkItemIDs {
			if referenceLineageBeforeProjectionEvidenceAppliesToCoverageWorkItemWithLineage(probe, fact, id, lineage) {
				return true
			}
		}
	}
	return false
}

func compareAnyFocusAttribution(t testing.TB, facts rc.RuntimeFacts, fact rc.EvidenceFact, lineage []rc.RuntimeCoverageProofLineage) {
	t.Helper()
	input := struct {
		Facts   rc.RuntimeFacts
		Fact    rc.EvidenceFact
		Lineage []rc.RuntimeCoverageProofLineage
	}{facts, fact, lineage}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got := EvidenceAppliesToAnyFocusWithLineage(facts, fact, lineage)
	if got != referenceAnyFocusBeforeBatch(facts, fact, lineage) || got != referenceAnyFocusCompleteFacts(facts, fact, lineage) {
		t.Fatal("batched attribution differs from original focus/member predicate")
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("attribution changed original inputs")
	}
}

func attributionBatchFixture() (rc.RuntimeFacts, rc.EvidenceFact) {
	binding := proofTestCoverageBinding("binding-original", "catalog-original")
	facts := proofTestCurrentFacts(binding)
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"absent", "cwi-1", "cwi-1", " cwi-1 "}
	facts.RemainingFocuses = []rc.CoverageFocus{{ID: "empty"}, facts.Focus.Normalized()}
	facts.CompletedFocuses = []rc.CoverageFocus{facts.Focus.Normalized()}
	fact := rc.EvidenceFact{ID: "source", Kind: rc.EvidenceKindStateChange, Quality: common.EvidenceQualityValid, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, BindingRefs: []string{binding.ID}}
	return facts, fact
}

func TestAnyFocusAttributionOriginalFieldsMembersAndQueues(t *testing.T) {
	facts, fact := attributionBatchFixture()
	if !EvidenceAppliesToAnyFocusWithLineage(facts, fact, nil) {
		t.Fatal("positive fixture is not attributable")
	}
	compareAnyFocusAttribution(t, facts, fact, nil)
	for i := 0; i < reflect.TypeOf(facts).NumField(); i++ {
		changed := (rc.RuntimeStateRecord{Facts: facts}).Clone().Facts
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		compareAnyFocusAttribution(t, changed, fact, nil)
	}
	for i := 0; i < reflect.TypeOf(fact).NumField(); i++ {
		changed := (rc.RuntimeStateRecord{Facts: rc.RuntimeFacts{Evidence: rc.EvidenceLedger{Accepted: []rc.EvidenceFact{fact}}}}).Clone().Facts.Evidence.Accepted[0]
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		compareAnyFocusAttribution(t, facts, changed, nil)
	}
	for _, name := range []string{"remaining-only", "completed-only", "raw-member", "empty-member", "wrong-binding", "no-binding", "unbound", "settled", "wrong-schema", "raw-schema", "raw-revision", "duplicate-row", "reverse", "no-focus", "json-reload"} {
		t.Run(name, func(t *testing.T) {
			f := (rc.RuntimeStateRecord{Facts: facts}).Clone().Facts
			switch name {
			case "remaining-only":
				f.Focus, f.CompletedFocuses = rc.CoverageFocus{}, nil
			case "completed-only":
				f.Focus, f.RemainingFocuses = rc.CoverageFocus{}, nil
			case "raw-member":
				f.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{" cwi-1 "}
				f.RemainingFocuses, f.CompletedFocuses = nil, nil
			case "empty-member":
				f.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{""}
				f.RemainingFocuses, f.CompletedFocuses = nil, nil
			case "wrong-binding":
				f.Coverage[0].CurrentBindingRef.BindingHash = "changed"
			case "no-binding":
				f.Coverage[0].CurrentBindingRef = nil
			case "unbound":
				f.Coverage[0].CWIConvergenceState = rc.RuntimeCWIConvergenceStateUnbound
			case "settled":
				f.Coverage[0].CWIConvergenceState = rc.RuntimeCWIConvergenceStateSettled
			case "wrong-schema":
				f.CWILedgerSchemaVersion = "wrong"
			case "raw-schema":
				f.CWILedgerSchemaVersion = " " + f.CWILedgerSchemaVersion + " "
			case "raw-revision":
				f.EvidenceRevision = " 2 "
			case "duplicate-row":
				f.Coverage = append(f.Coverage, f.Coverage[0])
			case "reverse":
				slices.Reverse(f.RemainingFocuses)
				slices.Reverse(f.Focus.CoverageWorkItemIDs)
			case "no-focus":
				f.Focus, f.RemainingFocuses, f.CompletedFocuses = rc.CoverageFocus{}, nil, nil
			case "json-reload":
				body, _ := json.Marshal(f)
				if err := json.Unmarshal(body, &f); err != nil {
					t.Fatal(err)
				}
			}
			compareAnyFocusAttribution(t, f, fact, nil)
		})
	}
}

func TestAnyFocusAttributionStaleSourceNeverRevives(t *testing.T) {
	facts, fact := attributionBatchFixture()
	facts.EvidenceRevision = "2"
	set := (rc.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1", Entries: []rc.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "owner", ResourceKind: "target", ResourceID: "target", Revision: "original", DependencyHash: "selector"}}}).WithHash()
	fact.ApplicabilityDependencies = &rc.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: rc.RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: set}
	current := rc.RuntimeEvidenceApplicabilityObservation{Request: rc.RuntimeEvidenceApplicabilityRequest{SourceEvidenceID: fact.ID, SourceEvidenceHash: rc.StableRuntimeEvidenceFactHash(fact), EvidenceRevision: "2"}, CurrentDependencies: rc.CloneRuntimeOwnerRevisionSet(&set)}
	stale := current.CloneRaw()
	stale.Missing, stale.CurrentDependencies, stale.Request.EvidenceRevision = true, nil, "1"
	for _, name := range []string{"current", "stale-first", "stale-last", "future-stale", "raw-revision", "wrong-source-hash"} {
		t.Run(name, func(t *testing.T) {
			f := (rc.RuntimeStateRecord{Facts: facts}).Clone().Facts
			f.EvidenceApplicabilityObservations = []rc.RuntimeEvidenceApplicabilityObservation{current.CloneRaw()}
			switch name {
			case "stale-first", "raw-revision":
				f.EvidenceApplicabilityObservations = append([]rc.RuntimeEvidenceApplicabilityObservation{stale.CloneRaw()}, f.EvidenceApplicabilityObservations...)
			case "stale-last":
				f.EvidenceApplicabilityObservations = append(f.EvidenceApplicabilityObservations, stale.CloneRaw())
			case "future-stale":
				s := stale.CloneRaw()
				s.Request.EvidenceRevision = "3"
				f.EvidenceApplicabilityObservations = append(f.EvidenceApplicabilityObservations, s)
			case "wrong-source-hash":
				s := stale.CloneRaw()
				s.Request.SourceEvidenceHash = "other"
				f.EvidenceApplicabilityObservations = append(f.EvidenceApplicabilityObservations, s)
			}
			if name == "raw-revision" {
				f.EvidenceRevision = " 2 "
			}
			compareAnyFocusAttribution(t, f, fact, nil)
			want := name != "stale-first" && name != "stale-last" && name != "raw-revision"
			if EvidenceAppliesToAnyFocusWithLineage(f, fact, nil) != want {
				t.Fatal("stale/current finding changed")
			}
		})
	}
}

func TestAnyFocusAttributionReadOnlyCompleteArchive(t *testing.T) {
	body, path, runs := evidenceLineageArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		for _, fact := range append(append([]rc.EvidenceFact(nil), r.Facts.Evidence.Accepted...), r.Facts.Evidence.Rejected...) {
			compareAnyFocusAttribution(t, r.Facts, fact, r.CoverageProofLineage)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("query changed original archive")
	}
}

func TestAnyFocusAttributionEquivalentSourceInCompletedQueue(t *testing.T) {
	old := proofTestCoverageBinding("binding-old", "catalog-old")
	current := proofTestCoverageBinding("binding-current", "catalog-current")
	facts := proofTestCurrentFacts(current)
	facts.CompletedFocuses, facts.Focus = []rc.CoverageFocus{facts.Focus}, rc.CoverageFocus{}
	fact := rc.EvidenceFact{ID: "source", Kind: rc.EvidenceKindStateChange, Quality: common.EvidenceQualityValid, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, BindingRefs: []string{old.ID}}
	oldIdentity, err := rc.DeriveCoverageEvidenceProofIdentity(old, fact.Kind)
	if err != nil {
		t.Fatal(err)
	}
	currentIdentity, err := rc.DeriveCoverageEvidenceProofIdentity(current, fact.Kind)
	if err != nil {
		t.Fatal(err)
	}
	source := rc.RuntimeCoverageProofLineage{ID: "source-lineage", Kind: rc.RuntimeCoverageProofLineageKindSource, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, CoverageWorkItemID: "cwi-1", EvidenceKind: fact.Kind, SourceEvidenceID: fact.ID, SourceBindingRef: old.ID, SourceBindingHash: old.Hash, SourceProofIdentity: oldIdentity}
	resolution := rc.RuntimeCoverageProofLineage{Kind: rc.RuntimeCoverageProofLineageKindResolution, SourceLineageID: source.ID, CurrentBindingRef: current.ID, CurrentBindingHash: current.Hash, CurrentProofIdentity: currentIdentity, Disposition: rc.RuntimeCoverageProofLineageDispositionEquivalent}
	for _, name := range []string{"unresolved", "equivalent", "stale-resolution", "other-workspace", "other-run"} {
		t.Run(name, func(t *testing.T) {
			lineage := []rc.RuntimeCoverageProofLineage{source}
			if name != "unresolved" {
				lineage = append(lineage, resolution)
			}
			if name == "stale-resolution" {
				lineage[1].CurrentBindingHash = "changed"
			}
			if name == "other-workspace" {
				lineage[0].WorkspaceID = "another"
			}
			if name == "other-run" {
				lineage[0].RunID = "another"
			}
			compareAnyFocusAttribution(t, facts, fact, lineage)
			if EvidenceAppliesToAnyFocusWithLineage(facts, fact, lineage) != (name == "equivalent") {
				t.Fatal("exact equivalent source/scope gate changed")
			}
		})
	}
}

func FuzzAnyFocusAttributionMatchesOriginal(f *testing.F) {
	for _, bits := range []uint16{0, 1, 2, 4, 8, 16, 32, 64, 255, 65535} {
		f.Add(bits)
	}
	f.Fuzz(func(t *testing.T, bits uint16) {
		facts, fact := attributionBatchFixture()
		if bits&1 != 0 {
			facts.Focus = rc.CoverageFocus{}
		}
		if bits&2 != 0 {
			facts.RemainingFocuses = nil
		}
		if bits&4 != 0 {
			facts.CompletedFocuses = nil
		}
		if bits&8 != 0 {
			facts.Coverage[0].CurrentBindingRef.BindingHash = "changed"
		}
		if bits&16 != 0 {
			fact.ZeroWork = true
		}
		if bits&32 != 0 {
			fact.BindingRefs = []string{"wrong"}
		}
		if bits&64 != 0 {
			facts.CWILedgerSchemaVersion = " " + facts.CWILedgerSchemaVersion + " "
		}
		if bits&128 != 0 {
			facts.Coverage[0].CWIConvergenceState = rc.RuntimeCWIConvergenceStateSettled
		}
		if bits&256 != 0 {
			facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{" cwi-1 "}
		}
		if bits&512 != 0 {
			fact.Quality = common.EvidenceQuality("")
		}
		compareAnyFocusAttribution(t, facts, fact, nil)
	})
}

func BenchmarkAnyFocusAttributionCompleteArchive(b *testing.B) {
	_, _, runs := evidenceLineageArchive(b)
	for _, run := range runs {
		r := run.RuntimeState
		for _, name := range []string{"reference", "current"} {
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					for _, fact := range r.Facts.Evidence.Accepted {
						if name == "reference" {
							referenceAnyFocusBeforeBatch(r.Facts, fact, r.CoverageProofLineage)
						} else {
							EvidenceAppliesToAnyFocusWithLineage(r.Facts, fact, r.CoverageProofLineage)
						}
					}
				}
			})
		}
	}
}
