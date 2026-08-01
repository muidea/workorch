package activationcore

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestMergeTemplateWorkspaceProfileContractPreservesWildcardAndAddsExactSemanticProof(t *testing.T) {
	template := map[string]any{
		"evidenceRequirements": []any{
			map[string]any{"scope": "dodItem", "id": "*", "validationRecipeId": "quality-gate", "requiredCheckRefs": []any{"quality-check"}},
		},
	}
	workspace := map[string]any{
		"evidenceRequirements": []any{
			map[string]any{"scope": "dodItem", "id": "dod-001", "targetRefs": []any{"cmd/lsp/main.go"}, "validationRecipeId": "lsp-startup", "requiredCheckRefs": []any{"lsp-startup-check"}},
		},
	}

	merged := MergeTemplateWorkspaceProfileContract(template, workspace)
	requirements := contextMapSliceValue(merged["evidenceRequirements"])
	if len(requirements) != 2 {
		t.Fatalf("merged evidence requirements = %#v, want wildcard plus exact declaration", requirements)
	}
	byID := map[string]map[string]any{}
	for _, requirement := range requirements {
		byID[requirement["id"].(string)] = requirement
	}
	if got := byID["*"]["validationRecipeId"]; got != "quality-gate" {
		t.Fatalf("wildcard quality gate = %v", got)
	}
	if got := byID["dod-001"]["validationRecipeId"]; got != "lsp-startup" {
		t.Fatalf("exact semantic recipe = %v", got)
	}
}

func TestMergeTemplateWorkspaceProfileContractWorkspaceOverridesSameEvidenceIdentity(t *testing.T) {
	template := map[string]any{
		"evidenceRequirements": []any{
			map[string]any{"scope": "dodItem", "id": "dod-001", "validationRecipeId": "old", "requiredCheckRefs": []any{"old-check"}},
		},
	}
	workspace := map[string]any{
		"evidenceRequirements": []any{
			map[string]any{"scope": "dodItem", "id": "dod-001", "validationRecipeId": "new", "requiredCheckRefs": []any{"new-check"}},
		},
	}

	requirements := contextMapSliceValue(MergeTemplateWorkspaceProfileContract(template, workspace)["evidenceRequirements"])
	if len(requirements) != 1 || requirements[0]["validationRecipeId"] != "new" {
		t.Fatalf("workspace exact declaration did not replace template declaration: %#v", requirements)
	}
}

func TestMergeTemplateWorkspaceEvidencePoliciesPreservesAndReplacesByID(t *testing.T) {
	policy := func(id, command string) workorcheventpayload.Payload {
		item, err := workorcheventpayload.FromMap(map[string]any{"id": id, "commandName": command})
		if err != nil {
			t.Fatalf("policy %q: %v", id, err)
		}
		return item
	}
	merged := MergeTemplateWorkspaceEvidencePolicies(
		[]workorcheventpayload.Payload{policy("baseline", "old"), policy("keep", "keep")},
		[]workorcheventpayload.Payload{policy("baseline", "new"), policy("workspace", "workspace")},
	)
	if len(merged) != 3 {
		t.Fatalf("merged policies = %#v, want three", merged)
	}
	byID := map[string]string{}
	for _, item := range merged {
		id, _ := item.String("id")
		command, _ := item.String("commandName")
		byID[id] = command
	}
	if byID["baseline"] != "new" || byID["keep"] != "keep" || byID["workspace"] != "workspace" {
		t.Fatalf("merged policy values = %#v", byID)
	}
}
