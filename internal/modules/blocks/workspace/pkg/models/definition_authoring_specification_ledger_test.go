package workspace

import (
	"testing"
)

func authoringLedgerFixture(t testing.TB) (DefinitionAuthoringRecord, GoalSpecification) {
	t.Helper()
	input, template, derived, spec := authoringSpecificationFixture(t)
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	var err error
	var derivation DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	return record, spec
}

func TestAuthoringSpecificationLedgerReservesAndReplaysWithoutSecondPermit(t *testing.T) {
	record, spec := authoringLedgerFixture(t)
	derivationPermit := record.DerivationRequests[0].PermitID
	request := DefinitionAuthoringSpecificationRequest{RequestID: "proposal-1", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	first, reserved, fresh, err := record.ReserveSpecification(request)
	if err != nil || !fresh || len(record.SpecificationRequests) != 0 {
		t.Fatalf("reserve: fresh=%v err=%v", fresh, err)
	}
	_, replay, fresh, err := first.ReserveSpecification(request)
	if err != nil || fresh || replay.PermitID != reserved.PermitID {
		t.Fatalf("replay granted another physical permit: %+v %v", replay, err)
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "binding-drift", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "other"}); err == nil {
		t.Fatal("specification used a different provider configuration from derivation")
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: request.RequestID, DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "other"}); err == nil {
		t.Fatal("request ID changed provider identity")
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "critic-early", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: spec.ContentHash(), ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("critic reserved without completed proposal")
	}
	completed, err := first.CompleteSpecification(reserved.PermitID, &spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := completed.CompleteSpecification(reserved.PermitID, &spec, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := completed.CompleteSpecification(reserved.PermitID, nil, nil, "provider_failed"); err == nil {
		t.Fatal("completed proposal changed outcome")
	}
	spec.Requirements[0].Text = "篡改"
	if completed.SpecificationRequests[0].Proposal.Requirements[0].Text != "交付结果" {
		t.Fatal("completion retained mutable caller proposal")
	}
	critic := DefinitionAuthoringSpecificationRequest{RequestID: "critic-1", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: completed.SpecificationRequests[0].Proposal.ContentHash(), ProviderBindingHash: "provider"}
	withCritic, criticPermit, fresh, err := completed.ReserveSpecification(critic)
	if err != nil || !fresh {
		t.Fatalf("critic reserve: %v", err)
	}
	review := GoalSpecificationCompletenessReview{Kind: critic.ReviewKind, Status: GoalReviewUnknown, SpecificationHash: critic.ProposalHash}
	withCritic, err = withCritic.CompleteSpecification(criticPermit.PermitID, nil, &review, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := withCritic.Validate(); err != nil {
		t.Fatal(err)
	}
	forged := withCritic.Clone()
	forged.SpecificationRequests[1].ProposalHash = "other"
	forged.SpecificationRequests[1].PermitID = forged.SpecificationRequests[1].ExpectedPermitID()
	if forged.Validate() == nil {
		t.Fatal("critic detached from completed proposal")
	}
}

func TestAuthoringSpecificationBudgetSurvivesRevisionAndRequestIDChanges(t *testing.T) {
	record, _ := authoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	for n := 0; n < DefinitionAuthoringSpecificationAttemptLimit; n++ {
		var err error
		record, _, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "proposal-" + string(rune('a'+n)), DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "proposal-extra", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("request ID reset attempt budget")
	}
	record.Input.Revision++
	record.Input.GoalText = "新目标"
	record.InputHash = record.Input.InputHash()
	if _, _, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "proposal-new-revision", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("new revision reused stale derivation")
	}
}
