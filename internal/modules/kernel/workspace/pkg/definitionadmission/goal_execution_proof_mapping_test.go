package definitionadmission

import (
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithGoalExecutionMapping() workspace.DefinitionAdmissionCandidate {
	c := candidateWithSpecification()
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "goal-original-work", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: runtimecontract.RuntimeGoalSourceExecutionProofV2}
	declaration := c.RuntimeProfileContracts[0].EvidenceRequirements[0]
	declaration.ID, declaration.Scope, declaration.LineageAssertions = c.GoalInput.CandidateItemRef, "goal", []runtimecontract.RuntimeLineageAssertionRule{rule}
	c.RuntimeProfileContracts[0].EvidenceRequirements = append(c.RuntimeProfileContracts[0].EvidenceRequirements, declaration)
	c.Specification.ExecutionProofMappings = []workspace.GoalExecutionProofMapping{{ContractRef: rule.ExecutionSourceContractRef, ConsumerItemRef: c.GoalInput.CandidateItemRef, RuleRef: rule.Ref, SourceItemRef: c.DoDInputs[0].CandidateItemRef, ConsumerGoalText: c.GoalInput.Text}}
	for n := range c.Specification.Reviews {
		c.Specification.Reviews[n].SpecificationHash = c.Specification.ContentHash()
	}
	c.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(c.RuntimeProfileContracts)
	return c.WithHash()
}

func TestGoalExecutionMappingAdmissionFullGoalAndConfirmedInventory(t *testing.T) {
	c := candidateWithGoalExecutionMapping()
	before := c.Clone()
	result, err := Compile(c)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("Goal map rejected: %+v %v", result, err)
	}
	if err := ValidateAuthoringVerificationApplicability(*c.Specification, c.DoDInputs, c.RuntimeProfileContracts, c.GoalInput); err != nil {
		t.Fatal(err)
	}
	if ValidateAuthoringVerificationApplicability(*c.Specification, c.DoDInputs, c.RuntimeProfileContracts) == nil {
		t.Fatal("accepted mapping-owned Goal as original preimage")
	}
	c, startup, admission := sourceObligationSealFixture(c)
	i, err := CompileRuntimeSourceObligationInventory(c, startup, admission)
	if err != nil || i.ExecutionProofMappings[0].ConsumerGoalText != c.GoalInput.Text {
		t.Fatalf("full Goal lost: %v", err)
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatal("compiler changed original candidate")
	}
	changed := c.Clone()
	changed.Specification.ExecutionProofMappings[0].ConsumerGoalText += "changed"
	changed = changed.WithHash()
	if result, err := Compile(changed); err == nil && result.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatal("changed map bypassed original reviews")
	}
	changed = c.Clone()
	changed.Specification.ExecutionProofMappings[0].SourceItemRef = "unknown"
	for n := range changed.Specification.Reviews {
		changed.Specification.Reviews[n].SpecificationHash = changed.Specification.ContentHash()
	}
	changed = changed.WithHash()
	if result, err := Compile(changed); err == nil && result.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatal("rehash/review cannot invent source")
	}
	admission.State = workspace.WorkspaceActivationPendingClarificationResolution
	if _, err := CompileRuntimeSourceObligationInventory(c, startup, admission); err == nil {
		t.Fatal("unconfirmed/ineligible mapping was promoted")
	}
}
