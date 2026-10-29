package definitionconfiguration

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const TemplateProofBindingsV1 = "workorch.template-source-proof/v1"

// CompileAuthoring uses the same pure source resolver as confirmation and
// activation. It neither invents a verification declaration nor grants credit.
func CompileAuthoring(d workspace.DefinitionDerivationRequest) (Bindings, error) {
	if d.Proposal == nil {
		return Bindings{}, fmt.Errorf("profile source requires completed derivation")
	}
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		return Bindings{}, err
	}
	return CompileForItems(&workspace.Workspace{DefinitionAuthoring: &workspace.DefinitionAuthoringRecord{Input: d.Input}}, &d.Template, items)
}
func configurationDoDItems(w *workspace.Workspace, t *workspace.WorkspaceTemplate) ([]workspace.DefinitionCandidateItem, error) {
	if w == nil || t == nil {
		return nil, fmt.Errorf("configuration requires workspace and template")
	}
	if w.DefinitionAuthoring != nil {
		r := w.DefinitionAuthoring
		// Configuration compilation also runs before derivation. Template ordinals
		// depend only on the exact input's user items, never generated tail items.
		p := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: r.Input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(t), AdditionalItems: []string{}}
		return p.CandidateItems(r.Input, t)
	}
	if w.StartupInput != nil || w.ActivationAdmission != nil || len(w.DefinitionAdmissionCandidates) != 0 {
		if _, usesSourceProof := t.ContextHints.Map()["templateProofBindings"]; usesSourceProof {
			return nil, fmt.Errorf("template proof requires original authoring source inventory")
		}
	}
	// Template-only configuration preview, without an authoring/admission source.
	// Production admission must use CompileForItems with its actual source inventory.
	items := []workspace.DefinitionCandidateItem{}
	for i, text := range t.DoDItems {
		items = append(items, workspace.DefinitionCandidateItem{CandidateItemRef: fmt.Sprintf("derived-dod:template:%06d", i+1), Text: strings.TrimSpace(text), SourceIdentity: workspace.WorkspaceTemplateSnapshotHash(t)})
	}
	return items, nil
}

func compileTemplateProofBindings(t *workspace.WorkspaceTemplate, workspaceHints, profiles map[string]any, items []workspace.DefinitionCandidateItem) (map[string]any, error) {
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
	sources, ok := definitionadmission.SpecificationDoDSources(items)
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
