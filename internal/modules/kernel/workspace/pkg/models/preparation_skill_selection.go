package workspace

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/skillselection"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const PreparationSkillSelectionVersion = "workorch-preparation-skill-selection-compilation/v1"

// These exact source values are copied by the private reducer from the owner
// confirmed journal after exact formal-row observations, never from a caller.
type PreparationSkillSelectionDefinition struct {
	WorkspaceID                                        common.WorkspaceID
	StartupInputHash, DefinitionRevision, SnapshotHash string
	GoalText, GoalTextHash, DoDItemSetHash             string
	DoDItems                                           []string
}

type PreparationSelectedSkill struct {
	Content                       workorchskillcontract.Content
	Provenance                    workorchskillcontract.Provenance
	SourceSkillID, ObservedStatus string
}

// A pure historical compilation, not an availability/freshness check or M3.
// Full source facts remain in their original archives. No success credential.
type PreparationSkillSelectionCompilation struct {
	Version                                                                       string
	Definition                                                                    PreparationSkillSelectionDefinition
	RegistryRef, SoulRef, SkillHubEffectRef, SoulContentHash, SkillHubContentHash string
	ExecutionSoulID                                                               common.SoulID
	AllowedSkillRefs                                                              []string
	Catalog                                                                       []skillselection.Candidate
	Selection                                                                     skillselection.TypedSelection
	Selected                                                                      []PreparationSelectedSkill
	FailureCode                                                                   string
	Context                                                                       *PreparationSkillContextCompilation `json:"Context,omitempty"`
	References                                                                    *PreparationSkillReferences         `json:",omitempty"`
	Status                                                                        *PreparationSkillStatusCompilation  `json:",omitempty"`
}

func (s PreparationSkillSelectionCompilation) Summary() string {
	if s.FailureCode == "" {
		return "compiled_checks_pending"
	}
	if s.FailureCode == "sources_incomplete" {
		return "sources_incomplete"
	}
	return "conflict"
}

func clonePreparationCandidate(c skillselection.Candidate) skillselection.Candidate {
	c.TriggerRules = slices.Clone(c.TriggerRules)
	return c
}

func (s PreparationSkillSelectionCompilation) Clone() PreparationSkillSelectionCompilation {
	if s.Status != nil {
		status := s.Status.Clone()
		s.Status = &status
	}
	if s.References != nil {
		r := s.References.Clone()
		s.References = &r
	}
	if s.Context != nil {
		c := s.Context.Clone()
		s.Context = &c
	}
	s.Definition.DoDItems = slices.Clone(s.Definition.DoDItems)
	s.AllowedSkillRefs = slices.Clone(s.AllowedSkillRefs)
	s.Catalog = slices.Clone(s.Catalog)
	for i := range s.Catalog {
		s.Catalog[i] = clonePreparationCandidate(s.Catalog[i])
	}
	s.Selection.Matches = slices.Clone(s.Selection.Matches)
	for i := range s.Selection.Matches {
		s.Selection.Matches[i].Candidate = clonePreparationCandidate(s.Selection.Matches[i].Candidate)
		s.Selection.Matches[i].Reasons = slices.Clone(s.Selection.Matches[i].Reasons)
	}
	s.Selected = slices.Clone(s.Selected)
	for i := range s.Selected {
		c := &s.Selected[i].Content
		c.TriggerRules = slices.Clone(c.TriggerRules)
		c.ReferenceRefs = slices.Clone(c.ReferenceRefs)
		c.RequiredToolKinds = slices.Clone(c.RequiredToolKinds)
		c.RequiredMCPServerRefs = slices.Clone(c.RequiredMCPServerRefs)
		c.RequiredCapabilityRefs = slices.Clone(c.RequiredCapabilityRefs)
		c.Constraints = slices.Clone(c.Constraints)
	}
	return s
}

// Compile from the entire exact historical dependency set. No I/O, bool input,
// hash-only trust, selected-only catalog or missing-source-to-empty fallback.
func CompilePreparationSkillSelection(d PreparationSkillSelectionDefinition, registry WorkspacePreparationSkillRegistryEvidence, souls *WorkspacePreparationSoulEvidence, effects *WorkspacePreparationSkillHubEffects) PreparationSkillSelectionCompilation {
	d.DoDItems = slices.Clone(d.DoDItems)
	s := PreparationSkillSelectionCompilation{Version: PreparationSkillSelectionVersion, Definition: d, RegistryRef: registry.Ref}
	fail := func(code string) PreparationSkillSelectionCompilation { s.FailureCode = code; return s }
	if souls != nil {
		s.SoulRef, s.SoulContentHash = souls.Ref, souls.ContentHash
	}
	if effects != nil {
		s.SkillHubEffectRef, s.SkillHubContentHash = effects.Ref, effects.ContentHash
	}
	if d.WorkspaceID == "" || !utf8.ValidString(d.GoalText) || StableDefinitionTextHash(d.GoalText) != d.GoalTextHash || StableDefinitionStringSetHash(d.DoDItems) != d.DoDItemSetHash || d.StartupInputHash == "" || d.DefinitionRevision == "" || d.SnapshotHash != registry.Facts.SnapshotHash || registry.Facts.Validate() != nil || registry.Facts.WorkspaceID != d.WorkspaceID {
		return fail("source_binding_conflict")
	}
	if souls == nil || effects == nil {
		return fail("sources_incomplete")
	}
	if souls.Validate() != nil || effects.Validate() != nil || souls.Facts.WorkspaceID != d.WorkspaceID || effects.WorkspaceID != d.WorkspaceID || souls.RequestID != registry.RequestID || effects.RequestID != registry.RequestID || souls.PayloadHash != registry.PayloadHash || effects.PayloadHash != registry.PayloadHash || souls.Facts.WorkspaceRevision != registry.Facts.WorkspaceRevision || effects.ExpectedWorkspaceRevision+1 != registry.Facts.WorkspaceRevision || souls.Facts.SnapshotHash != d.SnapshotHash || effects.Binding.StartupInputHash != d.StartupInputHash || effects.Binding.DefinitionRevision != d.DefinitionRevision {
		return fail("source_binding_conflict")
	}
	if effects.InstalledSummary() != "observed_selection_pending" {
		return fail("sources_incomplete")
	}
	if souls.Facts.ConfigurationHash != effects.Binding.AuthoringConfigurationHash {
		return fail("source_binding_conflict")
	}
	for _, selection := range souls.Facts.Selections {
		if selection.Stage == "execution" {
			s.ExecutionSoulID = selection.SoulID
		}
	}
	if s.ExecutionSoulID != "" {
		found := false
		for _, soul := range souls.Facts.Souls {
			if soul.ID == s.ExecutionSoulID {
				s.AllowedSkillRefs = slices.Clone(soul.AllowedSkillRefs)
				found = true
			}
		}
		if !found {
			return fail("execution_soul_missing")
		}
	}
	materials := make([]PreparationSelectedSkill, 0, len(registry.Facts.Catalog.Skills))
	for _, skill := range registry.Facts.Catalog.Skills {
		materials = append(materials, PreparationSelectedSkill{Content: skill.Content(), Provenance: skill.Provenance, SourceSkillID: string(skill.ID)})
	}
	first := func(values ...string) string {
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	for i, facts := range []*PreparationSkillHubStatusFacts{effects.Global, effects.Project} {
		dir, global := effects.ProjectDirectory, false
		if i == 0 {
			dir, global = "", true
		}
		status, err := skillhubpreparation.ParseStatus(facts.RawOutput, dir, global)
		if err != nil {
			return fail("source_binding_conflict")
		}
		index := 0
		for _, item := range status.Items {
			if global && item.Agent != "codex" {
				continue
			}
			file := facts.Installed.First[index]
			index++
			metadata, err := skillhubpreparation.ParsePreparationSkillHubMetadata(file.Raw)
			if err != nil {
				return fail("metadata_conflict")
			}
			id, scope, ws := "skillhub:global:"+item.SkillID, workorchskillcontract.ScopeGlobal, common.WorkspaceID("")
			if !global {
				id, scope, ws = "skillhub:workspace:"+string(d.WorkspaceID)+":"+item.SkillID, workorchskillcontract.ScopeWorkspace, d.WorkspaceID
			}
			content := workorchskillcontract.Content{ID: id, Name: first(metadata.Name, item.SkillID), Version: first(metadata.Version, item.LocalVersion, item.Version), Description: first(metadata.Description, item.Message), Instruction: string(file.Raw), TriggerRules: slices.Clone(metadata.TriggerRules), ReferenceRefs: slices.Clone(metadata.ReferenceRefs), RequiredToolKinds: slices.Clone(metadata.RequiredToolKinds), RequiredMCPServerRefs: slices.Clone(metadata.RequiredMCPServerRefs), RequiredCapabilityRefs: slices.Clone(metadata.RequiredCapabilityRefs), Constraints: slices.Clone(metadata.Constraints)}
			materials = append(materials, PreparationSelectedSkill{Content: content, Provenance: workorchskillcontract.Provenance{Source: workorchskillcontract.SourceSkillHub, Scope: scope, WorkspaceID: ws, ContentHash: workorchskillcontract.StableContentHash(content)}, SourceSkillID: item.SkillID, ObservedStatus: item.Status})
		}
	}
	s.Catalog = make([]skillselection.Candidate, 0, len(materials))
	for _, m := range materials {
		s.Catalog = append(s.Catalog, skillselection.Candidate{ID: m.Content.ID, Name: m.Content.Name, TriggerRules: slices.Clone(m.Content.TriggerRules), SourceSkillID: m.SourceSkillID, Scope: m.Provenance.Scope, Source: m.Provenance.Source})
	}
	var err error
	s.Selection, err = skillselection.AutoSelectTyped(s.Catalog, d.GoalText, d.DoDItems, s.AllowedSkillRefs)
	if err != nil {
		return fail("catalog_conflict")
	}
	byID := make(map[string]PreparationSelectedSkill, len(materials))
	for _, m := range materials {
		byID[m.Content.ID] = m
	}
	for _, match := range s.Selection.Matches {
		s.Selected = append(s.Selected, byID[match.Candidate.ID])
	}
	return s.Clone()
}

func validatePreparationSkillSelectionPersistence(w *Workspace) error {
	if w == nil {
		return common.ErrConflict
	}
	refs := map[string]WorkspacePreparationAttempt{}
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.SkillRegistryEvidenceRef != "" {
				refs[a.SkillRegistryEvidenceRef] = a
			}
		}
	}
	for _, registry := range w.PreparationSkillRegistryEvidence {
		a := refs[registry.Ref]
		if registry.Selection == nil {
			if a.SkillSelectionState != "" {
				return common.ErrConflict
			}
			continue
		}
		var souls *WorkspacePreparationSoulEvidence
		var effects *WorkspacePreparationSkillHubEffects
		for i := range w.PreparationSoulEvidence {
			if w.PreparationSoulEvidence[i].Ref == a.SoulEvidenceRef {
				souls = &w.PreparationSoulEvidence[i]
			}
		}
		for i := range w.PreparationSkillHubEffects {
			if w.PreparationSkillHubEffects[i].Ref == a.SkillHubEffectRef {
				effects = &w.PreparationSkillHubEffects[i]
			}
		}
		s := registry.Selection
		if s.Definition.StartupInputHash != a.Binding.StartupInputHash || s.Definition.DefinitionRevision != a.Binding.DefinitionRevision || s.Definition.WorkspaceID != w.ID || a.SkillSelectionState != s.Summary() {
			return common.ErrConflict
		}
		expected := CompilePreparationSkillSelection(s.Definition, registry, souls, effects)
		if s.Context != nil {
			if expected.FailureCode != "" || a.SkillContextState != s.Context.Summary() {
				return common.ErrConflict
			}
			context := CompilePreparationSkillContext(expected.Selected)
			expected.Context = &context
		} else if a.SkillContextState != "" {
			return common.ErrConflict
		}
		if s.References != nil {
			if expected.FailureCode != "" || a.SkillReferenceState != s.References.State {
				return common.ErrConflict
			}
			references := CompilePreparationSkillReferences(expected, effects)
			expected.References = &references
		} else if a.SkillReferenceState != "" {
			return common.ErrConflict
		}
		if s.Status != nil {
			if expected.FailureCode != "" || a.SkillStatusState != s.Status.State {
				return common.ErrConflict
			}
			status := CompilePreparationSkillStatus(expected, effects)
			expected.Status = &status
		} else if a.SkillStatusState != "" {
			return common.ErrConflict
		}
		if !reflect.DeepEqual(*s, expected) {
			return common.ErrConflict
		}
	}
	return nil
}
