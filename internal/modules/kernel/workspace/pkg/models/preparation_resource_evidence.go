package workspace

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationResourceEvidenceVersion = "workorch-preparation-resource-evidence/v1"

type WorkspacePreparationResourceObservation struct {
	ID                  string
	Required            bool
	ExpectedContentHash string
	Observation         fileobservation.Resource
}

func (r WorkspacePreparationResourceObservation) BlockedReason() string {
	if !r.Observation.Complete {
		return "resource_observation_incomplete"
	}
	expected, err := hex.DecodeString(strings.TrimPrefix(r.ExpectedContentHash, "sha256:"))
	if !strings.HasPrefix(r.ExpectedContentHash, "sha256:") || err != nil || len(expected) != 32 {
		return "resource_expected_hash_unavailable"
	}
	if r.ExpectedContentHash != "sha256:"+r.Observation.ContentSHA256 {
		return "resource_content_changed"
	}
	return "resource_observed_remaining_checks"
}

// Historical owner-private material, never a readiness grant. The original
// full resource bytes and their failed read diagnostics are excluded from all
// public Workspace JSON. Only the private original settlement may append it.
type WorkspacePreparationResourceEvidence struct {
	Version                                 string
	WorkspaceID                             common.WorkspaceID
	RequestID, PayloadHash                  string
	ObservedWorkspaceRevision               uint64
	SpecificationHash, DeliverableDirectory string
	StartedAt, CompletedAt                  time.Time
	Resources                               []WorkspacePreparationResourceObservation
	ContentHash                             string
}

func (e WorkspacePreparationResourceEvidence) Clone() WorkspacePreparationResourceEvidence {
	if e.Resources != nil {
		e.Resources = append([]WorkspacePreparationResourceObservation{}, e.Resources...)
		for i := range e.Resources {
			e.Resources[i].Observation = e.Resources[i].Observation.Clone()
		}
	}
	return e
}

func (e WorkspacePreparationResourceEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationResourceEvidence) Validate() error {
	if e.Version != PreparationResourceEvidenceVersion || !boundedPreparationIdentity(string(e.WorkspaceID)) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.ObservedWorkspaceRevision == 0 || !boundedPreparationIdentity(e.SpecificationHash) || !fileobservation.ValidPath(e.DeliverableDirectory) || e.StartedAt.IsZero() || e.CompletedAt.Before(e.StartedAt) || len(e.Resources) > fileobservation.ResourceLimit || !preparationFenceHex(e.ContentHash, 32) || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	total, last := 0, ""
	for _, r := range e.Resources {
		if !boundedPreparationIdentity(r.ID) || r.ID <= last || len(r.ExpectedContentHash) > 256 || !utf8.ValidString(r.ExpectedContentHash) || strings.ContainsRune(r.ExpectedContentHash, 0) || r.Observation.Validate() != nil {
			return common.ErrConflict
		}
		last = r.ID
		total += r.Observation.ContentBytes
	}
	if total > fileobservation.ResourceTotalBytes {
		return common.ErrConflict
	}
	return nil
}

func ClonePreparationResourceEvidence(items []WorkspacePreparationResourceEvidence) []WorkspacePreparationResourceEvidence {
	if items == nil {
		return nil
	}
	result := make([]WorkspacePreparationResourceEvidence, len(items))
	for i := range items {
		result[i] = items[i].Clone()
	}
	return result
}

func validatePreparationResourceEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationResourceEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.ResourceEvidenceHash != "" {
				if _, exists := refs[a.ResourceEvidenceHash]; exists {
					return common.ErrConflict
				}
				refs[a.ResourceEvidenceHash] = a
			}
		}
	}
	if len(refs) != len(w.PreparationResourceEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationResourceEvidence {
		a, ok := refs[e.ContentHash]
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || e.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.ObservedWorkspaceRevision != a.ExpectedWorkspaceRevision+1 || e.SpecificationHash != a.Binding.AuthoringRef.SpecificationHash || e.StartedAt.Before(a.StartedAt) || e.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		index := 0
		for _, check := range a.Checks {
			if !strings.HasPrefix(check.ID, "resource/") {
				continue
			}
			if index >= len(e.Resources) {
				return common.ErrConflict
			}
			r := e.Resources[index]
			if check.ID != "resource/"+r.ID || check.Owner != "file" || check.Required != r.Required {
				return common.ErrConflict
			}
			index++
		}
		if index != len(e.Resources) {
			return common.ErrConflict
		}
		delete(refs, e.ContentHash)
	}
	return nil
}

func validatePreparationResourceEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationResourceEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationResourceEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationResourceEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationResourceEvidence, next.PreparationResourceEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
	b := next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.ResourceEvidenceHash != "" || b.ResourceEvidenceHash != after[len(after)-1].ContentHash {
		return common.ErrConflict
	}
	return nil
}
