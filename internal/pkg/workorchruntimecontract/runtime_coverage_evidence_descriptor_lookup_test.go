package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"
)

func referenceOperationKindSupportsCoverageEvidence(kind, evidenceKind string) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	evidenceKind = strings.TrimSpace(evidenceKind)
	descriptor, found := RuntimeOperationDescriptorForKind(kind)
	if !found || !descriptor.RequiresOwnerReceipt {
		return false
	}
	switch evidenceKind {
	case EvidenceKindStateChange:
		return runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedStateChange)
	case EvidenceKindValidation:
		return (kind == RuntimeBlockKindManagedValidationSession || kind == StepKindMCPToolCall) &&
			runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedValidation)
	default:
		return false
	}
}

func TestCoverageEvidenceDescriptorReadOnlyLookupMatchesPublicClone(t *testing.T) {
	before := RuntimeOperationDescriptors()
	kinds := []string{"", "unknown", "file.edit", " FILE.EDIT "}
	for _, descriptor := range before {
		kinds = append(kinds, descriptor.Kind, " \t"+descriptor.Kind+"\n")
	}
	for _, kind := range kinds {
		for _, evidence := range []string{"", "unknown", EvidenceKindStateChange, EvidenceKindValidation, " \t" + EvidenceKindStateChange + "\n", " " + EvidenceKindValidation + " "} {
			for range 2 {
				got := RuntimeOperationKindSupportsCoverageEvidence(kind, evidence)
				want := referenceOperationKindSupportsCoverageEvidence(kind, evidence)
				if got != want {
					t.Fatalf("kind=%q evidence=%q: got %t, want %t", kind, evidence, got, want)
				}
			}
		}
	}
	if !reflect.DeepEqual(before, RuntimeOperationDescriptors()) {
		t.Fatal("资格查询修改了静态库存")
	}
	for _, descriptor := range before {
		copy, found := RuntimeOperationDescriptorForKind(descriptor.Kind)
		if !found || len(copy.AllowedStates) == 0 {
			continue
		}
		copy.AllowedStates[0] = ConvergenceState("tampered")
		again, _ := RuntimeOperationDescriptorForKind(descriptor.Kind)
		if !reflect.DeepEqual(again, descriptor) {
			t.Fatal("公开 getter 泄漏内部 slice")
		}
	}
}

func FuzzCoverageEvidenceDescriptorLookupMatchesReference(f *testing.F) {
	for _, descriptor := range RuntimeOperationDescriptors() {
		f.Add(descriptor.Kind, EvidenceKindStateChange)
		f.Add(descriptor.Kind, EvidenceKindValidation)
	}
	f.Add("unknown", "invalid")
	f.Fuzz(func(t *testing.T, kind, evidence string) {
		if RuntimeOperationKindSupportsCoverageEvidence(kind, evidence) != referenceOperationKindSupportsCoverageEvidence(kind, evidence) {
			t.Fatalf("查询不等价：%q / %q", kind, evidence)
		}
	})
}

func BenchmarkCoverageEvidenceDescriptorLookup(b *testing.B) {
	for _, lookup := range []struct {
		name string
		call func(string, string) bool
	}{{"reference_clone", referenceOperationKindSupportsCoverageEvidence}, {"read_only", RuntimeOperationKindSupportsCoverageEvidence}} {
		b.Run(lookup.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if !lookup.call(StepKindFileEdit, EvidenceKindStateChange) {
					b.Fatal("预期可提供 state-change receipt")
				}
			}
		})
	}
}
