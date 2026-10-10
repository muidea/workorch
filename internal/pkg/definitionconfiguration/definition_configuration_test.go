package definitionconfiguration

import (
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
