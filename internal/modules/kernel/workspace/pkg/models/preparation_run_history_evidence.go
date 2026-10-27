package workspace

import (
	"encoding/json"
	"reflect"
	"strings"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationRunHistoryEvidenceVersion = "workorch-preparation-run-history-evidence/v1"
const PreparationRunHistoryRefPrefix = "run-history-observation:"

// The complete ordered Run-owner result, not a visibility-filtered Run list or
// a no-Run permit. Empty history cannot discharge the final reservation/CAS gate.
type PreparationRunHistoryFacts struct {
	Version           string
	WorkspaceRevision uint64
	Observation       runevents.ObserveWorkspacePreparationHistoryResult
}

func (f PreparationRunHistoryFacts) Clone() PreparationRunHistoryFacts {
	f.Observation = f.Observation.Clone()
	return f
}

func (f PreparationRunHistoryFacts) Validate() error {
	c := runevents.ObserveWorkspacePreparationHistoryCommand{WorkspaceID: f.Observation.History.WorkspaceID, RequestID: f.Observation.RequestID}
	if f.Version != PreparationRunHistoryEvidenceVersion || f.WorkspaceRevision == 0 || f.Observation.ValidateFor(c) != nil {
		return common.ErrConflict
	}
	if r := f.Observation.History.Retirement; r != nil {
		for _, value := range []string{r.DeletionID, r.WorkspaceIntentHash, r.QuiescenceID, r.RuntimeInventoryHash} {
			if value != "" && !boundedPreparationIdentity(value) {
				return common.ErrConflict
			}
		}
	}
	return nil
}

func (f PreparationRunHistoryFacts) BlockedReason() string {
	if f.Validate() != nil {
		return "run_history_unavailable"
	}
	h := f.Observation.History
	if h.Retirement != nil {
		return "run_history_retired"
	}
	if h.GenerationHead != 0 || h.RunCount != 0 || h.PendingActivationCount != 0 || h.ReplacementCount != 0 {
		return "run_history_present"
	}
	return "run_history_empty_fence_not_final"
}

type WorkspacePreparationRunHistoryEvidence struct {
	Version, Ref           string
	RequestID, PayloadHash string
	Binding                WorkspacePreparationBinding
	CreationEpoch          string
	PreparationEpoch       uint64
	Facts                  PreparationRunHistoryFacts
	ContentHash            string
	FinalFence             *PreparationNoRunFinalFence `json:",omitempty"`
}

func ValidPreparationRunHistoryRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationRunHistoryRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationRunHistoryRefPrefix), 16)
}

func (e WorkspacePreparationRunHistoryEvidence) Clone() WorkspacePreparationRunHistoryEvidence {
	e.Facts = e.Facts.Clone()
	if e.FinalFence != nil {
		f := e.FinalFence.Clone()
		e.FinalFence = &f
	}
	return e
}

func (e WorkspacePreparationRunHistoryEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationRunHistoryEvidence) Validate() error {
	if e.FinalFence != nil && (!e.FinalFence.validFor(e.Facts.Observation.History.WorkspaceID) || e.FinalFence.WorkspaceRevision != e.Facts.WorkspaceRevision || e.FinalFence.Fence.CreationEpoch != e.CreationEpoch || e.PreparationEpoch != 0 || e.Facts.BlockedReason() != "run_history_empty_fence_not_final") {
		return common.ErrConflict
	}
	if e.Version != PreparationRunHistoryEvidenceVersion || !ValidPreparationRunHistoryRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Binding.Validate() != nil || !preparationFenceHex(e.CreationEpoch, 16) || e.Facts.Validate() != nil || e.RequestID != e.Facts.Observation.RequestID || e.ContentHash == "" || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	return nil
}

func ClonePreparationRunHistoryEvidence(items []WorkspacePreparationRunHistoryEvidence) []WorkspacePreparationRunHistoryEvidence {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationRunHistoryEvidence, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}

func validatePreparationRunHistoryEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationRunHistoryEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := map[string]WorkspacePreparationAttempt{}
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.NoRunEvaluationVersion != "" && !validPreparationNoRunFinalShape(a) {
				return common.ErrConflict
			}
			if a.RunHistoryEvidenceRef != "" {
				if _, exists := refs[a.RunHistoryEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.RunHistoryEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationRunHistoryEvidence) {
		return common.ErrConflict
	}
	if len(refs) > 0 && (w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationRunHistoryEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || f.Observation.History.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || !reflect.DeepEqual(e.Binding, a.Binding) || e.CreationEpoch != a.CreationEpoch || e.PreparationEpoch != a.PreparationEpoch || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || f.Observation.ObservedAt.Before(a.StartedAt) || f.Observation.ObservedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		if e.FinalFence != nil && !e.validFinalFor(w, a) || e.FinalFence == nil && a.NoRunEvaluationVersion != "" {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "first_activation_no_run" {
				final := e.FinalFence != nil && c.State == "passed" && c.Reason == PreparationNoRunFinalReason
				if c.Owner != "run" || !c.Required || !final && (e.FinalFence != nil || c.State != "blocked" || c.Reason != "binding_changed" && c.Reason != f.BlockedReason()) {
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

func validatePreparationRunHistoryEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationRunHistoryEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationRunHistoryEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationRunHistoryEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationRunHistoryEvidence, next.PreparationRunHistoryEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) > 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	e := after[len(after)-1]
	if a.State != WorkspacePreparationAccepted || b.State != WorkspacePreparationBlocked || a.RunHistoryEvidenceRef != "" || b.RunHistoryEvidenceRef != e.Ref || a.RequestID != e.RequestID || a.CreationEpoch != e.CreationEpoch || a.PreparationEpoch != e.PreparationEpoch {
		return common.ErrConflict
	}
	if f := e.FinalFence; f != nil && (current.Revision != f.WorkspaceRevision || current.ActiveRunID != f.ActiveRunID || next.ActiveRunID != "" || current.PreparationFence == nil || !reflect.DeepEqual(*current.PreparationFence, f.Fence) || !reflect.DeepEqual(current.PreparationFence, next.PreparationFence)) {
		return common.ErrConflict
	}
	return nil
}
