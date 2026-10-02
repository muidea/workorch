package eventmapping

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringDTOClonesPersistedProposalAndReviewWithoutRehashing(t *testing.T) {
	record := authoringMappingFixture(t)
	derivation := record.DerivationRequests[1]
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := workspace.AuthoringSpecificationResources(derivation.Input, derivation.Resources)
	if err != nil {
		t.Fatal(err)
	}
	proposal := workspace.GoalSpecification{
		Version: workspace.GoalSpecificationVersionCurrent, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef,
		GoalTextHash: workspace.StableDefinitionTextHash(record.Input.GoalText), Resources: resources,
		Dimensions:    []workspace.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}},
		Requirements:  []workspace.GoalSpecificationRequirement{{ID: "goal", DimensionID: "dimension", Required: true, Text: record.Input.GoalText, SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{workspace.DefinitionAuthoringGoalItemRef}}},
		Obligations:   []workspace.GoalSpecificationObligation{{ID: "goal-obligation", RequirementID: "goal", Text: "验证原目标", DoDItemRefs: []string{items[0].CandidateItemRef}}},
		Verifications: []workspace.GoalSpecificationVerification{{ID: "goal-verification", ObligationID: "goal-obligation", Declaration: "尚未声明", Status: workspace.GoalVerificationUnverified}},
	}
	for n, item := range items {
		kind := workspace.GoalRequirementDerived
		if strings.HasPrefix(item.CandidateItemRef, "derived-dod:user:") {
			kind = workspace.GoalRequirementUserExplicit
		} else if strings.HasPrefix(item.CandidateItemRef, "derived-dod:template:") {
			kind = workspace.GoalRequirementTemplateRequired
		}
		id := fmt.Sprintf("requirement-%d", n)
		obligation := fmt.Sprintf("obligation-%d", n)
		proposal.Requirements = append(proposal.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "dimension", Required: true, Text: item.Text, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}})
		proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: obligation, RequirementID: id, Text: item.Text, DoDItemRefs: []string{item.CandidateItemRef}})
		proposal.Verifications = append(proposal.Verifications, workspace.GoalSpecificationVerification{ID: "verification-" + id, ObligationID: obligation, Declaration: "尚未声明", Status: workspace.GoalVerificationUnverified})
	}
	record, request, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "second-proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteSpecification(request.PermitID, &proposal, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "critic", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: "dimension_coverage", ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	review := workspace.GoalSpecificationCompletenessReview{Kind: request.ReviewKind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立审查", EvidenceRefs: []string{items[0].CandidateItemRef}, RequirementIDs: []string{"goal"}}
	record, err = record.CompleteSpecification(request.PermitID, nil, &review, "")
	if err != nil {
		t.Fatal(err)
	}
	dto := DefinitionAuthoringRecordDTO(record)
	model, err := dto.Model()
	if err != nil || model.Validate() != nil || !reflect.DeepEqual(model, record) {
		t.Fatalf("proposal／review 往返改变封存身份: %v", err)
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
