package workspace

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const WorkspacePreparationWorkflowVersion = "workorch-preparation-record"
const WorkspacePreparationAttemptLimit = 64

// These are operation states, not Workspace lifecycle or activation permission.
// Current preparation supports explicit accepted -> blocked operations,
// optionally with versioned point-in-time File/Soul judgments, but deliberately
// cannot represent aggregate positive readiness, M3 or execution rights.
type WorkspacePreparationAttemptState string

const (
	WorkspacePreparationAccepted WorkspacePreparationAttemptState = "accepted"
	WorkspacePreparationBlocked  WorkspacePreparationAttemptState = "blocked"
	WorkspacePreparationFailed   WorkspacePreparationAttemptState = "failed"
	WorkspacePreparationUnknown  WorkspacePreparationAttemptState = "unknown"
)

type WorkspacePreparationBinding struct {
	AuthoringRef               DefinitionAuthoringAdmissionRef
	CandidateID                string
	CandidateRevision          uint64
	CandidateHash              string
	DefinitionRevision         string
	StartupInputHash           string
	AuthoringConfigurationHash string
	SnapshotHash               string
	ManifestHash               string
	CheckPolicyVersion         string
	CompilerVersion            string
}

type WorkspacePreparationAttempt struct {
	ExecutionInputVersion      string                            `json:",omitempty"`
	DefinitionCompilation      *PreparationDefinitionCompilation `json:",omitempty"`
	Assessment                 *PreparationAssessment            `json:",omitempty"`
	SealEvidenceRef            string
	MCPRegistryEvidenceRef     string
	RequestID                  string
	PayloadHash                string
	ExpectedWorkspaceRevision  uint64
	CommittedWorkspaceRevision uint64
	Binding                    WorkspacePreparationBinding
	StartedAt                  time.Time
	CompletedAt                time.Time
	ExpiresAt                  time.Time
	State                      WorkspacePreparationAttemptState
	CreationEpoch              string
	PreparationEpoch           uint64
	Checks                     []WorkspacePreparationCheckResult
	// Current point-in-time per-check judgments, never aggregate readiness.
	EvaluationVersion               string
	NoRunEvaluationVersion          string `json:",omitempty"`
	EmptySelectionEvaluationVersion string `json:",omitempty"`
	// Fixed at acceptance; recovery cannot opt an old operation into final reads.
	OwnerRevalidationVersion string `json:",omitempty"`
	// Exact snapshot identity after the original acceptance revision increment.
	// It is not the earlier manifest snapshot and never inferred on recovery.
	ObservationSnapshotHash string
	// Immutable historical observation reference, not a currentness/PASS claim.
	DirectoryEvidenceHash      string
	ResourceEvidenceHash       string
	ManagedConfigEvidenceRef   string
	SoulEvidenceRef            string
	SkillRegistryEvidenceRef   string
	SkillHubEffectRef          string
	SkillHubEffectState        PreparationSkillHubEffectState
	SkillHubInstalledState     string
	SkillSelectionState        string
	SkillContextState          string
	SkillReferenceState        string `json:",omitempty"`
	SkillStatusState           string `json:",omitempty"`
	ProviderCacheEvidenceRef   string
	RunHistoryEvidenceRef      string
	MemoryInventoryEvidenceRef string
}

// Check results contain only bounded machine codes and requirement identities,
// never raw resource content or diagnostics. There is no caller-authored PASS
// surface. Pending is not an observation of success; passed needs the original
// private settlement and complete owner material, not a caller success flag.
type WorkspacePreparationCheckResult struct {
	ID       string
	Owner    string
	Required bool
	State    WorkspacePreparationCheckState
	Reason   string
	// Hash of the original exact file requirement (not resource content or a
	// credential). Kept owner-side; immutable across the settlement transition.
	SubjectHash string
}

type WorkspacePreparationCheckState string

// WorkspacePreparationRecord stores bounded operation identities only. Raw
// resources, credentials, provider clients and diagnostic text cannot enter it.
// Reads never create this record or resume an accepted/unknown attempt.
type WorkspacePreparationRecord struct {
	Completions []WorkspacePreparationCompletion `json:",omitempty"`
	Version     string
	WorkspaceID common.WorkspaceID
	Attempts    []WorkspacePreparationAttempt
}

func (r WorkspacePreparationRecord) Clone() WorkspacePreparationRecord {
	r.Completions = slices.Clone(r.Completions)
	r.Attempts = slices.Clone(r.Attempts)
	for i := range r.Attempts {
		if r.Attempts[i].DefinitionCompilation != nil {
			value := *r.Attempts[i].DefinitionCompilation
			r.Attempts[i].DefinitionCompilation = &value
		}
		r.Attempts[i].Checks = slices.Clone(r.Attempts[i].Checks)
		if r.Attempts[i].Assessment != nil {
			s := r.Attempts[i].Assessment.Clone()
			r.Attempts[i].Assessment = &s
		}
	}
	return r
}

func (r WorkspacePreparationRecord) Validate(workspaceID common.WorkspaceID) error {
	if r.Version != WorkspacePreparationWorkflowVersion || r.WorkspaceID != workspaceID || workspaceID == "" || len(r.Attempts) == 0 || len(r.Attempts) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	if validatePreparationCompletions(r) != nil {
		return common.ErrConflict
	}
	seen := make(map[string]struct{}, len(r.Attempts))
	lastRevision := uint64(0)
	for _, attempt := range r.Attempts {

		if _, present := seen[attempt.RequestID]; present || attempt.Validate() != nil || attempt.CommittedWorkspaceRevision <= lastRevision {
			return common.ErrConflict
		}

		if attempt.EvaluationVersion != "" && attempt.EvaluationVersion != PreparationCheckEvaluationVersion {
			return common.ErrConflict
		}
		if attempt.EvaluationVersion == "" && attempt.ObservationSnapshotHash != "" || attempt.EvaluationVersion != "" && !boundedPreparationIdentity(attempt.ObservationSnapshotHash) {
			return common.ErrConflict
		}
		if !preparationFenceHex(attempt.CreationEpoch, 16) || len(attempt.Checks) == 0 || len(attempt.Checks) > 128 {
			return common.ErrConflict
		}
		lastID := ""
		for _, check := range attempt.Checks {
			if check.ID == "provider_cache" && IsPreparationProviderSelectionReason(check.Reason) && (attempt.ProviderCacheEvidenceRef == "" || check.Owner != "llm" || !check.Required || attempt.EvaluationVersion != PreparationCheckEvaluationVersion || attempt.State != WorkspacePreparationBlocked || check.State != "blocked") {
				return common.ErrConflict
			}
			if check.ID == "mcp_selection" && PreparationMCPReasonRequiresEvidence(check.Reason) && (attempt.MCPRegistryEvidenceRef == "" || check.Owner != "mcpregistry" || !check.Required) {
				return common.ErrConflict
			}
			if check.ID == "mcp_selection" && IsPreparationMCPSelectionReason(check.Reason) && (attempt.EvaluationVersion != PreparationCheckEvaluationVersion || attempt.State != WorkspacePreparationBlocked || check.State != "blocked") {
				return common.ErrConflict
			}
			if !validPreparationCheckSubject(attempt.EvaluationVersion, check) {
				return common.ErrConflict
			}
			if check.ID == "memory_selection" && PreparationMemoryReasonRequiresEvidence(check.Reason) && attempt.MemoryInventoryEvidenceRef == "" {
				return common.ErrConflict
			}
			if check.ID == "skill_selection" && strings.HasPrefix(check.Reason, "skill_selection_") && (attempt.SkillSelectionState == "" || check.Reason != "skill_selection_"+attempt.SkillSelectionState) {
				return common.ErrConflict
			}
			if check.ID == "skill_selection" && check.Reason == "skill_registry_observed_selection_pending" && attempt.SkillRegistryEvidenceRef == "" {
				return common.ErrConflict
			}
			if check.ID == "soul_selection" && check.Reason == "souls_observed_remaining_checks" && attempt.SoulEvidenceRef == "" {
				return common.ErrConflict
			}
			if !boundedPreparationIdentity(check.ID) || !boundedPreparationIdentity(check.Owner) || check.ID <= lastID {
				return common.ErrConflict
			}
			lastID = check.ID
			if attempt.State == WorkspacePreparationAccepted || attempt.State == WorkspacePreparationUnknown {
				if check.State != "pending" || check.Reason != "not_observed" {
					return common.ErrConflict
				}
			} else if !ValidPreparationEvaluatedCheck(attempt, check) {
				return common.ErrConflict
			}
		}

		seen[attempt.RequestID] = struct{}{}
		lastRevision = attempt.CommittedWorkspaceRevision
	}
	return nil
}

// Negative diagnostic classifications only. Even a complete fresh declaration
// or empty ordered history does not discharge a required preparation check.
func ValidPreparationBlockedReason(checkID, reason string) bool {
	if checkID == "skill_selection" && (reason == "skill_selection_compiled_checks_pending" || reason == "skill_selection_sources_incomplete" || reason == "skill_selection_conflict") {
		return true
	}
	if reason == "check_pipeline_incomplete" || reason == "original_check_interrupted" || reason == "binding_changed" {
		return true
	}
	if strings.HasPrefix(checkID, "resource/") {
		return reason == "resource_observation_unavailable" || reason == "resource_observation_incomplete" || reason == "resource_content_changed" || reason == "resource_expected_hash_unavailable" || reason == "resource_observed_remaining_checks"
	}
	switch checkID {
	case "mcp_selection":
		return reason == "mcp_registry_observation_unavailable_not_saved" || PreparationMCPReasonRequiresEvidence(reason)
	case "memory_selection":
		return reason == "memory_inventory_unavailable_not_saved" || reason == "memory_inventory_observed_selection_pending" || reason == "memory_semantic_evidence_required" || reason == "memory_selection_candidate_limit"
	case "skill_selection":
		return reason == "skill_registry_observation_unavailable_not_saved" || reason == "skill_registry_observed_selection_pending"
	case "managed_configuration":
		return reason == "managed_config_observation_unavailable" || reason == "managed_config_observation_incomplete" || reason == "managed_config_declaration_invalid" || reason == "managed_config_content_invalid" || reason == "managed_config_content_conflict" || reason == "managed_configs_observed_remaining_checks"
	case "soul_selection":
		return reason == "soul_observation_unavailable_not_saved" || reason == "souls_observed_remaining_checks"
	case "directory_boundary":
		return reason == "directory_observation_unavailable" || reason == "directory_observation_incomplete" || reason == "directory_permissions_denied" || reason == "directory_observed_remaining_checks"
	case "provider_cache":
		return reason == "provider_cache_unavailable" || reason == "provider_cache_miss" || reason == "provider_cache_expired" || reason == "provider_declaration_only" || IsPreparationProviderSelectionReason(reason)
	case "first_activation_no_run":
		return reason == "run_history_unavailable" || reason == "run_history_retired" || reason == "run_history_present" || reason == "run_history_empty_fence_not_final"
	}
	return false
}

func (a WorkspacePreparationAttempt) Validate() error {
	if a.OwnerRevalidationVersion != "" && (a.OwnerRevalidationVersion != PreparationOwnerRevalidationVersion || a.EvaluationVersion != PreparationCheckEvaluationVersion) {
		return common.ErrConflict
	}
	if a.ExecutionInputVersion != "" && (a.ExecutionInputVersion != PreparationExecutionInputsVersion || a.SealEvidenceRef == "" || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion) {
		return common.ErrConflict
	}
	if a.DefinitionCompilation != nil && a.DefinitionCompilation.ValidateFor(a) != nil {
		return common.ErrConflict
	}
	if a.EmptySelectionEvaluationVersion != "" && (a.EmptySelectionEvaluationVersion != PreparationEmptySelectionEvaluationVersion || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion) {
		return common.ErrConflict
	}
	if a.Assessment != nil && (a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || a.Assessment.ValidateFor(a.Binding.ManifestHash, a.Checks) != nil) {
		return common.ErrConflict
	}
	if a.NoRunEvaluationVersion != "" && !validPreparationNoRunFinalShape(a) {
		return common.ErrConflict
	}
	if a.RunHistoryEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationRunHistoryRef(a.RunHistoryEvidenceRef)) {
		return common.ErrConflict
	}
	if a.SealEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || !ValidPreparationSealRef(a.SealEvidenceRef)) {
		return common.ErrConflict
	}
	if a.MCPRegistryEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationMCPRegistryRef(a.MCPRegistryEvidenceRef)) {
		return common.ErrConflict
	}
	if a.MemoryInventoryEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationMemoryInventoryRef(a.MemoryInventoryEvidenceRef)) {
		return common.ErrConflict
	}
	if a.ProviderCacheEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationProviderCacheRef(a.ProviderCacheEvidenceRef)) {
		return common.ErrConflict
	}
	if a.SkillContextState != "" && (a.SkillSelectionState != "compiled_checks_pending" || a.State != WorkspacePreparationBlocked || a.SkillContextState != "compiled_checks_pending" && a.SkillContextState != "bounded_checks_pending" && a.SkillContextState != "rejected") {
		return common.ErrConflict
	}
	if a.SkillReferenceState != "" && (a.SkillSelectionState != "compiled_checks_pending" || a.State != WorkspacePreparationBlocked || !validPreparationSkillReferenceState(a.SkillReferenceState)) {
		return common.ErrConflict
	}
	if a.SkillStatusState != "" && (a.SkillSelectionState != "compiled_checks_pending" || a.State != WorkspacePreparationBlocked || !validPreparationSkillStatusState(a.SkillStatusState)) {
		return common.ErrConflict
	}
	if a.SkillSelectionState != "" && (a.State != WorkspacePreparationBlocked || a.SkillRegistryEvidenceRef == "" || a.SkillSelectionState != "compiled_checks_pending" && a.SkillSelectionState != "sources_incomplete" && a.SkillSelectionState != "conflict" || a.SkillSelectionState == "compiled_checks_pending" && (a.SoulEvidenceRef == "" || a.SkillHubEffectState != PreparationSkillHubRecorded || a.SkillHubInstalledState != "observed_selection_pending")) {
		return common.ErrConflict
	}
	if a.SkillHubInstalledState != "" && (a.State != WorkspacePreparationBlocked || a.SkillHubEffectRef == "" || a.SkillHubInstalledState != "incomplete" && a.SkillHubInstalledState != "observed_selection_pending") {
		return common.ErrConflict
	}
	if a.SkillHubEffectRef != "" {
		if !ValidPreparationSkillHubRef(a.SkillHubEffectRef) || (a.State == WorkspacePreparationAccepted || a.State == WorkspacePreparationUnknown) && a.SkillHubEffectState != PreparationSkillHubReserved || a.State == WorkspacePreparationBlocked && a.SkillHubEffectState != PreparationSkillHubRecorded && a.SkillHubEffectState != PreparationSkillHubUnresolved || a.State == WorkspacePreparationFailed {
			return common.ErrConflict
		}
	} else if a.SkillHubEffectState != "" {
		return common.ErrConflict
	}
	if a.SkillRegistryEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationSkillRegistryRef(a.SkillRegistryEvidenceRef)) {
		return common.ErrConflict
	}
	if a.SoulEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationSoulRef(a.SoulEvidenceRef)) {
		return common.ErrConflict
	}
	if a.ManagedConfigEvidenceRef != "" && (a.State != WorkspacePreparationBlocked || !ValidPreparationManagedConfigRef(a.ManagedConfigEvidenceRef)) {
		return common.ErrConflict
	}
	if a.ResourceEvidenceHash != "" && (a.State != WorkspacePreparationBlocked || !preparationFenceHex(a.ResourceEvidenceHash, 32)) {
		return common.ErrConflict
	}
	if a.DirectoryEvidenceHash != "" && (a.State != WorkspacePreparationBlocked || !preparationFenceHex(a.DirectoryEvidenceHash, 32)) {
		return common.ErrConflict
	}
	if !boundedPreparationIdentity(a.RequestID) || !boundedPreparationIdentity(a.PayloadHash) || a.ExpectedWorkspaceRevision == 0 || a.CommittedWorkspaceRevision <= a.ExpectedWorkspaceRevision || a.StartedAt.IsZero() || a.Binding.Validate() != nil {
		return common.ErrConflict
	}
	switch a.State {
	case WorkspacePreparationAccepted, WorkspacePreparationUnknown:
		if !a.CompletedAt.IsZero() || !a.ExpiresAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspacePreparationBlocked, WorkspacePreparationFailed:
		if a.CompletedAt.Before(a.StartedAt) || a.CompletedAt.IsZero() || !a.ExpiresAt.IsZero() && !a.ExpiresAt.After(a.CompletedAt) {
			return common.ErrConflict
		}
	default:
		return common.ErrConflict
	}
	return nil
}

func (b WorkspacePreparationBinding) Validate() error {
	if b.AuthoringRef.Validate() != nil || b.CandidateRevision == 0 {
		return common.ErrConflict
	}
	for _, value := range []string{b.AuthoringRef.InputHash, b.AuthoringRef.DerivationPermitID, b.AuthoringRef.ProposalHash, b.AuthoringRef.SpecificationHash, b.CandidateID, b.CandidateHash, b.DefinitionRevision, b.StartupInputHash, b.AuthoringConfigurationHash, b.SnapshotHash, b.ManifestHash, b.CheckPolicyVersion, b.CompilerVersion} {
		if !boundedPreparationIdentity(value) {
			return common.ErrConflict
		}
	}
	return nil
}

func boundedPreparationIdentity(value string) bool {
	return value != "" && len(value) <= 512 && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0) && utf8.ValidString(value)
}
