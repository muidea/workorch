package workspace

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const PreparationSkillStatusVersion = "workorch-preparation-skill-status-compilation/v1"

type PreparationSelectedSkillStatus struct {
	SkillID, ObservedStatus, Reason string
}

// A diagnosis of the exact original selected sources, never a status rewrite,
// freshness lease, context grant or readiness verdict. Raw status and installed
// versions remain intact in the original private effect archive.
type PreparationSkillStatusCompilation struct {
	Version, State string
	Entries        []PreparationSelectedSkillStatus
}

func (s PreparationSkillStatusCompilation) Clone() PreparationSkillStatusCompilation {
	s.Entries = slices.Clone(s.Entries)
	return s
}

func validPreparationSkillStatusState(state string) bool {
	return state == "observed_freshness_pending" || state == "status_blocked" || state == "version_conflict" || state == "sources_incomplete"
}

func CompilePreparationSkillStatus(s PreparationSkillSelectionCompilation, effects *WorkspacePreparationSkillHubEffects) PreparationSkillStatusCompilation {
	c := PreparationSkillStatusCompilation{Version: PreparationSkillStatusVersion, State: "observed_freshness_pending", Entries: []PreparationSelectedSkillStatus{}}
	if s.Version != PreparationSkillSelectionVersion || s.FailureCode != "" || effects == nil || effects.Validate() != nil || effects.InstalledSummary() != "observed_selection_pending" || s.SkillHubEffectRef != effects.Ref || s.SkillHubContentHash != effects.ContentHash {
		c.State = "sources_incomplete"
		return c
	}
	rank := map[string]int{"observed_freshness_pending": 0, "sources_incomplete": 1, "status_blocked": 2, "version_conflict": 3}
	for _, selected := range s.Selected {
		e := PreparationSelectedSkillStatus{SkillID: selected.Content.ID, ObservedStatus: selected.ObservedStatus, Reason: "sources_incomplete"}
		state := "sources_incomplete"
		switch selected.Provenance.Source {
		case workorchskillcontract.SourceWorkOrchRegistry:
			if selected.ObservedStatus == "" {
				e.Reason, state = "registry_observed_freshness_pending", "observed_freshness_pending"
			}
		case workorchskillcontract.SourceSkillHub:
			facts, directory, global := effects.Global, "", true
			if selected.Provenance.Scope == workorchskillcontract.ScopeWorkspace && selected.Provenance.WorkspaceID == effects.WorkspaceID {
				facts, directory, global = effects.Project, effects.ProjectDirectory, false
			} else if selected.Provenance.Scope != workorchskillcontract.ScopeGlobal || selected.Provenance.WorkspaceID != "" {
				break
			}
			if facts == nil || facts.Installed == nil {
				break
			}
			status, err := skillhubpreparation.ParseStatus(facts.RawOutput, directory, global)
			if err != nil {
				break
			}
			for _, item := range status.Items {
				if item.SkillID != selected.SourceSkillID || global && item.Agent != "codex" || item.Status != selected.ObservedStatus {
					continue
				}
				for _, file := range facts.Installed.First {
					if file.SkillID != item.SkillID || string(file.Raw) != selected.Content.Instruction {
						continue
					}
					metadata, err := skillhubpreparation.ParsePreparationSkillHubMetadata(file.Raw)
					if err != nil {
						break
					}
					e.Reason, state = "status_not_synced", "status_blocked"
					// Treat status as an exact enum; no trimming, case folding or
					// converting unknown values into a successful observation.
					if item.Status == "synced" {
						e.Reason, state = "synced_observed_freshness_pending", "observed_freshness_pending"
					}
					if metadata.Version != "" && item.LocalVersion != "" && metadata.Version != item.LocalVersion {
						e.Reason, state = "installed_status_version_conflict", "version_conflict"
					}
					break
				}
				break
			}
		}
		if rank[state] > rank[c.State] {
			c.State = state
		}
		c.Entries = append(c.Entries, e)
	}
	return c
}
