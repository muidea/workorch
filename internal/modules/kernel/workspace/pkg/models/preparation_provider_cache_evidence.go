package workspace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationProviderCacheEvidenceVersion = "workorch-preparation-provider-cache-evidence/v1"
const PreparationProviderCacheRefPrefix = "provider-cache-observation:"

// Complete cache-only owner value at one observation point. It is not a
// connectivity/selection proof or a claim that this process epoch remains live.
type PreparationProviderCacheFacts struct {
	Version           string
	WorkspaceRevision uint64
	Observation       llmevents.ObserveProviderCatalogCacheResult
}

func (f PreparationProviderCacheFacts) Clone() PreparationProviderCacheFacts {
	f.Observation = f.Observation.Clone()
	return f
}

func (f PreparationProviderCacheFacts) Validate() error {
	c := llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: f.Observation.WorkspaceID}
	if f.Observation.Budget != nil {
		// Historical shape validation only. The exact Workspace source is
		// independently checked by the selection compiler, not inferred here.
		target := f.Observation.Budget.Target
		c.Target = &target
	}
	if f.Version != PreparationProviderCacheEvidenceVersion || f.WorkspaceRevision == 0 || f.Observation.ValidateFor(c) != nil {
		return common.ErrConflict
	}
	return nil
}

func (f PreparationProviderCacheFacts) BlockedReason() string {
	if f.Validate() != nil {
		return "provider_cache_unavailable"
	}
	switch f.Observation.Catalog.State {
	case llmmodel.ProviderCatalogStateUnavailable:
		return "provider_cache_miss"
	case llmmodel.ProviderCatalogStateStale:
		return "provider_cache_expired"
	default:
		return "provider_declaration_only"
	}
}

type WorkspacePreparationProviderCacheEvidence struct {
	Version, Ref           string
	RequestID, PayloadHash string
	Binding                WorkspacePreparationBinding
	Facts                  PreparationProviderCacheFacts
	ContentHash            string
	Selection              *PreparationProviderSelectionCompilation `json:",omitempty"`
}

func ValidPreparationProviderCacheRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationProviderCacheRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationProviderCacheRefPrefix), 16)
}

func (e WorkspacePreparationProviderCacheEvidence) Clone() WorkspacePreparationProviderCacheEvidence {
	e.Facts = e.Facts.Clone()
	if e.Selection != nil {
		v := e.Selection.Clone()
		e.Selection = &v
	}
	return e
}

func (e WorkspacePreparationProviderCacheEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationProviderCacheEvidence) Validate() error {
	if e.Version != PreparationProviderCacheEvidenceVersion || !ValidPreparationProviderCacheRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Binding.Validate() != nil || e.Facts.Validate() != nil || e.ContentHash != e.Hash() || validatePreparationProviderSelection(e) != nil {
		return common.ErrConflict
	}
	return nil
}

func ClonePreparationProviderCacheEvidence(items []WorkspacePreparationProviderCacheEvidence) []WorkspacePreparationProviderCacheEvidence {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationProviderCacheEvidence, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}

// PreparationProviderCacheExpired projects only a known deadline of the latest
// operation's original observation. A false result is not currentness, owner
// epoch, authorization or readiness proof. It must never refresh a cache or
// replace the immutable original catalog state/reason with a new observation.
func PreparationProviderCacheExpired(w *Workspace, now time.Time) (bool, error) {
	if w == nil || w.ID == "" || w.Revision == 0 || now.IsZero() {
		return false, common.ErrInvalidArgument
	}
	if w.StartupPreparation != nil && w.StartupPreparation.Validate(w.ID) != nil || validatePreparationProviderCacheEvidencePersistence(w) != nil {
		return false, common.ErrConflict
	}
	if w.StartupPreparation == nil {
		return false, nil
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	if a.ProviderCacheEvidenceRef == "" {
		return false, nil // Old/missing observations acquire no new material.
	}
	for _, e := range w.PreparationProviderCacheEvidence {
		if e.Ref == a.ProviderCacheEvidenceRef {
			expires := e.Facts.Observation.Catalog.ExpiresAt
			return !expires.IsZero() && !now.Before(expires), nil
		}
	}
	return false, common.ErrConflict
}

func validatePreparationProviderCacheEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationProviderCacheEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.ProviderCacheEvidenceRef != "" {
				if _, exists := refs[a.ProviderCacheEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.ProviderCacheEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationProviderCacheEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationProviderCacheEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || f.Observation.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || !reflect.DeepEqual(e.Binding, a.Binding) || f.Observation.ObservedAt.Before(a.StartedAt) || f.Observation.ObservedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "provider_cache" {
				if c.Owner != "llm" || !c.Required || c.State != "blocked" || c.Reason != "binding_changed" && c.Reason != e.BlockedReason() {
					return common.ErrConflict
				}
				if f.Observation.Configuration != nil && c.Reason != "binding_changed" && e.Selection == nil {
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

func validatePreparationProviderCacheEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationProviderCacheEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationProviderCacheEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationProviderCacheEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationProviderCacheEvidence, next.PreparationProviderCacheEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.ProviderCacheEvidenceRef != "" || b.ProviderCacheEvidenceRef != after[len(after)-1].Ref || a.ExpectedWorkspaceRevision+1 != after[len(after)-1].Facts.WorkspaceRevision {
		return common.ErrConflict
	}
	if compiled := after[len(after)-1].Selection; compiled != nil {
		expected := PreparationProviderDefinition(current)
		// Scoped metadata omits separately stored Soul bindings; the accepted
		// original operation, not that partial metadata, pins full config identity.
		expected.ConfigurationHash = a.Binding.AuthoringConfigurationHash
		if !reflect.DeepEqual(compiled.Definition, expected) {
			return common.ErrConflict
		}
	}
	return nil
}
