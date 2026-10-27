package workspace

import (
	"encoding/json"
	"reflect"
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationMemoryInventoryEvidenceVersion = "workorch-preparation-memory-inventory-evidence/v1"
const PreparationMemoryInventoryRefPrefix = "memory-inventory-observation:"

// Complete local owner inventory at one observation point. Not a selection,
// mode change, semantic recall, continuous freshness or readiness proof.
type PreparationMemoryInventoryFacts struct {
	Version           string
	WorkspaceRevision uint64
	Observation       memorymodel.PreparationInventory
	SelectionSource   *PreparationMemorySelectionSource `json:"SelectionSource,omitempty"`
}

func (f PreparationMemoryInventoryFacts) Clone() PreparationMemoryInventoryFacts {
	f.Observation = f.Observation.Clone()
	if f.SelectionSource != nil {
		s := f.SelectionSource.Clone()
		f.SelectionSource = &s
	}
	return f
}

func (f PreparationMemoryInventoryFacts) Validate() error {
	if f.Version != PreparationMemoryInventoryEvidenceVersion || f.WorkspaceRevision == 0 || f.Observation.ValidateFor(f.Observation.WorkspaceID) != nil {
		return common.ErrConflict
	}
	if s := f.SelectionSource; s != nil && (s.Validate() != nil || s.WorkspaceID != f.Observation.WorkspaceID || s.WorkspaceRevision != f.WorkspaceRevision || f.Observation.ObservedAt.Before(s.StartedAt) || f.Observation.ObservedAt.After(s.CompletedAt)) {
		return common.ErrConflict
	}
	return nil
}

func (f PreparationMemoryInventoryFacts) BlockedReason() string {
	if f.Validate() != nil {
		return "memory_inventory_unavailable_not_saved"
	}
	return "memory_inventory_observed_selection_pending"
}

type WorkspacePreparationMemoryInventoryEvidence struct {
	Version, Ref           string
	RequestID, PayloadHash string
	Binding                WorkspacePreparationBinding
	Facts                  PreparationMemoryInventoryFacts
	ContentHash            string
	Selection              *PreparationMemorySelectionCompilation `json:"Selection,omitempty"`
}

func ValidPreparationMemoryInventoryRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationMemoryInventoryRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationMemoryInventoryRefPrefix), 16)
}

func (e WorkspacePreparationMemoryInventoryEvidence) Clone() WorkspacePreparationMemoryInventoryEvidence {
	e.Facts = e.Facts.Clone()
	if e.Selection != nil {
		s := e.Selection.Clone()
		e.Selection = &s
	}
	return e
}

func (e WorkspacePreparationMemoryInventoryEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationMemoryInventoryEvidence) Validate() error {
	if e.Version != PreparationMemoryInventoryEvidenceVersion || !ValidPreparationMemoryInventoryRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Binding.Validate() != nil || e.Facts.Validate() != nil || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	if e.Facts.SelectionSource == nil {
		if e.Selection != nil {
			return common.ErrConflict
		}
	} else {
		compiled := CompilePreparationMemorySelection(e.Facts)
		if e.Selection == nil || !reflect.DeepEqual(*e.Selection, compiled) {
			return common.ErrConflict
		}
	}
	return nil
}

func ClonePreparationMemoryInventoryEvidence(items []WorkspacePreparationMemoryInventoryEvidence) []WorkspacePreparationMemoryInventoryEvidence {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationMemoryInventoryEvidence, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}

func validatePreparationMemoryInventoryEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationMemoryInventoryEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.MemoryInventoryEvidenceRef != "" {
				if _, exists := refs[a.MemoryInventoryEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.MemoryInventoryEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationMemoryInventoryEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationMemoryInventoryEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || f.Observation.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || !reflect.DeepEqual(e.Binding, a.Binding) || f.Observation.ObservedAt.Before(a.StartedAt) || f.Observation.ObservedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		if s := f.SelectionSource; s != nil && (a.EvaluationVersion != PreparationCheckEvaluationVersion || s.SnapshotHash != a.ObservationSnapshotHash || s.StartedAt.Before(a.StartedAt) || s.CompletedAt.After(a.CompletedAt)) {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "memory_selection" {
				var evaluatedReason string
				if e.Selection != nil {
					_, evaluatedReason = e.Selection.CheckResult()
				}
				if c.Owner != "memory" || !c.Required || c.Reason != "binding_changed" && c.Reason != f.BlockedReason() && (evaluatedReason == "" || c.Reason != evaluatedReason) {
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

func validatePreparationMemoryInventoryEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationMemoryInventoryEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationMemoryInventoryEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationMemoryInventoryEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationMemoryInventoryEvidence, next.PreparationMemoryInventoryEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.MemoryInventoryEvidenceRef != "" || b.MemoryInventoryEvidenceRef != after[len(after)-1].Ref || a.ExpectedWorkspaceRevision+1 != after[len(after)-1].Facts.WorkspaceRevision {
		return common.ErrConflict
	}
	return nil
}
