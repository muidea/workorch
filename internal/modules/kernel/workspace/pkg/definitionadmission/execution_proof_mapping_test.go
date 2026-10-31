package definitionadmission

import (
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithExecutionProofMapping() workspace.DefinitionAdmissionCandidate {
	candidate := candidateWithSpecification()
	second := candidate.DoDInputs[0]
	second.CandidateItemRef = "dod:formal:item:000002"
	second.Text = "原工作和修复要求，不能以 consumer 的行为代替"
	candidate.DoDInputs = append(candidate.DoDInputs, second)
	profile := &candidate.RuntimeProfileContracts[0]
	profile.EvidenceRequirements[0].LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{{Ref: "original-work", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: runtimecontract.RuntimeDoDSourceExecutionProof}}
	declaration := profile.EvidenceRequirements[0]
	declaration.ID, declaration.LineageAssertions = "dod-002", nil
	profile.EvidenceRequirements = append(profile.EvidenceRequirements, declaration)
	spec := candidate.Specification
	spec.Requirements[0].SourceItemRefs = append(spec.Requirements[0].SourceItemRefs, second.CandidateItemRef)
	obligation := spec.Obligations[0]
	obligation.ID, obligation.Text, obligation.DoDItemRefs = "obligation-source", second.Text, []string{second.CandidateItemRef}
	spec.Obligations = append(spec.Obligations, obligation)
	verification := spec.Verifications[0]
	verification.ID, verification.ObligationID, verification.EvidenceRequirementID = "verify-source", obligation.ID, "dod-002"
	spec.Verifications = append(spec.Verifications, verification)
	spec.ExecutionProofMappings = []workspace.GoalExecutionProofMapping{{ContractRef: runtimecontract.RuntimeDoDSourceExecutionProof, ConsumerItemRef: candidate.DoDInputs[0].CandidateItemRef, RuleRef: "original-work", SourceItemRef: second.CandidateItemRef}}
	for index := range spec.Reviews {
		spec.Reviews[index].SpecificationHash = spec.ContentHash()
	}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	return candidate.WithHash()
}

func TestExecutionProofMappingAdmissionAndConfirmedInventory(t *testing.T) {
	candidate := candidateWithExecutionProofMapping()
	before := candidate.Clone()
	result, err := Compile(candidate)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("complete cross-source graph rejected: %+v %v", result, err)
	}
	candidate, startup, admission := sourceObligationSealFixture(candidate)
	startup.DoDDefinitionRefs = append(startup.DoDDefinitionRefs, "dod-materialized-2")
	startup.DoDItemSetHash = workspace.StableDefinitionStringSetHash([]string{candidate.DoDInputs[0].Text, candidate.DoDInputs[1].Text})
	startup.CandidateItemToItemRefMappings = append(startup.CandidateItemToItemRefMappings, workspace.CandidateItemToItemRefMapping{CandidateItemRef: candidate.DoDInputs[1].CandidateItemRef, ItemRef: "dod-materialized-2"})
	startup = startup.WithHash()
	admission.StartupInputHash = startup.StartupInputHash
	inventory, err := CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.ExecutionProofMappings) != 1 || inventory.ExecutionProofMappings[0].SourceItemRef != candidate.DoDInputs[1].CandidateItemRef {
		t.Fatal("confirmed mapping lost original source")
	}
	if !reflect.DeepEqual(candidate, before) {
		t.Fatal("source compilation mutated original candidate")
	}
	inventory.ExecutionProofMappings[0].SourceItemRef = "changed"
	if candidate.Specification.ExecutionProofMappings[0].SourceItemRef != candidate.DoDInputs[1].CandidateItemRef {
		t.Fatal("inventory aliases confirmation source")
	}
}

func TestExecutionProofMappingAdmissionRejectsMissingUnreviewedAndWrongSource(t *testing.T) {
	for name, change := range map[string]func(*workspace.DefinitionAdmissionCandidate){
		"missing specification": func(c *workspace.DefinitionAdmissionCandidate) { c.Specification = nil },
		"missing mapping":       func(c *workspace.DefinitionAdmissionCandidate) { c.Specification.ExecutionProofMappings = nil },
		"unknown rule": func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.ExecutionProofMappings[0].RuleRef = "unknown"
		},
		"duplicate mapping": func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.ExecutionProofMappings = append(c.Specification.ExecutionProofMappings, c.Specification.ExecutionProofMappings[0])
		},
		"unknown source": func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.ExecutionProofMappings[0].SourceItemRef = "unknown"
		},
		"self source": func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.ExecutionProofMappings[0].SourceItemRef = c.DoDInputs[0].CandidateItemRef
		},
		"unsupported version": func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.ExecutionProofMappings[0].ContractRef = "runtime/cross-source-execution-proof/v0"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := candidateWithExecutionProofMapping()
			change(&candidate)
			// No replacement review/confirmation is manufactured for changed maps.
			candidate = candidate.WithHash()
			result, err := Compile(candidate)
			if err == nil && result.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
				t.Fatal("invalid or unreviewed cross-source mapping was admitted")
			}
		})
	}
}
