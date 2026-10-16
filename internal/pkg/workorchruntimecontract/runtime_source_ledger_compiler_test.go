package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func sourceLedgerSemanticSession() *runtimeSemanticValidationSession {
	s := newRuntimeSemanticValidationSession()
	s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
	return s
}

func TestSourceLedgerCompilerOwnerFactCompleteColdOracle(t *testing.T) {
	m, _, binding := semanticInputReviewV2Fixture(t)
	a := RuntimeAssessmentAuthorityInput{
		Focus: CoverageFocus{RequiredOwnerFacts: []RuntimeOwnerFactRequirement{{OwnerModuleID: RuntimeModuleShell,
			FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession,
			ValidationProofRequirementHash: binding.RequirementHash}}},
		OperationAdmissibilities: CloneRuntimeOperationAdmissibilities(m.OwnerCatalogs),
	}
	s := sourceLedgerSemanticSession()
	compare := func(value RuntimeAssessmentAuthorityInput) {
		t.Helper()
		before, _ := json.Marshal(value)
		want, wantErr := deriveRuntimeAssessmentOwnerFactSources("definition", "ws", value)
		got, gotErr := deriveRuntimeAssessmentOwnerFactSourcesInSession("definition", "ws", value, s)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
			t.Fatalf("complete owner-fact result/first error differs: %v != %v", gotErr, wantErr)
		}
		if len(got) > 0 && got[0].OwnerRevisionSet != nil {
			got[0].OwnerRevisionSet.Entries[0].Revision = "caller copy"
		}
		after, _ := json.Marshal(value)
		if !bytes.Equal(before, after) {
			t.Fatal("catalog consumer mutated raw input")
		}
	}
	compare(a)
	if len(s.catalogs) != len(a.OperationAdmissibilities) {
		t.Fatal("complete catalog gate not connected to existing semantic compiler")
	}
	for range 2 {
		compare(a)
	}
	// Same snapshot, arbitrary field drift, nil/empty and nested alias changes
	// must all reach the independent complete gate, including duplicate order.
	changed := cloneRuntimeAssessmentAuthorityInputRaw(a)
	ownerLineageFieldMutations(reflect.ValueOf(&changed).Elem(), func() { compare(changed) })
	duplicate := cloneRuntimeAssessmentAuthorityInputRaw(a)
	duplicate.OperationAdmissibilities = append(duplicate.OperationAdmissibilities, duplicate.OperationAdmissibilities[0])
	compare(duplicate)
	duplicate.OperationAdmissibilities[len(duplicate.OperationAdmissibilities)-1].SnapshotID = "bad second catalog"
	compare(duplicate)
	compare(RuntimeAssessmentAuthorityInput{})
	wire, _ := json.Marshal(a)
	var reloaded RuntimeAssessmentAuthorityInput
	if err := json.Unmarshal(wire, &reloaded); err != nil {
		t.Fatal(err)
	}
	compare(reloaded)
	for _, entries := range s.catalogs {
		for _, entry := range entries {
			for _, caller := range a.OperationAdmissibilities {
				if reflect.DeepEqual(entry.raw, caller) {
					assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(entry.raw), reflect.ValueOf(caller), "source-ledger catalog")
				}
			}
		}
	}
}

func TestSourceLedgerCompilerPurposeColdOracle(t *testing.T) {
	for _, m := range []RuntimeSemanticPlanManifest{semanticWorkValidationManifest(t), func() RuntimeSemanticPlanManifest {
		m, _, _ := semanticInputReviewV2Fixture(t)
		return m
	}()} {
		s := sourceLedgerSemanticSession()
		if _, err := validateRuntimeSemanticPlanManifestInSession(m, s); err != nil {
			t.Fatal(err)
		}
		for _, raw := range m.ProofObligations {
			compare := func(o RuntimeSemanticProofObligation) {
				t.Helper()
				before, _ := json.Marshal(o)
				want, wantErr := ResolveRuntimeSemanticEvidenceObligation(o)
				got, gotErr := resolveRuntimeSemanticEvidenceObligationInSession(o, s)
				if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
					t.Fatalf("purpose complete result/first error differs: %v != %v", gotErr, wantErr)
				}
				after, _ := json.Marshal(o)
				if !bytes.Equal(before, after) {
					t.Fatal("purpose consumer changed original bytes")
				}
			}
			compare(raw)
			changed := cloneRuntimeSemanticProofObligationRaw(raw)
			ownerLineageFieldMutations(reflect.ValueOf(&changed).Elem(), func() { compare(changed) })
		}
	}
}

func compareSourceLedgerCompiler(t *testing.T, original, changed RuntimeStateRecord) {
	t.Helper()
	semantic := sourceLedgerSemanticSession()
	for _, r := range []RuntimeStateRecord{original, changed, original} {
		s := historicalSessionArchiveSources(r)
		before, _ := json.Marshal(s)
		// This new transaction shares only existing historical pure nodes. Full
		// operation/source success is not borrowed from a previous iteration.
		session := &runtimeHistoricalPreimageSession{semantic: semantic}
		got, want := runtimeValidateLineageOperationsInSession(s, session), runtimeValidateLineageOperations(s)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("complete operations first error differs: %v != %v", got, want)
		}
		var compared []CoverageEvidenceBinding
		for _, d := range s.Completed {
			for _, binding := range d.CoverageBindings {
				duplicate := false
				for _, prior := range compared {
					duplicate = duplicate || reflect.DeepEqual(prior, binding)
				}
				if duplicate {
					continue
				}
				compared = append(compared, binding)
				got, gotErr := compileRuntimeBindingValidationInputReviewInSession(binding, s.Semantic, session)
				want, wantErr := referenceLineagePreimageCompileRuntimeBindingValidationInputReview(binding, s.Semantic)
				if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
					t.Fatal("review complete original report/source changed", gotErr, wantErr)
				}
				cutoff := s.Facts.EvidenceRevision
				if runtimeConfirmedBindingRepairInSession(binding, s, cutoff, session) != referenceLineagePreimageruntimeConfirmedBindingRepairSatisfied(binding, s, cutoff) {
					t.Fatal("confirmed repair changed source/current/budget/cutoff result")
				}
			}
		}
		got = validateRuntimeAssessmentAdjudicationsInSession(s.Facts, s.Completed, s.ProofLineage, s, session)
		want = referenceLineagePreimageValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatal("assessment complete cold first error differs", got, want)
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(before, after) {
			t.Fatal("nested source-ledger consumer changed raw input")
		}
		semantic = retainRuntimeOwnerHistoricalSubtrees(semantic)
		if semantic == nil {
			semantic = sourceLedgerSemanticSession()
		}
		if len(semantic.workStates) != 0 {
			t.Fatal("complete current-state success escaped transaction")
		}
	}
}

func TestSourceLedgerCompilerCompleteFixtureOracle(t *testing.T) {
	r := terminalHistoricalAcceptanceFixture(t)
	for arm := uint8(0); arm < 18; arm++ {
		changed := r.Clone()
		mutateRuntimeStateConsumerSession(&changed, arm, "same hash raw\x00漂移")
		compareSourceLedgerCompiler(t, r, changed)
	}
}

func TestSourceLedgerCompilerReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		for _, arm := range []uint8{4, 5, 6, 7, 8, 13, 17} {
			changed := r.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "same hash raw\x00漂移")
			compareSourceLedgerCompiler(t, r, changed)
		}
		changed := r.Clone()
		changed.ExecutionBudget.Policy.PolicyHash = "same identity invalid budget"
		compareSourceLedgerCompiler(t, r, changed)
		body, _ := json.Marshal(r)
		var reloaded RuntimeStateRecord
		if err := json.Unmarshal(body, &reloaded); err != nil {
			t.Fatal(err)
		}
		compareSourceLedgerCompiler(t, r, reloaded)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzSourceLedgerCompilerCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 18; arm++ {
		f.Add(arm, "original\x00原文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := terminalHistoricalAcceptanceFixture(t)
		changed := r.Clone()
		mutateRuntimeStateConsumerSession(&changed, arm%18, raw)
		compareSourceLedgerCompiler(t, r, changed)
	})
}
