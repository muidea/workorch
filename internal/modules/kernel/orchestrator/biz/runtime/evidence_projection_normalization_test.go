package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeEvidenceCommitFactProjectionPreservesExactDetachedFact(t *testing.T) {
	exitCode := int64(0)
	fact := runtimecontract.EvidenceFact{
		ID: "evidence/a", Kind: " validation ", FocusID: " focus/a ",
		SubjectRef: " operation/a ", DispatchID: " dispatch/a ",
		EvidenceRefs: []string{" b ", "a", "a"}, TargetRefs: []string{" target/a "},
		BindingRefs: []string{" binding/a "}, CoverageWorkItemIDs: []common.CoverageWorkItemID{" cwi/a "},
		ExitCode: &exitCode,
	}
	for _, rejected := range []bool{false, true} {
		a := RuntimeAggregate{ProcessedEventIDs: []string{string(fact.ID)}}
		if rejected {
			a.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{fact}
		} else {
			a.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
		}
		want := a.Facts.Normalized().Evidence.Accepted
		if rejected {
			want = a.Facts.Normalized().Evidence.Rejected
		}
		got, err := runtimeEvidenceCommitFactProjectionForID(a, fact.ID)
		if err != nil || got.rejected != rejected || !reflect.DeepEqual(got.fact, want[0]) {
			t.Fatalf("证据投影改变: got=%+v rejected=%v err=%v", got, rejected, err)
		}
		got.fact.EvidenceRefs[0] = "changed"
		got.fact.TargetRefs[0] = "changed"
		got.fact.BindingRefs[0] = "changed"
		got.fact.CoverageWorkItemIDs[0] = "changed"
		*got.fact.ExitCode = 1
		if fact.EvidenceRefs[0] != " b " || fact.TargetRefs[0] != " target/a " ||
			fact.BindingRefs[0] != " binding/a " || fact.CoverageWorkItemIDs[0] != " cwi/a " || exitCode != 0 {
			t.Fatal("证据投影共享调用者嵌套值")
		}
	}
}

func TestRuntimeEvidenceCommitFactProjectionRetainsRawCardinalityAndFreshFence(t *testing.T) {
	fact := runtimecontract.EvidenceFact{ID: "evidence/a", Summary: "original"}
	for _, test := range []struct {
		name               string
		accepted, rejected []runtimecontract.EvidenceFact
		processed          []string
		want               string
	}{
		{"missing", nil, nil, []string{string(fact.ID)}, "must exist exactly once"},
		{"duplicate accepted", []runtimecontract.EvidenceFact{fact, fact}, nil, []string{string(fact.ID)}, "must exist exactly once"},
		{"duplicate rejected", nil, []runtimecontract.EvidenceFact{fact, fact}, []string{string(fact.ID)}, "must exist exactly once"},
		{"both ledgers", []runtimecontract.EvidenceFact{fact}, []runtimecontract.EvidenceFact{fact}, []string{string(fact.ID)}, "must exist exactly once"},
		{"missing fence", []runtimecontract.EvidenceFact{fact}, nil, nil, "one processed-event fence"},
		{"duplicate fence", []runtimecontract.EvidenceFact{fact}, nil, []string{string(fact.ID), string(fact.ID)}, "one processed-event fence"},
		{"aliased fence", []runtimecontract.EvidenceFact{fact}, nil, []string{" evidence/a "}, "one processed-event fence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := RuntimeAggregate{ProcessedEventIDs: test.processed}
			a.Facts.Evidence = runtimecontract.EvidenceLedger{Accepted: test.accepted, Rejected: test.rejected}
			if _, err := runtimeEvidenceCommitFactProjectionForID(a, fact.ID); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("原始账本反证被绕过: %v", err)
			}
		})
	}
	a := RuntimeAggregate{ProcessedEventIDs: []string{string(fact.ID)}}
	a.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
	if _, err := runtimeEvidenceCommitFactProjectionForID(a, fact.ID); err != nil {
		t.Fatal(err)
	}
	a.Facts.Evidence.Accepted[0].Summary = "changed"
	got, err := runtimeEvidenceCommitFactProjectionForID(a, fact.ID)
	if err != nil || got.fact.Summary != "changed" {
		t.Fatalf("之前成功的投影掩盖新值: %+v %v", got, err)
	}
	a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, fact)
	if _, err := runtimeEvidenceCommitFactProjectionForID(a, fact.ID); err == nil {
		t.Fatal("之前成功的投影掩盖后续重复身份")
	}
}
