package workorchruntimecontract

import (
	"encoding/json"
	"errors"
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
	compiler := &runtimeHistoricalSourceCompiler{}
	calls := 0
	body := func() error { calls++; return nil }
	if err := compiler.validate(source, false, body); err != nil {
		t.Fatal(err)
	}
	if err := compiler.validate(cloned, false, body); err != nil || calls != 1 {
		t.Fatal("identical detached source did not reuse its own consumer", err)
	}
	if err := compiler.validate(cloned, true, body); err != nil || calls != 2 {
		t.Fatal("behavior success certified a different consumer", err)
	}
	for i := 0; i < reflect.TypeOf(source).NumField(); i++ {
		changed := cloneRuntimeLineageSourcesRaw(source)
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		if reflect.DeepEqual(changed, source) {
			t.Fatalf("unpopulated field %s", reflect.TypeOf(source).Field(i).Name)
		}
		before := calls
		if err := compiler.validate(changed, false, body); err != nil || calls != before+1 {
			t.Fatalf("field %s borrowed prior success: %v", reflect.TypeOf(source).Field(i).Name, err)
		}
	}
	// Mutating the caller's nested storage must not mutate the detached key.
	source.ProcessedEventIDs[0] = "changed without changing any hash"
	before := calls
	if err := compiler.validate(source, false, body); err != nil || calls != before+1 {
		t.Fatal("raw alias borrowed old success", err)
	}
	want := errors.New("original consumer failure")
	failedCalls := 0
	for range 2 {
		if got := compiler.validate(RuntimeLineageAssertionSources{}, true, func() error { failedCalls++; return want }); got != want {
			t.Fatal("failure changed", got)
		}
	}
	if failedCalls != 2 {
		t.Fatal("failed validation registered a successful preimage")
	}
}

func compareHistoricalSourceCompiler(t testing.TB, initial, changed RuntimeLineageAssertionSources) {
	t.Helper()
	session := &runtimeHistoricalPreimageSession{sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	for _, source := range []RuntimeLineageAssertionSources{initial, initial, changed, changed, initial} {
		before := cloneRuntimeLineageSourcesRaw(source)
		got := validateRuntimeHistoricalBehaviorCompletionsInSession(source, session)
		want := referenceHistoricalCompletionsBeforePreimageSession(source)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("historical consumer first error differs: %v != %v", got, want)
		}
		got = validateRuntimeAssessmentAdjudicationsInSession(source.Facts, source.Completed, source.ProofLineage, source, session)
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
	if err := validateRuntimeAssessmentAdjudicationsInSession(s.Facts, s.Completed, s.ProofLineage, s, session); err != nil {
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
		got := validateRuntimeAssessmentAdjudicationsInSession(changed.Facts, changed.Completed, changed.ProofLineage, s, session)
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
