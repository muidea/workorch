package definitionadmission

import (
	"encoding/json"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithEvidencePurposeMapping() workspace.DefinitionAdmissionCandidate {
	c := candidateWithExecutionProofMapping()
	c.Specification.ExecutionProofMappings = nil
	p := &c.RuntimeProfileContracts[0]
	p.EvidenceRequirements[0].LineageAssertions = nil
	p.EvidenceRequirements[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	p.EvidenceRequirements[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
	c.Specification.EvidencePurposeMappings = []workspace.GoalEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurposeV1, HistoricalItemRef: c.DoDInputs[0].CandidateItemRef, AcceptanceItemRef: c.DoDInputs[1].CandidateItemRef}}
	for n := range c.Specification.Reviews {
		c.Specification.Reviews[n].SpecificationHash = c.Specification.ContentHash()
	}
	c.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(c.RuntimeProfileContracts)
	return c.WithHash()
}

func sealEvidencePurposeCandidate(c workspace.DefinitionAdmissionCandidate) (workspace.DefinitionAdmissionCandidate, workspace.WorkspaceStartupInputContract, workspace.WorkspaceActivationAdmission) {
	c, s, a := sourceObligationSealFixture(c)
	s.DoDDefinitionRefs = append(s.DoDDefinitionRefs, "dod-materialized-2")
	s.DoDItemSetHash = workspace.StableDefinitionStringSetHash([]string{c.DoDInputs[0].Text, c.DoDInputs[1].Text})
	s.CandidateItemToItemRefMappings = append(s.CandidateItemToItemRefMappings, workspace.CandidateItemToItemRefMapping{CandidateItemRef: c.DoDInputs[1].CandidateItemRef, ItemRef: "dod-materialized-2"})
	s = s.WithHash()
	a.StartupInputHash = s.StartupInputHash
	return c, s, a
}

func TestEvidencePurposeMappingPreservesReviewedOriginalsAndConfirmedInventory(t *testing.T) {
	c := candidateWithEvidencePurposeMapping()
	before := c.Clone()
	if err := ValidateAuthoringVerificationApplicability(*c.Specification, c.DoDInputs, c.RuntimeProfileContracts); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(c)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("reviewed source mapping: %+v %v", result, err)
	}
	c, s, a := sealEvidencePurposeCandidate(c)
	i, err := CompileRuntimeSourceObligationInventory(c, s, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.EvidencePurposeMappings) != 1 || i.EvidencePurposeMappings[0].AcceptanceItemRef != c.DoDInputs[1].CandidateItemRef {
		t.Fatal("original acceptance source lost")
	}
	for _, source := range i.Sources {
		for _, item := range c.DoDInputs {
			if source.CandidateItemRef == item.CandidateItemRef && source.Text != item.Text {
				t.Fatal("criterion was summarized or replaced")
			}
		}
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatal("source mapping compiler changed original candidate")
	}
	raw, _ := json.Marshal(i)
	var reload runtimecontract.RuntimeSourceObligationInventory
	if err := json.Unmarshal(raw, &reload); err != nil || !reflect.DeepEqual(*i, reload) {
		t.Fatal("reload lost exact purpose map")
	}
	i.EvidencePurposeMappings[0].AcceptanceItemRef = "changed"
	if c.Specification.EvidencePurposeMappings[0].AcceptanceItemRef != c.DoDInputs[1].CandidateItemRef {
		t.Fatal("projection aliases original candidate")
	}
}

func TestEvidencePurposeMappingRejectsUnreviewedUnconfirmedAndCoordinatedReplacement(t *testing.T) {
	for name, mutate := range map[string]func(*workspace.DefinitionAdmissionCandidate, *workspace.WorkspaceStartupInputContract, *workspace.WorkspaceActivationAdmission){
		"unconfirmed": func(_ *workspace.DefinitionAdmissionCandidate, _ *workspace.WorkspaceStartupInputContract, a *workspace.WorkspaceActivationAdmission) {
			a.State = workspace.WorkspaceActivationPendingClarificationResolution
			a.BlockingTicketID = "ticket"
		},
		"reviewed mapping removed": func(c *workspace.DefinitionAdmissionCandidate, _ *workspace.WorkspaceStartupInputContract, _ *workspace.WorkspaceActivationAdmission) {
			c.Specification.EvidencePurposeMappings = nil
			*c = c.WithHash()
		},
		"wrong current source": func(c *workspace.DefinitionAdmissionCandidate, _ *workspace.WorkspaceStartupInputContract, _ *workspace.WorkspaceActivationAdmission) {
			c.Specification.EvidencePurposeMappings[0].AcceptanceItemRef = "unknown"
			*c = c.WithHash()
		},
		"duplicate": func(c *workspace.DefinitionAdmissionCandidate, _ *workspace.WorkspaceStartupInputContract, _ *workspace.WorkspaceActivationAdmission) {
			c.Specification.EvidencePurposeMappings = append(c.Specification.EvidencePurposeMappings, c.Specification.EvidencePurposeMappings[0])
			*c = c.WithHash()
		},
		"coordinated candidate hash cannot replace admission": func(c *workspace.DefinitionAdmissionCandidate, _ *workspace.WorkspaceStartupInputContract, _ *workspace.WorkspaceActivationAdmission) {
			c.DoDInputs[1].Text += " changed"
			c.Specification.Obligations[1].Text = c.DoDInputs[1].Text
			for n := range c.Specification.Reviews {
				c.Specification.Reviews[n].SpecificationHash = c.Specification.ContentHash()
			}
			*c = c.WithHash()
		},
		"coordinated startup hash cannot replace candidate": func(_ *workspace.DefinitionAdmissionCandidate, s *workspace.WorkspaceStartupInputContract, a *workspace.WorkspaceActivationAdmission) {
			s.SpecificationHash = "replacement"
			*s = s.WithHash()
			a.StartupInputHash = s.StartupInputHash
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, s, a := sealEvidencePurposeCandidate(candidateWithEvidencePurposeMapping())
			mutate(&c, &s, &a)
			if _, err := CompileRuntimeSourceObligationInventory(c, s, a); err == nil {
				t.Fatal("unconfirmed or replaced mapping reached startup inventory")
			}
		})
	}
}
