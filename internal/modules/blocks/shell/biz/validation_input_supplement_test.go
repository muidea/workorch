package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationInputSupplementRequiresOriginalAndDeliveryBehavior(t *testing.T) {
	const native = "TestWorkOrchCWI_INPUT"
	for _, test := range []struct {
		name, body, extra                                   string
		wrongImplementation, originalFails, valid, notReady bool
	}{
		{name: "delivery entry", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, valid: true},
		{name: "incorrect delivery", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, wrongImplementation: true},
		{name: "original check fails", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, originalFails: true},
		{name: "input compile failure", body: `if delivery.UndeclaredEntry(4) != 11 { t.Fatal("delivery result") }`},
		{name: "source strings", body: `b,e:=os.ReadFile("../fixture.go"); if e!=nil || !strings.Contains(string(b),"DeliveredValue") { t.Fatal("source") }`},
		{name: "dependency bypass", body: `if strings.TrimSpace(" x ")!="x" { t.Fatal("dependency") }`},
		{name: "test replacement", body: `fake:=func(v int) int { return v+7 }; if fake(4)!=11 { t.Fatal("fake") }`},
		{name: "required check skipped", body: `t.Skip("not available"); if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`},
		{name: "required subcheck skipped", body: `t.Run("required",func(t *testing.T){ t.Skip("not available"); if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") } })`},
		{name: "deleted assertions", body: `_ = delivery.DeliveredValue(4)`, notReady: true},
		{name: "test main override", body: `if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`, extra: `func TestMain(m *testing.M) { os.Exit(0) }`, notReady: true},
		{name: "initialization override", body: `if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`, extra: `func init() { os.Exit(0) }`, notReady: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := fmt.Sprintf("package fixture\nimport (\"testing\";\"os\";\"strings\")\nfunc %s(t *testing.T){ b,e:=os.ReadFile(\"fixture.go\"); if e!=nil || !strings.Contains(string(b),\"DeliveredValue\") { t.Fatal(\"source\") }; %s }\n", native, map[bool]string{true: `t.Fatal("original criterion remains required")`}[test.originalFails])
			workspace := newGoValidationWorkspace(t, original)
			delta := 7
			if test.wrongImplementation {
				delta = 6
			}
			writeNativeStructureSource(t, workspace, "fixture.go", fmt.Sprintf("package fixture\nfunc DeliveredValue(v int) int { return v+%d }\n", delta))
			base := runtimecontract.RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "check", NativeCheckRef: native, ObservationProtocolRef: shellGoBehaviorObservationProtocolRef}
			refs := []string{"go.mod", "fixture.go", "fixture_test.go"}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "behavior", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000, TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: refs,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{base}}
			execute := func(req RuntimeCommandRequest) (RuntimeCommandResult, error) {
				t.Helper()
				if err := instrumentManagedValidationNativeChecks(&req); err != nil {
					t.Fatal(err)
				}
				return NewRuntimeAdapter().ExecuteOperation(context.Background(), req)
			}
			if test.valid {
				before, err := execute(request)
				if err == nil || before.ValidationFailureDiagnostic == nil || before.ValidationFailureDiagnostic.InputAddition.Validate() != nil {
					t.Fatalf("missing typed input contract: %+v %v", before.ValidationFailureDiagnostic, err)
				}
				addition := before.ValidationFailureDiagnostic.InputAddition
				if addition.TargetRef != shellSupplementTargetRef(native) || addition.NativeCheckRef != shellSupplementCheckRef(native) || addition.OriginalNativeCheckRef != native {
					t.Fatal("wrong input contract")
				}
			}
			target := shellSupplementTargetRef(native)
			content := fmt.Sprintf("package checks\nimport (\"testing\"; \"os\"; \"strings\"; delivery \"example.com/workorch-validation-fixture\")\nfunc %s(t *testing.T) { _=os.ReadFile; _=strings.Contains; _=delivery.DeliveredValue; %s }\n%s\n", shellSupplementCheckRef(native), test.body, test.extra)
			writeNativeStructureSource(t, workspace, target, content)
			refs = append(refs, target)
			checks := shellValidationSupplementAttestations([]runtimecontract.RuntimeValidationCheckAttestationBinding{base}, refs)
			if checks[0].NativeCheckRef != native || checks[0].SupplementalNativeCheckRef != shellSupplementCheckRef(native) || checks[0].ObservationProtocolRef != shellGoSupplementObservationProtocolRef {
				t.Fatal("supplement replaced original")
			}
			inspection := inspectShellNativeValidationChecks(workspace, refs, checks)
			if (inspection.Code != "") != test.notReady {
				t.Fatalf("readiness=%+v", inspection)
			}
			if test.notReady {
				return
			}
			request.ValidationScopeTargetRefs, request.ValidationCheckAttestations = refs, checks
			result, err := execute(request)
			passed := err == nil && result.Evidence.Quality == common.EvidenceQualityValid && runtimecontract.RuntimeValidationAttestationPassed(*result.ValidationObservation, checks[0])
			if passed != test.valid {
				t.Fatalf("passed=%t want=%t code=%s err=%v", passed, test.valid, result.FailureCode, err)
			}
			if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != shellGoSupplementObservationProtocolRef {
				t.Fatal("lost v3 protocol")
			}
			if result.FailureCode == "validation_workspace_behavior_unobserved" {
				t.Fatal("ineffective supplement could open another input-addition cycle")
			}
			if test.name == "input compile failure" && (result.ValidationFailureDiagnostic == nil || len(result.ValidationFailureDiagnostic.TargetRefs) != 0) {
				t.Fatalf("invalid input granted implementation-repair targets: %+v", result.ValidationFailureDiagnostic)
			}
			bytes, err := os.ReadFile(filepath.Join(workspace, "fixture_test.go"))
			if err != nil || string(bytes) != original {
				t.Fatal("original input changed")
			}
			if test.valid {
				request.ValidationCheckAttestations = []runtimecontract.RuntimeValidationCheckAttestationBinding{base}
				old, err := execute(request)
				if err == nil || old.FailureCode != "validation_workspace_behavior_unobserved" {
					t.Fatal("old sealed v2 input silently acquired supplement")
				}
			}
		})
	}
}

func TestMultiCheckProjectSupplementSelectionExecutionAndSecondRoundTermination(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok || len(checks) < 2 {
		t.Fatalf("expected multi-check attestations, got %d", len(checks))
	}
	root, internal := t.TempDir(), t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	var origSrc strings.Builder
	origSrc.WriteString("package checks\nimport (\"os\"; \"testing\")\n")
	for _, c := range checks {
		fmt.Fprintf(&origSrc, "func %s(t *testing.T){ b, e := os.ReadFile(\"../../../docs/spec.md\"); if e != nil || len(b) == 0 { t.Fatal(\"missing spec\") } }\n", c.NativeCheckRef)
	}
	files := map[string]string{
		"go.mod":            "module fixture\ngo 1.26\n",
		"docs/spec.md":      "spec ok\n",
		"delivery/value.go": "package delivery\nfunc Compute(v int) int { return v + 7 }\n",
		target:              origSrc.String(),
	}
	var refs []string
	for ref, body := range files {
		writeNativeStructureSource(t, root, ref, body)
		refs = append(refs, ref)
	}
	manifest := shellTestWorkspaceManifestBinding("multi-supp-manifest", refs)
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("missing multi-check binding: %+v", surfaces)
	}
	baseReq := RuntimeCommandRequest{
		WorkspaceID: "ws", RunID: "run", StepID: "dispatch/op",
		WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal,
		ValidationRecipeID: recipe.ID, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000,
		TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks,
		validationProofBinding: surfaces[0].ProofRequirementBindings[0],
		ManagedValidation:      runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	if err := resolveManagedValidationRecipe(&baseReq); err != nil {
		t.Fatal(err)
	}
	execution := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "op",
		WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal,
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey("ws", "run", "dispatch", "op"),
			Fence:        1, OperationHash: "op-hash", AuthorityHash: "auth-hash",
			ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		},
	}
	before, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), baseReq, execution)
	if err == nil || before.Result.ValidationFailureDiagnostic == nil || before.Result.ValidationFailureDiagnostic.OwnerCode != "validation_workspace_behavior_unobserved" {
		t.Fatalf("expected behavior_unobserved on multi-check project input, got err=%v diag=%+v", err, before.Result.ValidationFailureDiagnostic)
	}
	addition := before.Result.ValidationFailureDiagnostic.InputAddition
	if err := addition.Validate(); err != nil {
		t.Fatalf("multi-check InputAddition invalid: %v (%+v)", err, addition)
	}
	if !strings.HasPrefix(addition.RequiredCheckRef, "workorch-cwi-check-") || addition.ContentContractRef != shellProjectSupplementContentContractRef {
		t.Fatalf("unexpected primary selection or contract ref: %+v", addition)
	}
	if !strings.Contains(before.Result.EvidenceReason, addition.TargetRef) || !strings.Contains(before.Result.EvidenceReason, addition.NativeCheckRef) {
		t.Fatalf("EvidenceReason does not match selected InputAddition: %q", before.Result.EvidenceReason)
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding", FocusID: "focus", CoverageWorkItemID: requirement.CoverageWorkItemID, DefinitionRevision: requirement.DefinitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, ValidationRecipeID: recipe.ID, ValidationProofRequirementHash: requirement.Hash, ValidationCheckAttestations: checks}},
	}
	if !addition.MatchesBinding(binding) {
		t.Fatalf("multi-check InputAddition did not match multi-attestation binding: %+v", addition)
	}

	// AST rules must forbid receiver methods and extra Test* in the supplement file, matching ContentRequirements.
	for _, invalid := range []string{
		fmt.Sprintf("package checks\nimport \"testing\"\ntype helper struct{}\nfunc (helper) check() {}\nfunc %s(t *testing.T) { t.Fatal(\"x\") }\n", addition.NativeCheckRef),
		fmt.Sprintf("package checks\nimport \"testing\"\nfunc TestExtra(t *testing.T) {}\nfunc %s(t *testing.T) { t.Fatal(\"x\") }\n", addition.NativeCheckRef),
	} {
		writeNativeStructureSource(t, root, addition.TargetRef, invalid)
		supChecks := shellValidationSupplementAttestations(checks, append(append([]string(nil), refs...), addition.TargetRef))
		if insp := inspectShellNativeValidationChecks(root, append(append([]string(nil), refs...), addition.TargetRef), supChecks); insp.Code != "native_check_supplement_contract_invalid" {
			t.Fatalf("invalid supplement AST was not rejected consistently: %+v", insp)
		}
	}

	// Second unobserved run with valid AST (still no non-test Go coverage) must terminate with supplement_unobserved and empty InputAddition.
	writeNativeStructureSource(t, root, addition.TargetRef, fmt.Sprintf("package checks\nimport (\"os\"; \"testing\")\nfunc %s(t *testing.T) { if _, e := os.ReadFile(\"../../../docs/spec.md\"); e != nil { t.Fatal(e) } }\n", addition.NativeCheckRef))
	supRefs := append(append([]string(nil), refs...), addition.TargetRef)
	supChecks := shellValidationSupplementAttestations(checks, supRefs)
	supManifest := shellTestWorkspaceManifestBinding("multi-supp-manifest-2", supRefs)
	supSurfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, supManifest, "policy", root)
	if len(supSurfaces) != 1 || len(supSurfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("missing supplemented binding: %+v", supSurfaces)
	}
	supReq := baseReq
	supReq.Args = append([]string(nil), recipe.Args...)
	supReq.ValidationScopeTargetRefs = supRefs
	supReq.ValidationCheckAttestations = supChecks
	supReq.validationProofBinding = supSurfaces[0].ProofRequirementBindings[0]
	if err := resolveManagedValidationRecipe(&supReq); err != nil {
		t.Fatal(err)
	}
	secondFail, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), supReq, execution)
	if err == nil || secondFail.Result.ValidationFailureDiagnostic == nil || secondFail.Result.ValidationFailureDiagnostic.OwnerCode != "validation_workspace_behavior_supplement_unobserved" {
		t.Fatalf("expected supplement_unobserved on second unobserved run, got err=%v diag=%+v", err, secondFail.Result.ValidationFailureDiagnostic)
	}
	if secondFail.Result.ValidationFailureDiagnostic.InputAddition != (runtimecontract.RuntimeValidationInputAddition{}) {
		t.Fatalf("second unobserved run must not offer another InputAddition: %+v", secondFail.Result.ValidationFailureDiagnostic.InputAddition)
	}

	// Real delivery behavior in supplement closes all 4 project stages conjunctively.
	writeNativeStructureSource(t, root, addition.TargetRef, fmt.Sprintf("package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc assertComputed(t *testing.T) { if delivery.Compute(4) != 11 { t.Fatal(\"wrong\") } }\nfunc %s(t *testing.T) { assertComputed(t) }\n", addition.NativeCheckRef))
	if insp := inspectShellNativeValidationChecks(root, supRefs, supChecks); insp.Code != "" {
		t.Fatalf("valid supplement with non-method helper rejected: %+v", insp)
	}
	passed, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), supReq, execution)
	if err != nil || !passed.Conjunction.Complete || passed.Result.ValidationObservation == nil || passed.Result.ValidationObservation.ProofClass != runtimecontract.RuntimeValidationProofBehavioral {
		t.Fatalf("multi-check supplemented run did not complete with behavioral proof: err=%v res=%+v", err, passed.Result)
	}
	for _, check := range supChecks {
		if !runtimecontract.RuntimeValidationAttestationPassed(*passed.Result.ValidationObservation, check) {
			t.Fatalf("supplemented multi-check attestation did not pass: %+v", check)
		}
	}
}
