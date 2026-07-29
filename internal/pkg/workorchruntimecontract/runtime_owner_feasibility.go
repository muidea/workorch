package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOwnerFeasibilityDisposition is the complete, factual outcome of a
// side-effect-free capability preflight. It is deliberately distinct from a
// RuntimeCandidateFailure: capability owners report facts, while the
// Orchestrator alone chooses the next authority or terminal state.
type RuntimeOwnerFeasibilityDisposition string

const (
	RuntimeOwnerFeasibilityFeasible             RuntimeOwnerFeasibilityDisposition = "feasible"
	RuntimeOwnerFeasibilityStale                RuntimeOwnerFeasibilityDisposition = "stale"
	RuntimeOwnerFeasibilityPrerequisiteRequired RuntimeOwnerFeasibilityDisposition = "prerequisite_required"
	RuntimeOwnerFeasibilityInfeasible           RuntimeOwnerFeasibilityDisposition = "infeasible"
)

// RuntimeOwnerFeasibilityCategory is an owner-neutral taxonomy. Owner codes
// retain diagnostic precision but never enter the candidate correction matrix.
type RuntimeOwnerFeasibilityCategory string

const (
	RuntimeOwnerFeasibilityCategoryResourceAbsent              RuntimeOwnerFeasibilityCategory = "resource_absent"
	RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable RuntimeOwnerFeasibilityCategory = "validation_recipe_unavailable"
	RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable      RuntimeOwnerFeasibilityCategory = "environment_unavailable"
	RuntimeOwnerFeasibilityCategoryPolicyDisallowed            RuntimeOwnerFeasibilityCategory = "policy_disallowed"
	RuntimeOwnerFeasibilityCategoryTargetNotRegular            RuntimeOwnerFeasibilityCategory = "target_not_regular"
	RuntimeOwnerFeasibilityCategoryObservationUnknown          RuntimeOwnerFeasibilityCategory = "observation_unknown"
)

// RuntimeOwnerFeasibilityObservation is a pure persisted value. It carries no
// Runtime action, retry, terminal, governance or aggregate mutation fields.
type RuntimeOwnerFeasibilityObservation struct {
	Disposition         RuntimeOwnerFeasibilityDisposition
	Category            RuntimeOwnerFeasibilityCategory
	Code                string
	Signature           string
	Reason              string
	TargetRefs          []string
	Stale               *RuntimePrerequisiteStale
	PrerequisiteRequest *RuntimeOperationPrerequisiteRequest
}

func (o RuntimeOwnerFeasibilityObservation) Normalized() RuntimeOwnerFeasibilityObservation {
	o.Disposition = RuntimeOwnerFeasibilityDisposition(strings.TrimSpace(string(o.Disposition)))
	o.Category = RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(o.Category)))
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	o.Reason = strings.TrimSpace(o.Reason)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	if o.Stale != nil {
		copyValue := o.Stale.Normalized()
		o.Stale = &copyValue
	}
	if o.PrerequisiteRequest != nil {
		copyValue := o.PrerequisiteRequest.Normalized()
		o.PrerequisiteRequest = &copyValue
	}
	return o
}

func (o RuntimeOwnerFeasibilityObservation) Validate() error {
	o = o.Normalized()
	switch o.Disposition {
	case RuntimeOwnerFeasibilityFeasible:
		if o.Category != "" || o.Code != "" || o.Signature != "" || o.Reason != "" || len(o.TargetRefs) != 0 || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("feasible owner feasibility observation cannot carry failure detail")
		}
	case RuntimeOwnerFeasibilityStale:
		if o.Stale == nil || o.Category != "" || o.Code != "" || o.Signature != "" || o.PrerequisiteRequest != nil {
			return fmt.Errorf("stale owner feasibility observation requires only stale detail")
		}
		if err := o.Stale.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityPrerequisiteRequired:
		if o.PrerequisiteRequest == nil || o.Code == "" || o.Signature == "" || o.Reason == "" || o.Category == "" || o.Stale != nil {
			return fmt.Errorf("prerequisite-required owner feasibility observation requires category, failure identity and prerequisite")
		}
		if err := o.PrerequisiteRequest.Validate(); err != nil {
			return err
		}
	case RuntimeOwnerFeasibilityInfeasible:
		if o.Category == "" || o.Code == "" || o.Signature == "" || o.Reason == "" || o.Stale != nil || o.PrerequisiteRequest != nil {
			return fmt.Errorf("infeasible owner feasibility observation requires category and failure identity")
		}
	default:
		return fmt.Errorf("unknown owner feasibility disposition %q", o.Disposition)
	}
	return nil
}

func (o RuntimeOwnerFeasibilityObservation) Clone() RuntimeOwnerFeasibilityObservation {
	return o.Normalized()
}

// RuntimeOwnerFeasibilityObservationFromFailure is the only conversion from
// an owner FailureFact into the neutral observation contract. It is pure and
// intentionally does not derive a candidate correction directive.
func RuntimeOwnerFeasibilityObservationFromFailure(ownerModuleID string, failure *FailureFact, stale *RuntimePrerequisiteStale, prerequisiteRequest *RuntimeOperationPrerequisiteRequest) (RuntimeOwnerFeasibilityObservation, error) {
	if stale != nil {
		observation := RuntimeOwnerFeasibilityObservation{Disposition: RuntimeOwnerFeasibilityStale, Stale: stale}
		return observation, observation.Validate()
	}
	if failure == nil {
		return RuntimeOwnerFeasibilityObservation{}, fmt.Errorf("owner feasibility requires failure, stale, or feasible disposition")
	}
	category, err := RuntimeOwnerFeasibilityCategoryFor(ownerModuleID, failure.Code)
	if err != nil {
		return RuntimeOwnerFeasibilityObservation{}, err
	}
	disposition := RuntimeOwnerFeasibilityInfeasible
	if prerequisiteRequest != nil {
		disposition = RuntimeOwnerFeasibilityPrerequisiteRequired
	}
	observation := RuntimeOwnerFeasibilityObservation{
		Disposition: disposition, Category: category, Code: failure.Code, Signature: failure.Signature,
		Reason: failure.Message, TargetRefs: append([]string(nil), failure.TargetRefs...), PrerequisiteRequest: prerequisiteRequest,
	}
	return observation, observation.Validate()
}

// RuntimeOwnerFeasibilityCategoryFor is the exhaustive, pure taxonomy for
// facts emitted by capability owners. It deliberately takes the owner ID: a
// code is only meaningful in the vocabulary of the block that emitted it.
//
// Callers at the owner result boundary must reject an unknown pair. Returning
// observation_unknown for a known code is permitted only for a fact that has
// no more precise owner-neutral category; silently accepting a new string is
// not. This keeps vocabulary drift out of the Orchestrator state machine.
func RuntimeOwnerFeasibilityCategoryFor(ownerModuleID, code string) (RuntimeOwnerFeasibilityCategory, error) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	code = strings.TrimSpace(code)
	if ownerModuleID == "" || code == "" {
		return "", fmt.Errorf("owner feasibility category requires owner and code")
	}

	category, found := runtimeOwnerFeasibilityCategories[ownerModuleID+"/"+code]
	if !found {
		return "", fmt.Errorf("owner feasibility code %q is not declared for owner %q", code, ownerModuleID)
	}
	return category, nil
}

// This table is contract data rather than a transition table. It is kept in
// pkg because it is an immutable DTO vocabulary used to validate typed owner
// results; it does not select a Runtime action or mutate an aggregate.
var runtimeOwnerFeasibilityCategories = map[string]RuntimeOwnerFeasibilityCategory{
	// File
	RuntimeModuleFile + "/operation_admissibility_unavailable": RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleFile + "/operation_admissibility_stale":       RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/operation_replay_proof_invalid":      RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/operation_replay_proof_unavailable":  RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/operation_target_missing":            RuntimeOwnerFeasibilityCategoryResourceAbsent,
	RuntimeModuleFile + "/operation_target_invalid":            RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/operation_target_not_admissible":     RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/operation_target_not_regular":        RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	RuntimeModuleFile + "/operation_target_unreadable":         RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleFile + "/operation_base_hash_unavailable":     RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleFile + "/operation_base_hash_mismatch":        RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/patch_invalid_path":                  RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/patch_type_unsupported":              RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/patch_parent_conflict":               RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	RuntimeModuleFile + "/patch_read_failed":                   RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleFile + "/patch_binary_target":                 RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	RuntimeModuleFile + "/patch_basehash_mismatch":             RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/patch_write_failed":                  RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleFile + "/patch_target_missing":                RuntimeOwnerFeasibilityCategoryResourceAbsent,
	RuntimeModuleFile + "/patch_target_exists":                 RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/patch_text_replace_conflict":         RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/patch_unified_diff_conflict":         RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/patch_invalid_input":                 RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/unified_diff_invalid":                RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/unified_diff_target_mismatch":        RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleFile + "/unified_diff_apply_conflict":         RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleFile + "/unified_diff_infeasible":             RuntimeOwnerFeasibilityCategoryObservationUnknown,

	// Git
	RuntimeModuleGit + "/operation_admissibility_stale":               RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleGit + "/operation_admissibility_binding_unavailable": RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleGit + "/git_repository_unavailable":                  RuntimeOwnerFeasibilityCategoryResourceAbsent,
	RuntimeModuleGit + "/git_operation_contract_missing":              RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleGit + "/git_operation_variant_invalid":               RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleGit + "/git_operation_variant_not_admissible":        RuntimeOwnerFeasibilityCategoryPolicyDisallowed,

	// Shell
	RuntimeModuleShell + "/shell_operation_invalid":               RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/shell_command_invalid":                 RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/shell_working_directory_invalid":       RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/shell_working_directory_unavailable":   RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleShell + "/operation_admissibility_stale":         RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleShell + "/shell_executable_unavailable":          RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleShell + "/shell_command_policy_rejected":         RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/shell_validation_contract_rejected":    RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/shell_command_timeout_policy_rejected": RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleShell + "/validation_target_missing":             RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,

	// MCP
	RuntimeModuleMCP + "/mcp_server_catalog_unavailable": RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleMCP + "/operation_admissibility_stale":  RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleMCP + "/mcp_operation_invalid":          RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleMCP + "/mcp_server_not_admissible":      RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleMCP + "/mcp_prerequisite_unavailable":   RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,

	// SSH
	RuntimeModuleSSH + "/operation_admissibility_stale": RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleSSH + "/ssh_operation_invalid":         RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleSSH + "/ssh_prerequisite_unavailable":  RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,

	// Web
	RuntimeModuleWeb + "/web_provider_configuration_unavailable": RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	RuntimeModuleWeb + "/operation_admissibility_stale":          RuntimeOwnerFeasibilityCategoryObservationUnknown,
	RuntimeModuleWeb + "/web_operation_invalid":                  RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	RuntimeModuleWeb + "/web_operation_infeasible":               RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
}
