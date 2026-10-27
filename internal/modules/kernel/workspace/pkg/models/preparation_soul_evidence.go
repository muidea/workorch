package workspace

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
)

const PreparationSoulEvidenceVersion = "workorch-preparation-soul-evidence/v1"
const PreparationSoulRefPrefix = "soul-observation:"

type PreparationSoulSelection struct {
	Stage, Source string
	SoulID        common.SoulID
}

// PreparationSoulFacts archives complete before/after owner values. It has no
// PASS, currentness or readiness flag. The application observes process config;
// Workspace independently reconstructs selection from its exact sealed inputs.
type PreparationSoulFacts struct {
	Version                                     string
	WorkspaceID                                 common.WorkspaceID
	WorkspaceRevision                           uint64
	SnapshotHash, ConfigurationHash             string
	SystemDefaultSoulID, WorkspaceDefaultSoulID common.SoulID
	WorkspaceBindings                           []soulbinding.Binding
	DefaultLookupSoulID                         common.SoulID
	Selections                                  []PreparationSoulSelection
	Souls, ReobservedSouls                      []*soulmodel.Soul
	ReobservedSystemDefaultSoulID               common.SoulID
	ReobservedSnapshotHash                      string
	StartedAt, CompletedAt                      time.Time
}

func PreparationSoulPlan(workspaceDefault common.SoulID, bindings []soulbinding.Binding, systemDefault common.SoulID) ([]PreparationSoulSelection, common.SoulID, error) {
	stages := []string{"planning", "execution", "decision", "approval", "validation", "artifact"}
	if workspaceDefault != "" && !soulmodel.ValidPreparationSoulID(workspaceDefault) || systemDefault != "" && !soulmodel.ValidPreparationSoulID(systemDefault) || len(bindings) > len(stages) {
		return nil, "", common.ErrConflict
	}
	byStage := make(map[string]common.SoulID)
	for _, binding := range bindings {
		if !slices.Contains(stages, binding.Stage) || !soulmodel.ValidPreparationSoulID(binding.SoulID) || byStage[binding.Stage] != "" {
			return nil, "", common.ErrConflict
		}
		byStage[binding.Stage] = binding.SoulID
	}
	defaultID, source := workspaceDefault, "workspace_default"
	if defaultID == "" {
		defaultID, source = systemDefault, "system_default"
	}
	lookup := defaultID
	if workspaceDefault == "" && len(bindings) > 0 {
		lookup = ""
	}
	plan := make([]PreparationSoulSelection, 0, len(stages))
	for _, stage := range stages {
		id, origin := defaultID, source
		if byStage[stage] != "" {
			id, origin = byStage[stage], "workspace_stage_binding"
		}
		if id == "" {
			origin = "unbound"
		}
		plan = append(plan, PreparationSoulSelection{Stage: stage, Source: origin, SoulID: id})
	}
	return plan, lookup, nil
}

func (f PreparationSoulFacts) Clone() PreparationSoulFacts {
	f.WorkspaceBindings = slices.Clone(f.WorkspaceBindings)
	f.Selections = slices.Clone(f.Selections)
	f.Souls = soulmodel.CloneSouls(f.Souls)
	f.ReobservedSouls = soulmodel.CloneSouls(f.ReobservedSouls)
	return f
}

func (f PreparationSoulFacts) Validate() error {
	plan, lookup, err := PreparationSoulPlan(f.WorkspaceDefaultSoulID, f.WorkspaceBindings, f.SystemDefaultSoulID)
	if err != nil || f.Version != "workorch-preparation-souls/v1" || !boundedPreparationIdentity(string(f.WorkspaceID)) || f.WorkspaceRevision == 0 || !boundedPreparationIdentity(f.SnapshotHash) || !boundedPreparationIdentity(f.ConfigurationHash) || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || f.ReobservedSystemDefaultSoulID != f.SystemDefaultSoulID || f.ReobservedSnapshotHash != f.SnapshotHash || lookup != f.DefaultLookupSoulID || !reflect.DeepEqual(plan, f.Selections) {
		return common.ErrConflict
	}
	ids := []common.SoulID{}
	add := func(id common.SoulID) {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	add(lookup)
	for _, s := range plan {
		add(s.SoulID)
	}
	if len(f.Souls) != len(ids) || len(f.ReobservedSouls) != len(ids) {
		return common.ErrConflict
	}
	for i, id := range ids {
		if !soulmodel.ValidPreparationSnapshot(f.Souls[i], id) || !soulmodel.ValidPreparationSnapshot(f.ReobservedSouls[i], id) || !reflect.DeepEqual(f.Souls[i], f.ReobservedSouls[i]) {
			return common.ErrConflict
		}
	}
	return nil
}

type WorkspacePreparationSoulEvidence struct {
	Version, Ref           string
	RequestID, PayloadHash string
	Facts                  PreparationSoulFacts
	ContentHash            string
}

func ValidPreparationSoulRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationSoulRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationSoulRefPrefix), 16)
}

func (e WorkspacePreparationSoulEvidence) Clone() WorkspacePreparationSoulEvidence {
	e.Facts = e.Facts.Clone()
	return e
}
func (e WorkspacePreparationSoulEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}
func (e WorkspacePreparationSoulEvidence) Validate() error {
	if e.Version != PreparationSoulEvidenceVersion || !ValidPreparationSoulRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Facts.Validate() != nil || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	return nil
}
func ClonePreparationSoulEvidence(items []WorkspacePreparationSoulEvidence) []WorkspacePreparationSoulEvidence {
	if items == nil {
		return nil
	}
	result := make([]WorkspacePreparationSoulEvidence, len(items))
	for i := range items {
		result[i] = items[i].Clone()
	}
	return result
}

func validatePreparationSoulEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationSoulEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.SoulEvidenceRef != "" {
				if _, exists := refs[a.SoulEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.SoulEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationSoulEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationSoulEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || f.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || f.ConfigurationHash != a.Binding.AuthoringConfigurationHash || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "soul_selection" {
				if c.Owner != "soul" || !c.Required || c.Reason != "binding_changed" && c.Reason != "souls_observed_remaining_checks" {
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

func validatePreparationSoulEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationSoulEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationSoulEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationSoulEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationSoulEvidence, next.PreparationSoulEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.SoulEvidenceRef != "" || b.SoulEvidenceRef != after[len(after)-1].Ref {
		return common.ErrConflict
	}
	return nil
}
