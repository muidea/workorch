package definitionconfiguration

import (
	"fmt"
	"strings"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

const TemplateProofBindingsV1 = workspace.TemplateProofBindingsV1

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

func compileTemplateProofBindings(t *workspace.WorkspaceTemplate, hints, profiles map[string]any, items []workspace.DefinitionCandidateItem) (map[string]any, error) {
	return workspace.CompileTemplateProofProfiles(t, hints, profiles, items)
}
