package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefinitionAuthoringDiagnosticPreservesBytesAndBounds(t *testing.T) {
	parseErr := "\n原样错误\twith spacing\n"
	payload := []byte{0, 0xff, '\n', ' ', 'x'}
	diagnostic := NewDefinitionAuthoringDiagnostic(parseErr, payload)
	if err := diagnostic.Validate(); err != nil {
		t.Fatal(err)
	}
	if !diagnostic.ParseErrorSaved || diagnostic.ParseError != parseErr || !diagnostic.PayloadSaved || !reflect.DeepEqual(diagnostic.Payload, payload) || diagnostic.PayloadBytes != uint64(len(payload)) {
		t.Fatalf("diagnostic changed source data: %+v", diagnostic)
	}
	cloned := diagnostic.Clone()
	cloned.Payload[0] = 1
	if diagnostic.Payload[0] != 0 {
		t.Fatal("diagnostic clone shares payload bytes")
	}
	tooLarge := NewDefinitionAuthoringDiagnostic(strings.Repeat("e", DefinitionAuthoringDiagnosticErrorLimit+1), make([]byte, DefinitionAuthoringDiagnosticDocumentLimit+1))
	if err := tooLarge.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := diagnostic.Clone()
	bad.NotSavedReason = "forged reason"
	if bad.Validate() == nil {
		t.Fatal("accepted caller-supplied unsaved reason")
	}
	bad = diagnostic.Clone()
	bad.PayloadSHA256 = strings.ToUpper(bad.PayloadSHA256)
	if bad.Validate() == nil {
		t.Fatal("accepted noncanonical hash metadata")
	}
	bad = diagnostic.Clone()
	bad.PayloadBytes = DefinitionAuthoringDiagnosticDocumentLimit + 1
	if bad.Validate() == nil {
		t.Fatal("accepted saved payload above hard boundary")
	}
	if tooLarge.ParseErrorSaved || tooLarge.PayloadSaved || tooLarge.Payload != nil || tooLarge.PayloadBytes != DefinitionAuthoringDiagnosticDocumentLimit+1 || tooLarge.PayloadSHA256 == "" || tooLarge.NotSavedReason == "" {
		t.Fatalf("oversize diagnostic not explicitly withheld: %+v", tooLarge)
	}
}

func TestAuthoringSpecificationFailureDiagnosticExactDuplicateAndConflict(t *testing.T) {
	record, _ := authoringLedgerFixture(t)
	derivationPermit := record.DerivationRequests[0].PermitID
	request := DefinitionAuthoringSpecificationRequest{RequestID: "proposal-diagnostic", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	reservedRecord, reserved, _, err := record.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := reservedRecord.ReserveSpecification(request); err != nil {
		t.Fatal(err)
	}
	diagnostic := NewDefinitionAuthoringDiagnostic("  parse error\n", []byte("{\ninvalid\n}"))
	failed, err := reservedRecord.CompleteSpecification(reserved.PermitID, nil, nil, "invalid_proposal", &diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := failed.CompleteSpecification(reserved.PermitID, nil, nil, "invalid_proposal", &diagnostic)
	if err != nil || !reflect.DeepEqual(failed, replayed) {
		t.Fatalf("exact duplicate changed diagnosis: %v", err)
	}
	conflict := NewDefinitionAuthoringDiagnostic("different parse error", []byte("raw"))
	if _, err := failed.CompleteSpecification(reserved.PermitID, nil, nil, "invalid_proposal", &conflict); err == nil {
		t.Fatal("conflicting duplicate diagnosis accepted")
	}
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}
}

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
