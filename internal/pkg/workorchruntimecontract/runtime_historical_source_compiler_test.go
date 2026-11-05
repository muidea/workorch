package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHistoricalSourceCompilerFullRawCloneAndEveryFieldFence(t *testing.T) {
	var source RuntimeLineageAssertionSources
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&source).Elem(), map[reflect.Type]int{}, &sequence)
	cloned := cloneRuntimeLineageSourcesRaw(source)
	if !reflect.DeepEqual(source, cloned) {
		t.Fatal("full source raw copier omitted or normalized a field")
	}
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(source), reflect.ValueOf(cloned), "lineage sources")
	for i := 0; i < reflect.TypeOf(source).NumField(); i++ {
		changed := cloneRuntimeLineageSourcesRaw(source)
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		if reflect.DeepEqual(changed, source) {
			t.Fatalf("unpopulated source field %s", reflect.TypeOf(source).Field(i).Name)
		}
		if !reflect.DeepEqual(changed, cloneRuntimeLineageSourcesRaw(changed)) {
			t.Fatalf("raw clone changed field %d", i)
		}
	}
}

func compareHistoricalSourceCompiler(t testing.TB, initial, changed RuntimeLineageAssertionSources) {
	t.Helper()
	session := &runtimeHistoricalPreimageSession{sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	for _, source := range []RuntimeLineageAssertionSources{initial, initial, changed, changed, initial} {
		before := cloneRuntimeLineageSourcesRaw(source)
		got := auditCommittedHistoricalResults(source, session)
		want := referenceHistoricalCompletionsBeforePreimageSession(source)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("historical consumer first error differs: %v != %v", got, want)
		}
		got = auditCommittedAssessmentAdjudicationsInSession(source.Facts, source.Completed, source.ProofLineage, source, session)
		want = referenceLineagePreimageValidateRuntimeAssessmentAdjudications(source.Facts, source.Completed, source.ProofLineage, source)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("adjudication consumer first error differs: %v != %v", got, want)
		}
		if !reflect.DeepEqual(source, before) {
			t.Fatal("complete consumer changed raw input")
		}
	}
}

func TestHistoricalSourceCompilerCompleteConsumerOracle(t *testing.T) {
	for arm := uint8(0); arm < 12; arm++ {
		initial := historicalSessionFixture(t)
		changed := cloneRuntimeLineageSourcesRaw(initial)
		mutateHistoricalSession(&changed, arm, "same-hash raw\x00变化")
		compareHistoricalSourceCompiler(t, initial, changed)
		body, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		var loaded RuntimeLineageAssertionSources
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareHistoricalSourceCompiler(t, initial, loaded)
	}
}

func TestHistoricalSourceCompilerExplicitAssessmentInputsRemainIndependent(t *testing.T) {
	s := historicalSessionFixture(t)
	session := &runtimeHistoricalPreimageSession{sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	if err := auditCommittedAssessmentAdjudicationsInSession(s.Facts, s.Completed, s.ProofLineage, s, session); err != nil {
		t.Fatal(err)
	}
	for arm := range 3 {
		changed := cloneRuntimeLineageSourcesRaw(s)
		switch arm {
		case 0:
			changed.Facts.Assessments = nil
		case 1:
			changed.Completed = append(changed.Completed, changed.Completed[len(changed.Completed)-1])
		case 2:
			changed.ProofLineage = append(changed.ProofLineage, RuntimeCoverageProofLineage{})
		}
		got := auditCommittedAssessmentAdjudicationsInSession(changed.Facts, changed.Completed, changed.ProofLineage, s, session)
		want := referenceLineagePreimageValidateRuntimeAssessmentAdjudications(changed.Facts, changed.Completed, changed.ProofLineage, s)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("explicit argument %d borrowed source success: %v != %v", arm, got, want)
		}
	}
}

func TestHistoricalSourceCompilerReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		for arm := uint8(0); arm < 18; arm++ {
			changed := run.RuntimeState.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "same hash raw drift")
			compareHistoricalSourceCompiler(t, historicalSessionArchiveSources(run.RuntimeState), historicalSessionArchiveSources(changed))
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzHistoricalSourceCompilerCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 12; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		initial := historicalSessionFixture(t)
		changed := cloneRuntimeLineageSourcesRaw(initial)
		mutateHistoricalSession(&changed, arm, raw)
		compareHistoricalSourceCompiler(t, initial, changed)
	})
}
