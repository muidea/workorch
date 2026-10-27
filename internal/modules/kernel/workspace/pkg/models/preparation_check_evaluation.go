package workspace

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationCheckEvaluationVersion = "workorch-preparation-check-evaluation/v1"

// PreparationFileSubjectHash seals only exact requirement identity, not a
// successful observation. Never pass configuration bytes or secrets here.
func PreparationFileSubjectHash(id, internal, deliverable, subject, expected string) string {
	raw, _ := json.Marshal([]string{id, internal, deliverable, subject, expected})
	return fileobservation.Digest(raw)
}

func preparationFileCheck(id string) bool {
	return id == "directory_boundary" || id == "managed_configuration" || strings.HasPrefix(id, "resource/")
}

func validPreparationCheckSubject(version string, c WorkspacePreparationCheckResult) bool {
	if version == "" || !preparationFileCheck(c.ID) {
		return c.SubjectHash == ""
	}
	return preparationFenceHex(c.SubjectHash, 32) && c.Owner == "file" && (strings.HasPrefix(c.ID, "resource/") || c.Required)
}

// Structural/public shape only. Full persistence additionally reproduces every
// File/Soul/Memory judgment from the complete archived raw material below. No generic PASS
// flag, checksum hit, unavailable input, policy fallback or N/A is accepted.
func ValidPreparationEvaluatedCheck(a WorkspacePreparationAttempt, c WorkspacePreparationCheckResult) bool {
	if c.State == "blocked" {
		return ValidPreparationBlockedReason(c.ID, c.Reason)
	}
	if a.EvaluationVersion != PreparationCheckEvaluationVersion || a.State != WorkspacePreparationBlocked {
		return false
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
	if c.ID == "skill_selection" {
		return c.Owner == "skill" && c.Required && c.State == "passed" && c.Reason == PreparationEmptySkillReason && a.EmptySelectionEvaluationVersion == PreparationEmptySelectionEvaluationVersion && a.SkillRegistryEvidenceRef != "" && a.SoulEvidenceRef != "" && a.SkillHubEffectState == PreparationSkillHubRecorded && a.SkillSelectionState == "compiled_checks_pending" && a.SkillContextState == "compiled_checks_pending" && a.SkillReferenceState == "observed_checks_pending" && a.SkillStatusState == "observed_freshness_pending"
	}
	if c.ID == "mcp_selection" {
		return c.Owner == "mcpregistry" && c.Required && c.State == "passed" && c.Reason == PreparationEmptyMCPReason && a.EmptySelectionEvaluationVersion == PreparationEmptySelectionEvaluationVersion && a.MCPRegistryEvidenceRef != "" && a.SkillRegistryEvidenceRef != ""
	}
	if c.ID == "first_activation_no_run" {
		return c.Owner == "run" && c.Required && a.NoRunEvaluationVersion == PreparationNoRunFinalVersion && a.PreparationEpoch == 0 && a.RunHistoryEvidenceRef != "" && a.SealEvidenceRef != "" && c.State == "passed" && c.Reason == PreparationNoRunFinalReason
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

// EvaluatePreparationChecks adds the complete exact Soul double-read consumer.
// Missing/invalid sources never create a PASS or alter another owner's result.
func EvaluatePreparationChecks(a WorkspacePreparationAttempt, d *WorkspacePreparationDirectoryEvidence, r *WorkspacePreparationResourceEvidence, c *WorkspacePreparationManagedConfigEvidence, s *WorkspacePreparationSoulEvidence) []WorkspacePreparationCheckResult {
	checks := EvaluatePreparationFileChecks(a, d, r, c)
	if a.EvaluationVersion != PreparationCheckEvaluationVersion || a.State != WorkspacePreparationBlocked {
		return checks
	}
	for i, check := range checks {
		if check.ID != "soul_selection" || check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" {
			continue
		}
		state := WorkspacePreparationCheckState("blocked")
		reason := check.Reason
		if s != nil && s.Validate() == nil && s.Ref == a.SoulEvidenceRef && s.Facts.SnapshotHash == a.ObservationSnapshotHash && s.Facts.ConfigurationHash == a.Binding.AuthoringConfigurationHash && s.Facts.WorkspaceRevision == a.ExpectedWorkspaceRevision+1 && s.RequestID == a.RequestID && s.PayloadHash == a.PayloadHash {
			reason = "souls_observed_remaining_checks"
			if completePreparationSoulSelection(s.Facts) {
				state = "passed"
			}
		} else if check.State != "blocked" {
			reason = "check_pipeline_incomplete"
		}
		checks[i].State, checks[i].Reason = state, reason
	}
	return checks
}

func completePreparationSoulSelection(f PreparationSoulFacts) bool {
	// An accurately observed unbound stage is still an unresolved requirement,
	// not a default policy granting PASS/N/A. Validate checks the exact six-stage
	// plan and double read before this completeness gate is reached.
	for _, selection := range f.Selections {
		if selection.SoulID == "" {
			return false
		}
	}
	return len(f.Selections) == 6
}

// EvaluatePreparationFileChecks is a detached, pure operation. Other owner
// checks remain unchanged. A file PASS means this original point-in-time read
// satisfied its exact requirement; not future currentness, all-of, ready or M3.
func EvaluatePreparationFileChecks(a WorkspacePreparationAttempt, directory *WorkspacePreparationDirectoryEvidence, resources *WorkspacePreparationResourceEvidence, configs *WorkspacePreparationManagedConfigEvidence) []WorkspacePreparationCheckResult {
	checks := append([]WorkspacePreparationCheckResult(nil), a.Checks...)
	if a.EvaluationVersion != PreparationCheckEvaluationVersion || a.State != WorkspacePreparationBlocked {
		return checks
	}
	for i, c := range checks {
		if !preparationFileCheck(c.ID) || c.Reason == "binding_changed" || c.Reason == "original_check_interrupted" {
			continue
		}
		// All three file sources must agree on the original directories. The
		// accepted requirement hashes independently pin these paths; an archive
		// cannot substitute a sibling with identical bytes and rehash it.
		state, reason := WorkspacePreparationCheckState("blocked"), c.Reason
		switch c.ID {
		case "directory_boundary":
			if directory != nil && directory.Validate() == nil && directory.ContentHash == a.DirectoryEvidenceHash && c.SubjectHash == PreparationFileSubjectHash(c.ID, directory.Internal.Path, directory.Deliverable.Path, directory.Deliverable.Path, "") {
				reason = directory.BlockedReason()
				if reason == "directory_observed_remaining_checks" {
					state = "passed"
				}
				if reason == "directory_permissions_denied" {
					state = "failed"
				}
			}
		case "managed_configuration":
			// The accepted requirement uses both original directory roots. A
			// config read without the independent directory identity stays blocked.
			if directory != nil && configs != nil && configs.Validate() == nil && configs.Ref == a.ManagedConfigEvidenceRef && configs.InternalDirectory == directory.Internal.Path && c.SubjectHash == PreparationFileSubjectHash(c.ID, configs.InternalDirectory, directory.Deliverable.Path, configs.InternalDirectory, "") {
				reason = configs.BlockedReason()
				if reason == "managed_configs_observed_remaining_checks" {
					state = "passed"
				}
				if reason == "managed_config_content_invalid" || reason == "managed_config_content_conflict" {
					state = "failed"
				}
			}
		default:
			if directory != nil && resources != nil && resources.Validate() == nil && resources.ContentHash == a.ResourceEvidenceHash && resources.DeliverableDirectory == directory.Deliverable.Path {
				for _, r := range resources.Resources {
					if c.ID == "resource/"+r.ID && c.Required == r.Required && c.SubjectHash == PreparationFileSubjectHash(c.ID, directory.Internal.Path, resources.DeliverableDirectory, r.Observation.Ref, r.ExpectedContentHash) {
						reason = r.BlockedReason()
						if reason == "resource_observed_remaining_checks" {
							state = "passed"
						}
						if reason == "resource_content_changed" {
							state = "failed"
						}
					}
				}
			}
		}
		// Missing/mismatched material must not preserve a caller-authored PASS.
		if state == "blocked" && c.State != "blocked" {
			reason = "check_pipeline_incomplete"
		}
		checks[i].State, checks[i].Reason = state, reason
	}
	return checks
}

func validatePreparationCheckEvaluationPersistence(w *Workspace) error {
	if w == nil {
		return common.ErrConflict
	}
	if w.StartupPreparation == nil {
		return nil
	}
	for _, a := range w.StartupPreparation.Attempts {
		if a.EvaluationVersion == "" || a.State != WorkspacePreparationBlocked {
			continue
		}
		var d *WorkspacePreparationDirectoryEvidence
		var r *WorkspacePreparationResourceEvidence
		var c *WorkspacePreparationManagedConfigEvidence
		var s *WorkspacePreparationSoulEvidence
		var m *WorkspacePreparationMemoryInventoryEvidence
		for i := range w.PreparationDirectoryEvidence {
			if w.PreparationDirectoryEvidence[i].ContentHash == a.DirectoryEvidenceHash {
				d = &w.PreparationDirectoryEvidence[i]
			}
		}
		for i := range w.PreparationResourceEvidence {
			if w.PreparationResourceEvidence[i].ContentHash == a.ResourceEvidenceHash {
				r = &w.PreparationResourceEvidence[i]
			}
		}
		for i := range w.PreparationManagedConfigEvidence {
			if w.PreparationManagedConfigEvidence[i].Ref == a.ManagedConfigEvidenceRef {
				c = &w.PreparationManagedConfigEvidence[i]
			}
		}
		for i := range w.PreparationSoulEvidence {
			if w.PreparationSoulEvidence[i].Ref == a.SoulEvidenceRef {
				s = &w.PreparationSoulEvidence[i]
			}
		}
		for i := range w.PreparationMemoryInventoryEvidence {
			if w.PreparationMemoryInventoryEvidence[i].Ref == a.MemoryInventoryEvidenceRef {
				m = &w.PreparationMemoryInventoryEvidence[i]
			}
		}
		copy := a
		copy.Checks = EvaluatePreparationChecks(a, d, r, c, s)
		copy.Checks = EvaluatePreparationMemoryCheck(copy, m)
		if !reflect.DeepEqual(a.Checks, EvaluatePreparationSealChecks(w, copy)) {
			return common.ErrConflict
		}
	}
	return nil
}
