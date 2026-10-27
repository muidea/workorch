package workspace

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const PreparationSkillReferencesVersion = "workorch-preparation-skill-reference-compilation/v1"

type PreparationSelectedSkillReference struct {
	SkillID, Ref, Root, Path, RawSHA256, State string
}

// Links to full original auxiliary reads, never hash-only proof, context input,
// status normalization, a currentness lease or preparation readiness.
type PreparationSkillReferences struct {
	Version, State string
	Entries        []PreparationSelectedSkillReference
}

func (r PreparationSkillReferences) Clone() PreparationSkillReferences {
	r.Entries = slices.Clone(r.Entries)
	return r
}

func CompilePreparationSkillReferences(s PreparationSkillSelectionCompilation, effects *WorkspacePreparationSkillHubEffects) PreparationSkillReferences {
	r := PreparationSkillReferences{Version: PreparationSkillReferencesVersion, State: "observed_checks_pending", Entries: []PreparationSelectedSkillReference{}}
	if s.FailureCode != "" || effects == nil || effects.Validate() != nil {
		r.State = "sources_incomplete"
		return r
	}
	rank := map[string]int{"observed_checks_pending": 0, "sources_incomplete": 1, "unavailable": 2, "source_unresolved": 3}
	for _, selected := range s.Selected {
		for _, ref := range selected.Content.ReferenceRefs {
			e := PreparationSelectedSkillReference{SkillID: selected.Content.ID, Ref: ref, State: "sources_incomplete"}
			if selected.Provenance.Source == workorchskillcontract.SourceWorkOrchRegistry {
				// Registry content has no owner-defined filesystem root. Never
				// guess the Workspace, process cwd or SkillHub counterpart.
				e.State = "source_unresolved"
			} else if selected.Provenance.Source == workorchskillcontract.SourceSkillHub {
				facts := effects.Global
				if selected.Provenance.Scope == workorchskillcontract.ScopeWorkspace {
					facts = effects.Project
				}
				if facts != nil && facts.Installed != nil {
					installed := *facts.Installed
					if aux := installed.References; aux != nil {
						e.State = "unavailable"
						if aux.CompleteFor(installed) {
							for _, original := range aux.First {
								if original.File.SkillID == selected.SourceSkillID && original.Ref == ref {
									e.Root, e.Path, e.RawSHA256, e.State = installed.Root, original.File.Path, original.File.RawSHA256, "observed_checks_pending"
									break
								}
							}
						}
					}
				}
			}
			if rank[e.State] > rank[r.State] {
				r.State = e.State
			}
			r.Entries = append(r.Entries, e)
		}
	}
	return r
}

func validPreparationSkillReferenceState(state string) bool {
	return state == "observed_checks_pending" || state == "sources_incomplete" || state == "source_unresolved" || state == "unavailable"
}
