package workspace

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

const PreparationManagedConfigEvidenceVersion = "workorch-preparation-managed-config-evidence/v1"
const PreparationManagedConfigRefPrefix = "managed-config-observation:"

func ValidPreparationManagedConfigRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationManagedConfigRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationManagedConfigRefPrefix), 16)
}

type WorkspacePreparationManagedConfigObservation struct {
	Scope, Path, Policy string
	DefaultsJSON        []byte
	Observation         fileobservation.Resource
}

func (r WorkspacePreparationManagedConfigObservation) BlockedReason() string {
	if !r.Observation.Complete {
		return "managed_config_observation_incomplete"
	}
	return workorchmanagedconfig.PreparationJSONReason(r.Policy, r.DefaultsJSON, r.Observation.Content)
}

// Configuration bytes, expected objects and content digests never enter the
// public operation ledger. Ref is random and independent of secret values.
// This private archive is immutable history, not a currentness/readiness grant.
type WorkspacePreparationManagedConfigEvidence struct {
	Version, Ref                         string
	WorkspaceID                          common.WorkspaceID
	RequestID, PayloadHash               string
	ObservedWorkspaceRevision            uint64
	ConfigurationHash, InternalDirectory string
	StartedAt, CompletedAt               time.Time
	Files                                []WorkspacePreparationManagedConfigObservation
	ContentHash                          string
}

func (e WorkspacePreparationManagedConfigEvidence) Clone() WorkspacePreparationManagedConfigEvidence {
	e.Files = slices.Clone(e.Files)
	for i := range e.Files {
		e.Files[i].DefaultsJSON = slices.Clone(e.Files[i].DefaultsJSON)
		e.Files[i].Observation = e.Files[i].Observation.Clone()
	}
	return e
}

func (e WorkspacePreparationManagedConfigEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationManagedConfigEvidence) Validate() error {
	if e.Version != PreparationManagedConfigEvidenceVersion || !ValidPreparationManagedConfigRef(e.Ref) || !boundedPreparationIdentity(string(e.WorkspaceID)) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.ObservedWorkspaceRevision == 0 || !boundedPreparationIdentity(e.ConfigurationHash) || !fileobservation.ValidPath(e.InternalDirectory) || e.StartedAt.IsZero() || e.CompletedAt.Before(e.StartedAt) || len(e.Files) > fileobservation.ResourceLimit || !preparationFenceHex(e.ContentHash, 32) || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	total, defaultsTotal := 0, 0
	seen := make(map[string]bool)
	workspaceSeen := false
	for _, r := range e.Files {
		key := r.Scope + ":" + r.Path
		if !workorchmanagedconfig.ValidPreparationTarget(r.Scope, r.Path) || seen[key] || r.Scope == workorchmanagedconfig.ScopeSystem && (workspaceSeen || r.Policy != workorchmanagedconfig.PolicySystemExact) || r.Scope == workorchmanagedconfig.ScopeWorkspace && r.Policy != workorchmanagedconfig.PolicyWorkspacePreserve && r.Policy != workorchmanagedconfig.PolicyWorkspaceMergeMissing || r.Observation.Ref != r.Path || r.Observation.Validate() != nil || workorchmanagedconfig.PreparationJSONReason(r.Policy, r.DefaultsJSON, []byte(`{}`)) == "managed_config_declaration_invalid" {
			return common.ErrConflict
		}
		seen[key] = true
		if r.Scope == workorchmanagedconfig.ScopeWorkspace {
			workspaceSeen = true
		}
		total += r.Observation.ContentBytes
		defaultsTotal += len(r.DefaultsJSON)
	}
	if total > fileobservation.ResourceTotalBytes || defaultsTotal > fileobservation.ResourceTotalBytes {
		return common.ErrConflict
	}
	return nil
}

func (e WorkspacePreparationManagedConfigEvidence) BlockedReason() string {
	// Preserve every per-file observation; choose the first actionable failure
	// in original source order rather than hiding it behind an earlier match.
	for _, r := range e.Files {
		if reason := r.BlockedReason(); reason != "managed_configs_observed_remaining_checks" {
			return reason
		}
	}
	return "managed_configs_observed_remaining_checks"
}

func ClonePreparationManagedConfigEvidence(items []WorkspacePreparationManagedConfigEvidence) []WorkspacePreparationManagedConfigEvidence {
	if items == nil {
		return nil
	}
	result := make([]WorkspacePreparationManagedConfigEvidence, len(items))
	for i := range items {
		result[i] = items[i].Clone()
	}
	return result
}

func validatePreparationManagedConfigEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationManagedConfigEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.ManagedConfigEvidenceRef != "" {
				if _, exists := refs[a.ManagedConfigEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.ManagedConfigEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationManagedConfigEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationManagedConfigEvidence {
		a, ok := refs[e.Ref]
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || e.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.ObservedWorkspaceRevision != a.ExpectedWorkspaceRevision+1 || e.ConfigurationHash != a.Binding.AuthoringConfigurationHash || e.StartedAt.Before(a.StartedAt) || e.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		found := false
		for _, check := range a.Checks {
			if check.ID == "managed_configuration" {
				if check.Owner != "file" || !check.Required || check.Reason != "binding_changed" && check.Reason != e.BlockedReason() {
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

func validatePreparationManagedConfigEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationManagedConfigEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationManagedConfigEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationManagedConfigEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationManagedConfigEvidence, next.PreparationManagedConfigEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
	b := next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.ManagedConfigEvidenceRef != "" || b.ManagedConfigEvidenceRef != after[len(after)-1].Ref {
		return common.ErrConflict
	}
	if b.EvaluationVersion == PreparationCheckEvaluationVersion {
		// New positive consumers require the complete original declaration set,
		// not merely a self-consistent (possibly empty/subset) archive. Rebuild
		// from independent current preimage roots only during the original CAS;
		// later configuration drift never rewrites immutable historical material.
		e := after[len(after)-1]
		for _, c := range b.Checks {
			if c.ID != "managed_configuration" || c.State != "passed" && c.State != "failed" {
				continue
			}
			// The scoped metadata backend stores Soul bindings separately. Its
			// metadata row cannot reproduce the full configuration hash. Exact
			// configuration identity was checked against the immutable original
			// attempt above; independently compare all managed declarations here.
			if current.DefinitionAuthoring == nil || e.InternalDirectory != current.InternalDirectory {
				return common.ErrConflict
			}
			declarations, err := workorchmanagedconfig.CompilePreparationDeclarations(current.DefinitionAuthoring.TemplateSnapshot.ContextHints, current.ContextHints)
			if err != nil || len(declarations) != len(e.Files) {
				return common.ErrConflict
			}
			for i, d := range declarations {
				raw, err := json.Marshal(d.Defaults)
				r := e.Files[i]
				if err != nil || d.Scope != r.Scope || d.Path != r.Path || d.Policy != r.Policy || !bytes.Equal(raw, r.DefaultsJSON) {
					return common.ErrConflict
				}
			}
		}
	}
	return nil
}
