package definitionconfiguration

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompileProducesNormalizedBindingsAndEffectiveMerge(t *testing.T) {
	templateHints := payload(t, map[string]any{
		"profileContracts": map[string]any{"profile-a": map[string]any{
			"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "proof", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"test"}}},
		}},
		"executionLimits": limits(3, 2),
	})
	workspaceHints := payload(t, map[string]any{
		"profileContracts": map[string]any{"profile-a": map[string]any{
			"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "proof", "requiredEvidenceKinds": []any{"validation", "artifact"}, "acceptedStepKinds": []any{"test"}}},
		}},
	})
	templatePolicy := payload(t, map[string]any{"id": "template-policy", "appliesTo": "shell_command", "rule": "test"})
	workspacePolicy := payload(t, map[string]any{"id": "workspace-policy", "appliesTo": "shell_command", "rule": "test"})
	workspace := &workspacemodel.Workspace{ID: "ws", ContextHints: workspaceHints, EvidencePolicies: []workorcheventpayload.Payload{workspacePolicy}}
	template := &workspacemodel.WorkspaceTemplate{ID: "tpl", ContextHints: templateHints, EvidencePolicies: []workorcheventpayload.Payload{templatePolicy}}

	got, err := Compile(workspace, template)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(got.ProfileContracts) != 1 || len(got.ProfileContracts[0].EvidenceRequirements) != 1 ||
		!reflect.DeepEqual(got.ProfileContracts[0].EvidenceRequirements[0].RequiredEvidenceKinds, []string{"artifact", "validation"}) {
		t.Fatalf("effective profile merge = %+v", got.ProfileContracts)
	}
	if got.ProfileContractsHash != runtimecontract.StableRuntimeProfileContractsHash(got.ProfileContracts) {
		t.Fatalf("profile hash %q does not hash returned normalized profiles", got.ProfileContractsHash)
	}
	if len(got.PolicyBindings) != 4 {
		t.Fatalf("policy bindings len = %d, want profile + two evidence policies + limits: %+v", len(got.PolicyBindings), got.PolicyBindings)
	}
	if got.PolicyBindings[0].PolicyRef != "profile:profile-a" || got.PolicyBindings[1].PolicyRef != "template-evidence-policy:000001:template-policy" ||
		got.PolicyBindings[2].PolicyRef != "workspace-evidence-policy:000001:workspace-policy" || got.PolicyBindings[3].PolicyRef != executionLimitsAdmissionRef {
		t.Fatalf("policy binding order/source identity changed: %+v", got.PolicyBindings)
	}
	if len(got.RequiredOwnerFacts) != 0 || len(got.OwnerFactConvergences) != 0 {
		t.Fatalf("unexpected owner facts from empty declaration: %+v / %+v", got.RequiredOwnerFacts, got.OwnerFactConvergences)
	}
}

func TestCompileRejectsInvalidOriginalSourceBeforeTemplatePrecedence(t *testing.T) {
	invalidWorkspace := &workspacemodel.Workspace{ID: "ws", ContextHints: payload(t, map[string]any{"executionLimits": map[string]any{
		"protocolRef": runtimecontract.RuntimeExecutionLimitsV1, "maxProviderCalls": 2, "maxCapabilityOperations": 1, "unexpected": true,
	}})}
	template := &workspacemodel.WorkspaceTemplate{ID: "tpl", ContextHints: payload(t, map[string]any{"executionLimits": limits(4, 3)})}
	if _, err := Compile(invalidWorkspace, template); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid Workspace limits error = %v, want invalid argument", err)
	}
}

func TestCompileResolvesRetiredInheritedDefaultsBeforeCurrentValidation(t *testing.T) {
	current, input, _ := templateProofFixture(t, 1)
	original := current.Clone()
	hints := original.ContextHints.Map()
	for _, raw := range hints["profileContracts"].(map[string]any) {
		raw.(map[string]any)["retiredBuiltinProcessingRule"] = true
	}
	original.ContextHints = payload(t, hints)
	w := &workspacemodel.Workspace{ID: input.WorkspaceID, TemplateID: current.ID, ContextHints: original.ContextHints.Clone(),
		DefinitionAuthoring: &workspacemodel.DefinitionAuthoringRecord{Input: input, TemplateSnapshot: original}}
	before := w.ContextHints.Clone()
	for range 2 {
		got, err := Compile(w, &current)
		if err != nil || len(got.ProfileContracts) == 0 {
			t.Fatalf("obsolete inherited declaration blocked current compilation: %v", err)
		}
		if !reflect.DeepEqual(before, w.ContextHints) {
			t.Fatal("current compilation rewrote original inputs")
		}
	}
	// Only an exact inherited value can be replaced. An unknown explicit
	// project override must still be rejected by the current strict contract.
	hints = w.ContextHints.Map()
	for _, raw := range hints["profileContracts"].(map[string]any) {
		raw.(map[string]any)["retiredBuiltinProcessingRule"] = false
	}
	w.ContextHints = payload(t, hints)
	if _, err := Compile(w, &current); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unknown explicit override accepted: %v", err)
	}
}

func TestCompileRejectsLossyEvidencePolicyAndLegacyTargetInputs(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "ws"}
	template := &workspacemodel.WorkspaceTemplate{ID: "tpl", EvidencePolicies: []workorcheventpayload.Payload{{}}}
	if _, err := Compile(workspace, template); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing evidence policy ID error = %v", err)
	}
	template.EvidencePolicies = []workorcheventpayload.Payload{
		payload(t, map[string]any{"id": "same", "appliesTo": "shell_command", "rule": "test"}),
		payload(t, map[string]any{"id": "same", "appliesTo": "shell_command", "rule": "test"}),
	}
	if _, err := Compile(workspace, template); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("duplicate evidence policy ID error = %v", err)
	}
	template.EvidencePolicies = []workorcheventpayload.Payload{payload(t, map[string]any{"id": "legacy", "appliesTo": "shell_command", "prospectiveTargetRef": "target"})}
	if _, err := Compile(workspace, template); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("legacy evidence target error = %v", err)
	}
}

func payload(t *testing.T, value map[string]any) workorcheventpayload.Payload {
	t.Helper()
	result, err := workorcheventpayload.FromMap(value)
	if err != nil {
		t.Fatalf("create payload: %v", err)
	}
	return result
}

func limits(providerCalls, operations int) map[string]any {
	return map[string]any{"protocolRef": runtimecontract.RuntimeExecutionLimitsV1, "maxProviderCalls": providerCalls, "maxCapabilityOperations": operations}
}

func TestCapacityPreflightHonorsSealedPolicyBeforeReview(t *testing.T) {
	items := make([]workspacemodel.DefinitionCandidateItem, 67)
	for i := range items {
		items[i] = workspacemodel.DefinitionCandidateItem{Text: "independent required item"}
	}
	for _, limit := range []int{64, 128} {
		template := &workspacemodel.WorkspaceTemplate{ID: "capacity", ContextHints: payload(t, map[string]any{"acceptanceSlicePolicy": map[string]any{"maxFocuses": limit}, "profileContracts": map[string]any{"p": map[string]any{"deliverableExpectations": []any{map[string]any{"id": "source_change", "required": true}, map[string]any{"id": "validation_report", "required": true}, map[string]any{"id": "version_control_snapshot", "required": true}}}}})}
		_, err := CompileForItems(&workspacemodel.Workspace{ID: "ws"}, template, items)
		if (err == nil) != (limit == 128) {
			t.Fatalf("sealed limit %d: %v", limit, err)
		}
	}
}

func TestCurrentMethodRetiresInheritedInputRevisionAllowanceWithoutRewritingWorkspace(t *testing.T) {
	current, input, _ := templateProofFixture(t, 1)
	original := current.Clone()
	hints := original.ContextHints.Map()
	declarations := 0
	for _, raw := range hints["profileContracts"].(map[string]any) {
		for _, item := range raw.(map[string]any)["evidenceRequirements"].([]any) {
			if policy, ok := item.(map[string]any)["implementationWorkPolicy"].(map[string]any); ok {
				policy["maxInputRevisionsPerCWI"] = 4
				declarations++
			}
		}
	}
	if declarations == 0 {
		t.Fatal("fixture has no input revision declaration")
	}
	original.ContextHints = payload(t, hints)
	w := &workspacemodel.Workspace{ID: input.WorkspaceID, TemplateID: current.ID, ContextHints: original.ContextHints.Clone(), DefinitionAuthoring: &workspacemodel.DefinitionAuthoringRecord{Input: input, TemplateSnapshot: original}}
	before, _ := json.Marshal(w)
	for range 2 {
		compiled, err := Compile(w, &current)
		if err != nil {
			t.Fatal("inherited allowance blocked current method", err)
		}
		seen := 0
		for _, profile := range compiled.ProfileContracts {
			for _, requirement := range profile.EvidenceRequirements {
				if p := requirement.ImplementationWorkPolicy; p != nil {
					seen++
					if p.MaxInputRevisionsPerCWI != 0 || p.InputRevisionProtocolRef != runtimecontract.RuntimeValidationInputRevisionProtocolV1 {
						t.Fatal("current method retained limit or lost permission")
					}
				}
			}
		}
		if seen != declarations {
			t.Fatal("declaration lost during refresh")
		}
		after, _ := json.Marshal(w)
		if string(before) != string(after) {
			t.Fatal("compilation rewrote original owner/source image")
		}
	}
}
