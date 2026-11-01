package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func feedbackLedgerFixture(t *testing.T, class string) (DefinitionAuthoringRecord, GoalSpecification) {
	t.Helper()
	r, seed := declaredAuthoringLedgerFixture(t)
	d := r.DerivationRequests[0]
	q := DefinitionAuthoringSpecificationRequest{WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, "proposal"), RequestID: "first", DerivationPermitID: d.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: d.ProviderBindingHash}
	r, q, _, err := r.ReserveSpecification(q)
	if err != nil {
		t.Fatal(err)
	}
	r, err = completeCurrentProposalFixture(t, r, q.PermitID, &seed)
	if err != nil {
		t.Fatal(err)
	}
	return completeFeedbackReviews(t, r, seed, class), seed
}

func completeFeedbackReviews(t *testing.T, r DefinitionAuthoringRecord, s GoalSpecification, class string) DefinitionAuthoringRecord {
	t.Helper()
	d := r.DerivationRequests[0]
	for i, kind := range goalReviewKinds {
		q := DefinitionAuthoringSpecificationRequest{WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, kind), RequestID: kind + "/" + s.ContentHash(), DerivationPermitID: d.PermitID, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: s.ContentHash(), ProviderBindingHash: d.ProviderBindingHash}
		var err error
		r, q, _, err = r.ReserveSpecification(q)
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: s.ContentHash(), Reason: "fixed independent original obligations", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(s)}
		if i == 0 && class != "" {
			review.Status = GoalReviewUnknown
			review.Issues = []GoalSpecificationReviewIssue{{Class: class, Detail: "missing concrete project decision and independent expected outcome", RequirementIDs: review.RequirementIDs, EvidenceRefs: review.EvidenceRefs}}
		}
		r, err = completeTestSpecification(r, q.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func correctiveRequest(t *testing.T, r DefinitionAuthoringRecord, s GoalSpecification) (DefinitionAuthoringSpecificationRequest, DefinitionAuthoringFeedback) {
	t.Helper()
	d := r.DerivationRequests[0]
	f, err := r.SpecificationFeedback(d.PermitID, s.ContentHash())
	if err != nil {
		t.Fatal(err)
	}
	return DefinitionAuthoringSpecificationRequest{WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, "proposal"), RequestID: "one-correction", DerivationPermitID: d.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: d.ProviderBindingHash, PreviousProposalHash: s.ContentHash(), FeedbackHash: f.ContentHash()}, f
}

func TestAuthoringFeedbackRevisionPreservesOriginalsAndRequiresNewExactConfirmation(t *testing.T) {
	r, s := feedbackLedgerFixture(t, AuthoringIssueDesign)
	original, _ := json.Marshal(r)
	q, f := correctiveRequest(t, r, s)
	pending, reserved, fresh, err := r.ReserveSpecification(q)
	if err != nil || !fresh {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := pending.ConfirmableSpecification(q.DerivationPermitID, s.ContentHash()); err == nil {
		t.Fatal("old candidate confirmed after successor reservation")
	}
	if _, _, fresh, err := pending.ReserveSpecification(q); err != nil || fresh {
		t.Fatal("duplicate granted another physical call", err)
	}
	for _, mutate := range []func(*DefinitionAuthoringSpecificationRequest){func(q *DefinitionAuthoringSpecificationRequest) { q.RequestID = "random" }, func(q *DefinitionAuthoringSpecificationRequest) { q.FeedbackHash = "foreign" }, func(q *DefinitionAuthoringSpecificationRequest) { q.PreviousProposalHash = "stale" }} {
		bad := q
		mutate(&bad)
		if _, _, _, err := pending.ReserveSpecification(bad); err == nil {
			t.Fatal("pending/stale feedback granted another call")
		}
	}
	m := r.SpecificationRequests[0].Mapping.Clone()
	m.DesignDecisions = []GoalSpecificationDesignDecision{{CandidateItemRef: m.Obligations[0].CandidateItemRef, Decision: "Select an explicit in-scope implementation rule", VerificationBasis: "Compare fixed input and expected output independently"}}
	m.IssueResolutions = []DefinitionAuthoringIssueResolution{{IssueRef: f.IssueRefs()[0], Change: "Added concrete design and independent verification basis under the original obligation"}}
	for _, name := range []string{"same_candidate", "missing_response", "unknown_issue"} {
		bad := m.Clone()
		switch name {
		case "same_candidate":
			bad.DesignDecisions = nil
		case "missing_response":
			bad.IssueResolutions = nil
		case "unknown_issue":
			bad.IssueResolutions[0].IssueRef = "foreign"
		}
		if _, err := pending.CompleteSpecificationMapping(reserved.PermitID, bad, mappingAuditFixture(t, bad)); err == nil {
			t.Fatal("invalid revision accepted", name)
		}
	}
	completed, err := pending.CompleteSpecificationMapping(reserved.PermitID, m, mappingAuditFixture(t, m))
	if err != nil {
		t.Fatal(err)
	}
	var reopened DefinitionAuthoringRecord
	raw, _ := json.Marshal(completed)
	if json.Unmarshal(raw, &reopened) != nil || reopened.Validate() != nil {
		t.Fatal("revision lost on reopen")
	}
	next := reopened.LatestSpecificationRequest(q.DerivationPermitID).Proposal.Clone()
	if next.ContentHash() == s.ContentHash() {
		t.Fatal("revision did not bind changed design")
	}
	if _, _, err := reopened.ConfirmableSpecification(q.DerivationPermitID, next.ContentHash()); err == nil {
		t.Fatal("new candidate inherited old reviews")
	}
	finished := completeFeedbackReviews(t, reopened, next, "")
	_, reviewed, err := finished.ConfirmableSpecification(q.DerivationPermitID, next.ContentHash())
	if err != nil || len(reviewed.Reviews) != 6 {
		t.Fatal(err)
	}
	if _, _, err := finished.ConfirmableSpecification(q.DerivationPermitID, s.ContentHash()); err == nil {
		t.Fatal("old confirmation identity accepted")
	}
	if !reflect.DeepEqual(r.Input, finished.Input) || !reflect.DeepEqual(r.DerivationRequests, finished.DerivationRequests) || !reflect.DeepEqual(r.SpecificationRequests, finished.SpecificationRequests[:len(r.SpecificationRequests)]) {
		t.Fatal("revision rewrote original scope or evidence")
	}
	_, _, fresh, err = finished.ReserveSpecification(q)
	if err != nil || fresh {
		t.Fatal("completed revision replay consumed budget", err)
	}
	q.RequestID = "third"
	q.PreviousProposalHash = next.ContentHash()
	if _, _, _, err := finished.ReserveSpecification(q); err == nil {
		t.Fatal("third proposal exceeded bounded correction")
	}
	after, _ := json.Marshal(r)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("caller original changed")
	}
}

func TestAuthoringFeedbackDoesNotResamplePassingMissingUnknownOrForeignInputs(t *testing.T) {
	for _, class := range []string{"", AuthoringIssueClarification, AuthoringIssueCapability} {
		r, s := feedbackLedgerFixture(t, class)
		if _, err := r.SpecificationFeedback(r.DerivationRequests[0].PermitID, s.ContentHash()); err == nil {
			t.Fatal("noncorrective feedback eligible", class)
		}
	}
	r, s := feedbackLedgerFixture(t, AuthoringIssueVerification)
	for n := 1; n < 7; n++ {
		partial := r.Clone()
		partial.SpecificationRequests = partial.SpecificationRequests[:n]
		if _, err := partial.SpecificationFeedback(r.DerivationRequests[0].PermitID, s.ContentHash()); err == nil {
			t.Fatal("partial feedback eligible")
		}
	}
	q, _ := correctiveRequest(t, r, s)
	foreign := r.Clone()
	foreign.Input.WorkspaceID = "another-workspace"
	foreign.InputHash = foreign.Input.InputHash()
	if _, _, _, err := foreign.ReserveSpecification(q); err == nil {
		t.Fatal("foreign workspace reused revision")
	}
	if _, err := r.SpecificationFeedback(q.DerivationPermitID, "foreign-candidate"); err == nil {
		t.Fatal("foreign candidate feedback accepted")
	}
}

func TestAuthoringSourceGroupingIsBoundedDetachedAndRetainsEveryObligation(t *testing.T) {
	r, m, _ := fullSizeMappingFixture(t)
	d := r.DerivationRequests[0]
	a, err := BindAuthoringSourceGroups(d, m)
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		b, err := BindAuthoringSourceGroups(d, m)
		if err != nil || !reflect.DeepEqual(a, b) {
			t.Fatal("grouping drift")
		}
	}
	s, err := CompileDefinitionAuthoringMapping(d, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Obligations) != 37 || len(s.Requirements) != 4 {
		t.Fatal("lost original obligations or expanded arbitrary groups")
	}
	originalRef := m.Requirements[0].SourceItemRefs[0]
	a.Requirements[0].SourceItemRefs[0] = "mutated"
	if m.Requirements[0].SourceItemRefs[0] != originalRef {
		t.Fatal("grouping not detached")
	}
	bad := m.Clone()
	bad.DesignDecisions = []GoalSpecificationDesignDecision{{CandidateItemRef: "foreign", Decision: "x", VerificationBasis: "y"}}
	if _, err := CompileDefinitionAuthoringMapping(d, bad); err == nil {
		t.Fatal("foreign design source accepted")
	}
}

func TestAuthoringReviewIssuesRejectMissingUnclassifiedAndForeignSources(t *testing.T) {
	r, s := feedbackLedgerFixture(t, AuthoringIssueDesign)
	q := r.SpecificationRequests[1]
	d := r.DerivationRequests[0]
	context, err := newAuthoringSpecificationContext(d.Input, &d.Template, *d.Proposal, d.Resources)
	if err != nil {
		t.Fatal(err)
	}
	reviewContext := authoringProposalReviewContext(context, s, s.ContentHash())
	for _, name := range []string{"missing", "class", "foreign_requirement", "foreign_evidence", "empty_detail", "pass_with_issue"} {
		t.Run(name, func(t *testing.T) {
			v := *q.Clone().Review
			switch name {
			case "missing":
				v.Issues = nil
			case "class":
				v.Issues[0].Class = "retry_me"
			case "foreign_requirement":
				v.Issues[0].RequirementIDs = []string{"another-workspace"}
			case "foreign_evidence":
				v.Issues[0].EvidenceRefs = []string{"another-profile"}
			case "empty_detail":
				v.Issues[0].Detail = " \n"
			case "pass_with_issue":
				v.Status = GoalReviewPass
			}
			raw, err := json.Marshal(struct {
				Review struct {
					Status         string
					Reason         string
					RequirementIDs []string
					EvidenceRefs   []string
					Issues         []GoalSpecificationReviewIssue `json:",omitempty"`
				} `json:"review"`
			}{Review: struct {
				Status         string
				Reason         string
				RequirementIDs []string
				EvidenceRefs   []string
				Issues         []GoalSpecificationReviewIssue `json:",omitempty"`
			}{v.Status, v.Reason, v.RequirementIDs, v.EvidenceRefs, v.Issues}})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := DecodeOwnerBoundReview(q, raw)
			if err == nil && reviewContext.validate(parsed) == nil {
				t.Fatal("invalid issue admitted")
			}
		})
	}
}

func TestAuthoringCorrectivePreparationDiagnosticsFollowExactNextRequest(t *testing.T) {
	r, s := feedbackLedgerFixture(t, AuthoringIssueDesign)
	q, _ := correctiveRequest(t, r, s)
	d := r.DerivationRequests[0]
	diagnostic := DefinitionAuthoringPreparationDiagnostic{InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash, DerivationPermitID: d.PermitID, Stage: DefinitionAuthoringPreparationProposal, TargetRequestID: SpecificationRequestID(d.PermitID, "", "", q.FeedbackHash)}
	if !r.PreparationDiagnosticCurrent(&d, diagnostic) {
		t.Fatal("corrective preparation failure hidden")
	}
	stale := diagnostic
	stale.TargetRequestID = SpecificationRequestID(d.PermitID, "", "", "")
	if r.PreparationDiagnosticCurrent(&d, stale) {
		t.Fatal("original proposal failure reused for correction")
	}
	pending, _, _, err := r.ReserveSpecification(q)
	if err != nil || pending.PreparationDiagnosticCurrent(&d, diagnostic) {
		t.Fatal("pending provider relabeled preparation failure", err)
	}
}
