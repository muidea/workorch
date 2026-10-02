package workspace

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func confirmedAuthoringSourceFixture(t testing.TB) (DefinitionAuthoringRecord, DefinitionAuthoringConfirmation) {
	t.Helper()
	record, proposal := authoringLedgerFixture(t)
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
	proposalHash := proposal.ContentHash()
	for _, kind := range goalReviewKinds {
		record, request, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "review-" + kind, DerivationPermitID: permit, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposalHash, ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: proposalHash, Reason: "独立审查", EvidenceRefs: []string{DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal"}}
		record, err = record.CompleteSpecification(request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	derivation, specification, err := record.ConfirmableSpecification(permit, proposalHash)
	if err != nil {
		t.Fatal(err)
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, len(items))
	for index, item := range items {
		texts[index] = item.Text
	}
	id := StableDefinitionAuthoringConfirmationID(record.Input.WorkspaceID, record.InputHash, permit, specification.AdmissionHash())
	confirmation := DefinitionAuthoringConfirmation{
		Phase: DefinitionAuthoringConfirmationMaterialized, InputHash: record.InputHash,
		DerivationPermitID: permit, ProposalHash: proposalHash, SpecificationHash: specification.AdmissionHash(),
		TemplateSnapshotHash: record.TemplateSnapshotHash, ResourceSnapshotHash: derivation.ResourceSnapshotHash, ConfigurationHash: "config",
		Goal: Goal{ID: common.GoalID("authoring-goal-" + id), WorkspaceID: record.Input.WorkspaceID, Text: record.Input.GoalText, DefinitionRevision: authoringFormalRevision("goal", record.Input.GoalText), LineageState: "active"},
		DoD:  DoD{ID: common.DoDID("authoring-dod-" + id), WorkspaceID: record.Input.WorkspaceID, Items: texts, State: common.DoDStateReady, DefinitionRevision: authoringFormalRevision("dod", strings.Join(texts, "\n")), LineageState: "active"},
	}
	if err := confirmation.Validate(&record, record.Input.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	return record, confirmation
}

func TestConfirmedAuthoringAdmissionSourceValidatesEntireLedgerPerCall(t *testing.T) {
	record, confirmation := confirmedAuthoringSourceFixture(t)
	var historical DefinitionDerivationRequest
	var err error
	record, historical, _, err = record.ReserveDerivation("unconfirmed-derivation", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := *record.DerivationRequests[0].Proposal
	record, err = record.CompleteDerivation(historical.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	record, _, _, err = record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "unconfirmed-proposal", DerivationPermitID: historical.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	before := record.Clone()
	goal, dod, spec, ref, err := ConfirmedAuthoringAdmissionSource(&record, confirmation, record.Input.WorkspaceID)
	if err != nil || ref.InputHash != confirmation.InputHash || ref.SpecificationHash != spec.AdmissionHash() || goal.Text != confirmation.Goal.Text || len(dod) != len(confirmation.DoD.Items) {
		t.Fatalf("合并校验改变正式来源: %v", err)
	}
	dod[0].Text = "调用方修改"
	spec.Requirements[0].Text = "调用方修改"
	if !reflect.DeepEqual(before, record) {
		t.Fatal("返回数据保留了 ledger 可变引用")
	}
	for name, change := range map[string]func(*DefinitionAuthoringRecord){
		"input":    func(r *DefinitionAuthoringRecord) { r.Input.GoalText += " changed" },
		"template": func(r *DefinitionAuthoringRecord) { r.TemplateSnapshot.Description += " changed" },
		"proposal": func(r *DefinitionAuthoringRecord) {
			r.SpecificationRequests[0].Proposal.Obligations[0].Text += " changed"
		},
		"review_hash":    func(r *DefinitionAuthoringRecord) { r.SpecificationRequests[1].Review.SpecificationHash = "forged" },
		"review_unknown": func(r *DefinitionAuthoringRecord) { r.SpecificationRequests[1].Review.Status = GoalReviewUnknown },
		"review_provenance": func(r *DefinitionAuthoringRecord) {
			r.SpecificationRequests[1].Review.RequirementIDs = []string{"missing"}
		},
		"unselected_derivation": func(r *DefinitionAuthoringRecord) { r.DerivationRequests[1].Template.Description += " changed" },
		"unselected_request": func(r *DefinitionAuthoringRecord) {
			r.SpecificationRequests[len(r.SpecificationRequests)-1].PermitID = "forged"
		},
		"duplicate_review": func(r *DefinitionAuthoringRecord) {
			duplicate := r.SpecificationRequests[1].Clone()
			duplicate.RequestID = "another-review"
			duplicate.PermitID = duplicate.ExpectedPermitID()
			r.SpecificationRequests = append(r.SpecificationRequests, duplicate)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := record.Clone()
			change(&changed)
			if confirmation.Validate(&changed, record.Input.WorkspaceID) == nil {
				t.Fatal("confirmation 忽略 ledger 篡改")
			}
			if _, _, _, _, err := ConfirmedAuthoringAdmissionSource(&changed, confirmation, record.Input.WorkspaceID); err == nil {
				t.Fatal("合并校验复用了前一次成功身份")
			}
			if _, _, _, err := ConfirmedAuthoringCandidateInputs(&changed, confirmation, record.Input.WorkspaceID); err == nil {
				t.Fatal("正式输入入口忽略 ledger 篡改")
			}
			if _, err := ConfirmedAuthoringAdmissionRef(&changed, confirmation, record.Input.WorkspaceID); err == nil {
				t.Fatal("准入引用入口忽略 ledger 篡改")
			}
		})
	}
}

func TestConfirmedAuthoringAdmissionSourceRejectsCrossWorkspaceAfterImages(t *testing.T) {
	record, confirmation := confirmedAuthoringSourceFixture(t)
	other := common.WorkspaceID("other-workspace")
	id := StableDefinitionAuthoringConfirmationID(other, confirmation.InputHash, confirmation.DerivationPermitID, confirmation.SpecificationHash)
	confirmation.Goal.ID, confirmation.Goal.WorkspaceID = common.GoalID("authoring-goal-"+id), other
	confirmation.DoD.ID, confirmation.DoD.WorkspaceID = common.DoDID("authoring-dod-"+id), other
	if record.Validate() != nil {
		t.Fatal("跨 scope fixture 不得依赖坏 ledger")
	}
	if confirmation.Validate(&record, other) == nil {
		t.Fatal("其他 Workspace 的 after-images 复用了有效 ledger")
	}
	if _, _, _, _, err := ConfirmedAuthoringAdmissionSource(&record, confirmation, other); err == nil {
		t.Fatal("来源校验未绑定 record Workspace")
	}
	if _, _, _, err := ConfirmedAuthoringCandidateInputs(&record, confirmation, other); err == nil {
		t.Fatal("正式输入未绑定 record Workspace")
	}
	if _, err := ConfirmedAuthoringAdmissionRef(&record, confirmation, other); err == nil {
		t.Fatal("准入引用未绑定 record Workspace")
	}
}

func BenchmarkConfirmedAuthoringAdmissionSource(b *testing.B) {
	record, confirmation := confirmedAuthoringSourceFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, _, _, err := ConfirmedAuthoringAdmissionSource(&record, confirmation, record.Input.WorkspaceID); err != nil {
			b.Fatal(err)
		}
	}
}
