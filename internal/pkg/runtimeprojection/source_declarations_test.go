package runtimeprojection

import (
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"testing"
)

func TestSourceDeclarationsRetainIndependentTargetAndBudgetGates(t *testing.T) {
	work := map[string]any{"maxInputRevisionsPerCWI": 4, "inputRevisionProtocolRef": "validation/input-revision/v1"}
	evidence := map[string]any{"implementationWorkPolicy": work}
	input := map[string]any{"profileContracts": map[string]any{"p": map[string]any{"evidenceRequirements": []any{evidence}}}}
	if err := ValidateRuntimeSourceTargetDeclarations(input, nil); err != nil {
		t.Fatal(err)
	}
	if ValidateRuntimePresealTargetDeclarations(input, nil) == nil {
		t.Fatal("source metadata admitted into fresh configuration")
	}
	for _, key := range []string{"prospectiveTargetRef", "unknown"} {
		evidence[key] = "target"
		if ValidateRuntimeSourceTargetDeclarations(input, nil) == nil {
			t.Fatal("source target gate relaxed")
		}
		delete(evidence, key)
	}
	input["executionLimits"] = map[string]any{"maxProviderCalls": 1.5}
	if ValidateRuntimeSourceTargetDeclarations(input, nil) == nil {
		t.Fatal("source budget gate relaxed")
	}
	delete(input, "executionLimits")
	if err := runtimeprofiledecode.ValidateSourceShape(input); err != nil {
		t.Fatal(err)
	}
}
