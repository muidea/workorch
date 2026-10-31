package workspace

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestConfirmableSpecificationRequiresUniqueCompletedPASSReviews(t *testing.T) {
	record, proposal := declaredAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	proposalHash := proposal.ContentHash()
	request := DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	var err error
	var reserved DefinitionAuthoringSpecificationRequest
	record, reserved, _, err = record.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, reserved.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := record.ConfirmableSpecification(permit, proposalHash); err == nil {
		t.Fatal("proposal without six reviews was confirmable")
	}
	for _, kind := range goalReviewKinds {
		request = DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind), RequestID: "review-" + kind, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposalHash, ProviderBindingHash: "provider"}
		record, reserved, _, err = record.ReserveSpecification(request)
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposalHash, Reason: "覆盖", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
		record, err = completeTestSpecification(record, reserved.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	_, reviewed, err := record.ConfirmableSpecification(permit, proposalHash)
	if err != nil || len(reviewed.Reviews) != len(goalReviewKinds) {
		t.Fatalf("six PASS reviews not reconstructed: %+v %v", reviewed.Reviews, err)
	}
	if _, _, err := record.ConfirmableSpecification(permit, "different"); err == nil {
		t.Fatal("different proposal hash confirmed")
	}
	unknown := record.Clone()
	unknown.SpecificationRequests[len(unknown.SpecificationRequests)-1].Review.Status = GoalReviewUnknown
	if _, _, err := unknown.ConfirmableSpecification(permit, proposalHash); err == nil {
		t.Fatal("UNKNOWN review confirmed")
	}
	duplicate := record.Clone()
	extra := duplicate.SpecificationRequests[len(duplicate.SpecificationRequests)-1].Clone()
	extra.RequestID = "duplicate-review"
	extra.PermitID = extra.ExpectedPermitID()
	duplicate.SpecificationRequests = append(duplicate.SpecificationRequests, extra)
	if _, _, err := duplicate.ConfirmableSpecification(permit, proposalHash); err == nil {
		t.Fatal("ambiguous review confirmed")
	}
	input := record.Input.Clone()
	input.Revision++
	input.GoalText = "edited"
	record.Input, record.InputHash = input, input.InputHash()
	if _, _, err := record.ConfirmableSpecification(permit, proposalHash); err == nil {
		t.Fatal("stale revision confirmed")
	}
}

func TestAuthoringConfirmationJournalRejectsChangedAfterImages(t *testing.T) {
	record, proposal := declaredAuthoringLedgerFixture(t)
	permit := record.DerivationRequests[0].PermitID
	var err error
	var request DefinitionAuthoringSpecificationRequest
	record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal", DerivationPermitID: permit, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeCurrentProposalFixture(t, record, request.PermitID, &proposal)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range goalReviewKinds {
		record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, kind), RequestID: "review-" + kind, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "覆盖", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: currentRequiredRequirementIDsFixture(proposal)}
		record, err = completeTestSpecification(record, request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	derivation := record.DerivationRequests[0]
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	_, reviewed, err := record.ConfirmableSpecification(permit, proposal.ContentHash())
	if err != nil {
		t.Fatal(err)
	}
	id := StableDefinitionAuthoringConfirmationID("ws", record.InputHash, permit, reviewed.AdmissionHash())
	journal := DefinitionAuthoringConfirmation{Phase: DefinitionAuthoringConfirmationPrepared, InputHash: record.InputHash, DerivationPermitID: permit, ProposalHash: proposal.ContentHash(), SpecificationHash: reviewed.AdmissionHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, ResourceSnapshotHash: derivation.ResourceSnapshotHash, ConfigurationHash: "config", Goal: Goal{ID: common.GoalID("authoring-goal-" + id), WorkspaceID: "ws", Text: record.Input.GoalText, DefinitionRevision: authoringFormalRevision("goal", record.Input.GoalText), LineageState: "active"}, DoD: DoD{ID: common.DoDID("authoring-dod-" + id), WorkspaceID: "ws", Items: texts, State: common.DoDStateReady, DefinitionRevision: authoringFormalRevision("dod", "用户要求\n模板要求\n补充要求"), LineageState: "active"}}
	if err := journal.Validate(&record, "ws"); err != nil {
		t.Fatal(err)
	}
	journal.Phase = DefinitionAuthoringConfirmationMaterialized
	goal, formalItems, formal, err := ConfirmedAuthoringCandidateInputs(&record, journal, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if goal.CandidateItemRef != DefinitionAuthoringGoalItemRef || goal.SourceIdentity != "goal:"+string(journal.Goal.ID)+"@"+journal.Goal.DefinitionRevision || formal.GoalTextHash != proposal.GoalTextHash || formal.AdmissionHash() != reviewed.AdmissionHash() {
		t.Fatalf("candidate specification lost reviewed identity: %+v %+v", goal, formal)
	}
	if len(formalItems) != len(journal.DoD.Items) || formalItems[0].CandidateItemRef != "derived-dod:user:000001" || formalItems[0].SourceIdentity != "dod:"+string(journal.DoD.ID)+"@"+journal.DoD.DefinitionRevision {
		t.Fatalf("candidate DoD mapping lost source identity: %+v", formalItems)
	}
	if len(formal.Reviews) != len(goalReviewKinds) || formal.Reviews[0].SpecificationHash != proposal.ContentHash() || formal.Reviews[0].EvidenceRefs[0] != DefinitionAuthoringGoalItemRef {
		t.Fatalf("review provenance was rewritten after materialization: %+v", formal.Reviews)
	}
	ref, err := ConfirmedAuthoringAdmissionRef(&record, journal, "ws")
	if err != nil || ref.InputHash != journal.InputHash || ref.SpecificationHash != formal.AdmissionHash() {
		t.Fatalf("admission reference lost persisted review provenance: %+v %v", ref, err)
	}
	for index, verification := range formal.Verifications {
		if verification != reviewed.Verifications[index] {
			t.Fatalf("formal translation changed reviewed declaration: %+v", verification)
		}
	}
	for _, change := range []func(*DefinitionAuthoringRecord){
		func(r *DefinitionAuthoringRecord) {
			r.SpecificationRequests[len(r.SpecificationRequests)-1].Review.Status = GoalReviewUnknown
		},
		func(r *DefinitionAuthoringRecord) { r.Input.Revision++ },
	} {
		changed := record.Clone()
		change(&changed)
		if _, _, _, err := ConfirmedAuthoringCandidateInputs(&changed, journal, "ws"); err == nil {
			t.Fatal("stale or downgraded review was translated")
		}
	}
	journal.Phase = DefinitionAuthoringConfirmationPrepared
	if _, _, _, err := ConfirmedAuthoringCandidateInputs(&record, journal, "ws"); err == nil {
		t.Fatal("prepared confirmation became formal specification")
	}
	for _, change := range []func(*DefinitionAuthoringConfirmation){
		func(j *DefinitionAuthoringConfirmation) { j.DoD.Items[0] = "changed" },
		func(j *DefinitionAuthoringConfirmation) { j.Goal.ID = "other" },
		func(j *DefinitionAuthoringConfirmation) { j.Goal.DefinitionRevision = "other" },
		func(j *DefinitionAuthoringConfirmation) { j.ConfigurationHash = "" },
		func(j *DefinitionAuthoringConfirmation) { j.SpecificationHash = "other" },
	} {
		changed := journal.Clone()
		change(&changed)
		if err := changed.Validate(&record, "ws"); err == nil {
			t.Fatal("changed confirmation after-image passed validation")
		}
	}
}
