package activationports

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func implementationWorkInputFixture() map[string]any {
	return map[string]any{
		"protocolRef": runtimecontract.RuntimeImplementationWorkProtocolV1, "ownerModuleId": runtimecontract.RuntimeModuleFile,
		"allowedOperations":     []any{map[string]any{"kind": runtimecontract.StepKindFileWrite}, map[string]any{"kind": runtimecontract.RuntimeBlockKindPatchApply, "variantId": "text_replace"}},
		"allowedTargetPolicies": []string{"existing_owner_scope", "prospective_owner_target"},
		"targetSelectorRef":     "work-scope", "targetSelectorOwnerRef": "profile/example", "targetSelectorSubjectRef": "implementation",
		"targetSelectorCandidateRefs": []string{"resource-a", "resource-b"}, "prospectiveTargetAllOfConstraintRefs": []string{"constraint-a", "constraint-b"},
		"maxRoundsPerCWI": 2, "maxTargetsPerRound": 2,
		"requireFreshObservation": true, "requireFreshValidation": true, "protectValidationInputs": true,
	}
}

func implementationWorkProfileInput(raw any) map[string]any {
	return map[string]any{"stateChangePolicy": string(runtimecontract.RuntimeStateChangeAllowedButNotRequired), "profileContracts": map[string]any{"example": map[string]any{
		"evidenceRequirements": []any{map[string]any{
			"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []string{runtimecontract.EvidenceKindValidation},
			"acceptedStepKinds": []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, "implementationWorkPolicy": raw,
		}},
	}}}
}

func TestRuntimeImplementationWorkPolicyInputIsLosslessAndUnavailable(t *testing.T) {
	raw := implementationWorkInputFixture()
	input := implementationWorkProfileInput(raw)
	if err := validateRuntimeProfileInputShape(input); err != nil {
		t.Fatal(err)
	}
	contract := runtimeExecutionContract(input)
	actual := contract.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy
	if actual == nil {
		t.Fatal("decoder dropped declared policy")
	}
	wire, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var expected runtimecontract.RuntimeImplementationWorkPolicy
	if err := json.Unmarshal(wire, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual.Normalized(), expected.Normalized()) {
		t.Fatalf("lossy decode: got %+v want %+v", actual, expected)
	}
	if err := runtimecontract.ValidateRuntimeProfileContracts(contract.ProfileContracts); err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(); err != nil {
		t.Fatalf("合法声明被纯合同校验拒绝: %v", err)
	}
	if err := contract.ValidateExecutableAdmission(); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("unimplemented execution accepted: %v", err)
	}
	// Keep the closed wire shape and all typed fields in lockstep. A newly
	// allowed name must not become a silent producer/consumer mismatch.
	fields := runtimeProfileFields("implementationWork")
	known := map[string]bool{}
	for _, key := range strings.Fields(fields.text + " " + fields.boolean + " " + fields.integer + " " + fields.stringList) {
		known[key] = true
	}
	for key := range fields.arrays {
		known[key] = true
	}
	typ := reflect.TypeOf(expected)
	for i := 0; i < typ.NumField(); i++ {
		key := typ.Field(i).Tag.Get("json")
		if !known[key] {
			t.Fatalf("typed field %s has no exact raw guard", key)
		}
		delete(known, key)
	}
	if len(known) != 0 {
		t.Fatalf("raw guard claims undeclared fields: %v", known)
	}
}

func TestRuntimeImplementationWorkPolicyMalformedInputCannotDisappear(t *testing.T) {
	cases := map[string]any{"null": nil, "string": "enabled", "array": []any{}, "empty": map[string]any{}}
	for _, field := range []string{"maxRoundsPerCWI", "maxTargetsPerRound"} {
		for name, value := range map[string]any{"fraction": 1.5, "overflow": math.Inf(1), "string": "2", "zero": 0} {
			raw := implementationWorkInputFixture()
			raw[field] = value
			cases[field+"/"+name] = raw
		}
	}
	for name, change := range map[string]func(map[string]any){
		"unknown field":        func(p map[string]any) { p["nextAction"] = "execute" },
		"wrong case":           func(p map[string]any) { p["OwnerModuleID"] = p["ownerModuleId"]; delete(p, "ownerModuleId") },
		"wrong bool":           func(p map[string]any) { p["protectValidationInputs"] = "true" },
		"missing protection":   func(p map[string]any) { delete(p, "protectValidationInputs") },
		"blank candidates":     func(p map[string]any) { p["targetSelectorCandidateRefs"] = []any{"ok", " "} },
		"duplicate candidates": func(p map[string]any) { p["targetSelectorCandidateRefs"] = []string{"same", "same"} },
		"null operation":       func(p map[string]any) { p["allowedOperations"] = []any{nil} },
		"operation authority": func(p map[string]any) {
			p["allowedOperations"] = []any{map[string]any{"kind": runtimecontract.StepKindFileWrite, "targetRef": "arbitrary"}}
		},
	} {
		raw := implementationWorkInputFixture()
		change(raw)
		cases[name] = raw
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			input := implementationWorkProfileInput(raw)
			p := runtimeImplementationWorkPolicy(map[string]any{"implementationWorkPolicy": raw})
			if p == nil || p.Validate() == nil {
				t.Fatal("malformed policy became absent or valid")
			}
			decoded := runtimeExecutionContract(input)
			if len(decoded.ProfileContracts) != 1 || decoded.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy == nil {
				t.Fatal("profile decoding erased invalid presence")
			}
			if err := runtimecontract.ValidateRuntimeProfileContracts(decoded.ProfileContracts); err == nil {
				t.Fatal("typed owner admission accepts malformed input")
			}
			// Direct activation must also enforce the raw shape gate.
			if validateRuntimeProfileInputShape(input) != nil {
				if err := validateRuntimePresealTargetDeclarations(input, nil); err == nil {
					t.Fatal("invalid work declaration bypassed activation guard")
				}
			}
		})
	}
	if runtimeImplementationWorkPolicy(map[string]any{}) != nil {
		t.Fatal("absence enabled work")
	}
}

func TestRuntimeImplementationWorkPolicyRejectsMalformedSourcesBeforeMerge(t *testing.T) {
	for _, source := range []string{"workspace", "template"} {
		t.Run(source, func(t *testing.T) {
			builtin := softwareDevProfileTemplateForTest(t)
			good, err := workorcheventpayload.FromMap(implementationWorkProfileInput(implementationWorkInputFixture()))
			if err != nil {
				t.Fatal(err)
			}
			raw := implementationWorkInputFixture()
			raw["nextAction"] = "execute"
			bad, err := workorcheventpayload.FromMap(implementationWorkProfileInput(raw))
			if err != nil {
				t.Fatal(err)
			}
			workspace := &workspacemodel.Workspace{ID: "workspace-profile", TemplateID: builtin.ID, ContextHints: good}
			builtin.ContextHints = bad
			if source == "workspace" {
				builtin.ContextHints, workspace.ContextHints = good, bad
			}
			before := workspace.ContextHints.Clone()
			if _, _, _, _, _, err := definitionAdmissionProfileBindings(workspace, builtin); err == nil {
				t.Fatal("invalid source hidden by merge")
			}
			if !reflect.DeepEqual(before, workspace.ContextHints) {
				t.Fatal("source rewritten")
			}
		})
	}
}
