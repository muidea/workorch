package workorchruntimecontract

import "testing"

func TestRuntimeOwnerFeasibilityObservationSeparatesOwnerFactsFromCandidateCorrection(t *testing.T) {
	observation, err := RuntimeOwnerFeasibilityObservationFromFailure(RuntimeModuleShell, &FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "validation_target_missing",
		Signature: "shell/validation-target-missing", Message: "no validation target exists", TargetRefs: []string{"main_test.go"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Disposition != RuntimeOwnerFeasibilityInfeasible || observation.Category != RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		t.Fatalf("observation = %+v", observation)
	}
	if _, declared := RuntimeCandidateRejectionCodeDisposition(observation.Code); declared {
		t.Fatalf("owner feasibility code %q must not be a candidate correction code", observation.Code)
	}
}

func TestRuntimeOwnerFeasibilityCategoryForIsTotalForDeclaredOwnerCodes(t *testing.T) {
	cases := []struct {
		owner string
		code  string
		want  RuntimeOwnerFeasibilityCategory
	}{
		{RuntimeModuleFile, "operation_target_missing", RuntimeOwnerFeasibilityCategoryResourceAbsent},
		{RuntimeModuleFile, "operation_admissibility_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleFile, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "operation_replay_proof_invalid", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "operation_replay_proof_unavailable", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "operation_target_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "operation_target_not_admissible", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "operation_target_not_regular", RuntimeOwnerFeasibilityCategoryTargetNotRegular},
		{RuntimeModuleFile, "operation_target_unreadable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleFile, "operation_base_hash_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleFile, "operation_base_hash_mismatch", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "patch_invalid_path", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "patch_type_unsupported", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "patch_parent_conflict", RuntimeOwnerFeasibilityCategoryTargetNotRegular},
		{RuntimeModuleFile, "patch_read_failed", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleFile, "patch_binary_target", RuntimeOwnerFeasibilityCategoryTargetNotRegular},
		{RuntimeModuleFile, "patch_basehash_mismatch", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "patch_write_failed", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleFile, "patch_target_missing", RuntimeOwnerFeasibilityCategoryResourceAbsent},
		{RuntimeModuleFile, "patch_target_exists", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "patch_text_replace_conflict", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "patch_unified_diff_conflict", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "patch_invalid_input", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "unified_diff_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "unified_diff_target_mismatch", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleFile, "unified_diff_apply_conflict", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleFile, "unified_diff_infeasible", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleGit, "git_repository_unavailable", RuntimeOwnerFeasibilityCategoryResourceAbsent},
		{RuntimeModuleGit, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleGit, "operation_admissibility_binding_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleGit, "git_operation_contract_missing", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleGit, "git_operation_variant_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleGit, "git_operation_variant_not_admissible", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "validation_target_missing", RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable},
		{RuntimeModuleShell, "shell_operation_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "shell_command_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "shell_working_directory_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "shell_working_directory_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleShell, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleShell, "shell_executable_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleShell, "shell_command_policy_rejected", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "shell_validation_contract_rejected", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleShell, "shell_command_timeout_policy_rejected", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleMCP, "mcp_server_not_admissible", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleMCP, "mcp_server_catalog_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleMCP, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleMCP, "mcp_operation_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleMCP, "mcp_prerequisite_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleSSH, "ssh_prerequisite_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleSSH, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleSSH, "ssh_operation_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
		{RuntimeModuleWeb, "web_operation_infeasible", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleWeb, "web_provider_configuration_unavailable", RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable},
		{RuntimeModuleWeb, "operation_admissibility_stale", RuntimeOwnerFeasibilityCategoryObservationUnknown},
		{RuntimeModuleWeb, "web_operation_invalid", RuntimeOwnerFeasibilityCategoryPolicyDisallowed},
	}
	for _, tc := range cases {
		got, err := RuntimeOwnerFeasibilityCategoryFor(tc.owner, tc.code)
		if err != nil || got != tc.want {
			t.Fatalf("category owner=%s code=%s got=%q err=%v want=%q", tc.owner, tc.code, got, err, tc.want)
		}
	}
	if _, err := RuntimeOwnerFeasibilityCategoryFor(RuntimeModuleFile, "new_unmapped_code"); err == nil {
		t.Fatal("unknown owner code accepted")
	}
	if _, err := RuntimeOwnerFeasibilityCategoryFor(RuntimeModuleShell, "operation_target_missing"); err == nil {
		t.Fatal("code accepted for wrong owner")
	}
}

func TestRuntimeOwnerFeasibilityObservationValidatesExclusiveOutcomes(t *testing.T) {
	observation := RuntimeOwnerFeasibilityObservation{Disposition: RuntimeOwnerFeasibilityFeasible, Code: "must-not-appear"}
	if err := observation.Validate(); err == nil {
		t.Fatal("feasible observation accepted failure detail")
	}
}
