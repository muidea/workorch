package activationcore

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestProfileMergePreservesInvalidSourceIdentitiesForAdmission(t *testing.T) {
	for _, key := range []string{"acceptanceDimensions", "deliverableExpectations", "evidenceRequirements", "coverageDependencies", "bootstrapRecipes"} {
		for _, duplicateLayer := range []string{"template", "workspace"} {
			t.Run(key+"/"+duplicateLayer, func(t *testing.T) {
				item := map[string]any{"id": "entry", "scope": "dodItem", "required": true}
				template, workspace := map[string]any{key: []map[string]any{item}}, map[string]any{key: []map[string]any{item}}
				invalid := []map[string]any{item, {"id": "entry", "scope": "dodItem", "required": false}, {"id": " "}}
				if duplicateLayer == "template" {
					template[key] = invalid
				} else {
					workspace[key] = invalid
				}
				beforeTemplate, beforeWorkspace := cloneContextMapDeep(template), cloneContextMapDeep(workspace)
				merged := contextMapSliceValue(MergeTemplateWorkspaceProfileContract(template, workspace)[key])
				if len(merged) != 4 {
					t.Fatalf("invalid source declarations collapsed: %#v", merged)
				}
				merged[0]["id"] = "changed"
				if !reflect.DeepEqual(template, beforeTemplate) || !reflect.DeepEqual(workspace, beforeWorkspace) {
					t.Fatal("merge aliased or modified caller policy")
				}
			})
		}
	}
}

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
