package runtime

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeTargetGroundingDerivesSnapshotOwnerFromOperationDescriptor(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-repository", RequireTargetGrounding: true,
			TargetRole: "repository", TargetRefs: []string{"repository://current"},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindGitWrite}},
	}

	requirements := runtimeTargetGroundingRequirements(facts)
	if len(requirements) != 1 || requirements[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot ||
		requirements[0].OwnerModuleID != runtimecontract.RuntimeModuleGit || requirements[0].SubjectRef != "repository://current" {
		t.Fatalf("descriptor-derived target snapshot requirements = %+v", requirements)
	}
}

func TestRuntimeTargetGroundingRejectsAmbiguousDescriptorOwners(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-ambiguous", RequireTargetGrounding: true,
			TargetRole: "deliverable", TargetRefs: []string{"target"},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindGitWrite,
		}},
	}

	if requirements := runtimeTargetGroundingRequirements(facts); len(requirements) != 0 {
		t.Fatalf("ambiguous descriptor owners produced target authority: %+v", requirements)
	}
}

func TestRuntimeTargetGroundingDerivesSelectionAndManifestOwnerFromDescriptor(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-selection", RequireTargetGrounding: true,
			TargetRole: "repository", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "profile/repository-targets", TargetSelectorOwnerRef: "profile/repository", TargetSelectorSubjectRef: "repository",
		},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindGitWrite}},
	}

	requirements := runtimeTargetGroundingRequirements(facts)
	if len(requirements) != 2 {
		t.Fatalf("descriptor-derived discovery requirements = %+v", requirements)
	}
	for _, requirement := range requirements {
		if requirement.OwnerModuleID != runtimecontract.RuntimeModuleGit {
			t.Fatalf("discovery requirement invented a different owner: %+v", requirement)
		}
		if requirement.Kind != runtimecontract.PrerequisiteKindManifest && requirement.Kind != runtimecontract.PrerequisiteKindTargetSelection {
			t.Fatalf("unexpected discovery requirement: %+v", requirement)
		}
	}
}

func TestRuntimeValidationDiscoveryRequiresSealedManifestContract(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{Activation: runtimecontract.RuntimeActivationContext{
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ValidationPolicies: []runtimecontract.RuntimeValidationPolicy{{
			ID: "manifest-discovery", Scope: "validation", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			DiscoveryRequired: true, DiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		}}},
	}}

	if _, err := reconcileRuntimeValidationDiscoveryPrerequisites(facts); err == nil || !strings.Contains(err.Error(), "no typed prerequisite contract") {
		t.Fatalf("missing sealed manifest contract error = %v", err)
	}
}

func TestFreshTargetSnapshotPreservesExactObservedOwner(t *testing.T) {
	requirement := runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleGit,
		SubjectRef: "repository://current", Reason: "current repository snapshot",
	}
	prerequisite := runtimePrerequisiteFromRequirement(requirement)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: "repository-r1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleGit,
			SubjectRef: requirement.SubjectRef, Revision: "repository-r1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts := runtimecontract.RuntimeFacts{
		Focus:         runtimecontract.CoverageFocus{ID: "focus-repository", RequireTargetGrounding: true, TargetRole: "repository", TargetRefs: []string{requirement.SubjectRef}},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{prerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			TargetSnapshots:          []runtimecontract.TargetSnapshot{{TargetRef: requirement.SubjectRef, ContentHash: "repository-r1", ContentState: runtimecontract.SnapshotContentStateComplete}},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{observation},
		},
	}

	updated := requireFreshTargetSnapshots(facts, []string{requirement.SubjectRef})
	if len(updated.Resources.TargetSnapshots) != 0 || len(updated.Prerequisites.Satisfied) != 0 || len(updated.Resources.PrerequisiteObservations) != 0 ||
		len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].OwnerModuleID != runtimecontract.RuntimeModuleGit {
		t.Fatalf("target refresh lost exact observed owner lineage: %+v", updated)
	}
}

func TestRuntimeWorkspaceManifestObservationUsesSatisfiedOwnerLineage(t *testing.T) {
	aggregate := RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{Resources: runtimecontract.RuntimeResourceFacts{
		Manifest: &runtimecontract.WorkspaceManifest{Revision: "manifest-r1", TargetRefs: []string{"repository://current"}},
	}}}
	addManifestLineage := func(owner string) {
		requirement := runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: owner}
		prerequisite := runtimePrerequisiteFromRequirement(requirement)
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: owner, ContentHash: "manifest-r1",
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: owner, SubjectRef: "workspace", Revision: "manifest-r1",
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
		aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
	}

	addManifestLineage(runtimecontract.RuntimeModuleGit)
	observation := runtimeWorkspaceManifestObservation(aggregate)
	if observation == nil || observation.OwnerModuleID != runtimecontract.RuntimeModuleGit {
		t.Fatalf("non-default manifest owner observation = %+v", observation)
	}

	addManifestLineage(runtimecontract.RuntimeModuleMCP)
	if observation := runtimeWorkspaceManifestObservation(aggregate); observation != nil {
		t.Fatalf("ambiguous manifest owners produced execution handoff observation: %+v", observation)
	}
}
