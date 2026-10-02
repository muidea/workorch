package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSemanticBindingCompilerReusesOnlyCallLocalValidatedManifest(t *testing.T) {
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	manifest := ledger.Manifest.Clone()
	before, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := validateRuntimeSemanticPlanManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want, wantContract, err := CompileRuntimeSemanticBindings(manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		t.Fatal(err)
	}
	got, gotContract, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatalf("私有 compiler 改变完整产物: %v", err)
	}
	after, err := json.Marshal(ledger)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("编译修改调用者 ledger: %v", err)
	}
	for _, scenario := range []string{"document", "owner", "missing_owner", "request"} {
		t.Run(scenario, func(t *testing.T) {
			document := ledger.Document.Clone()
			facts := append([]RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...)
			request := ledger.LogicalRequestID
			switch scenario {
			case "document":
				document.ManifestHash = "drift"
			case "owner":
				facts[0].OwnerModuleID = RuntimeModuleShell
			case "missing_owner":
				facts = nil
			case "request":
				request = ""
			}
			if _, _, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, request, document, facts); err == nil {
				t.Fatal("manifest 校验产物绕过逐批 document/owner/request 反证")
			}
		})
	}
	manifest.ProofObligations[0].DeclarationRef = "drift"
	if _, _, err := CompileRuntimeSemanticBindings(manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities); err == nil {
		t.Fatal("前次成功掩盖公开入口的新 manifest 漂移")
	}
}
