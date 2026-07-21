package orchestrator

import (
	"testing"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2ValidationDiscoveryRequiresFreshOwnerFacts(t *testing.T) {
	facts := runtimeValidationDiscoveryFacts(t)
	if missing := runtimeV2MissingValidationDiscoverySources(facts); len(missing) != 0 {
		t.Fatalf("fresh validation discovery missing = %v", missing)
	}
	observations := facts.Resources.PrerequisiteObservations[:0]
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if observation.PrerequisiteID != "capabilities" {
			observations = append(observations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = observations
	missing := runtimeV2MissingValidationDiscoverySources(facts)
	if len(missing) != 1 || missing[0] != runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities {
		t.Fatalf("missing discovery sources = %v, want available capabilities", missing)
	}
}

func runtimev2AggregateForValidationDiscovery(t *testing.T) runtimev2.RuntimeAggregate {
	t.Helper()
	facts := runtimeValidationDiscoveryFacts(t)
	facts.Focus.ID = "focus-discovery"
	// Deliberately remove the capability observation: all other prerequisite
	// facts remain present, so the gate is specifically about fresh discovery.
	observations := facts.Resources.PrerequisiteObservations[:0]
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if observation.PrerequisiteID != "capabilities" {
			observations = append(observations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = observations
	return runtimev2.RuntimeAggregate{
		Facts: facts,
		Decision: runtimecontract.RuntimeDecision{
			Action: runtimecontract.RuntimeActionProduceValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			},
		},
	}
}

func TestRuntimeV2ValidationDiscoveryGatePreventsProviderCandidate(t *testing.T) {
	aggregate := runtimev2AggregateForValidationDiscovery(t)
	fact, err := runtimeV2FeasibilityFact(aggregate)
	if err != nil || fact == nil {
		t.Fatalf("discovery feasibility = %+v, err=%v", fact, err)
	}
	if fact.Action != runtimecontract.RuntimeActionProduceValidation || fact.Reason == "" {
		t.Fatalf("discovery feasibility fact = %+v", fact)
	}
}

func runtimeValidationDiscoveryFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	manifest := runtimecontract.WorkspaceManifest{Revision: "tree-r1"}
	manifestRequirement := runtimecontract.RuntimePrerequisite{ID: "manifest", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile}
	commandsRequirement := runtimecontract.RuntimePrerequisite{ID: "commands", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell}
	capabilitiesRequirement := runtimecontract.RuntimePrerequisite{ID: "capabilities", Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell}
	observations := []runtimecontract.RuntimePrerequisiteObservation{
		runtimeValidationDiscoveryObservation(manifestRequirement, "manifest-r1", runtimecontract.RuntimeResourceWorkspaceTree),
		runtimeValidationDiscoveryObservation(commandsRequirement, "commands-r1", runtimecontract.RuntimeResourceOwnerCatalog),
		runtimeValidationDiscoveryObservation(capabilitiesRequirement, "capabilities-r1", runtimecontract.RuntimeResourceOwnerCatalog),
	}
	return runtimecontract.RuntimeFacts{
		Resources: runtimecontract.RuntimeResourceFacts{
			Manifest: &manifest,
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{{
				PrerequisiteID: "commands", SnapshotID: "commands-snapshot", OwnerModuleID: runtimecontract.RuntimeModuleShell,
				OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ObservedRevision: "commands-r1",
			}},
			PrerequisiteObservations: observations,
		},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{manifestRequirement, commandsRequirement, capabilitiesRequirement}},
		Capabilities:  runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		Activation: runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{ValidationPolicies: []runtimecontract.RuntimeValidationPolicy{{
			ID: "discovery", Scope: "test", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, DiscoveryRequired: true,
			DiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles, runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands, runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities},
		}}}},
	}
}

func runtimeValidationDiscoveryObservation(prerequisite runtimecontract.RuntimePrerequisite, hash string, domain runtimecontract.RuntimeResourceDomain) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: hash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: domain, OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: "fixture", Revision: hash}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}
