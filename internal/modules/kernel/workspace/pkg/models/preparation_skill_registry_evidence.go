package workspace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

const PreparationSkillRegistryEvidenceVersion = "workorch-preparation-skill-registry-evidence/v1"
const PreparationSkillRegistryRefPrefix = "skill-registry-observation:"

// Complete registry history only, not the combined SkillHub catalog, selected
// instruction proof, currentness, readiness or M3. No caller success flag.
type PreparationSkillCatalogFacts struct {
	Version                              string
	WorkspaceID                          common.WorkspaceID
	WorkspaceRevision                    uint64
	SnapshotHash, ReobservedSnapshotHash string
	Catalog, ReobservedCatalog           skillmodel.PreparationRegistryCatalog
	StartedAt, CompletedAt               time.Time
}

func (f PreparationSkillCatalogFacts) Clone() PreparationSkillCatalogFacts {
	f.Catalog = f.Catalog.Clone()
	f.ReobservedCatalog = f.ReobservedCatalog.Clone()
	return f
}
func (f PreparationSkillCatalogFacts) Validate() error {
	if f.Version != "workorch-preparation-skill-registry-facts/v1" || !boundedPreparationIdentity(string(f.WorkspaceID)) || f.WorkspaceRevision == 0 || !boundedPreparationIdentity(f.SnapshotHash) || f.ReobservedSnapshotHash != f.SnapshotHash || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || f.Catalog.ValidateFor(f.WorkspaceID) != nil || f.ReobservedCatalog.ValidateFor(f.WorkspaceID) != nil || f.Catalog.ObservedAt.Before(f.StartedAt) || f.ReobservedCatalog.ObservedAt.Before(f.Catalog.ObservedAt) || f.CompletedAt.Before(f.ReobservedCatalog.ObservedAt) || f.Catalog.OwnerEpoch != f.ReobservedCatalog.OwnerEpoch || !reflect.DeepEqual(f.Catalog.Skills, f.ReobservedCatalog.Skills) {
		return common.ErrConflict
	}
	return nil
}

type WorkspacePreparationSkillRegistryEvidence struct {
	Version, Ref           string
	RequestID, PayloadHash string
	Facts                  PreparationSkillCatalogFacts
	ContentHash            string
	Selection              *PreparationSkillSelectionCompilation `json:"Selection,omitempty"`
}

func ValidPreparationSkillRegistryRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationSkillRegistryRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationSkillRegistryRefPrefix), 16)
}

func (e WorkspacePreparationSkillRegistryEvidence) Clone() WorkspacePreparationSkillRegistryEvidence {
	e.Facts = e.Facts.Clone()
	if e.Selection != nil {
		s := e.Selection.Clone()
		e.Selection = &s
	}
	return e
}
func (e WorkspacePreparationSkillRegistryEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}
func (e WorkspacePreparationSkillRegistryEvidence) Validate() error {
	if e.Selection != nil && e.Selection.Status != nil && (e.Selection.Status.Version != PreparationSkillStatusVersion || !validPreparationSkillStatusState(e.Selection.Status.State) || len(e.Selection.Status.Entries) > 5) {
		return common.ErrConflict
	}
	if e.Selection != nil && e.Selection.References != nil && (e.Selection.References.Version != PreparationSkillReferencesVersion || !validPreparationSkillReferenceState(e.Selection.References.State) || len(e.Selection.References.Entries) > 5*skillhubpreparation.ReferenceCountLimit) {
		return common.ErrConflict
	}
	if e.Selection != nil && (e.Selection.Version != PreparationSkillSelectionVersion || len(e.Selection.Catalog) > 3*1024 || len(e.Selection.Selected) > 5 || len(e.Selection.Selection.Matches) > 5) {
		return common.ErrConflict
	}
	if e.Selection != nil && e.Selection.Context != nil && (e.Selection.Context.Version != PreparationSkillContextVersion || len(e.Selection.Context.Projection) > 5 || len(e.Selection.Context.Diagnostic) > PreparationSkillHubErrorLimit) {
		return common.ErrConflict
	}
	if e.Version != PreparationSkillRegistryEvidenceVersion || !ValidPreparationSkillRegistryRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Facts.Validate() != nil || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	return nil
}
func ClonePreparationSkillRegistryEvidence(items []WorkspacePreparationSkillRegistryEvidence) []WorkspacePreparationSkillRegistryEvidence {
	if items == nil {
		return nil
	}
	result := make([]WorkspacePreparationSkillRegistryEvidence, len(items))
	for i := range items {
		result[i] = items[i].Clone()
	}
	return result
}

func validatePreparationSkillRegistryEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationSkillRegistryEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.SkillRegistryEvidenceRef != "" {
				if _, exists := refs[a.SkillRegistryEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.SkillRegistryEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationSkillRegistryEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationSkillRegistryEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || f.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "skill_selection" {
				if c.Owner != "skill" || !c.Required || c.Reason != "binding_changed" && c.Reason != "skill_registry_observed_selection_pending" && c.Reason != "skill_selection_"+a.SkillSelectionState && !(c.State == "passed" && c.Reason == PreparationEmptySkillReason && ValidPreparationEvaluatedCheck(a, c)) {
					return common.ErrConflict
				}
				found = true
			}
		}
		if !found {
			return common.ErrConflict
		}
		delete(refs, e.Ref)
	}
	return nil
}

func validatePreparationSkillRegistryEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationSkillRegistryEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationSkillRegistryEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationSkillRegistryEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationSkillRegistryEvidence, next.PreparationSkillRegistryEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.SkillRegistryEvidenceRef != "" || b.SkillRegistryEvidenceRef != after[len(after)-1].Ref {
		return common.ErrConflict
	}
	if compiled := after[len(after)-1].Selection; compiled != nil {
		// New history must originate in the actual owner-confirmed preimage,
		// not self-consistent caller text with newly computed hashes. Later
		// definition changes cannot rewrite this append-only historical image.
		if current.StartupInput == nil || current.DefinitionAuthoringConfirmation == nil {
			return common.ErrConflict
		}
		input, journal := current.StartupInput, current.DefinitionAuthoringConfirmation
		d := compiled.Definition
		if input.StartupInputHash != a.Binding.StartupInputHash || d.StartupInputHash != input.StartupInputHash || d.DefinitionRevision != input.DefinitionRevision || d.GoalTextHash != input.GoalTextHash || d.DoDItemSetHash != input.DoDItemSetHash || d.GoalText != journal.Goal.Text || !reflect.DeepEqual(d.DoDItems, journal.DoD.Items) {
			return common.ErrConflict
		}
	}
	return nil
}
