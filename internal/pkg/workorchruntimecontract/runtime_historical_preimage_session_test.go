package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func historicalSessionFixture(t *testing.T) RuntimeLineageAssertionSources {
	t.Helper()
	s := historicalAcceptanceProofFixture(t)
	proof, err := referenceHistoricalAcceptanceBeforePreimageSession(s, "history", "original-assessment")
	if err != nil {
		t.Fatal(err)
	}
	s.Facts.Coverage[0].HistoricalAcceptanceProofs = []RuntimeHistoricalAcceptanceProof{proof}
	if err := referenceHistoricalCompletionsBeforePreimageSession(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func mutateHistoricalSession(s *RuntimeLineageAssertionSources, arm uint8, value string) {
	switch arm % 12 {
	case 0:
	case 1:
		s.Semantic.Manifest.OwnerCatalogs[0].SnapshotID = value
	case 2:
		s.Semantic.DefinitionSeed.CoverageItems[0].TargetText = value
	case 3:
		s.Semantic.BatchDocuments[0].ManifestHash = value
	case 4:
		s.Facts.Coverage[0].HistoricalBehavior.EvidenceRefs[0] = value
	case 5:
		s.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceEvidenceRefs[0] = value
	case 6:
		s.Facts.Coverage[0].HistoricalAcceptanceProofs = append(s.Facts.Coverage[0].HistoricalAcceptanceProofs, s.Facts.Coverage[0].HistoricalAcceptanceProofs[0])
	case 7:
		s.Receipts[0].Fence++
	case 8:
		s.ProcessedEventIDs = nil
	case 9:
		s.Facts.Assessments = nil
	case 10:
		s.Completed[0].CompletedOperationIDs = nil
	case 11:
		s.Facts.Coverage[0].CurrentBindingRef.SemanticManifestHash = value
	}
}

func compareHistoricalSession(t *testing.T, s RuntimeLineageAssertionSources) {
	t.Helper()
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got, want := ValidateRuntimeHistoricalBehaviorCompletions(s), referenceHistoricalCompletionsBeforePreimageSession(s)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("historical complete first error differs: %v != %v", got, want)
	}
	proof, proofErr := CompileRuntimeHistoricalAcceptanceProof(s, "history", "original-assessment")
	old, oldErr := referenceHistoricalAcceptanceBeforePreimageSession(s, "history", "original-assessment")
	if cwiFocusBindingsError(proofErr) != cwiFocusBindingsError(oldErr) || !reflect.DeepEqual(proof, old) {
		t.Fatalf("historical proof differs: %v != %v", proofErr, oldErr)
	}
	if proofErr == nil {
		proof.AcceptanceEvidenceRefs[0], proof.BehaviorEvidenceRefs[0] = "caller", "caller"
		again, err := CompileRuntimeHistoricalAcceptanceProof(s, "history", "original-assessment")
		if err != nil || !reflect.DeepEqual(again, old) {
			t.Fatal("returned proof polluted later call")
		}
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("historical consumer changed original source")
	}
}

func TestHistoricalPreimageSessionMatchesOriginalCompleteConsumers(t *testing.T) {
	for arm := uint8(0); arm < 12; arm++ {
		s := historicalSessionFixture(t)
		mutateHistoricalSession(&s, arm, "same hash raw drift")
		compareHistoricalSession(t, s)
		body, _ := json.Marshal(s)
		var loaded RuntimeLineageAssertionSources
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareHistoricalSession(t, loaded)
	}
}

func TestHistoricalPreimageSessionCannotReuseMutatedLedger(t *testing.T) {
	s := historicalSessionFixture(t)
	session := &runtimeHistoricalPreimageSession{}
	entries, err := session.preimages(s.Semantic)
	if err != nil || len(entries) == 0 {
		t.Fatal("invalid fixture", err)
	}
	s.Semantic.Manifest.OwnerCatalogs[0].TargetRefs[0] = "same-hash raw mutation"
	_, got := session.preimages(s.Semantic)
	_, want := runtimeAssessmentPurposePreimagesFromSemanticLedger(s.Semantic, false)
	if got == nil || cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatal("raw mutation reused old historical preimage")
	}
}

func FuzzHistoricalPreimageSessionMatchesOriginal(f *testing.F) {
	for arm := uint8(0); arm < 12; arm++ {
		f.Add(arm, "raw")
	}
	f.Fuzz(func(t *testing.T, arm uint8, value string) {
		if len(value) > 128 {
			t.Skip()
		}
		s := historicalSessionFixture(t)
		mutateHistoricalSession(&s, arm, value)
		compareHistoricalSession(t, s)
	})
}

func historicalSessionArchiveSources(r RuntimeStateRecord) RuntimeLineageAssertionSources {
	return RuntimeLineageAssertionSources{Definition: r.Definition, Facts: r.Facts, Work: r.ImplementationWork, Semantic: r.SemanticPlanning, Active: r.ActiveDispatch, Completed: r.CompletedDispatches, Receipts: r.OperationReceipts,
		Governance: r.OperationGovernanceBindings, Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence, Resolutions: r.GovernanceResolutions, Attempts: r.CandidateAttempts, Sessions: r.CandidateRequestSessions,
		ProcessedEventIDs: r.ProcessedEventIDs, ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations, Reconciliation: r.Reconciliation}
}

func TestHistoricalPreimageSessionExplicitArchiveEquivalence(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit local archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		base := run.RuntimeState
		s := historicalSessionArchiveSources(base)
		if err := ValidateRuntimeHistoricalBehaviorCompletions(s); err != nil {
			t.Fatal(err)
		}
		if err := referenceHistoricalCompletionsBeforePreimageSession(s); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*RuntimeStateRecord){
			func(r *RuntimeStateRecord) {},
			func(r *RuntimeStateRecord) {
				r.SemanticPlanning.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "raw drift"
			},
			func(r *RuntimeStateRecord) { r.OperationReceipts[0].Fence++ },
			func(r *RuntimeStateRecord) { r.Definition.StartupInputHash = "raw drift" },
			func(r *RuntimeStateRecord) {
				r.Facts.DefinitionSeed.SourceObligations.Sources[0].Text = "same hash changed raw"
			},
		} {
			changed := base.Clone()
			mutate(&changed)
			s := historicalSessionArchiveSources(changed)
			got, want := ValidateRuntimeHistoricalBehaviorCompletions(s), referenceHistoricalCompletionsBeforePreimageSession(s)
			if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
				t.Fatalf("public archive gate differs: %v != %v", got, want)
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only test changed archive")
	}
}

func BenchmarkHistoricalPreimageSessionPublicArchiveCompleteGate(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit local archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		b.Fatal(err)
	}
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, reference := range []bool{true, false} {
			name := "session"
			if reference {
				name = "original"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if reference {
						err = referenceHistoricalCompletionsBeforePreimageSession(s)
					} else {
						err = ValidateRuntimeHistoricalBehaviorCompletions(s)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
