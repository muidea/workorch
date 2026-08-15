package proof

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProofPredicatesRemainBindingAndQualityBound(t *testing.T) {
	if !CoverageWorkItemIncluded([]common.CoverageWorkItemID{"cwi-1"}, "cwi-1") {
		t.Fatal("coverage item was not found")
	}
	evidence := runtimecontract.EvidenceFact{BindingRefs: []string{"binding-1"}}
	if !EvidenceReferencesBinding(evidence, "binding-1") || EvidenceReferencesBinding(evidence, "binding-2") {
		t.Fatal("binding reference predicate mismatch")
	}
	if !WeakEvidence(runtimecontract.EvidenceFact{ZeroWork: true}) {
		t.Fatal("zero-work evidence must be weak")
	}
	focus := runtimecontract.CoverageFocus{AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}, RejectedStepKinds: []string{runtimecontract.StepKindFileRead}}
	if !OperationAllowedForFocus(focus, runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileWrite}) || OperationAllowedForFocus(focus, runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileRead}) {
		t.Fatal("focus operation predicate mismatch")
	}
}
