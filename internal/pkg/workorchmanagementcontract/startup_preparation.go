package workorchmanagementcontract

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// StartupPreparationView reports persisted operations and historical per-check
// judgments and historical completion metadata, never activation permission. A historical
// no-Run final fence is not a claim about current Run inventory.
// ActiveRunID is an independent Workspace fact, not an inventory proof.
type StartupPreparationView struct {
	WorkspaceID             string                                  `json:"workspaceId"`
	WorkspaceRevision       uint64                                  `json:"workspaceRevision"`
	ActiveRunID             string                                  `json:"activeRunId,omitempty"`
	OperationState          string                                  `json:"operationState"`
	CurrentValidity         string                                  `json:"currentValidity"`
	ResourceInitializations []PreparationResourceInitializationView `json:"resourceInitializations,omitempty"`
	Record                  *PreparationRecordView                  `json:"record,omitempty"`
	SkillHubProjectIssue    string                                  `json:"skillHubProjectIssue,omitempty"`
}

type PreparationRecordView struct {
	Completions []PreparationCompletionView `json:"completions,omitempty"`
	Version     string                      `json:"version"`
	Attempts    []PreparationAttemptView    `json:"attempts"`
}

type PreparationAttemptView struct {
	ExecutionInputVersion           string                                `json:"executionInputVersion,omitempty"`
	DefinitionCompilation           *PreparationDefinitionCompilationView `json:"definitionCompilation,omitempty"`
	Assessment                      *PreparationAssessmentView            `json:"assessment,omitempty"`
	MCPRegistryEvidenceRef          string                                `json:"mcpRegistryEvidenceRef,omitempty"`
	RequestID                       string                                `json:"requestId"`
	PayloadHash                     string                                `json:"payloadHash"`
	ExpectedWorkspaceRevision       uint64                                `json:"expectedWorkspaceRevision"`
	CommittedWorkspaceRevision      uint64                                `json:"committedWorkspaceRevision"`
	Binding                         PreparationBindingView                `json:"binding"`
	StartedAt                       time.Time                             `json:"startedAt"`
	CompletedAt                     *time.Time                            `json:"completedAt,omitempty"`
	ExpiresAt                       *time.Time                            `json:"expiresAt,omitempty"`
	State                           string                                `json:"state"`
	CreationEpoch                   string                                `json:"creationEpoch,omitempty"`
	PreparationEpoch                uint64                                `json:"preparationEpoch"` // Zero is the first preparation identity; keep it explicit on the wire.
	Checks                          []PreparationCheckView                `json:"checks,omitempty"`
	EvaluationVersion               string                                `json:"evaluationVersion,omitempty"`
	NoRunEvaluationVersion          string                                `json:"noRunEvaluationVersion,omitempty"`
	EmptySelectionEvaluationVersion string                                `json:"emptySelectionEvaluationVersion,omitempty"`
	DirectoryEvidenceHash           string                                `json:"directoryEvidenceHash,omitempty"`
	ResourceEvidenceHash            string                                `json:"resourceEvidenceHash,omitempty"`
	ManagedConfigEvidenceRef        string                                `json:"managedConfigEvidenceRef,omitempty"`
	SoulEvidenceRef                 string                                `json:"soulEvidenceRef,omitempty"`
	SealEvidenceRef                 string                                `json:"sealEvidenceRef,omitempty"`
	SkillRegistryEvidenceRef        string                                `json:"skillRegistryEvidenceRef,omitempty"`
	SkillHubEffectRef               string                                `json:"skillHubEffectRef,omitempty"`
	SkillHubEffectState             string                                `json:"skillHubEffectState,omitempty"`
	SkillHubInstalledState          string                                `json:"skillHubInstalledState,omitempty"`
	SkillSelectionState             string                                `json:"skillSelectionState,omitempty"`
	SkillContextState               string                                `json:"skillContextState,omitempty"`
	SkillReferenceState             string                                `json:"skillReferenceState,omitempty"`
	SkillStatusState                string                                `json:"skillStatusState,omitempty"`
	ProviderCacheEvidenceRef        string                                `json:"providerCacheEvidenceRef,omitempty"`
	RunHistoryEvidenceRef           string                                `json:"runHistoryEvidenceRef,omitempty"`
	MemoryInventoryEvidenceRef      string                                `json:"memoryInventoryEvidenceRef,omitempty"`
}

type PreparationCheckView struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"`
	Required bool   `json:"required"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
}

type PreparationCheckRequest struct {
	RequestID                 string `json:"requestId"`
	ExpectedWorkspaceRevision uint64 `json:"expectedWorkspaceRevision"`
}

func (r PreparationCheckRequest) Validate() error {
	if !preparationViewIdentity(r.RequestID) || len(r.RequestID) > 256 || strings.ContainsAny(r.RequestID, "\r\n") || r.ExpectedWorkspaceRevision == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

// RequestID selects the original operation even when the view also contains
// later attempts. A lost response never authorizes silently changing identity.
type PreparationOperationView struct {
	RequestID   string                 `json:"requestId"`
	Preparation StartupPreparationView `json:"preparation"`
	// Explicit check/recover compilation only; GET remains a persisted-state
	// projection. This is not stored or consumed as a readiness credential.
	Compilation *PreparationBootstrapCompilationView `json:"compilation,omitempty"`
}

type PreparationBootstrapCompilationView struct {
	Version       string `json:"version"`
	FailureCode   string `json:"failureCode,omitempty"`
	SnapshotHash  string `json:"snapshotHash"`
	ManifestHash  string `json:"manifestHash"`
	BootstrapHash string `json:"bootstrapHash,omitempty"`
}

func (v PreparationBootstrapCompilationView) ValidateFor(a PreparationAttemptView) error {
	if v.Version != "workorch-original-prepared-bootstrap/v1" || v.SnapshotHash != a.Binding.SnapshotHash || v.ManifestHash != a.Binding.ManifestHash {
		return common.ErrConflict
	}
	if v.FailureCode == "" {
		if a.ExecutionInputVersion != "workorch-preparation-execution-inputs/v1" || a.DefinitionCompilation == nil || !preparationViewIdentity(v.BootstrapHash) || !strings.HasPrefix(v.BootstrapHash, "prepared-bootstrap-sha256:") || len(v.BootstrapHash) != len("prepared-bootstrap-sha256:")+64 {
			return common.ErrConflict
		}
		for _, c := range v.BootstrapHash[len("prepared-bootstrap-sha256:"):] {
			if c < '0' || c > '9' && c < 'a' || c > 'f' {
				return common.ErrConflict
			}
		}
		return nil
	}
	if v.BootstrapHash != "" {
		return common.ErrConflict
	}
	switch v.FailureCode {
	case "original_inputs_unavailable", "original_policy_unavailable", "soul_source_unavailable", "skill_source_unavailable", "memory_source_unavailable", "memory_semantic_evidence_required", "mcp_source_unavailable", "provider_declaration_unavailable", "activation_structure_rejected", "bootstrap_structure_rejected":
		return nil
	}
	return common.ErrConflict
}

func (v PreparationOperationView) ValidateFor(workspaceID string, request PreparationCheckRequest) error {
	if request.Validate() != nil || v.RequestID != request.RequestID || v.Preparation.ValidateFor(workspaceID) != nil || v.Preparation.Record == nil {
		return common.ErrConflict
	}
	for _, a := range v.Preparation.Record.Attempts {
		if a.RequestID == request.RequestID && a.ExpectedWorkspaceRevision == request.ExpectedWorkspaceRevision {
			if v.Compilation != nil && v.Compilation.ValidateFor(a) != nil {
				return common.ErrConflict
			}
			return nil
		}
	}
	return common.ErrConflict
}

type PreparationBindingView struct {
	InputHash                  string `json:"inputHash"`
	DerivationPermitID         string `json:"derivationPermitId"`
	ProposalHash               string `json:"proposalHash"`
	SpecificationHash          string `json:"specificationHash"`
	CandidateID                string `json:"candidateId"`
	CandidateRevision          uint64 `json:"candidateRevision"`
	CandidateHash              string `json:"candidateHash"`
	DefinitionRevision         string `json:"definitionRevision"`
	StartupInputHash           string `json:"startupInputHash"`
	AuthoringConfigurationHash string `json:"authoringConfigurationHash"`
	SnapshotHash               string `json:"snapshotHash"`
	ManifestHash               string `json:"manifestHash"`
	CheckPolicyVersion         string `json:"checkPolicyVersion"`
	CompilerVersion            string `json:"compilerVersion"`
}

func (v StartupPreparationView) ValidateFor(workspaceID string) error {
	if workspaceID == "" || v.WorkspaceID != workspaceID || v.WorkspaceRevision == 0 {
		return common.ErrConflict
	}
	if v.SkillHubProjectIssue != "" {
		if v.SkillHubProjectIssue != "project_registration_required" || v.Record == nil || v.Record.Version != "workorch-preparation-record/v2" || len(v.Record.Attempts) == 0 {
			return common.ErrConflict
		}
		a := v.Record.Attempts[len(v.Record.Attempts)-1]
		if a.State != "blocked" || a.SkillHubEffectState != "unresolved" {
			return common.ErrConflict
		}
	}
	if len(v.ResourceInitializations) > 16 {
		return common.ErrConflict
	}
	seenInit := map[string]bool{}
	for _, r := range v.ResourceInitializations {
		if r.Validate(v.WorkspaceRevision) != nil || seenInit[r.RequestID] {
			return common.ErrConflict
		}
		seenInit[r.RequestID] = true
	}
	if v.Record == nil {
		if v.OperationState != "not_checked" || v.CurrentValidity != "not_checked" {
			return common.ErrConflict
		}
		return nil
	}
	if (v.Record.Version != "workorch-preparation-record/v1" && v.Record.Version != "workorch-preparation-record/v2") || len(v.Record.Attempts) == 0 || len(v.Record.Attempts) > 64 || (v.CurrentValidity != "not_evaluated" && v.CurrentValidity != "stale" && v.CurrentValidity != "observed") {
		return common.ErrConflict
	}
	lastRevision := uint64(0)
	seen := make(map[string]bool, len(v.Record.Attempts))
	for _, a := range v.Record.Attempts {
		if a.ExecutionInputVersion != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.ExecutionInputVersion != "workorch-preparation-execution-inputs/v1" || a.SealEvidenceRef == "") {
			return common.ErrConflict
		}
		if a.EmptySelectionEvaluationVersion != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.EmptySelectionEvaluationVersion != "workorch-preparation-empty-selection-evaluation/v1") {
			return common.ErrConflict
		}
		if a.Assessment != nil && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.Assessment.ValidateFor(a.Binding.ManifestHash, a.Checks) != nil) {
			return common.ErrConflict
		}
		if a.DefinitionCompilation != nil && (v.Record.Version != "workorch-preparation-record/v2" || a.DefinitionCompilation.ValidateFor(a) != nil) {
			return common.ErrConflict
		}
		if a.NoRunEvaluationVersion != "" && (v.Record.Version != "workorch-preparation-record/v2" || !validPreparationNoRunFinalShape(a)) {
			return common.ErrConflict
		}
		if ref := a.MCPRegistryEvidenceRef; ref != "" {
			if v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || !strings.HasPrefix(ref, "mcp-registry-observation:") || len(strings.TrimPrefix(ref, "mcp-registry-observation:")) != 32 || strings.Trim(strings.TrimPrefix(ref, "mcp-registry-observation:"), "0123456789abcdef") != "" {
				return common.ErrConflict
			}
			found := false
			for _, c := range a.Checks {
				if c.ID == "mcp_selection" && c.Owner == "mcpregistry" && c.Required && (preparationMCPReasonRequiresEvidence(c.Reason) || c.Reason == "binding_changed" || c.State == "passed" && validPreparationEvaluatedCheck(a, c)) {
					found = true
				}
			}
			if !found {
				return common.ErrConflict
			}
		}
		if a.EvaluationVersion != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1") {
			return common.ErrConflict
		}
		if ref := a.RunHistoryEvidenceRef; ref != "" {
			if v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || !strings.HasPrefix(ref, "run-history-observation:") || len(strings.TrimPrefix(ref, "run-history-observation:")) != 32 || strings.Trim(strings.TrimPrefix(ref, "run-history-observation:"), "0123456789abcdef") != "" {
				return common.ErrConflict
			}
			found := false
			for _, c := range a.Checks {
				if c.ID == "first_activation_no_run" && c.Owner == "run" && c.Required && (c.State == "passed" && validPreparationNoRunFinalShape(a) || c.State == "blocked" && (c.Reason == "run_history_retired" || c.Reason == "run_history_present" || c.Reason == "run_history_empty_fence_not_final" || c.Reason == "binding_changed")) {
					found = true
				}
			}
			if !found {
				return common.ErrConflict
			}
		}
		if ref := a.ProviderCacheEvidenceRef; ref != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || !strings.HasPrefix(ref, "provider-cache-observation:") || len(strings.TrimPrefix(ref, "provider-cache-observation:")) != 32 || strings.Trim(strings.TrimPrefix(ref, "provider-cache-observation:"), "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.ProviderCacheEvidenceRef != "" {
			found := false
			for _, check := range a.Checks {
				if check.ID == "provider_cache" && check.Owner == "llm" && check.Required && (check.Reason == "provider_cache_miss" || check.Reason == "provider_cache_expired" || check.Reason == "provider_declaration_only" || check.Reason == "binding_changed" || check.Reason == "provider_configuration_observed_remaining_checks" || isPreparationProviderSelectionReason(check.Reason)) {
					found = true
				}
			}
			if !found {
				return common.ErrConflict
			}
		}
		if ref := a.MemoryInventoryEvidenceRef; ref != "" {
			if v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || !strings.HasPrefix(ref, "memory-inventory-observation:") || len(strings.TrimPrefix(ref, "memory-inventory-observation:")) != 32 || strings.Trim(strings.TrimPrefix(ref, "memory-inventory-observation:"), "0123456789abcdef") != "" {
				return common.ErrConflict
			}
			found := false
			for _, check := range a.Checks {
				if check.ID == "memory_selection" && check.Owner == "memory" && check.Required && (preparationMemoryReasonRequiresEvidence(check.Reason) || check.Reason == "binding_changed") {
					found = true
				}
			}
			if !found {
				return common.ErrConflict
			}
		}
		if a.SkillContextState != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.SkillSelectionState != "compiled_checks_pending" || a.SkillContextState != "compiled_checks_pending" && a.SkillContextState != "bounded_checks_pending" && a.SkillContextState != "rejected") {
			return common.ErrConflict
		}
		if a.SkillReferenceState != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.SkillSelectionState != "compiled_checks_pending" || a.SkillReferenceState != "observed_checks_pending" && a.SkillReferenceState != "sources_incomplete" && a.SkillReferenceState != "source_unresolved" && a.SkillReferenceState != "unavailable") {
			return common.ErrConflict
		}
		if a.SkillStatusState != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.SkillSelectionState != "compiled_checks_pending" || a.SkillStatusState != "observed_freshness_pending" && a.SkillStatusState != "sources_incomplete" && a.SkillStatusState != "status_blocked" && a.SkillStatusState != "version_conflict") {
			return common.ErrConflict
		}
		if a.SkillSelectionState != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.SkillRegistryEvidenceRef == "" || a.SkillSelectionState != "compiled_checks_pending" && a.SkillSelectionState != "sources_incomplete" && a.SkillSelectionState != "conflict" || a.SkillSelectionState == "compiled_checks_pending" && (a.SoulEvidenceRef == "" || a.SkillHubEffectState != "recorded" || a.SkillHubInstalledState != "observed_selection_pending")) {
			return common.ErrConflict
		}
		if a.SkillHubInstalledState != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.SkillHubEffectRef == "" || a.SkillHubInstalledState != "incomplete" && a.SkillHubInstalledState != "observed_selection_pending") {
			return common.ErrConflict
		}
		if a.SkillHubEffectRef == "" {
			if a.SkillHubEffectState != "" {
				return common.ErrConflict
			}
		} else {
			ref := strings.TrimPrefix(a.SkillHubEffectRef, "skillhub-status:")
			if v.Record.Version != "workorch-preparation-record/v2" || ref == a.SkillHubEffectRef || len(ref) != 32 || strings.Trim(ref, "0123456789abcdef") != "" {
				return common.ErrConflict
			}
			if a.State == "accepted" || a.State == "unknown" {
				if a.SkillHubEffectState != "reserved" {
					return common.ErrConflict
				}
			} else if a.State != "blocked" || (a.SkillHubEffectState != "recorded" && a.SkillHubEffectState != "unresolved") {
				return common.ErrConflict
			}
		}
		if a.SkillRegistryEvidenceRef != "" && (a.State != "blocked" || !strings.HasPrefix(a.SkillRegistryEvidenceRef, "skill-registry-observation:") || len(strings.TrimPrefix(a.SkillRegistryEvidenceRef, "skill-registry-observation:")) != 32 || strings.Trim(strings.TrimPrefix(a.SkillRegistryEvidenceRef, "skill-registry-observation:"), "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.SealEvidenceRef != "" && (v.Record.Version != "workorch-preparation-record/v2" || a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || !strings.HasPrefix(a.SealEvidenceRef, "seal-observation:") || len(strings.TrimPrefix(a.SealEvidenceRef, "seal-observation:")) != 32 || strings.Trim(strings.TrimPrefix(a.SealEvidenceRef, "seal-observation:"), "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.SoulEvidenceRef != "" && (a.State != "blocked" || !strings.HasPrefix(a.SoulEvidenceRef, "soul-observation:") || len(strings.TrimPrefix(a.SoulEvidenceRef, "soul-observation:")) != 32 || strings.Trim(strings.TrimPrefix(a.SoulEvidenceRef, "soul-observation:"), "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.ManagedConfigEvidenceRef != "" && (a.State != "blocked" || !strings.HasPrefix(a.ManagedConfigEvidenceRef, "managed-config-observation:") || len(strings.TrimPrefix(a.ManagedConfigEvidenceRef, "managed-config-observation:")) != 32 || strings.Trim(strings.TrimPrefix(a.ManagedConfigEvidenceRef, "managed-config-observation:"), "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.ResourceEvidenceHash != "" && (a.State != "blocked" || len(a.ResourceEvidenceHash) != 64 || strings.Trim(a.ResourceEvidenceHash, "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if a.DirectoryEvidenceHash != "" && (a.State != "blocked" || len(a.DirectoryEvidenceHash) != 64 || strings.Trim(a.DirectoryEvidenceHash, "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if !preparationViewIdentity(a.RequestID) || !preparationViewIdentity(a.PayloadHash) || a.Binding.validate() != nil || seen[a.RequestID] || a.ExpectedWorkspaceRevision == 0 || a.CommittedWorkspaceRevision <= a.ExpectedWorkspaceRevision || a.CommittedWorkspaceRevision <= lastRevision || a.CommittedWorkspaceRevision > v.WorkspaceRevision || a.StartedAt.IsZero() {
			return common.ErrConflict
		}
		seen[a.RequestID] = true
		lastRevision = a.CommittedWorkspaceRevision
		if v.Record.Version == "workorch-preparation-record/v2" {
			if len(a.CreationEpoch) != 32 || strings.Trim(a.CreationEpoch, "0123456789abcdef") != "" || len(a.Checks) == 0 || len(a.Checks) > 128 {
				return common.ErrConflict
			}
			lastID := ""
			for _, check := range a.Checks {
				if check.ID == "provider_cache" && isPreparationProviderSelectionReason(check.Reason) && (a.ProviderCacheEvidenceRef == "" || check.Owner != "llm" || !check.Required || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.State != "blocked" || check.State != "blocked") {
					return common.ErrConflict
				}
				if check.ID == "mcp_selection" && preparationMCPReasonRequiresEvidence(check.Reason) && (a.MCPRegistryEvidenceRef == "" || check.Owner != "mcpregistry" || !check.Required) {
					return common.ErrConflict
				}
				if check.ID == "mcp_selection" && isPreparationMCPSelectionReason(check.Reason) && (a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.State != "blocked" || check.State != "blocked") {
					return common.ErrConflict
				}
				if check.ID == "memory_selection" && preparationMemoryReasonRequiresEvidence(check.Reason) && a.MemoryInventoryEvidenceRef == "" {
					return common.ErrConflict
				}
				if check.ID == "skill_selection" && strings.HasPrefix(check.Reason, "skill_selection_") && (a.SkillSelectionState == "" || check.Reason != "skill_selection_"+a.SkillSelectionState) {
					return common.ErrConflict
				}
				if check.ID == "skill_selection" && check.Reason == "skill_registry_observed_selection_pending" && a.SkillRegistryEvidenceRef == "" {
					return common.ErrConflict
				}
				if check.ID == "soul_selection" && check.Reason == "souls_observed_remaining_checks" && a.SoulEvidenceRef == "" {
					return common.ErrConflict
				}
				if !preparationViewIdentity(check.ID) || !preparationViewIdentity(check.Owner) || check.ID <= lastID {
					return common.ErrConflict
				}
				lastID = check.ID
				if a.State == "accepted" || a.State == "unknown" {
					if check.State != "pending" || check.Reason != "not_observed" {
						return common.ErrConflict
					}
				} else if !validPreparationEvaluatedCheck(a, check) {
					return common.ErrConflict
				}
			}
		} else if a.CreationEpoch != "" || a.PreparationEpoch != 0 || len(a.Checks) != 0 || a.DirectoryEvidenceHash != "" || a.ResourceEvidenceHash != "" || a.ManagedConfigEvidenceRef != "" || a.SoulEvidenceRef != "" || a.SkillRegistryEvidenceRef != "" {
			return common.ErrConflict
		}
		switch a.State {
		case "accepted", "unknown":
			if a.CompletedAt != nil || a.ExpiresAt != nil {
				return common.ErrConflict
			}
		case "blocked", "failed":
			if a.CompletedAt == nil || a.CompletedAt.IsZero() || a.CompletedAt.Before(a.StartedAt) || a.ExpiresAt != nil && !a.ExpiresAt.After(*a.CompletedAt) {
				return common.ErrConflict
			}
		default:
			return common.ErrConflict
		}
	}
	if validatePreparationCompletionViews(v.Record) != nil {
		return common.ErrConflict
	}
	expected := v.Record.Attempts[len(v.Record.Attempts)-1].State
	for _, c := range v.Record.Completions {
		if c.RequestID == v.Record.Attempts[len(v.Record.Attempts)-1].RequestID {
			expected = "completed"
		}
	}
	if v.OperationState != expected || v.CurrentValidity == "observed" && expected != "completed" {
		return common.ErrConflict
	}
	return nil
}

// Public shape validation is not an owner proof validator. Positive file
// judgments require the versioned original observation reference. Only an
// independent owner completion represents historical M3; neither grants execution.
func preparationMemoryReasonRequiresEvidence(reason string) bool {
	switch reason {
	case "memory_inventory_observed_selection_pending", "memory_semantic_evidence_required", "memory_selection_candidate_limit", "memory_list_selection_observed_remaining_checks", "memory_empty_selection_observed_remaining_checks":
		return true
	}
	return false
}

func validPreparationEvaluatedCheck(a PreparationAttemptView, c PreparationCheckView) bool {
	if c.State == "blocked" {
		return validPreparationBlockedReason(c.ID, c.Reason)
	}
	if a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.State != "blocked" {
		return false
	}
	if c.ID == "first_activation_no_run" {
		return c.Owner == "run" && c.Required && a.NoRunEvaluationVersion == "workorch-preparation-no-run-final-fence/v1" && a.PreparationEpoch == 0 && a.RunHistoryEvidenceRef != "" && a.SealEvidenceRef != "" && c.State == "passed" && c.Reason == "run_history_empty_final_fence_observed"
	}
	if c.ID == "exact_seal" || c.ID == "persistence_integrity" {
		reason := "seal_observed_remaining_checks"
		if c.ID == "persistence_integrity" {
			reason = "persistence_binding_observed_remaining_checks"
		}
		return c.Owner == "workspace" && c.Required && a.SealEvidenceRef != "" && c.State == "passed" && c.Reason == reason
	}
	if c.ID == "soul_selection" {
		return c.Owner == "soul" && c.Required && a.SoulEvidenceRef != "" && c.State == "passed" && c.Reason == "souls_observed_remaining_checks"
	}
	if c.ID == "provider_cache" {
		return c.Owner == "llm" && c.Required && a.ProviderCacheEvidenceRef != "" && c.State == "passed" && c.Reason == "provider_configuration_observed_remaining_checks"
	}
	if c.ID == "skill_selection" {
		return c.Owner == "skill" && c.Required && c.State == "passed" && c.Reason == "skill_empty_selection_observed_remaining_checks" && a.EmptySelectionEvaluationVersion == "workorch-preparation-empty-selection-evaluation/v1" && a.SkillRegistryEvidenceRef != "" && a.SoulEvidenceRef != "" && a.SkillHubEffectState == "recorded" && a.SkillSelectionState == "compiled_checks_pending" && a.SkillContextState == "compiled_checks_pending" && a.SkillReferenceState == "observed_checks_pending" && a.SkillStatusState == "observed_freshness_pending"
	}
	if c.ID == "mcp_selection" {
		return c.Owner == "mcpregistry" && c.Required && c.State == "passed" && (c.Reason == "mcp_empty_selection_observed_remaining_checks" || c.Reason == "mcp_connection_observed_remaining_checks") && a.EmptySelectionEvaluationVersion == "workorch-preparation-empty-selection-evaluation/v1" && a.MCPRegistryEvidenceRef != "" && a.SkillRegistryEvidenceRef != ""
	}
	if c.ID == "memory_selection" {
		return c.Owner == "memory" && c.Required && a.MemoryInventoryEvidenceRef != "" && c.State == "passed" && (c.Reason == "memory_list_selection_observed_remaining_checks" || c.Reason == "memory_empty_selection_observed_remaining_checks")
	}
	if c.Owner != "file" {
		return false
	}
	switch c.ID {
	case "directory_boundary":
		return c.Required && a.DirectoryEvidenceHash != "" && (c.State == "passed" && c.Reason == "directory_observed_remaining_checks" || c.State == "failed" && c.Reason == "directory_permissions_denied")
	case "managed_configuration":
		return c.Required && a.ManagedConfigEvidenceRef != "" && (c.State == "passed" && c.Reason == "managed_configs_observed_remaining_checks" || c.State == "failed" && (c.Reason == "managed_config_content_invalid" || c.Reason == "managed_config_content_conflict"))
	default:
		return strings.HasPrefix(c.ID, "resource/") && a.ResourceEvidenceHash != "" && (c.State == "passed" && c.Reason == "resource_observed_remaining_checks" || c.State == "failed" && c.Reason == "resource_content_changed")
	}
}

// Public shape only; the Workspace repository independently checks the complete
// private original history, seal and actual settlement CAS preimage.
func validPreparationNoRunFinalShape(a PreparationAttemptView) bool {
	if a.NoRunEvaluationVersion != "workorch-preparation-no-run-final-fence/v1" || a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.PreparationEpoch != 0 || a.RunHistoryEvidenceRef == "" || a.SealEvidenceRef == "" || a.ExpectedWorkspaceRevision > ^uint64(0)-2 || a.CommittedWorkspaceRevision != a.ExpectedWorkspaceRevision+2 {
		return false
	}
	count := 0
	for _, c := range a.Checks {
		if c.ID == "first_activation_no_run" || c.ID == "exact_seal" || c.ID == "persistence_integrity" {
			if c.State != "passed" || !validPreparationEvaluatedCheck(a, c) {
				return false
			}
			count++
		}
	}
	return count == 3
}

func validPreparationBlockedReason(checkID, reason string) bool {
	if reason == "check_pipeline_incomplete" || reason == "original_check_interrupted" || reason == "binding_changed" {
		return true
	}
	if strings.HasPrefix(checkID, "resource/") {
		return reason == "resource_observation_unavailable" || reason == "resource_observation_incomplete" || reason == "resource_content_changed" || reason == "resource_expected_hash_unavailable" || reason == "resource_observed_remaining_checks"
	}
	switch checkID {
	case "mcp_selection":
		return reason == "mcp_registry_observation_unavailable_not_saved" || preparationMCPReasonRequiresEvidence(reason)
	case "memory_selection":
		return reason == "memory_inventory_unavailable_not_saved" || reason == "memory_inventory_observed_selection_pending" || reason == "memory_semantic_evidence_required" || reason == "memory_selection_candidate_limit"
	case "skill_selection":
		return reason == "skill_registry_observation_unavailable_not_saved" || reason == "skill_registry_observed_selection_pending" || reason == "skill_selection_sources_incomplete" || reason == "skill_selection_conflict" || reason == "skill_selection_compiled_checks_pending"
	case "managed_configuration":
		return reason == "managed_config_observation_unavailable" || reason == "managed_config_observation_incomplete" || reason == "managed_config_declaration_invalid" || reason == "managed_config_content_invalid" || reason == "managed_config_content_conflict" || reason == "managed_configs_observed_remaining_checks"
	case "soul_selection":
		return reason == "soul_observation_unavailable_not_saved" || reason == "souls_observed_remaining_checks"
	case "directory_boundary":
		return reason == "directory_observation_unavailable" || reason == "directory_observation_incomplete" || reason == "directory_permissions_denied" || reason == "directory_observed_remaining_checks"
	case "provider_cache":
		return reason == "provider_cache_unavailable" || reason == "provider_cache_miss" || reason == "provider_cache_expired" || reason == "provider_declaration_only" || isPreparationProviderSelectionReason(reason)
	case "first_activation_no_run":
		return reason == "run_history_unavailable" || reason == "run_history_retired" || reason == "run_history_present" || reason == "run_history_empty_fence_not_final"
	}
	return false
}

func (b PreparationBindingView) validate() error {
	if b.CandidateRevision == 0 {
		return common.ErrConflict
	}
	for _, value := range []string{b.InputHash, b.DerivationPermitID, b.ProposalHash, b.SpecificationHash} {
		if !preparationViewIdentity(value) || len(value) > 256 {
			return common.ErrConflict
		}
	}
	for _, value := range []string{b.CandidateID, b.CandidateHash, b.DefinitionRevision, b.StartupInputHash, b.AuthoringConfigurationHash, b.SnapshotHash, b.ManifestHash, b.CheckPolicyVersion, b.CompilerVersion} {
		if !preparationViewIdentity(value) {
			return common.ErrConflict
		}
	}
	return nil
}

func preparationViewIdentity(value string) bool {
	return value != "" && len(value) <= 512 && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0) && utf8.ValidString(value)
}

// Only non-sensitive historical completion metadata crosses HTTP.
type PreparationCompletionView struct {
	Version                    string    `json:"version"`
	CompletionID               string    `json:"completionId"`
	RequestID                  string    `json:"requestId"`
	BootstrapHash              string    `json:"bootstrapHash"`
	CreationEpoch              string    `json:"creationEpoch"`
	CompletedAt                time.Time `json:"completedAt"`
	ExpiresAt                  time.Time `json:"expiresAt"`
	CommittedWorkspaceRevision uint64    `json:"committedWorkspaceRevision"`
}

func validatePreparationCompletionViews(r *PreparationRecordView) error {
	if len(r.Completions) > len(r.Attempts) || len(r.Completions) > 0 && r.Version != "workorch-preparation-record/v2" {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	last := uint64(0)
	for _, c := range r.Completions {
		found := false
		for _, a := range r.Attempts {
			if a.RequestID == c.RequestID {
				found = a.State == "blocked" && a.NoRunEvaluationVersion == "workorch-preparation-no-run-final-fence/v1" && a.Assessment != nil && a.Assessment.Outcome == "checks_passed" && a.DefinitionCompilation != nil && a.CompletedAt != nil && c.CompletedAt.Equal(*a.CompletedAt) && c.CreationEpoch == a.CreationEpoch && a.PreparationEpoch == 0 && c.CommittedWorkspaceRevision == a.CommittedWorkspaceRevision
			}
		}
		if !found || seen[c.RequestID] || c.Version != "workorch-preparation-completion/v1" || !preparationDigest(c.CompletionID, "preparation-completion-sha256:") || !preparationDigest(c.BootstrapHash, "prepared-bootstrap-sha256:") || c.ExpiresAt.Sub(c.CompletedAt) != 30*time.Second || c.CommittedWorkspaceRevision <= last {
			return common.ErrConflict
		}
		seen[c.RequestID] = true
		last = c.CommittedWorkspaceRevision
	}
	return nil
}

func preparationDigest(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	for _, c := range value[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
