package llm

import (
	"encoding/json"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateInputRepairProjectsExactOwnerDeltaWithoutGrantingProviderAuthority(t *testing.T) {
	repair := &runtimecontract.RuntimeValidationInputRepair{ContractRef: "owner/input-case/v1", RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", OriginalNativeCheckRef: "original", SupplementalNativeCheckRef: "supplement", TargetRef: "checks/input.test", BeforeHash: strings.Repeat("a", 64), AfterHash: strings.Repeat("b", 64), Corrections: []runtimecontract.RuntimeValidationInputSymbolCorrection{{Line: 137, Column: 23, Before: "expectedvalue", After: "expectedValue"}}}
	projections := runtimeCandidateOperationContractsForManifest([]runtimecontract.RuntimeCandidateOperationManifest{{Kind: runtimecontract.RuntimeBlockKindPatchApply, InputRepair: repair}}, runtimecontract.RuntimeCandidateWireContract{})
	payload, err := json.Marshal(projections)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"inputRepair"`, repair.ContractRef, repair.BeforeHash, repair.AfterHash, `"line":137`, `"column":23`, `"before":"expectedvalue"`, `"after":"expectedValue"`, "Preserve every other byte", "no validation credit"} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("C3 lost exact input repair contract %q: %s", want, payload)
		}
	}
	projections[0].InputRepair.Corrections[0].After = "other"
	if repair.Corrections[0].After != "expectedValue" {
		t.Fatal("provider projection aliases owner fact")
	}
	context := runtimecontract.RuntimeCandidateContext{RequiredState: runtimecontract.ConvergenceStateNeedValidation, Failure: &runtimecontract.FailureFact{ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{InputRepair: repair}}}
	if runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(context).InputRepair != nil {
		t.Fatal("fresh validation regained input write authority")
	}
}
