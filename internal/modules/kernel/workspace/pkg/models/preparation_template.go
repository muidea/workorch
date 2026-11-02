package workspace

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// PreparationTemplate is the owner-selected declaration for one check. It is
// typed for EventHub, immutable after acceptance, and never a project override.
type PreparationTemplate struct {
	ID, Name, Description, GoalText, DefaultSoulID string
	ParentTemplateIDs                              []string
	EffectiveSources                               []TemplateSource
	DoDItems                                       []string
	SoulBindings                                   []soulbinding.Binding
	ContextHints                                   workorcheventpayload.Payload
	EvidencePolicies                               []workorcheventpayload.Payload
	SnapshotHash                                   string
}

func NewPreparationTemplate(t WorkspaceTemplate) PreparationTemplate {
	t = t.Clone()
	return PreparationTemplate{ID: t.ID, Name: t.Name, Description: t.Description, GoalText: t.GoalText, DefaultSoulID: t.DefaultSoulID, ParentTemplateIDs: t.ParentTemplateIDs, EffectiveSources: t.EffectiveSources, DoDItems: t.DoDItems, SoulBindings: soulbinding.FromStringMap(t.SoulBindings), ContextHints: t.ContextHints, EvidencePolicies: t.EvidencePolicies, SnapshotHash: WorkspaceTemplateSnapshotHash(&t)}
}
func (d PreparationTemplate) Template() WorkspaceTemplate {
	t := WorkspaceTemplate{ID: d.ID, Name: d.Name, Description: d.Description, GoalText: d.GoalText, DefaultSoulID: d.DefaultSoulID, ParentTemplateIDs: d.ParentTemplateIDs, EffectiveSources: d.EffectiveSources, DoDItems: d.DoDItems, SoulBindings: soulbinding.StringMap(d.SoulBindings), ContextHints: d.ContextHints, EvidencePolicies: d.EvidencePolicies}
	return t.Clone()
}
func (d PreparationTemplate) Validate() error {
	t := d.Template()
	if d.ID == "" || soulbinding.Validate(d.SoulBindings) != nil || d.SnapshotHash != WorkspaceTemplateSnapshotHash(&t) {
		return common.ErrConflict
	}
	return nil
}
func (d PreparationTemplate) Clone() PreparationTemplate {
	d.ParentTemplateIDs = slices.Clone(d.ParentTemplateIDs)
	d.EffectiveSources = slices.Clone(d.EffectiveSources)
	d.DoDItems = slices.Clone(d.DoDItems)
	d.SoulBindings = slices.Clone(d.SoulBindings)
	d.ContextHints = d.ContextHints.Clone()
	d.EvidencePolicies = workorcheventpayload.CloneSlice(d.EvidencePolicies)
	return d
}

// PreparationWorkspace uses only the declaration already selected for this
// operation. It neither fetches defaults nor rewrites the business record.
func PreparationWorkspace(w *Workspace, a WorkspacePreparationAttempt) (*Workspace, error) {
	t, err := PreparationTemplateForAttempt(w, a)
	if err != nil {
		return nil, err
	}
	return WorkspaceForCurrentTemplate(w, &t)
}

// Historical checks without a separate selection used the confirmed source.
// Replaying that source never fetches today's catalog or grants a new check.
// New operation acceptance independently requires an ExecutionTemplate.
func PreparationTemplateForAttempt(w *Workspace, a WorkspacePreparationAttempt) (WorkspaceTemplate, error) {
	if w == nil || w.DefinitionAuthoring == nil {
		return WorkspaceTemplate{}, common.ErrConflict
	}
	if selected := a.ExecutionTemplate; selected != nil {
		if selected.Validate() != nil || selected.ID != w.TemplateID {
			return WorkspaceTemplate{}, common.ErrConflict
		}
		return selected.Template(), nil
	}
	return OriginalPreparationTemplate(w, a)
}

// OriginalPreparationTemplate follows the exact confirmed candidate epoch.
// Neither a newer authoring record nor today's catalog can supply an old check.
func OriginalPreparationTemplate(w *Workspace, a WorkspacePreparationAttempt) (WorkspaceTemplate, error) {
	if w == nil {
		return WorkspaceTemplate{}, common.ErrConflict
	}
	var source *WorkspaceTemplate
	templateRef := ""
	for index, c := range w.DefinitionAdmissionCandidates {
		if c.CandidateHash != a.Binding.CandidateHash || c.CandidateID != a.Binding.CandidateID || c.CandidateRevision != a.Binding.CandidateRevision {
			continue
		}
		if templateRef != "" {
			return WorkspaceTemplate{}, common.ErrConflict
		}
		templateRef = c.WorkspaceTemplateSnapshot.TemplateRef
		if index < len(w.DefinitionAuthoringHistory) {
			source = &w.DefinitionAuthoringHistory[index].Record.TemplateSnapshot
		} else if index == len(w.DefinitionAuthoringHistory) && w.DefinitionAuthoring != nil {
			source = &w.DefinitionAuthoring.TemplateSnapshot
		}
	}
	if source == nil || source.ID != templateRef {
		return WorkspaceTemplate{}, common.ErrConflict
	}
	return source.Clone(), nil
}
