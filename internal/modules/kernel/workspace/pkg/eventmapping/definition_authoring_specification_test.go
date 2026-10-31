package eventmapping

import (
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
)

func TestAuthoringDTOClonesPersistedProposalAndReviewWithoutRehashing(t *testing.T) {
	record := authoringMappingFixture(t)
	derivation := record.DerivationRequests[1]
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	mapping, audit, proposal, err := testkit.Proposal(derivation, workspace.DefinitionAuthoringVerificationMapping{Declaration: "尚未声明", Status: workspace.GoalVerificationUnverified})
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "proposal"), RequestID: "second-proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteSpecificationMapping(request.PermitID, mapping, audit)
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "dimension_coverage"), RequestID: "critic", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: "dimension_coverage", ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := proposal.RequiredReviewRequirementIDs(request.ReviewKind)
	if err != nil {
		t.Fatal(err)
	}
	review := workspace.GoalSpecificationCompletenessReview{Kind: request.ReviewKind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立审查", EvidenceRefs: []string{items[0].CandidateItemRef}, RequirementIDs: ids}
	record, err = completeTestSpecification(record, request.PermitID, nil, &review, "")
	if err != nil {
		t.Fatal(err)
	}
	dto := DefinitionAuthoringRecordDTO(record)
	expected := record.Clone()
	for i := range expected.DerivationRequests {
		expected.DerivationRequests[i] = expected.DerivationRequests[i].Metadata()
	}
	for i := range expected.SpecificationRequests {
		expected.SpecificationRequests[i] = expected.SpecificationRequests[i].Metadata()
	}
	model, err := dto.Model()
	if err != nil || model.ValidateView() != nil || model.Validate() == nil || !reflect.DeepEqual(model, expected) {
		t.Fatalf("proposal／review metadata 往返改变封存身份或冒充完整存储: %v", err)
	}
	if dto.SpecificationRequests[1].SuccessAudit.Document.Content != nil || dto.SpecificationRequests[1].SuccessAudit.Wire.Content != nil {
		t.Fatal("普通 typed DTO 泄漏成功原文")
	}
	clone := dto.Clone()
	clone.SpecificationRequests[1].Proposal.Requirements[0].SourceItemRefs[0] = "changed"
	clone.SpecificationRequests[1].Proposal.Obligations[0].DoDItemRefs[0] = "changed"
	clone.SpecificationRequests[2].Review.RequirementIDs[0] = "changed"
	model.SpecificationRequests[1].Proposal.Resources[0].SnapshotHash = "changed"
	model.SpecificationRequests[1].Proposal.Verifications[0].Declaration = "changed"
	model.SpecificationRequests[2].Review.EvidenceRefs[0] = "changed"
	if !reflect.DeepEqual(dto, DefinitionAuthoringRecordDTO(record)) {
		t.Fatal("DTO Clone 或 model 映射共享 proposal／review 嵌套字段")
	}
	if dto.Validate() != nil || clone.Validate() == nil || model.Validate() == nil {
		t.Fatal("返回来源校验被修改后的成功结果绕过")
	}
}
