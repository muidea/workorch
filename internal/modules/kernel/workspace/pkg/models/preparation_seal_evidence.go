package workspace

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationSealEvidenceVersion = "workorch-preparation-seal-evidence/v1"
const PreparationSealRefPrefix = "seal-observation:"

// The complete detached sealed input, not a validation token. The preparation
// package retains its independent all_of compiler; this value only shares the
// exact storage/hash shape so repository readback need not trust a hash hit.
type PreparationSealSnapshot struct {
	SchemaVersion              string
	WorkspaceID                common.WorkspaceID
	WorkspaceRevision          uint64
	AuthoringRef               DefinitionAuthoringAdmissionRef
	AuthoringConfigurationHash string
	InternalDirectory          string
	Candidate                  DefinitionAdmissionCandidate
	StartupInput               WorkspaceStartupInputContract
	ActivationAdmission        WorkspaceActivationAdmission
	Goal                       Goal
	DoD                        DoD
	FormalDoDItemRefs          []string
	SnapshotHash               string
}

func (s PreparationSealSnapshot) Clone() PreparationSealSnapshot {
	s.Candidate = s.Candidate.Clone()
	s.StartupInput = s.StartupInput.Clone()
	s.DoD = *CloneDoD(&s.DoD)
	s.FormalDoDItemRefs = slices.Clone(s.FormalDoDItemRefs)
	return s
}

func (s PreparationSealSnapshot) Hash() string {
	s.SnapshotHash = ""
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > 2*1024*1024 {
		return ""
	}
	return "startup-snapshot-sha256:" + fileobservation.Digest(raw)
}

type WorkspacePreparationSealEvidence struct {
	FinalOwners            *PreparationOwnerRevalidation `json:",omitempty"`
	ExecutionInputs        *PreparationExecutionInputs   `json:",omitempty"`
	Version, Ref           string
	RequestID, PayloadHash string
	Binding                WorkspacePreparationBinding
	StartedAt, CompletedAt time.Time
	Before, After          PreparationSealSnapshot
	ContentHash            string
}

func ValidPreparationSealRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationSealRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationSealRefPrefix), 16)
}

func (e WorkspacePreparationSealEvidence) Clone() WorkspacePreparationSealEvidence {
	if e.FinalOwners != nil {
		v := e.FinalOwners.Clone()
		e.FinalOwners = &v
	}
	e.Before, e.After = e.Before.Clone(), e.After.Clone()
	if e.ExecutionInputs != nil {
		inputs := e.ExecutionInputs.Clone()
		e.ExecutionInputs = &inputs
	}
	return e
}

func ClonePreparationSealEvidence(items []WorkspacePreparationSealEvidence) []WorkspacePreparationSealEvidence {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationSealEvidence, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}

func (e WorkspacePreparationSealEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > 4*1024*1024+64*1024 {
		return ""
	}
	return fileobservation.Digest(raw)
}

// Reuse the original full authoring ledger, not a projected confirmation flag.
// A later definition may retire these formal rows; the immutable historical
// observation remains tied to the exact original confirmed source.
func (e WorkspacePreparationSealEvidence) validateFor(w *Workspace, a WorkspacePreparationAttempt) error {
	if r := e.FinalOwners; r != nil && (w == nil || a.OwnerRevalidationVersion != PreparationOwnerRevalidationVersion || r.Validate() != nil || r.WorkspaceID != w.ID || r.RequestID != a.RequestID || r.PayloadHash != a.PayloadHash || a.ExpectedWorkspaceRevision == ^uint64(0) || r.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || r.StartedAt.Before(a.StartedAt) || r.CompletedAt.After(e.CompletedAt)) {
		return common.ErrConflict
	}
	s := e.Before
	if w == nil || e.Version != PreparationSealEvidenceVersion || !ValidPreparationSealRef(e.Ref) || e.Ref != a.SealEvidenceRef || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.Binding != a.Binding || e.StartedAt.Before(a.StartedAt) || e.StartedAt.IsZero() || e.CompletedAt.Before(e.StartedAt) || e.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || !reflect.DeepEqual(s, e.After) || e.ContentHash == "" || e.ContentHash != e.Hash() || s.SchemaVersion != "workorch-startup-preparation/v1" || s.WorkspaceID != w.ID || s.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || s.SnapshotHash == "" || s.SnapshotHash != s.Hash() || s.SnapshotHash != a.ObservationSnapshotHash || s.AuthoringRef != a.Binding.AuthoringRef || s.AuthoringConfigurationHash != a.Binding.AuthoringConfigurationHash || s.Candidate.CandidateID != a.Binding.CandidateID || s.Candidate.CandidateRevision != a.Binding.CandidateRevision || s.Candidate.CandidateHash != a.Binding.CandidateHash || s.StartupInput.StartupInputHash != a.Binding.StartupInputHash || s.StartupInput.DefinitionRevision != a.Binding.DefinitionRevision || s.ActivationAdmission.State != WorkspaceActivationEligible || s.InternalDirectory == "" {
		return common.ErrConflict
	}
	if !utf8.ValidString(s.InternalDirectory) || strings.ContainsRune(s.InternalDirectory, 0) {
		return common.ErrConflict
	}
	prior := s
	prior.WorkspaceRevision--
	if prior.Hash() != a.Binding.SnapshotHash {
		return common.ErrConflict
	}
	index := -1
	for i, c := range w.DefinitionAdmissionCandidates {
		if c.CandidateID == s.Candidate.CandidateID && c.CandidateRevision == s.Candidate.CandidateRevision && c.CandidateHash == s.Candidate.CandidateHash {
			if index >= 0 || !reflect.DeepEqual(c, s.Candidate) {
				return common.ErrConflict
			}
			index = i
		}
	}
	if index < 0 || index > len(w.DefinitionAuthoringHistory) {
		return common.ErrConflict
	}
	count := 0
	stale := false
	for _, c := range a.Checks {
		if c.ID == "exact_seal" || c.ID == "persistence_integrity" {
			if c.Owner != "workspace" || !c.Required || c.Reason == "original_check_interrupted" {
				return common.ErrConflict
			}
			stale = stale || c.Reason == "binding_changed"
			count++
		}
	}
	if count != 2 {
		return common.ErrConflict
	}
	if stale {
		for _, c := range a.Checks {
			if (c.ID == "exact_seal" || c.ID == "persistence_integrity") && (c.State != "blocked" || c.Reason != "binding_changed") {
				return common.ErrConflict
			}
		}
	}
	var record *DefinitionAuthoringRecord
	var journal *DefinitionAuthoringConfirmation
	if index < len(w.DefinitionAuthoringHistory) {
		record, journal = &w.DefinitionAuthoringHistory[index].Record, &w.DefinitionAuthoringHistory[index].Confirmation
	} else {
		record, journal = w.DefinitionAuthoring, w.DefinitionAuthoringConfirmation
	}
	if record == nil || journal == nil || journal.Phase != DefinitionAuthoringConfirmationMaterialized || journal.ConfigurationHash != s.AuthoringConfigurationHash || !reflect.DeepEqual(journal.Goal, s.Goal) || !reflect.DeepEqual(journal.DoD, s.DoD) {
		return common.ErrConflict
	}
	if (e.ExecutionInputs == nil) != (a.ExecutionInputVersion == "") || e.ExecutionInputs != nil && (a.ExecutionInputVersion != PreparationExecutionInputsVersion || e.ExecutionInputs.validateFor(s, a, record) != nil) {
		return common.ErrConflict
	}
	// Validate the original complete provenance and exact startup/admission using
	// the existing persistence gate. Do not substitute the successor's inputs.
	source := *w
	source.DefinitionAuthoring, source.DefinitionAuthoringConfirmation = record, journal
	source.DefinitionAuthoringHistory = w.DefinitionAuthoringHistory[:index]
	source.DefinitionAdmissionCandidates = w.DefinitionAdmissionCandidates[:index+1]
	source.StartupInput, source.ActivationAdmission = &s.StartupInput, &s.ActivationAdmission
	_, present, compiledSource, err := source.compileActivationAdmissionPersistence(false)
	if err != nil || !present {
		return common.ErrConflict
	}
	if compiledSource == nil || compiledSource.ref != s.AuthoringRef || len(compiledSource.items) != len(s.FormalDoDItemRefs) {
		return common.ErrConflict
	}
	for i := range compiledSource.items {
		if compiledSource.items[i].CandidateItemRef != s.FormalDoDItemRefs[i] {
			return common.ErrConflict
		}
	}
	return nil
}

func EvaluatePreparationSealChecks(w *Workspace, a WorkspacePreparationAttempt) []WorkspacePreparationCheckResult {
	checks := slices.Clone(a.Checks)
	if a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion {
		return checks
	}
	var evidence *WorkspacePreparationSealEvidence
	if w != nil {
		for i := range w.PreparationSealEvidence {
			if w.PreparationSealEvidence[i].Ref == a.SealEvidenceRef {
				evidence = &w.PreparationSealEvidence[i]
			}
		}
	}
	complete := evidence != nil && evidence.validateFor(w, a) == nil
	return evaluatePreparationSealChecks(a, complete)
}

func evaluatePreparationSealChecks(a WorkspacePreparationAttempt, complete bool) []WorkspacePreparationCheckResult {
	checks := slices.Clone(a.Checks)
	for i, c := range checks {
		if c.ID != "exact_seal" && c.ID != "persistence_integrity" || c.Reason == "binding_changed" || c.Reason == "original_check_interrupted" {
			continue
		}
		if complete && c.Owner == "workspace" && c.Required {
			checks[i].State, checks[i].Reason = "passed", "seal_observed_remaining_checks"
			if c.ID == "persistence_integrity" {
				checks[i].Reason = "persistence_binding_observed_remaining_checks"
			}
		} else if c.State != "blocked" {
			checks[i].State, checks[i].Reason = "blocked", "check_pipeline_incomplete"
		}
	}
	return checks
}

func validatePreparationSealEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationSealEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := map[string]WorkspacePreparationAttempt{}
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.SealEvidenceRef != "" {
				if _, exists := refs[a.SealEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.SealEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationSealEvidence) {
		return common.ErrConflict
	}
	if len(refs) != 0 && (w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationSealEvidence {
		a, ok := refs[e.Ref]
		if !ok || e.validateFor(w, a) != nil || !reflect.DeepEqual(a.Checks, evaluatePreparationSealChecks(a, true)) {
			return common.ErrConflict
		}
		delete(refs, e.Ref)
	}
	return nil
}

func validatePreparationSealEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationSealEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationSealEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	before, after := current.PreparationSealEvidence, next.PreparationSealEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) > 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	e := after[len(after)-1]
	s := e.Before
	if a.State != WorkspacePreparationAccepted || b.State != WorkspacePreparationBlocked || a.SealEvidenceRef != "" || b.SealEvidenceRef != e.Ref || a.RequestID != e.RequestID {
		return common.ErrConflict
	}
	stale := false
	for _, c := range b.Checks {
		if c.ID == "exact_seal" && c.State == "blocked" && c.Reason == "binding_changed" {
			stale = true
		}
	}
	if !stale && (current.Revision != a.ExpectedWorkspaceRevision+1 || current.InternalDirectory != s.InternalDirectory || current.StartupInput == nil || current.ActivationAdmission == nil || !reflect.DeepEqual(*current.StartupInput, s.StartupInput) || !reflect.DeepEqual(*current.ActivationAdmission, s.ActivationAdmission)) {
		return common.ErrConflict
	}
	if inputs := e.ExecutionInputs; inputs != nil {
		observed, expected := CaptureWorkspaceConfiguration(current), inputs.Configuration.Clone()
		// Lower scoped metadata intentionally omits Soul fields. Full raw input
		// remains bound to the original configuration hash above; the scoped
		// adapter independently re-reads and compares real sidecars under lock.
		// Present Soul declarations must still match; they are never overwritten.
		if observed.DefaultSoulID == "" && len(observed.SoulBindings) == 0 {
			expected.DefaultSoulID, expected.SoulBindings = observed.DefaultSoulID, observed.SoulBindings
		}
		if stale || current.Revision != a.ExpectedWorkspaceRevision+1 || !reflect.DeepEqual(observed, expected) {
			return common.ErrConflict
		}
	}
	return nil
}
