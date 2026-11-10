package workspace

import (
	"reflect"
	"testing"
)

func TestCommittedAuthoringReadsKeepSourcesAndAfterImages(t *testing.T) {
	record, confirmation := confirmedAuthoringSourceFixture(t)
	w := &Workspace{ID: record.Input.WorkspaceID, DefinitionAuthoring: &record, DefinitionAuthoringConfirmation: &confirmation}
	original := record.Clone()
	for range 3 {
		if err := w.ReadCommittedAuthoring(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(original, record) {
		t.Fatal("read rewrote original authoring")
	}
	for _, scenario := range []string{"input", "proposal", "review", "wire", "goal", "dod", "source", "history"} {
		t.Run(scenario, func(t *testing.T) {
			r, c := record.Clone(), confirmation.Clone()
			changed := *w
			changed.DefinitionAuthoring, changed.DefinitionAuthoringConfirmation = &r, &c
			switch scenario {
			case "input":
				r.Input.GoalText += "drift"
			case "proposal":
				r.SpecificationRequests[0].Proposal.Requirements[0].Text += "drift"
			case "review":
				r.SpecificationRequests[1].Review.Reason += "drift"
			case "wire":
				r.SpecificationRequests[0].SuccessAudit.Document.Content[0] ^= 1
			case "goal":
				c.Goal.Text += "drift"
			case "dod":
				c.DoD.Items[0] += "drift"
			case "source":
				c.DerivationPermitID += "foreign"
			case "history":
				c.FormalSequence = 2
				c.PreviousInputHash = "missing"
			}
			if changed.ReadCommittedAuthoring() == nil {
				t.Fatal("read accepted changed source/identity")
			}
		})
	}
}

func TestCommittedAdmissionOutcomeAppendAndImmutability(t *testing.T) {
	c := (DefinitionAdmissionCandidate{WorkspaceID: "ws", CandidateID: "candidate", CandidateRevision: 1, AdmissionCompilerVersion: DefinitionAdmissionCompilerVersionCurrent}).WithHash()
	before := &Workspace{ID: "ws", DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{c}}
	next := *before
	outcome := AdmittedDefinitionResult(c)
	next.DefinitionAdmissionResults = []DefinitionAdmissionResult{outcome}
	if err := ValidateDefinitionAdmissionResultTransition(before, &next); err != nil {
		t.Fatalf("first owner result: %v", err)
	}
	if err := ValidateDefinitionAdmissionResultTransition(&next, &next); err != nil {
		t.Fatalf("idempotent read: %v", err)
	}
	for _, scenario := range []string{"erase", "rewrite", "duplicate", "candidate", "scope"} {
		t.Run(scenario, func(t *testing.T) {
			changed := next
			changed.DefinitionAdmissionResults = []DefinitionAdmissionResult{outcome.Clone()}
			changed.DefinitionAdmissionCandidates = []DefinitionAdmissionCandidate{c.Clone()}
			switch scenario {
			case "erase":
				changed.DefinitionAdmissionResults = nil
			case "rewrite":
				changed.DefinitionAdmissionResults[0].CanonicalAdmittedInputHash = "forged"
				changed.DefinitionAdmissionResults[0] = changed.DefinitionAdmissionResults[0].WithHash()
			case "duplicate":
				changed.DefinitionAdmissionResults = append(changed.DefinitionAdmissionResults, outcome)
			case "candidate":
				changed.DefinitionAdmissionCandidates[0].GoalInput.Text += "changed"
			case "scope":
				changed.ID = "other"
			}
			if ValidateDefinitionAdmissionResultTransition(&next, &changed) == nil {
				t.Fatal("committed result rewritten or detached from source")
			}
		})
	}
	// A new revision requires its own formal result; it cannot borrow the old one.
	newer := c.Clone()
	newer.CandidateRevision++
	newer = newer.WithHash()
	if _, err := next.ReadDefinitionAdmissionResult(newer); err == nil {
		t.Fatal("new input borrowed old admission")
	}
	got, err := next.ReadDefinitionAdmissionResult(c)
	if err != nil || !reflect.DeepEqual(got, outcome) {
		t.Fatalf("committed read: %v", err)
	}
}

func TestCommittedAuthoringTransitionProtectsOriginalOutcome(t *testing.T) {
	record, confirmation := confirmedAuthoringSourceFixture(t)
	before := &Workspace{ID: record.Input.WorkspaceID, DefinitionAuthoring: &record, DefinitionAuthoringConfirmation: &confirmation}
	if err := ValidateCommittedAuthoringTransition(before, before); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"proposal", "review", "raw", "erase", "history"} {
		t.Run(scenario, func(t *testing.T) {
			next := *before
			r := record.Clone()
			next.DefinitionAuthoring = &r
			switch scenario {
			case "proposal":
				r.SpecificationRequests[0].Proposal.Requirements[0].Text += "changed"
			case "review":
				r.SpecificationRequests[1].Review.Reason += "changed"
			case "raw":
				r.SpecificationRequests[0].SuccessAudit.Document.Content[0] ^= 1
			case "erase":
				r.SpecificationRequests = nil
			case "history":
				next.DefinitionAuthoring = nil
			}
			if ValidateCommittedAuthoringTransition(before, &next) == nil {
				t.Fatal("rewrote completed source")
			}
		})
	}
}

func TestCommittedConfirmationTransitionKeepsAfterImages(t *testing.T) {
	record, c := confirmedAuthoringSourceFixture(t)
	c.Phase = DefinitionAuthoringConfirmationPrepared
	before := &Workspace{ID: record.Input.WorkspaceID, DefinitionAuthoring: &record, DefinitionAuthoringConfirmation: &c}
	materialized := c.Clone()
	materialized.Phase = DefinitionAuthoringConfirmationMaterialized
	next := *before
	next.DefinitionAuthoringConfirmation = &materialized
	if err := ValidateCommittedAuthoringTransition(before, &next); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"goal", "dod", "erase", "phase_regression"} {
		t.Run(scenario, func(t *testing.T) {
			changed := next
			value := materialized.Clone()
			changed.DefinitionAuthoringConfirmation = &value
			switch scenario {
			case "goal":
				value.Goal.Text += "changed"
			case "dod":
				value.DoD.Items[0] += "changed"
			case "erase":
				changed.DefinitionAuthoringConfirmation = nil
			case "phase_regression":
				value.Phase = DefinitionAuthoringConfirmationPrepared
			}
			if ValidateCommittedAuthoringTransition(&next, &changed) == nil {
				t.Fatal("rewrote committed confirmation")
			}
		})
	}
}
