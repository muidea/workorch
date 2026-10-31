package workspace

import (
	"encoding/json"
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
	request := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-diagnostic", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	reservedRecord, reserved, _, err := record.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := reservedRecord.ReserveSpecification(request); err != nil {
		t.Fatal(err)
	}
	diagnostic := NewDefinitionAuthoringDiagnostic("  parse error\n", []byte("{\ninvalid\n}"))
	failed, err := completeTestSpecification(reservedRecord, reserved.PermitID, nil, nil, "invalid_proposal", &diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := completeTestSpecification(failed, reserved.PermitID, nil, nil, "invalid_proposal", &diagnostic)
	if err != nil || !reflect.DeepEqual(failed, replayed) {
		t.Fatalf("exact duplicate changed diagnosis: %v", err)
	}
	conflict := NewDefinitionAuthoringDiagnostic("different parse error", []byte("raw"))
	if _, err := completeTestSpecification(failed, reserved.PermitID, nil, nil, "invalid_proposal", &conflict); err == nil {
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
	record, err = completeTestDerivation(record, derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, spec = currentProposalFixture(t, record.DerivationRequests[0], spec)
	return record, spec
}

func summaryAuthoringLedgerFixture(t testing.TB) (DefinitionAuthoringRecord, GoalSpecification) {
	t.Helper()
	record, spec := authoringLedgerFixture(t)
	derivation := record.DerivationRequests[0].Clone()
	derivation.RequestID = "derive/" + record.InputHash
	derivation.PermitID = derivation.ExpectedPermitID()
	record.DerivationRequests[0] = derivation
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	return record, spec
}

func TestDefinitionAuthoringSummaryUsesCurrentInputAndPendingPriority(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-summary", Revision: 1, GoalText: "goal", TemplateID: "template"}
	template := WorkspaceTemplate{ID: input.TemplateID}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	state, _, _, count := record.CurrentAuthoringSummary()
	if state != "draft" || count != 0 {
		t.Fatalf("draft summary = %q, count=%d", state, count)
	}
	reserved, derivation, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _, _ = reserved.CurrentAuthoringSummary()
	if state != "outcome_unresolved" {
		t.Fatalf("pending derivation must be unresolved, got %q", state)
	}
	completed, err := completeTestDerivation(reserved, derivation.PermitID, &DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"item"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	failedPending, _, _, err := completed.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-fail", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := completeTestSpecification(failedPending, failedPending.SpecificationRequests[0].PermitID, nil, nil, "invalid_proposal")
	if err != nil {
		t.Fatal(err)
	}
	pending, _, _, err := failed.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-pending", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	state, phase, code, _ := pending.CurrentAuthoringSummary()
	if state != "outcome_unresolved" || phase != "" || code != "" {
		t.Fatalf("pending permit did not outrank prior failure: %+v %q %q %q", pending, state, phase, code)
	}
}

func TestDefinitionAuthoringSummaryMatchesSixReviewViewSemantics(t *testing.T) {
	record, proposal := summaryAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	request := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal/" + permit, DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	var err error
	record, request, _, err = record.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, request.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}

	checkSummary := func(wantState string, wantCount int) {
		t.Helper()
		state, phase, code, count := record.CurrentAuthoringSummary()
		if state != wantState || phase != "" || code != "" || count != wantCount {
			t.Fatalf("summary = %q/%q/%q reviews=%d, want %q/empty/empty reviews=%d", state, phase, code, count, wantState, wantCount)
		}
	}

	for index, kind := range goalReviewKinds {
		request = DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind), RequestID: "review/" + kind + "/" + permit, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"}
		record, request, _, err = record.ReserveSpecification(request)
		if err != nil {
			t.Fatal(err)
		}
		status := GoalReviewPass
		if index == 0 {
			status = GoalReviewFail
		}
		wantState := "reviewing"
		if status == GoalReviewFail || index > 0 {
			wantState = "review_blocked"
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: status, SpecificationHash: proposal.ContentHash(), Reason: "review", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
		record, err = completeTestSpecification(record, request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
		checkSummary(wantState, index+1)
	}
	checkSummary("review_blocked", len(goalReviewKinds))
	if _, _, err := record.ConfirmableSpecification(permit, proposal.ContentHash()); err == nil {
		t.Fatal("FAIL review became confirmable")
	}

	unknown := record.Clone()
	last := len(unknown.SpecificationRequests) - 1
	unknown.SpecificationRequests[last].Review.Status = GoalReviewUnknown
	raw, _ := json.Marshal(map[string]any{"review": map[string]any{"Status": GoalReviewUnknown, "Reason": unknown.SpecificationRequests[last].Review.Reason, "EvidenceRefs": unknown.SpecificationRequests[last].Review.EvidenceRefs, "RequirementIDs": unknown.SpecificationRequests[last].Review.RequirementIDs}})
	audit := semanticAudit(raw)
	unknown.SpecificationRequests[last].ReviewAudit = &audit
	if err := unknown.Validate(); err != nil {
		t.Fatalf("UNKNOWN review fixture invalid: %v", err)
	}
	state, phase, code, count := unknown.CurrentAuthoringSummary()
	if state != "review_blocked" || phase != "" || code != "" || count != len(goalReviewKinds) {
		t.Fatalf("UNKNOWN review summary = %q/%q/%q reviews=%d", state, phase, code, count)
	}
	if _, _, err := unknown.ConfirmableSpecification(permit, proposal.ContentHash()); err == nil {
		t.Fatal("UNKNOWN review became confirmable")
	}
}

func TestDefinitionAuthoringSummaryDoesNotConfuseHistoricalRequestFailure(t *testing.T) {
	record, _ := summaryAuthoringLedgerFixture(t)
	derivationPermit := record.DerivationRequests[0].PermitID
	request := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-fail", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	var err error
	record, request, _, err = record.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestSpecification(record, request.PermitID, nil, nil, "invalid_proposal")
	if err != nil {
		t.Fatal(err)
	}
	state, phase, code, _ := record.CurrentAuthoringSummary()
	if state != "failed" || phase != "proposal" || code != "invalid_proposal" {
		t.Fatalf("current failure summary = %q/%q/%q", state, phase, code)
	}
	// 后续输入修订保留历史记录，但当前阶段不再显示旧失败。
	newInput := record.Input.Clone()
	newInput.Revision++
	newInput.GoalText = "revised goal"
	record.Input = newInput
	record.InputHash = newInput.InputHash()
	state, phase, code, _ = record.CurrentAuthoringSummary()
	if state != "draft" || phase != "" || code != "" {
		t.Fatalf("historical failure leaked to current revision: %q/%q/%q", state, phase, code)
	}
}

func TestAuthoringSpecificationLedgerReservesAndReplaysWithoutSecondPermit(t *testing.T) {
	record, spec := authoringLedgerFixture(t)
	derivationPermit := record.DerivationRequests[0].PermitID
	request := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-1", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	first, reserved, fresh, err := record.ReserveSpecification(request)
	if err != nil || !fresh || len(record.SpecificationRequests) != 0 {
		t.Fatalf("reserve: fresh=%v err=%v", fresh, err)
	}
	_, replay, fresh, err := first.ReserveSpecification(request)
	if err != nil || fresh || replay.PermitID != reserved.PermitID {
		t.Fatalf("replay granted another physical permit: %+v %v", replay, err)
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "binding-drift", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "other"}); err == nil {
		t.Fatal("specification used a different provider configuration from derivation")
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: request.RequestID, DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "other"}); err == nil {
		t.Fatal("request ID changed provider identity")
	}
	if _, _, _, err := first.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, GoalReviewTraceability), RequestID: "critic-early", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: spec.ContentHash(), ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("critic reserved without completed proposal")
	}
	completed, err := completeCurrentProposalFixture(t, first, reserved.PermitID, &spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := completeCurrentProposalFixture(t, completed, reserved.PermitID, &spec); err != nil {
		t.Fatal(err)
	}
	if _, err := completeTestSpecification(completed, reserved.PermitID, nil, nil, "provider_failed"); err == nil {
		t.Fatal("completed proposal changed outcome")
	}
	originalText := completed.SpecificationRequests[0].Proposal.Requirements[0].Text
	spec.Requirements[0].Text = "篡改"
	if completed.SpecificationRequests[0].Proposal.Requirements[0].Text != originalText {
		t.Fatal("completion retained mutable caller proposal")
	}
	critic := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, GoalReviewTraceability), RequestID: "critic-1", DerivationPermitID: derivationPermit, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: completed.SpecificationRequests[0].Proposal.ContentHash(), ProviderBindingHash: "provider"}
	withCritic, criticPermit, fresh, err := completed.ReserveSpecification(critic)
	if err != nil || !fresh {
		t.Fatalf("critic reserve: %v", err)
	}
	review := GoalSpecificationCompletenessReview{Kind: critic.ReviewKind, Status: GoalReviewUnknown, SpecificationHash: critic.ProposalHash}
	withCritic, err = completeTestSpecification(withCritic, criticPermit.PermitID, nil, &review, "")
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
		record, _, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-" + string(rune('a'+n)), DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-extra", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("request ID reset attempt budget")
	}
	record.Input.Revision++
	record.Input.GoalText = "新目标"
	record.InputHash = record.Input.InputHash()
	if _, _, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal-new-revision", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}); err == nil {
		t.Fatal("new revision reused stale derivation")
	}
}
