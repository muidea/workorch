package decision

import (
	"strings"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func TestContractChangeMaterialValidationDoesNotNormalizeOrGrantReadiness(t *testing.T) {
	original := contractChangeCommandForTest().Proposal
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*decisionmodel.ContractChangeProposal){
		"schema":               func(p *decisionmodel.ContractChangeProposal) { p.SchemaVersion++ },
		"baseline":             func(p *decisionmodel.ContractChangeProposal) { p.BaseStartupInputHash = "" },
		"missing reason":       func(p *decisionmodel.ContractChangeProposal) { p.WhyChange = "" },
		"missing evidence gap": func(p *decisionmodel.ContractChangeProposal) { p.EvidenceGaps = "" },
		"invalid UTF8":         func(p *decisionmodel.ContractChangeProposal) { p.Differences[0].OriginalText = string([]byte{0xff}) },
		"duplicate difference": func(p *decisionmodel.ContractChangeProposal) { p.Differences = append(p.Differences, p.Differences[0]) },
		"unknown impact":       func(p *decisionmodel.ContractChangeProposal) { p.Impacts[0].ProposedHandling = "no_impact" },
		"empty final hash": func(p *decisionmodel.ContractChangeProposal) {
			p.FinalCandidate = &decisionmodel.ContractChangeCandidate{ID: "candidate", Revision: 1}
		},
		"over boundary": func(p *decisionmodel.ContractChangeProposal) {
			p.WhyChange = strings.Repeat("x", decisionmodel.ContractChangeDocumentByteLimit)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := original.Clone()
			change(&p)
			if p.Validate() == nil {
				t.Fatal("缺失材料或超界被接受")
			}
		})
	}
	p := original.Clone()
	p.BaseDefinitionRevision, p.BaseStartupInputHash = "", ""
	p.BaseInputRevision, p.BaseInputHash = 2, "input-hash"
	if err := p.Validate(); err != nil {
		t.Fatalf("真实无Run输入scope=%v", err)
	}
	p.RunID = "run-without-sealed-definition"
	if p.Validate() == nil {
		t.Fatal("Run提案缺封存基线")
	}
	p = original.Clone()
	p.WhyChange = strings.ReplaceAll(p.WhyChange, "\r\n", "\n")
	if p.Hash() == original.Hash() {
		t.Fatal("hash抹去原文差异")
	}
}
