package workorchruntimecontract

import "testing"

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
