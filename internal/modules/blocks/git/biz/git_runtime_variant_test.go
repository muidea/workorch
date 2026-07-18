package git

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSupportedRuntimeOperationVariantsMatchDescriptor(t *testing.T) {
	got := runtimecontract.NormalizeRuntimeOperationVariantRefs(SupportedRuntimeOperationVariants())
	want := []runtimecontract.RuntimeOperationVariantRef{}
	for _, kind := range []string{runtimecontract.StepKindGitRead, runtimecontract.StepKindGitWrite} {
		contract, ok := runtimecontract.RuntimeOperationContractForKind(kind)
		if !ok {
			t.Fatalf("missing operation contract for %s", kind)
		}
		for _, variant := range contract.Variants {
			want = append(want, runtimecontract.RuntimeOperationVariantRef{Kind: kind, VariantID: variant.ID})
		}
	}
	want = runtimecontract.NormalizeRuntimeOperationVariantRefs(want)
	if !runtimecontract.SameRuntimeOperationVariantRefs(got, want) {
		t.Fatalf("git owner variants = %+v, descriptor variants = %+v", got, want)
	}
}
