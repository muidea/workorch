package definitionadmission

import (
	"encoding/json"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithProfileEvidencePurposeMapping() workspace.DefinitionAdmissionCandidate {
	c := candidateWithEvidencePurposeMapping()
	p := &c.RuntimeProfileContracts[0]
	p.DeliverableExpectations = []runtimecontract.RuntimeDeliverableExpectation{{ID: "initial-delivery", Required: true, Description: "Profile 原交付义务全文\n与原 DoD 全部合取。"}}
	p.EvidenceRequirements = append(p.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{ID: "initial-delivery", Scope: "deliverable", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}})
	c.Specification.EvidencePurposeMappings = []workspace.GoalEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeProfileDeliverableEvidencePurposeV2,
		HistoricalItemRef: runtimecontract.RuntimeProfileDeliverablePurposeRef(p.ID, "initial-delivery"), AcceptanceItemRef: c.DoDInputs[1].CandidateItemRef,
		HistoricalProfileID: p.ID, HistoricalDeliverableID: "initial-delivery", HistoricalText: p.DeliverableExpectations[0].Description,
		HistoricalProfileHash: runtimecontract.StableRuntimeProfileContractsHash([]runtimecontract.RuntimeProfileContract{*p})}}
	for n := range c.Specification.Reviews {
		c.Specification.Reviews[n].SpecificationHash = c.Specification.ContentHash()
	}
	c.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(c.RuntimeProfileContracts)
	return c.WithHash()
}

func TestProfileEvidencePurposeAdmissionConfirmsSnapshotWithoutInventingDoD(t *testing.T) {
	c := candidateWithProfileEvidencePurposeMapping()
	before := c.Clone()
	if err := ValidateAuthoringVerificationApplicability(*c.Specification, c.DoDInputs, c.RuntimeProfileContracts); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(c)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("preseal: %v %+v", err, result)
	}
	c, s, a := sealEvidencePurposeCandidate(c)
	i, err := CompileRuntimeSourceObligationInventory(c, s, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.EvidencePurposeMappings) != 1 || i.EvidencePurposeMappings[0] != runtimecontract.RuntimeEvidencePurposeMapping(c.Specification.EvidencePurposeMappings[0]) || len(i.Sources) != 3 {
		t.Fatal("snapshot fields lost or Profile source fabricated as original DoD")
	}
	if !reflect.DeepEqual(before, c) {
		t.Fatal("compiler rewrote original candidate")
	}
	body, _ := json.Marshal(i)
	var reload runtimecontract.RuntimeSourceObligationInventory
	if json.Unmarshal(body, &reload) != nil || runtimecontract.ValidateRuntimeSourceObligationApplicability(&reload, c.RuntimeProfileContracts) != nil {
		t.Fatal("confirmed inventory could not independently reload")
	}
	for _, mutate := range []func(*workspace.DefinitionAdmissionCandidate){
		func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.EvidencePurposeMappings[0].HistoricalText = "summary"
		},
		func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.EvidencePurposeMappings[0].HistoricalProfileHash = "new snapshot"
		},
		func(c *workspace.DefinitionAdmissionCandidate) {
			c.RuntimeProfileContracts[0].DeliverableExpectations[0].Description = "new obligation"
		},
	} {
		changed := c.Clone()
		mutate(&changed)
		for n := range changed.Specification.Reviews {
			changed.Specification.Reviews[n].SpecificationHash = changed.Specification.ContentHash()
		}
		changed.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(changed.RuntimeProfileContracts)
		changed = changed.WithHash()
		if _, err := CompileRuntimeSourceObligationInventory(changed, s, a); err == nil {
			t.Fatal("coordinated rehash substituted reviewed/confirmed source")
		}
	}
	a.State, a.BlockingTicketID = workspace.WorkspaceActivationPendingClarificationResolution, "ticket"
	if _, err := CompileRuntimeSourceObligationInventory(c, s, a); err == nil {
		t.Fatal("unconfirmed source reached Runtime")
	}
}
