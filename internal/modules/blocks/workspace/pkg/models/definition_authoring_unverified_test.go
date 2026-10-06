package workspace

import "testing"

func TestAuthoringUnverifiedDraftWithSixPASSCannotConfirm(t *testing.T) {
	record, proposal := summaryAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	var request DefinitionAuthoringSpecificationRequest
	var err error
	record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteSpecification(request.PermitID, &proposal, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range goalReviewKinds {
		record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "review-" + kind, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立审查", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal"}}
		record, err = record.CompleteSpecification(request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("UNVERIFIED draft lost validity: %v", err)
	}
	if _, _, err := record.ConfirmableSpecification(permit, proposal.ContentHash()); err == nil {
		t.Fatal("six PASS reviews granted confirmation despite missing declarations")
	}
	state, phase, code, count := record.CurrentAuthoringSummary()
	if state != "review_blocked" || phase != "" || code != "" || count != len(goalReviewKinds) {
		t.Fatalf("unverified summary = %q/%q/%q reviews=%d", state, phase, code, count)
	}
}
