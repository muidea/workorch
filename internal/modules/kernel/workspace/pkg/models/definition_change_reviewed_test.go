package workspace

import (
	"reflect"
	"testing"
)

func reviewedDefinitionChangeFixture(t *testing.T) (*Workspace, DefinitionChangeCandidate) {
	t.Helper()
	original, proposal := declaredAuthoringLedgerFixture(t)
	record := original.Clone()
	record.Input.Revision++
	record.Input.GoalText = "完整新目标\r\n原文保留"
	record.InputHash = record.Input.InputHash()
	var derivation DefinitionDerivationRequest
	var err error
	record, derivation, _, err = record.ReserveDerivation("candidate-derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := *original.DerivationRequests[0].Proposal
	derived.AuthoringInputHash = record.InputHash
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	proposal.GoalTextHash = StableDefinitionTextHash(record.Input.GoalText)
	proposal.Requirements[0].Text = record.Input.GoalText
	_, _, proposal = currentProposalFixture(t, record.DerivationRequests[len(record.DerivationRequests)-1], proposal)
	var request DefinitionAuthoringSpecificationRequest
	record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "candidate-proposal", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, request.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range goalReviewKinds {
		record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "candidate-review-" + kind, DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立完整覆盖", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
		record, err = record.CompleteSpecification(request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	w := &Workspace{ID: original.Input.WorkspaceID, Revision: 2, DefinitionAuthoring: &original}
	c := DefinitionChangeCandidate{RequestID: "prepare", ExpectedWorkspaceRevision: 1, RecordedWorkspaceRevision: 2, TicketID: "ticket", ProposalRevision: 1, ProposalHash: "source-proposal", BaseInputHash: original.InputHash, Record: record}
	if err := c.Validate(w); err != nil {
		t.Fatal(err)
	}
	return w, c
}

func reviewedDefinitionChangeMetadata(c DefinitionChangeCandidate) DefinitionChangeCandidate {
	c = c.Clone()
	for n := range c.Record.SpecificationRequests {
		c.Record.SpecificationRequests[n] = c.Record.SpecificationRequests[n].Metadata()
	}
	return c
}

func TestDefinitionChangeReviewedIdentityRequiresExactCompletedLedger(t *testing.T) {
	w, c := reviewedDefinitionChangeFixture(t)
	before := c.Clone()
	identity, err := c.ReviewedIdentity(w)
	if err != nil || identity.InputHash != c.Record.InputHash || identity.Revision != c.Record.Input.Revision || identity.Hash == "" {
		t.Fatalf("已审身份=%+v err=%v", identity, err)
	}
	if view, err := reviewedDefinitionChangeMetadata(c).ReviewedIdentityView(w.ID); err != nil || view != identity {
		t.Fatalf("detached 身份=%+v err=%v", view, err)
	}
	if !reflect.DeepEqual(c, before) || w.DefinitionAuthoringConfirmation != nil || w.ActivationAdmission != nil || w.StartupInput != nil {
		t.Fatal("身份查询改写记录或产生确认／准入")
	}
	for name, change := range map[string]func(*DefinitionChangeCandidate){
		"unknown_review": func(c *DefinitionChangeCandidate) {
			c.Record.SpecificationRequests[len(c.Record.SpecificationRequests)-1].Review.Status = GoalReviewUnknown
		},
		"failed_review": func(c *DefinitionChangeCandidate) {
			c.Record.SpecificationRequests[len(c.Record.SpecificationRequests)-1].Review.Status = GoalReviewFail
		},
		"pending_review": func(c *DefinitionChangeCandidate) {
			c.Record.SpecificationRequests[len(c.Record.SpecificationRequests)-1].Review = nil
		},
		"missing_review": func(c *DefinitionChangeCandidate) {
			c.Record.SpecificationRequests = c.Record.SpecificationRequests[:len(c.Record.SpecificationRequests)-1]
		},
		"missing_coverage": func(c *DefinitionChangeCandidate) {
			c.Record.SpecificationRequests[len(c.Record.SpecificationRequests)-1].Review.RequirementIDs = nil
		},
		"ambiguous_review": func(c *DefinitionChangeCandidate) {
			r := c.Record.SpecificationRequests[len(c.Record.SpecificationRequests)-1].Clone()
			r.RequestID = "duplicate-review"
			r.PermitID = r.ExpectedPermitID()
			c.Record.SpecificationRequests = append(c.Record.SpecificationRequests, r)
		},
		"pending_derivation": func(c *DefinitionChangeCandidate) {
			c.Record.DerivationRequests[len(c.Record.DerivationRequests)-1].Proposal = nil
		},
		"ambiguous_derivation": func(c *DefinitionChangeCandidate) {
			r := c.Record.DerivationRequests[len(c.Record.DerivationRequests)-1].Clone()
			r.RequestID = "duplicate-derivation"
			r.PermitID = r.ExpectedPermitID()
			c.Record.DerivationRequests = append(c.Record.DerivationRequests, r)
		},
		"stale_input": func(c *DefinitionChangeCandidate) {
			c.Record.Input.Revision++
			c.Record.InputHash = c.Record.Input.InputHash()
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := c.Clone()
			change(&changed)
			if _, err := changed.ReviewedIdentity(w); err == nil {
				t.Fatal("不完整或歧义 ledger 产生 owner 已审身份")
			}
			if _, err := reviewedDefinitionChangeMetadata(changed).ReviewedIdentityView(w.ID); err == nil {
				t.Fatal("不完整或歧义 ledger 产生 detached 已审身份")
			}
		})
	}
	changed := c.Clone()
	changed.ProposalRevision++
	changed.ProposalHash = "追加最终材料后的来源版本"
	if actual, err := changed.ReviewedIdentity(w); err != nil || actual != identity {
		t.Fatalf("材料追加产生循环身份=%+v err=%v", actual, err)
	}
	if _, err := reviewedDefinitionChangeMetadata(c).ReviewedIdentityView("other-workspace"); err == nil {
		t.Fatal("跨 Workspace 身份被接受")
	}
}
