package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func transientDiagnosticFixture(code, failureClass string, wireBytes, docBytes []byte, docComplete bool) *DefinitionAuthoringDiagnostic {
	d := NewDefinitionAuthoringDiagnostic(code+" transport failure", nil)
	sumHex := func(data []byte) string {
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:])
	}
	doc := DefinitionProviderObservation{Reason: "not_observed"}
	if len(docBytes) > 0 {
		doc = DefinitionProviderObservation{
			Content:       append([]byte(nil), docBytes...),
			ObservedBytes: uint64(len(docBytes)),
			SHA256:        sumHex(docBytes),
			Observed:      true,
			Complete:      docComplete,
			Saved:         true,
		}
		if !docComplete {
			doc.Reason = "document_incomplete"
		}
	}
	wire := DefinitionProviderObservation{Reason: "not_observed"}
	if len(wireBytes) > 0 {
		wire = DefinitionProviderObservation{
			Content:       append([]byte(nil), wireBytes...),
			ObservedBytes: uint64(len(wireBytes)),
			SHA256:        sumHex(wireBytes),
			Observed:      true,
			Complete:      true,
			Saved:         true,
		}
	}
	d.ProviderCode = code
	d.ProviderFailureClass = failureClass
	d.ProviderDocument = &doc
	d.ProviderWire = &wire
	causeBytes := []byte("unexpected EOF")
	causeObs := DefinitionProviderObservation{
		Content:       causeBytes,
		ObservedBytes: uint64(len(causeBytes)),
		SHA256:        sumHex(causeBytes),
		Observed:      true,
		Complete:      true,
		Saved:         true,
	}
	if failureClass == string(runtimecontract.RuntimeCandidateFailureClassProviderTransient) && code == "request_failed" {
		d.ProviderCause = &causeObs
	}
	return &d
}

func TestAuthoringDerivationTransientRetryBudgetAndNonRetryableRules(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-retry-derive", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	baseID := DefinitionDerivationRequestID(record.InputHash, record.TemplateSnapshotHash)

	for attempt := 0; attempt < AuthoringProviderTransientRetryLimit; attempt++ {
		reqID := record.NextDerivationRequestID()
		wantID := AuthoringRetryRequestID(baseID, attempt)
		if reqID != wantID {
			t.Fatalf("attempt %d next requestID=%q want=%q", attempt, reqID, wantID)
		}
		var req DefinitionDerivationRequest
		var fresh bool
		var err error
		record, req, fresh, err = record.ReserveDerivation(reqID, "provider", nil)
		if err != nil || !fresh {
			t.Fatalf("attempt %d reserve: fresh=%v err=%v", attempt, fresh, err)
		}
		diag := transientDiagnosticFixture("request_failed", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte("{"), false)
		record, err = completeTestDerivation(record, req.PermitID, nil, "provider_failed", diag)
		if err != nil {
			t.Fatalf("attempt %d complete: %v", attempt, err)
		}
		ordinal, retryable := record.DerivationRetryOrdinalForCurrent()
		if !retryable || ordinal != attempt+1 {
			t.Fatalf("attempt %d expected retryable ordinal %d, got ordinal=%d retryable=%v", attempt, attempt+1, ordinal, retryable)
		}
		summaryState, phase, code, _ := record.CurrentAuthoringSummary()
		if summaryState != "deriving" || phase != "" || code != "" {
			t.Fatalf("retryable derivation summary=%q/%q/%q", summaryState, phase, code)
		}
	}

	// Succeed on 3rd retry (4th physical attempt).
	successBranch := record.Clone()
	reqID := successBranch.NextDerivationRequestID()
	if reqID != AuthoringRetryRequestID(baseID, AuthoringProviderTransientRetryLimit) {
		t.Fatalf("final retry ID=%q", reqID)
	}
	successBranch, req, fresh, err := successBranch.ReserveDerivation(reqID, "provider", nil)
	if err != nil || !fresh {
		t.Fatalf("final retry reserve: fresh=%v err=%v", fresh, err)
	}
	derived := DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充"}}
	successBranch, err = completeTestDerivation(successBranch, req.PermitID, &derived, "")
	if err != nil {
		t.Fatalf("final retry complete: %v", err)
	}
	if _, retryable := successBranch.DerivationRetryOrdinalForCurrent(); retryable {
		t.Fatal("completed derivation stayed retryable")
	}
	if summaryState, _, _, _ := successBranch.CurrentAuthoringSummary(); summaryState != "derived" {
		t.Fatalf("completed retry derivation summary=%q want=derived", summaryState)
	}
	if err := successBranch.Validate(); err != nil {
		t.Fatalf("valid transient derivation retry ledger failed Validate: %v", err)
	}

	// Exhaust on 3rd retry (4th failure) and confirm 5th reserve refuses cleanly.
	exhausted := record.Clone()
	exhausted, lastReq, _, err := exhausted.ReserveDerivation(exhausted.NextDerivationRequestID(), "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	diag := transientDiagnosticFixture("stream_ended_before_completion", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte("{"), false)
	exhausted, err = completeTestDerivation(exhausted, lastReq.PermitID, nil, "provider_failed", diag)
	if err != nil {
		t.Fatal(err)
	}
	if _, retryable := exhausted.DerivationRetryOrdinalForCurrent(); retryable {
		t.Fatal("exhausted derivation remained retryable")
	}
	if summaryState, phase, code, _ := exhausted.CurrentAuthoringSummary(); summaryState != "failed" || phase != "derivation" || code != "provider_failed" {
		t.Fatalf("exhausted summary=%q/%q/%q", summaryState, phase, code)
	}
	_, noPermit, fresh, err := exhausted.ReserveDerivation(baseID+"/retry/4", "provider", nil)
	var refusal *DefinitionAuthoringBudgetRefusalError
	if !errors.As(err, &refusal) || refusal.Budget != "attempt" || refusal.Stage != DefinitionAuthoringPreparationDerivation || fresh || noPermit.PermitID != "" {
		t.Fatalf("exhausted 5th reserve did not return attempt budget refusal: %+v %v", noPermit, err)
	}

	// Forged 5th transient derivation in ledger is rejected by Validate().
	forged := exhausted.Clone()
	extra := forged.DerivationRequests[0].Clone()
	extra.RequestID = "forged-5th"
	extra.PermitID = extra.ExpectedPermitID()
	forged.DerivationRequests = append(forged.DerivationRequests, extra)
	if forged.Validate() == nil {
		t.Fatal("ledger with >4 derivation transient attempts passed Validate")
	}

	// request_canceled and tls_verification_failed are never retryable.
	for _, code := range []string{"request_canceled", "tls_verification_failed"} {
		d := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
		d, firstReq, _, err := d.ReserveDerivation(baseID, "provider", nil)
		if err != nil {
			t.Fatal(err)
		}
		nonRetryDiag := transientDiagnosticFixture(code, string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), nil, nil, false)
		d, err = completeTestDerivation(d, firstReq.PermitID, nil, "provider_failed", nonRetryDiag)
		if err != nil {
			t.Fatal(err)
		}
		if _, retryable := d.DerivationRetryOrdinalForCurrent(); retryable {
			t.Fatalf("%s unexpectedly marked retryable", code)
		}
		if state, _, failureCode, _ := d.CurrentAuthoringSummary(); state != "failed" || failureCode != "provider_failed" {
			t.Fatalf("%s summary=%q/%q want=failed/provider_failed", code, state, failureCode)
		}
	}
}

func TestAuthoringSpecificationProposalCriticAndRevisionTransientRetries(t *testing.T) {
	record, proposal := declaredAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID

	// 1. Gen 0 proposal fails twice transiently, then succeeds on /retry/2.
	for attempt := 0; attempt < 2; attempt++ {
		req := DefinitionAuthoringSpecificationRequest{
			WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"),
			RequestID: AuthoringRetryRequestID("proposal-first", attempt), DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider",
		}
		var reserved DefinitionAuthoringSpecificationRequest
		var err error
		record, reserved, _, err = record.ReserveSpecification(req)
		if err != nil {
			t.Fatal(err)
		}
		diag := transientDiagnosticFixture("request_failed", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte(`{"mapping":`), false)
		record, err = completeTestSpecification(record, reserved.PermitID, nil, nil, "provider_failed", diag)
		if err != nil {
			t.Fatal(err)
		}
		if state, _, _, _ := record.CurrentAuthoringSummary(); state != "derived" {
			t.Fatalf("retryable Gen0 proposal summary=%q want=derived", state)
		}
	}
	reqGen0 := DefinitionAuthoringSpecificationRequest{
		WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"),
		RequestID: AuthoringRetryRequestID("proposal-first", 2), DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider",
	}
	var err error
	var reservedGen0 DefinitionAuthoringSpecificationRequest
	record, reservedGen0, _, err = record.ReserveSpecification(reqGen0)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, reservedGen0.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Critics: first 5 pass directly; final critic fails transiently twice then gives UNKNOWN feedback to drive Gen 1.
	for i, kind := range goalReviewKinds {
		if i == len(goalReviewKinds)-1 {
			for attempt := 0; attempt < 2; attempt++ {
				req := DefinitionAuthoringSpecificationRequest{
					WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
					RequestID: AuthoringRetryRequestID("critic-"+kind, attempt), DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider",
				}
				var reserved DefinitionAuthoringSpecificationRequest
				record, reserved, _, err = record.ReserveSpecification(req)
				if err != nil {
					t.Fatal(err)
				}
				diag := transientDiagnosticFixture("http_503", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("upstream 503"), nil, false)
				record, err = completeTestSpecification(record, reserved.PermitID, nil, nil, "provider_failed", diag)
				if err != nil {
					t.Fatal(err)
				}
				if state, _, _, count := record.CurrentAuthoringSummary(); state != "reviewing" || count != len(goalReviewKinds)-1 {
					t.Fatalf("retryable critic summary=%q count=%d", state, count)
				}
			}
		}
		retryOrdinal := 0
		if i == len(goalReviewKinds)-1 {
			retryOrdinal = 2
		}
		req := DefinitionAuthoringSpecificationRequest{
			WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
			RequestID: AuthoringRetryRequestID("critic-"+kind, retryOrdinal), DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider",
		}
		var reserved DefinitionAuthoringSpecificationRequest
		record, reserved, _, err = record.ReserveSpecification(req)
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{
			Kind: kind, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "covered",
			EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal),
		}
		if i == len(goalReviewKinds)-1 {
			review.Status = GoalReviewUnknown
			review.Issues = []GoalSpecificationReviewIssue{{Class: AuthoringIssueDesign, Detail: "Add explicit design", RequirementIDs: review.RequirementIDs, EvidenceRefs: review.EvidenceRefs}}
		}
		record, err = completeTestSpecification(record, reserved.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}

	feedback, err := record.SpecificationFeedback(permit, proposal.ContentHash())
	if err != nil {
		t.Fatalf("SpecificationFeedback after critic retries: %v", err)
	}

	// 3. Gen 1 revision fails once transiently, then succeeds on /retry/1.
	gen1Req := DefinitionAuthoringSpecificationRequest{
		WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"),
		RequestID: "proposal-revise", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider",
		PreviousProposalHash: proposal.ContentHash(), FeedbackHash: feedback.ContentHash(),
	}
	var reservedGen1 DefinitionAuthoringSpecificationRequest
	record, reservedGen1, _, err = record.ReserveSpecification(gen1Req)
	if err != nil {
		t.Fatal(err)
	}
	diag := transientDiagnosticFixture("request_failed", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte(`{"mapping":`), false)
	record, err = completeTestSpecification(record, reservedGen1.PermitID, nil, nil, "provider_failed", diag)
	if err != nil {
		t.Fatal(err)
	}
	if state, _, _, _ := record.CurrentAuthoringSummary(); state != "reviewing" {
		t.Fatalf("retryable Gen1 revision summary=%q want=reviewing", state)
	}

	gen1Retry := gen1Req
	gen1Retry.RequestID = AuthoringRetryRequestID(gen1Req.RequestID, 1)
	record, reservedGen1, _, err = record.ReserveSpecification(gen1Retry)
	if err != nil {
		t.Fatal(err)
	}
	var gen0Mapping *DefinitionAuthoringMapping
	for _, r := range record.SpecificationRequests {
		if r.Proposal != nil && r.Proposal.ContentHash() == proposal.ContentHash() {
			m := r.Mapping.Clone()
			gen0Mapping = &m
			break
		}
	}
	revisedMapping := gen0Mapping.Clone()
	revisedMapping.DesignDecisions = []GoalSpecificationDesignDecision{{CandidateItemRef: revisedMapping.Obligations[0].CandidateItemRef, Decision: "Use bounded retry", VerificationBasis: "Compare fixed test output"}}
	revisedMapping.IssueResolutions = []DefinitionAuthoringIssueResolution{{IssueRef: feedback.IssueRefs()[0], Change: "Added concrete design"}}
	record, err = record.CompleteSpecificationMapping(reservedGen1.PermitID, revisedMapping, mappingAuditFixture(t, revisedMapping))
	if err != nil {
		t.Fatalf("Gen1 revision completion after transient retry: %v", err)
	}
	nextProposal := record.LatestSpecificationRequest(permit).Proposal.Clone()

	// Complete all 6 PASS critics for revised proposal (with a transient retry on critic 0 too).
	for i, kind := range goalReviewKinds {
		if i == 0 {
			req := DefinitionAuthoringSpecificationRequest{
				WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
				RequestID: "gen1-critic-" + kind, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: nextProposal.ContentHash(), ProviderBindingHash: "provider",
			}
			var reserved DefinitionAuthoringSpecificationRequest
			record, reserved, _, err = record.ReserveSpecification(req)
			if err != nil {
				t.Fatal(err)
			}
			record, err = completeTestSpecification(record, reserved.PermitID, nil, nil, "provider_failed", diag)
			if err != nil {
				t.Fatal(err)
			}
		}
		ordinal := 0
		if i == 0 {
			ordinal = 1
		}
		req := DefinitionAuthoringSpecificationRequest{
			WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
			RequestID: AuthoringRetryRequestID("gen1-critic-"+kind, ordinal), DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: nextProposal.ContentHash(), ProviderBindingHash: "provider",
		}
		var reserved DefinitionAuthoringSpecificationRequest
		record, reserved, _, err = record.ReserveSpecification(req)
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{
			Kind: kind, Status: GoalReviewPass, SpecificationHash: nextProposal.ContentHash(), Reason: "covered",
			EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(nextProposal),
		}
		record, err = completeTestSpecification(record, reserved.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := record.Validate(); err != nil {
		t.Fatalf("full transient retry record invalid: %v", err)
	}
	_, confirmedSpec, err := record.ConfirmableSpecification(permit, nextProposal.ContentHash())
	if err != nil || len(confirmedSpec.Reviews) != len(goalReviewKinds) {
		t.Fatalf("ConfirmableSpecification after retries failed: %v", err)
	}
	if state, _, _, count := record.CurrentAuthoringSummary(); state != "reviewed" || count != len(goalReviewKinds) {
		t.Fatalf("final summary=%q count=%d", state, count)
	}

	// Verify DefinitionAuthoringConfirmation.ReadCommitted passes with superseded critic retries.
	items, err := record.DerivationRequests[0].Proposal.CandidateItems(record.Input, &record.TemplateSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	confirmationID := StableDefinitionAuthoringConfirmationID(record.Input.WorkspaceID, record.InputHash, permit, confirmedSpec.AdmissionHash())
	journal := DefinitionAuthoringConfirmation{
		Phase: DefinitionAuthoringConfirmationMaterialized, InputHash: record.InputHash, DerivationPermitID: permit,
		ProposalHash: nextProposal.ContentHash(), SpecificationHash: confirmedSpec.AdmissionHash(),
		TemplateSnapshotHash: record.TemplateSnapshotHash, ResourceSnapshotHash: record.DerivationRequests[0].ResourceSnapshotHash,
		ConfigurationHash: "cfg", FormalSequence: 1,
		Goal: Goal{ID: common.GoalID("authoring-goal-" + confirmationID), WorkspaceID: record.Input.WorkspaceID, Text: record.Input.GoalText, DefinitionRevision: authoringFormalRevisionAt("goal", 1, record.Input.GoalText), LineageState: "active"},
		DoD:  DoD{ID: common.DoDID("authoring-dod-" + confirmationID), WorkspaceID: record.Input.WorkspaceID, Items: texts, State: common.DoDStateReady, DefinitionRevision: authoringFormalRevisionAt("dod", 1, strings.Join(texts, "\n")), LineageState: "active"},
	}
	if err := journal.ReadCommitted(&record, record.Input.WorkspaceID); err != nil {
		t.Fatalf("DefinitionAuthoringConfirmation.ReadCommitted rejected retried ledger: %v", err)
	}
}

func TestAuthoringCriticExhaustionAndNegativeRetryValidation(t *testing.T) {
	record, proposal := declaredAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	req := DefinitionAuthoringSpecificationRequest{
		WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"),
		RequestID: "proposal", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider",
	}
	var err error
	record, req, _, err = record.ReserveSpecification(req)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, req.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}

	kind := GoalReviewVerifierFeasibility
	for attempt := 0; attempt <= AuthoringProviderTransientRetryLimit; attempt++ {
		cReq := DefinitionAuthoringSpecificationRequest{
			WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
			RequestID: AuthoringRetryRequestID("critic-"+kind, attempt), DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider",
		}
		var reserved DefinitionAuthoringSpecificationRequest
		record, reserved, _, err = record.ReserveSpecification(cReq)
		if err != nil {
			t.Fatalf("critic attempt %d reserve: %v", attempt, err)
		}
		diag := transientDiagnosticFixture("request_failed", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte("{"), false)
		record, err = completeTestSpecification(record, reserved.PermitID, nil, nil, "provider_failed", diag)
		if err != nil {
			t.Fatalf("critic attempt %d complete: %v", attempt, err)
		}
		wantState := "reviewing"
		wantCode := ""
		if attempt == AuthoringProviderTransientRetryLimit {
			wantState = "failed"
			wantCode = "provider_failed"
		}
		if state, _, code, _ := record.CurrentAuthoringSummary(); state != wantState || code != wantCode {
			t.Fatalf("critic attempt %d summary=%q/%q want=%q/%q", attempt, state, code, wantState, wantCode)
		}
	}

	// 5th reserve in exhausted critic slot must fail with *DefinitionAuthoringBudgetRefusalError.
	overflow := DefinitionAuthoringSpecificationRequest{
		WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
		RequestID: AuthoringRetryRequestID("critic-"+kind, 4), DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider",
	}
	_, noPermit, fresh, err := record.ReserveSpecification(overflow)
	var refusal *DefinitionAuthoringBudgetRefusalError
	if !errors.As(err, &refusal) || refusal.Stage != DefinitionAuthoringPreparationReview || refusal.Budget != "attempt" || fresh || noPermit.PermitID != "" {
		t.Fatalf("critic overflow did not return budget refusal: %+v %v", noPermit, err)
	}

	// Forging 5th transient attempt directly fails Validate().
	forged5 := record.Clone()
	extra := overflow
	extra.PermitID = extra.ExpectedPermitID()
	forged5.SpecificationRequests = append(forged5.SpecificationRequests, extra)
	if forged5.Validate() == nil {
		t.Fatal("5th critic transient attempt passed Validate")
	}

	// Negative ledger matrix: retries after non-transient / parser / completed / canceled must all reject on Reserve and Validate().
	nonRetryCases := []struct {
		name string
		fail func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord)
	}{
		{
			name: "http_400",
			fail: func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord) {
				base := record.Clone()
				base.SpecificationRequests = base.SpecificationRequests[:1]
				r, res, _, _ := base.ReserveSpecification(overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 0))
				d := transientDiagnosticFixture("http_400", string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration), []byte("bad"), nil, false)
				r, _ = completeTestSpecification(r, res.PermitID, nil, nil, "provider_failed", d)
				return overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 1), r
			},
		},
		{
			name: "output_limit",
			fail: func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord) {
				base := record.Clone()
				base.SpecificationRequests = base.SpecificationRequests[:1]
				r, res, _, _ := base.ReserveSpecification(overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 0))
				d := transientDiagnosticFixture("output_limit", string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol), []byte("wire"), []byte("{"), false)
				r, _ = completeTestSpecification(r, res.PermitID, nil, nil, "provider_failed", d)
				return overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 1), r
			},
		},
		{
			name: "invalid_review",
			fail: func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord) {
				base := record.Clone()
				base.SpecificationRequests = base.SpecificationRequests[:1]
				r, res, _, _ := base.ReserveSpecification(overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 0))
				d := NewDefinitionAuthoringDiagnostic("bad review json", []byte("{}"))
				doc := DefinitionProviderObservation{Content: []byte("{}"), ObservedBytes: 2, SHA256: d.PayloadSHA256, Observed: true, Complete: true, Saved: true}
				wire := doc.Clone()
				d.ProviderDocument, d.ProviderWire = &doc, &wire
				r, _ = completeTestSpecification(r, res.PermitID, nil, nil, "invalid_review", &d)
				return overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 1), r
			},
		},
		{
			name: "request_canceled",
			fail: func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord) {
				base := record.Clone()
				base.SpecificationRequests = base.SpecificationRequests[:1]
				r, res, _, _ := base.ReserveSpecification(overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 0))
				d := transientDiagnosticFixture("request_canceled", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), nil, nil, false)
				r, _ = completeTestSpecification(r, res.PermitID, nil, nil, "provider_failed", d)
				return overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 1), r
			},
		},
		{
			name: "completed_review_reroll",
			fail: func() (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringRecord) {
				base := record.Clone()
				base.SpecificationRequests = base.SpecificationRequests[:1]
				r, res, _, _ := base.ReserveSpecification(overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 0))
				rev := GoalSpecificationCompletenessReview{Kind: GoalReviewTraceability, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "ok", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
				r, _ = completeTestSpecification(r, res.PermitID, nil, &rev, "")
				return overflowReq(permit, GoalReviewTraceability, proposal.ContentHash(), 1), r
			},
		},
	}

	for _, tc := range nonRetryCases {
		t.Run(tc.name, func(t *testing.T) {
			nextReq, r := tc.fail()
			if _, _, _, err := r.ReserveSpecification(nextReq); err == nil {
				t.Fatalf("%s unexpectedly permitted retry reserve", tc.name)
			}
			forged := r.Clone()
			bad := nextReq
			bad.PermitID = bad.ExpectedPermitID()
			forged.SpecificationRequests = append(forged.SpecificationRequests, bad)
			if forged.Validate() == nil {
				t.Fatalf("%s unexpectedly passed Validate with forged successor", tc.name)
			}
		})
	}
}

func TestDefinitionChangeCandidateReviewedIdentityAllowsTransientRetries(t *testing.T) {
	original, proposal := declaredAuthoringLedgerFixture(t)
	record := original.Clone()
	record.Input.Revision++
	record.Input.GoalText = "完整新目标\r\n原文保留"
	record.InputHash = record.Input.InputHash()

	// Fail derivation once transiently, then succeed.
	d0, dReq0, _, err := record.ReserveDerivation("candidate-derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	diag := transientDiagnosticFixture("request_failed", string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), []byte("data: partial"), []byte("{"), false)
	record, err = completeTestDerivation(d0, dReq0.PermitID, nil, "provider_failed", diag)
	if err != nil {
		t.Fatal(err)
	}
	var derivation DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("candidate-derive/retry/1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := *original.DerivationRequests[0].Proposal
	derived.AuthoringInputHash = record.InputHash
	record, err = completeTestDerivation(record, derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	proposal.GoalTextHash = StableDefinitionTextHash(record.Input.GoalText)
	proposal.Requirements[0].Text = record.Input.GoalText
	_, _, proposal = currentProposalFixture(t, record.DerivationRequests[len(record.DerivationRequests)-1], proposal)

	// Fail proposal once transiently, then succeed.
	var request DefinitionAuthoringSpecificationRequest
	record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "candidate-proposal", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestSpecification(record, request.PermitID, nil, nil, "provider_failed", diag)
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "candidate-proposal/retry/1", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, request.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}
	for i, kind := range goalReviewKinds {
		if i == 0 {
			record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind), RequestID: "candidate-review-" + kind, DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
			if err != nil {
				t.Fatal(err)
			}
			record, err = completeTestSpecification(record, request.PermitID, nil, nil, "provider_failed", diag)
			if err != nil {
				t.Fatal(err)
			}
		}
		retry := 0
		if i == 0 {
			retry = 1
		}
		record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind), RequestID: AuthoringRetryRequestID("candidate-review-"+kind, retry), DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立完整覆盖", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
		record, err = completeTestSpecification(record, request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	w := &Workspace{ID: original.Input.WorkspaceID, Revision: 2, DefinitionAuthoring: &original}
	c := DefinitionChangeCandidate{RequestID: "prepare", ExpectedWorkspaceRevision: 1, RecordedWorkspaceRevision: 2, TicketID: "ticket", ProposalRevision: 1, ProposalHash: "source-proposal", BaseInputHash: original.InputHash, Record: record}
	identity, err := c.ReviewedIdentity(w)
	if err != nil || identity.InputHash != c.Record.InputHash {
		t.Fatalf("ReviewedIdentity rejected retried change candidate: %+v %v", identity, err)
	}
}

func overflowReq(derivationPermitID, kind, proposalHash string, retry int) DefinitionAuthoringSpecificationRequest {
	return DefinitionAuthoringSpecificationRequest{
		WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind),
		RequestID: AuthoringRetryRequestID(fmt.Sprintf("critic-%s", kind), retry), DerivationPermitID: derivationPermitID,
		Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposalHash, ProviderBindingHash: "provider",
	}
}
