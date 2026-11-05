package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestExecutionLineageConsumerEverySourceBindingAndCutoffFence(t *testing.T) {
	var s RuntimeLineageAssertionSources
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&s).Elem(), map[reflect.Type]int{}, &sequence)
	var binding CoverageEvidenceBinding
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&binding).Elem(), map[reflect.Type]int{}, &sequence)
	c := &runtimeHistoricalSourceCompiler{}
	calls := 0
	body := func() bool { calls++; return true }
	if !c.executionSatisfied(binding, s, "cutoff", body) || !c.executionSatisfied(cloneCoverageEvidenceBindingRaw(binding), cloneRuntimeLineageSourcesRaw(s), "cutoff", body) || calls != 1 {
		t.Fatal("complete detached input was not reused")
	}
	for i := 0; i < reflect.TypeOf(s).NumField(); i++ {
		changed := cloneRuntimeLineageSourcesRaw(s)
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		before := calls
		if reflect.DeepEqual(changed, s) || !c.executionSatisfied(binding, changed, "cutoff", body) || calls != before+1 {
			t.Fatalf("source field %s did not run the complete guard", reflect.TypeOf(s).Field(i).Name)
		}
	}
	for i := 0; i < reflect.TypeOf(binding).NumField(); i++ {
		changed := cloneCoverageEvidenceBindingRaw(binding)
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		before := calls
		if reflect.DeepEqual(changed, binding) || !c.executionSatisfied(changed, s, "cutoff", body) || calls != before+1 {
			t.Fatalf("binding field %s did not run the complete guard", reflect.TypeOf(binding).Field(i).Name)
		}
	}
	before := calls
	if !c.executionSatisfied(binding, s, "later", body) || calls != before+1 {
		t.Fatal("cutoff drift reused an old guard")
	}
	for range 2 {
		if c.executionSatisfied(binding, s, "failure", func() bool { calls++; return false }) {
			t.Fatal("false became true")
		}
	}
	if calls != before+3 {
		t.Fatal("false guard was retained")
	}
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(s), reflect.ValueOf(c.images[0].raw), "execution source")
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(binding), reflect.ValueOf(c.images[0].execution[0].binding), "execution binding")

}

func compareExecutionLineageConsumers(t testing.TB, s RuntimeLineageAssertionSources) {
	t.Helper()
	session := &runtimeHistoricalPreimageSession{sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	before := cloneRuntimeLineageSourcesRaw(s)
	for _, dispatch := range s.Completed {
		for _, binding := range dispatch.CoverageBindings {
			for range 2 {
				got := runtimeExecutionLineageAssertionsInSession(binding, s, dispatch.Transaction.EvidenceRevision, session)
				want := referenceExecutionLineageBeforeConsumerReuse(binding, s, dispatch.Transaction.EvidenceRevision, &runtimeHistoricalPreimageSession{})
				if got != want {
					t.Fatal("complete execution guard differs from independent cold oracle")
				}
			}
			changed := cloneCoverageEvidenceBindingRaw(binding)
			changed.Hash = "same binding identity, changed body"
			if got, want := runtimeExecutionLineageAssertionsInSession(changed, s, dispatch.Transaction.EvidenceRevision, session), referenceExecutionLineageBeforeConsumerReuse(changed, s, dispatch.Transaction.EvidenceRevision, &runtimeHistoricalPreimageSession{}); got != want {
				t.Fatal("binding change borrowed original guard")
			}
		}
	}
	if !reflect.DeepEqual(before, s) {
		t.Fatal("guard reuse changed original source")
	}
}

func TestExecutionLineageConsumerMatchesColdAndReload(t *testing.T) {
	base := historicalSessionFixture(t)
	compareExecutionLineageConsumers(t, base)
	for arm := uint8(0); arm < 12; arm++ {
		changed := cloneRuntimeLineageSourcesRaw(base)
		mutateHistoricalSession(&changed, arm, "same hash raw drift")
		compareExecutionLineageConsumers(t, changed)
	}
}

func TestExecutionLineageConsumerReadOnlyArchive(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		compareExecutionLineageConsumers(t, historicalSessionArchiveSources(run.RuntimeState))
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkExecutionLineageConsumerArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, name := range []string{"original_twice", "lexical_twice"} {
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					session := &runtimeHistoricalPreimageSession{}
					if name == "lexical_twice" {
						session.sourceCompiler = &runtimeHistoricalSourceCompiler{}
					}
					for range 2 {
						for _, d := range s.Completed {
							if d.Candidate.Assessment == nil {
								continue
							}
							for _, binding := range d.CoverageBindings {
								_ = runtimeExecutionLineageAssertionsInSession(binding, s, d.Transaction.EvidenceRevision, session)
							}
						}
					}
				}
			})
		}
	}
}
