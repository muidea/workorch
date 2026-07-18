package file

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSupportedRuntimeOperationVariantsMatchDescriptor(t *testing.T) {
	want := []runtimecontract.RuntimeOperationVariantRef{}
	for _, kind := range []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindPatchPreview} {
		contract, ok := runtimecontract.RuntimeOperationContractForKind(kind)
		if !ok {
			t.Fatalf("missing operation contract for %s", kind)
		}
		for _, variant := range contract.Variants {
			want = append(want, runtimecontract.RuntimeOperationVariantRef{Kind: kind, VariantID: variant.ID})
		}
	}
	if got := runtimecontract.NormalizeRuntimeOperationVariantRefs(SupportedRuntimeOperationVariants()); !runtimecontract.SameRuntimeOperationVariantRefs(got, want) {
		t.Fatalf("patch owner variants = %+v, descriptor variants = %+v", got, want)
	}
}
