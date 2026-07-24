package workorchruntimecontract

import "testing"

func TestRuntimeCoverageDependenciesNormalizeAndRejectIncompleteBindings(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{
		{ID: "source-refreshes-validation", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation", TriggerEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindStateChange}},
	}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid coverage dependency rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	dependency := normalized[0].CoverageDependencies[0]
	if len(dependency.TriggerEvidenceKinds) != 1 || dependency.TriggerEvidenceKinds[0] != EvidenceKindStateChange {
		t.Fatalf("coverage dependency normalization = %+v", dependency)
	}
	for name, dependency := range map[string]RuntimeCoverageDependency{
		"missing trigger": {ID: "dep", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation"},
		"self reference":  {ID: "dep", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "source", TriggerEvidenceKinds: []string{EvidenceKindStateChange}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{dependency}}}); err == nil {
				t.Fatalf("invalid coverage dependency %+v was accepted", dependency)
			}
		})
	}
}

func TestValidateRuntimeProfileContractsRequiresCapabilityStateOwnerToMatchAcceptedOperationOwner(t *testing.T) {
	valid := RuntimeProfileContract{
		ID: "version-control",
		EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
			ID: "snapshot", Scope: "deliverable", AcceptedStepKinds: []string{StepKindGitWrite},
			CapabilityStateRequirements: []RuntimeProfileCapabilityStateRequirement{{
				OwnerModuleID: RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
			}},
		}},
	}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("canonical Git capability-state requirement rejected: %v", err)
	}

	invalid := valid
	invalid.EvidenceRequirements[0].CapabilityStateRequirements[0].OwnerModuleID = "git"
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("legacy non-owner capability-state destination was accepted")
	}
}

func TestRuntimeProfileTargetGroundingIsNormalizedAndRequiredWhenDeclared(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "source", Scope: "deliverable", RequireDirectEvidence: true,
		RequireTargetGrounding: true, TargetRole: "implementation", TargetRefs: []string{"b.go", "a.go", "a.go"},
		TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest, RuntimeValidationDiscoverySourceProjectFiles},
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid target grounding rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	got := normalized[0].EvidenceRequirements[0]
	if len(got.TargetRefs) != 2 || got.TargetRefs[0] != "a.go" || got.TargetRefs[1] != "b.go" || len(got.TargetDiscoverySources) != 2 {
		t.Fatalf("target grounding was not normalized: %+v", got)
	}

	invalid := valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].TargetRole = ""
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("target-grounding profile without target role was accepted")
	}
	invalid = valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].TargetRefs = nil
	invalid.EvidenceRequirements[0].TargetDiscoverySources = nil
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("target-grounding profile without refs or discovery was accepted")
	}
	invalid = valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].TargetRefs = nil
	invalid.EvidenceRequirements[0].TargetDiscoverySources = []string{"free_form_path_search"}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("target-grounding profile with an unknown discovery source was accepted")
	}
	invalid = valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].TargetRefs = nil
	invalid.EvidenceRequirements[0].TargetSelectorRef = ""
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("target-grounding profile with discovery but no owner selector was accepted")
	}
}

func TestRuntimeProfileProspectiveTargetRequiresGroundedSelectorAndAffectsEquality(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "initial-deliverable", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "deliverable",
		TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/workspace_deliverable",
		ProspectiveTargetRef: "  initial-deliverable.txt  ",
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid prospective target rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	if got := normalized[0].EvidenceRequirements[0].ProspectiveTargetRef; got != "initial-deliverable.txt" {
		t.Fatalf("prospective target normalization = %q", got)
	}
	changed := append([]RuntimeProfileContract(nil), normalized...)
	changed[0].EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), normalized[0].EvidenceRequirements...)
	changed[0].EvidenceRequirements[0].ProspectiveTargetRef = "other-deliverable.txt"
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("prospective target drift was not detected")
	}
	invalid := valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].TargetSelectorRef = ""
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("prospective target without target selector was accepted")
	}
}

func TestRuntimeProfileDeclaredSelectorCandidatesAreBoundedAndAffectEquality(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile-lsp", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "implementation", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "implementation",
		TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/profile_declared_candidates",
		TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"internal/lsp/server.go", "internal/lsp/manager.go", "internal/lsp/manager.go"},
		ProspectiveTargetRef: "internal/lsp/manager.go",
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid profile-declared selector candidates rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	requirement := normalized[0].EvidenceRequirements[0]
	if requirement.TargetSelectorOwnerRef != "" || requirement.TargetSelectorSubjectRef != "implementation" || !SameRuntimeTargetSelectorCandidateRefs(requirement.TargetSelectorCandidateRefs, []string{"internal/lsp/manager.go", "internal/lsp/server.go"}) {
		t.Fatalf("profile-declared selector candidate normalization = %+v", requirement)
	}
	changed := append([]RuntimeProfileContract(nil), normalized...)
	changed[0].EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), normalized[0].EvidenceRequirements...)
	changed[0].EvidenceRequirements[0].TargetSelectorCandidateRefs = []string{"internal/lsp/other.go"}
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("profile-declared selector candidate drift was not detected")
	}
	for name, mutate := range map[string]func(*RuntimeProfileEvidenceRequirement){
		"missing selector subject":              func(item *RuntimeProfileEvidenceRequirement) { item.TargetSelectorSubjectRef = "" },
		"prospective target outside candidates": func(item *RuntimeProfileEvidenceRequirement) { item.ProspectiveTargetRef = "internal/lsp/other.go" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := valid
			invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
			mutate(&invalid.EvidenceRequirements[0])
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
				t.Fatalf("invalid selector declaration was accepted: %+v", invalid.EvidenceRequirements[0])
			}
		})
	}
}
