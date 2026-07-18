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
