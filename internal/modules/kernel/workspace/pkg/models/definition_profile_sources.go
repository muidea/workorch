package workspace

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const TemplateProofBindingsV1 = "workorch.template-source-proof/v1"

// AuthoringProfileContracts compiles from the immutable derivation, never model metadata.
func AuthoringProfileContracts(d DefinitionDerivationRequest) ([]runtimecontract.RuntimeProfileContract, error) {
	if d.Proposal == nil {
		return nil, fmt.Errorf("profile source requires completed derivation")
	}
	hints := d.Template.ContextHints.Map()
	if err := runtimeprofiledecode.ValidateInputShape(hints); err != nil {
		return nil, err
	}
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		return nil, err
	}
	profiles, err := CompileTemplateProofProfiles(&d.Template, nil, runtimeprofiledecode.Map(hints["profileContracts"]), items)
	if err != nil {
		return nil, err
	}
	return runtimeprofiledecode.Decode(profiles), nil
}
func CompileTemplateProofProfiles(t *WorkspaceTemplate, workspaceHints, profiles map[string]any, items []DefinitionCandidateItem) (map[string]any, error) {
	raw, present := t.ContextHints.Map()["templateProofBindings"]
	if other, exists := workspaceHints["templateProofBindings"]; exists && (!present || !reflect.DeepEqual(other, raw)) {
		return nil, fmt.Errorf("Workspace cannot replace template proof source contract")
	}
	if !present {
		return profiles, nil
	}
	binding, ok := raw.(map[string]any)
	if !ok || len(binding) != 2 || binding["protocolRef"] != TemplateProofBindingsV1 {
		return nil, fmt.Errorf("unsupported template proof binding contract")
	}
	entries, ok := binding["bindings"].([]any)
	if !ok || len(entries) == 0 || len(entries) > 32 {
		return nil, fmt.Errorf("invalid template proof binding inventory")
	}
	sources, ok := SpecificationDoDSources(items)
	if !ok {
		return nil, fmt.Errorf("template proof source inventory is incomplete")
	}
	// Clone the entire raw Profile tree: no mutation of configuration or snapshots.
	bytes, err := json.Marshal(profiles)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		b, ok := entry.(map[string]any)
		if !ok || len(b) != 4 {
			return nil, fmt.Errorf("invalid exact template proof binding")
		}
		for _, key := range []string{"candidateItemRef", "text", "profileId", "lineageAssertions"} {
			if _, ok = b[key]; !ok {
				return nil, fmt.Errorf("missing template proof binding field %s", key)
			}
		}
		ref, ok := b["candidateItemRef"].(string)
		if !ok || !strings.HasPrefix(ref, "derived-dod:template:") || seen[ref] {
			return nil, fmt.Errorf("ambiguous template proof source")
		}
		seen[ref] = true
		text, ok := b["text"].(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("missing original template proof text")
		}
		found := false
		for i, original := range t.DoDItems {
			if ref == fmt.Sprintf("derived-dod:template:%06d", i+1) && text == original {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("template proof source text or order drift")
		}
		id := ""
		count := 0
		for _, item := range items {
			if item.CandidateItemRef == ref {
				if item.Text != text {
					return nil, fmt.Errorf("template proof sealed source drift")
				}
				id = sources[ref]
				count++
			}
		}
		if count != 1 || id == "" {
			return nil, fmt.Errorf("missing exact template proof source")
		}
		pid, ok := b["profileId"].(string)
		if !ok || pid == "" {
			return nil, fmt.Errorf("missing template proof Profile")
		}
		profile, ok := result[pid].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing template proof Profile")
		}
		requirements, ok := profile["evidenceRequirements"].([]any)
		if !ok {
			return nil, fmt.Errorf("missing ordinary validation declaration")
		}
		var ordinary map[string]any
		for _, v := range requirements {
			r, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Profile requirement")
			}
			if r["scope"] == "dodItem" && r["id"] == id {
				return nil, fmt.Errorf("exact template proof declaration conflicts")
			}
			if r["scope"] == "dodItem" && r["id"] == "*" {
				if ordinary != nil {
					return nil, fmt.Errorf("ambiguous ordinary validation declaration")
				}
				ordinary = r
			}
		}
		if ordinary == nil || len(runtimeprofiledecode.Strings(ordinary["requiredEvidenceKinds"])) != 1 || runtimeprofiledecode.Strings(ordinary["requiredEvidenceKinds"])[0] != "validation" {
			return nil, fmt.Errorf("template proof requires independent ordinary validation")
		}
		ruleRaw, ok := b["lineageAssertions"].([]any)
		if !ok || len(ruleRaw) == 0 {
			return nil, fmt.Errorf("template proof lacks consumers")
		}
		duplicate := map[string]any{"id": id, "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": ordinary["acceptedStepKinds"], "lineageAssertions": ruleRaw}

		profile["evidenceRequirements"] = append(requirements, duplicate)
	}
	if err := runtimeprofiledecode.ValidateInputShape(map[string]any{"profileContracts": result}); err != nil {
		return nil, err
	}
	for _, p := range runtimeprofiledecode.Decode(result) {
		for _, r := range p.EvidenceRequirements {
			if err := runtimecontract.ValidateRuntimeLineageAssertionRules(r.LineageAssertions); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
