package workorchruntimecontract

import "testing"

func TestRuntimeProfileEvidenceRequirementNormalizationPreservesCardinalityAndRejectsDuplicateScopeID(t *testing.T) {
	profile := RuntimeProfileContract{ID: "software-dev", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{
		{ID: " validation ", Scope: " dodItem ", Description: "first", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}},
		{ID: "source", Scope: "deliverable", Description: "other", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}},
		{ID: "validation", Scope: "dodItem", Description: "second", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}},
	}}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{profile})
	if len(normalized) != 1 || len(normalized[0].EvidenceRequirements) != len(profile.EvidenceRequirements) {
		t.Fatalf("normalization changed evidence requirement cardinality: %+v", normalized)
	}
	got := normalized[0].EvidenceRequirements
	if got[0].Scope != "deliverable" || got[0].ID != "source" || got[1].Description != "first" || got[2].Description != "second" {
		t.Fatalf("normalization did not stably order requirements while preserving duplicates: %+v", got)
	}
	if err := ValidateRuntimeProfileContracts(normalized); err == nil {
		t.Fatal("duplicate normalized Profile evidence requirement scope+id was accepted")
	}
}

func TestRuntimeProfileRequiredCWIsRequireExactOwnerReceiptProofClosure(t *testing.T) {
	valid := RuntimeProfileContract{
		ID: "software-dev",
		DeliverableExpectations: []RuntimeDeliverableExpectation{{
			ID: "source_change", Required: true, EvidenceRole: "primary_state_change",
		}},
		EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
			ID: "source_change", Scope: "deliverable", RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			AcceptedStepKinds: []string{StepKindFileWrite},
		}},
	}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid profile proof closure rejected: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeProfileContract){
		"missing exact declaration": func(profile *RuntimeProfileContract) { profile.EvidenceRequirements = nil },
		"wildcard cannot replace exact declaration": func(profile *RuntimeProfileContract) {
			profile.EvidenceRequirements[0].ID = "*"
		},
		"missing evidence kind": func(profile *RuntimeProfileContract) {
			profile.EvidenceRequirements[0].RequiredEvidenceKinds = nil
		},
		"unsupported evidence kind": func(profile *RuntimeProfileContract) {
			profile.EvidenceRequirements[0].RequiredEvidenceKinds = []string{"claim"}
		},
		"operation cannot attest proof": func(profile *RuntimeProfileContract) {
			profile.EvidenceRequirements[0].AcceptedStepKinds = []string{StepKindFileRead}
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := valid
			invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
			mutate(&invalid)
			violations := RuntimeProfileProofClosureViolations([]RuntimeProfileContract{invalid})
			if len(violations) == 0 || violations[0].SourceRef() != "profile/software-dev/deliverable/source_change" {
				t.Fatalf("proof closure violations = %+v", violations)
			}
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
				t.Fatal("invalid profile proof closure was accepted")
			}
		})
	}

	duplicate := valid
	duplicate.ID = "second-profile"
	violations := RuntimeProfileProofClosureViolations([]RuntimeProfileContract{valid, duplicate})
	if len(violations) != 2 || violations[0].Rule != "duplicate_required_profile_source" || violations[1].Rule != "duplicate_required_profile_source" {
		t.Fatalf("duplicate required Profile source violations = %+v", violations)
	}

	invalidVariant := valid
	invalidVariant.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalidVariant.EvidenceRequirements[0].AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "commit"}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalidVariant}); err == nil {
		t.Fatal("operation variant outside accepted operation kinds was accepted")
	}
}

func TestRuntimeCoverageDependenciesNormalizeAndRejectIncompleteBindings(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{
		{ID: "source-precedes-and-refreshes-validation", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation", RequireSourceSettled: true, TriggerEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindStateChange}},
	}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid coverage dependency rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	dependency := normalized[0].CoverageDependencies[0]
	if !dependency.RequireSourceSettled || len(dependency.TriggerEvidenceKinds) != 1 || dependency.TriggerEvidenceKinds[0] != EvidenceKindStateChange {
		t.Fatalf("coverage dependency normalization = %+v", dependency)
	}
	for name, dependency := range map[string]RuntimeCoverageDependency{
		"missing semantics": {ID: "dep", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation"},
		"self reference":    {ID: "dep", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "source", TriggerEvidenceKinds: []string{EvidenceKindStateChange}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{dependency}}}); err == nil {
				t.Fatalf("invalid coverage dependency %+v was accepted", dependency)
			}
		})
	}
}

func TestRuntimeCoverageDependencyAdmissionAndInvalidationAreIndependent(t *testing.T) {
	base := RuntimeCoverageDependency{ID: "source-validation", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation"}
	for name, dependency := range map[string]RuntimeCoverageDependency{
		"admission only": func() RuntimeCoverageDependency { value := base; value.RequireSourceSettled = true; return value }(),
		"invalidation only": func() RuntimeCoverageDependency {
			value := base
			value.TriggerEvidenceKinds = []string{EvidenceKindStateChange}
			return value
		}(),
		"combined": func() RuntimeCoverageDependency {
			value := base
			value.RequireSourceSettled = true
			value.TriggerEvidenceKinds = []string{EvidenceKindStateChange}
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{dependency}}}); err != nil {
				t.Fatalf("valid dependency rejected: %v", err)
			}
		})
	}

	admission := base
	admission.RequireSourceSettled = true
	invalidation := base
	invalidation.TriggerEvidenceKinds = []string{EvidenceKindStateChange}
	if SameRuntimeProfileContracts(
		[]RuntimeProfileContract{{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{admission}}},
		[]RuntimeProfileContract{{ID: "profile", CoverageDependencies: []RuntimeCoverageDependency{invalidation}}},
	) {
		t.Fatal("admission and invalidation dependencies were treated as aliases")
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

func TestRuntimeProfileOwnerFactRequirementIsNormalizedBoundedAndCompared(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: " blocks/shell ", FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: " validation/managed_session ",
	}
	valid := RuntimeProfileContract{
		ID: "software-dev",
		EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
			ID: "validation_preparation", Scope: "deliverable",
			TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation_preparation",
			AcceptedStepKinds:  []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession},
			RequiredOwnerFacts: []RuntimeOwnerFactRequirement{requirement},
			OwnerFactConvergences: []RuntimeOwnerFactConvergence{{
				Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
				ProspectiveSatisfactionContractRef: "shell/managed-validation/workspace-manifest-v1",
				AcquisitionOwnerModuleID:           RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
				AcquisitionTargetPolicy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "workspace-relative-new-file-target",
				TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation_preparation",
				ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface, MaxAcquisitionAttempts: 1,
			}},
		}},
	}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid profile owner-fact requirement rejected: %v", err)
	}
	selectorDrift := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})[0]
	selectorDrift.EvidenceRequirements[0].OwnerFactConvergences[0].TargetSelectorSubjectRef = "different-definition-subject"
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{selectorDrift}); err == nil {
		t.Fatal("owner-fact acquisition selector drift from its Profile declaration was accepted")
	}
	defaultedSelector := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})[0]
	defaultedSelector.EvidenceRequirements[0].TargetSelectorOwnerRef = ""
	defaultedSelector.EvidenceRequirements[0].TargetSelectorSubjectRef = ""
	defaultedSelector.EvidenceRequirements[0].OwnerFactConvergences[0].TargetSelectorSubjectRef = "deliverable"
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{defaultedSelector}); err != nil {
		t.Fatalf("deterministically defaulted Profile selector rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	got := normalized[0].EvidenceRequirements[0].RequiredOwnerFacts
	if len(got) != 1 || got[0] != (RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}) {
		t.Fatalf("profile owner-fact normalization = %+v", got)
	}

	changed := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{normalized[0]})
	changed[0].EvidenceRequirements[0].RequiredOwnerFacts = nil
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("profile owner-fact drift was not detected")
	}

	invalid := valid
	invalid.EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), valid.EvidenceRequirements...)
	invalid.EvidenceRequirements[0].AcceptedStepKinds = []string{StepKindFileWrite}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
		t.Fatal("profile owner-fact operation outside accepted kinds was accepted")
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

func TestRuntimeProfileDeclaredSelectorCandidatesAreBoundedAndAffectEquality(t *testing.T) {
	valid := RuntimeProfileContract{ID: "profile-lsp", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "implementation", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "implementation",
		TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/profile_declared_candidates",
		TargetSelectorOwnerRef: "profile-lsp", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"internal/lsp/server.go", "internal/lsp/manager.go", "internal/lsp/manager.go"},
		ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector", "workspace-relative-path"},
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatalf("valid profile-declared selector candidates rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	requirement := normalized[0].EvidenceRequirements[0]
	if requirement.TargetSelectorOwnerRef != "profile-lsp" || requirement.TargetSelectorSubjectRef != "implementation" || !SameRuntimeTargetSelectorCandidateRefs(requirement.TargetSelectorCandidateRefs, []string{"internal/lsp/manager.go", "internal/lsp/server.go"}) ||
		!SameRuntimeProspectiveTargetAllOfConstraintRefs(requirement.ProspectiveTargetAllOfConstraintRefs, []string{"new-target-requires-owner-selector", "workspace-relative-path"}) {
		t.Fatalf("profile-declared selector candidate normalization = %+v", requirement)
	}
	changed := append([]RuntimeProfileContract(nil), normalized...)
	changed[0].EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), normalized[0].EvidenceRequirements...)
	changed[0].EvidenceRequirements[0].TargetSelectorCandidateRefs = []string{"internal/lsp/other.go"}
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("profile-declared selector candidate drift was not detected")
	}
	changed = NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})
	changed[0].EvidenceRequirements[0].ProspectiveTargetAllOfConstraintRefs = []string{"workspace-relative-path"}
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("profile prospective target all-of declaration drift was not detected")
	}
	for name, mutate := range map[string]func(*RuntimeProfileEvidenceRequirement){
		"missing target grounding": func(item *RuntimeProfileEvidenceRequirement) { item.RequireTargetGrounding = false },
		"missing selector":         func(item *RuntimeProfileEvidenceRequirement) { item.TargetSelectorRef = "" },
		"missing selector owner":   func(item *RuntimeProfileEvidenceRequirement) { item.TargetSelectorOwnerRef = "" },
		"missing selector subject": func(item *RuntimeProfileEvidenceRequirement) { item.TargetSelectorSubjectRef = "" },
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

func TestRuntimeBootstrapRecipeIsBoundedOrderedAndAffectsProfileEquality(t *testing.T) {
	profile := RuntimeProfileContract{ID: "software-dev", BootstrapRecipes: []RuntimeBootstrapRecipe{{
		ID: "go-module", RequiredGoalTerms: []string{" Go ", "Go"}, RequiredDoDTerms: []string{"test"},
		Artifacts: []RuntimeBootstrapArtifact{
			{ID: "source", TargetRole: "implementation", AllowedOperationKinds: []string{StepKindFileWrite}, DependsOnArtifactIDs: []string{"module"}},
			{ID: "module", TargetRole: "module_definition", AllowedOperationKinds: []string{StepKindFileWrite}},
			{ID: "test", TargetRole: "executable_validation", AllowedOperationKinds: []string{StepKindFileWrite}, DependsOnArtifactIDs: []string{"source"}},
		},
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{profile}); err != nil {
		t.Fatalf("valid bootstrap recipe rejected: %v", err)
	}
	normalized := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{profile})
	ordered, err := normalized[0].BootstrapRecipes[0].OrderedArtifacts()
	if err != nil {
		t.Fatalf("ordered artifacts: %v", err)
	}
	if got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID}; got[0] != "module" || got[1] != "source" || got[2] != "test" {
		t.Fatalf("artifact dependency order = %v, want module/source/test", got)
	}
	changed := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{normalized[0]})
	changed[0].BootstrapRecipes = append([]RuntimeBootstrapRecipe(nil), normalized[0].BootstrapRecipes...)
	changed[0].BootstrapRecipes[0].Artifacts = append([]RuntimeBootstrapArtifact(nil), normalized[0].BootstrapRecipes[0].Artifacts...)
	changed[0].BootstrapRecipes[0].Artifacts[0].TargetRole = "changed-role"
	if SameRuntimeProfileContracts(normalized, changed) {
		t.Fatal("bootstrap artifact role drift was not detected")
	}
	for name, mutate := range map[string]func(*RuntimeBootstrapRecipe){
		"missing matcher":    func(recipe *RuntimeBootstrapRecipe) { recipe.RequiredGoalTerms = nil; recipe.RequiredDoDTerms = nil },
		"missing role":       func(recipe *RuntimeBootstrapRecipe) { recipe.Artifacts[0].TargetRole = "" },
		"unknown dependency": func(recipe *RuntimeBootstrapRecipe) { recipe.Artifacts[0].DependsOnArtifactIDs = []string{"missing"} },
		"unknown operation": func(recipe *RuntimeBootstrapRecipe) {
			recipe.Artifacts[0].AllowedOperationKinds = []string{"unknown/operation"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := profile
			invalid.BootstrapRecipes = append([]RuntimeBootstrapRecipe(nil), profile.BootstrapRecipes...)
			invalid.BootstrapRecipes[0].Artifacts = append([]RuntimeBootstrapArtifact(nil), profile.BootstrapRecipes[0].Artifacts...)
			mutate(&invalid.BootstrapRecipes[0])
			if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
				t.Fatalf("invalid bootstrap recipe was accepted: %+v", invalid.BootstrapRecipes[0])
			}
		})
	}
}
