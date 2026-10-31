package eventmapping

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func authoringMappingFixture(t *testing.T) workspace.DefinitionAuthoringRecord {
	t.Helper()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-mapping", Revision: 1, GoalText: "交付目标", TemplateID: "template", AcceptanceRequirements: []string{"用户验收"}, ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{
		ID: input.TemplateID, ParentTemplateIDs: []string{"parent"},
		EffectiveSources: []workspace.TemplateSource{{TemplateID: "parent", ContentHash: "parent-hash"}, {TemplateID: "template", ContentHash: "template-hash"}},
		Name:             "模板", DoDItems: []string{"模板验收"}, SoulBindings: map[string]string{"validation": "reviewer", "planning": "planner"},
		ContextHints:     workorcheventpayload.New(workorcheventpayload.ObjectField("nested", workorcheventpayload.New(workorcheventpayload.StringField("value", "original")))),
		EvidencePolicies: []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringArrayField("checks", []string{"original"}))},
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	content := "有界验收资源"
	checksum := sha256.Sum256([]byte(content))
	resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: input.ResourceRefs[0], Content: content, ContentHash: "sha256:" + hex.EncodeToString(checksum[:])}}
	record, first, _, err := record.ReserveDerivation("failed-derivation", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestDerivation(record, first.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, second, _, err := record.ReserveDerivation("completed-derivation", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	record, err = completeTestDerivation(record, second.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "proposal"), RequestID: "proposal", DerivationPermitID: second.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestSpecification(record, request.PermitID, nil, nil, "invalid_proposal")
	if err != nil {
		t.Fatal(err)
	}
	receipt := workspace.DefinitionAuthoringSaveReceipt{RequestID: "save", RequestHash: workspace.DefinitionAuthoringSaveRequestHash("save", input, 1, "", nil), InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, WorkspaceRevision: 2}
	record.SaveReceipts = []workspace.DefinitionAuthoringSaveReceipt{receipt}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestAuthoringDTOConversionPreservesIdentityBudgetAndDetachedSnapshots(t *testing.T) {
	record := authoringMappingFixture(t)
	dto := DefinitionAuthoringRecordDTO(record)
	if err := dto.Validate(); err != nil {
		t.Fatal(err)
	}
	expected := record.Clone()
	for i := range expected.DerivationRequests {
		expected.DerivationRequests[i] = expected.DerivationRequests[i].Metadata()
	}
	model, err := dto.Model()
	if err != nil || !reflect.DeepEqual(model, expected) {
		t.Fatalf("创作记录往返改变来源或请求预算: %v", err)
	}
	if len(dto.TemplateSnapshot.SoulBindings) != 2 || dto.TemplateSnapshot.SoulBindings[0].Stage != "planning" {
		t.Fatal("binding 未按稳定顺序输出")
	}
	cloned := dto.Clone()
	cloned.Input.AcceptanceRequirements[0] = "changed"
	cloned.TemplateSnapshot.ParentTemplateIDs[0] = "changed"
	cloned.TemplateSnapshot.EffectiveSources[0].ContentHash = "changed"
	cloned.TemplateSnapshot.DoDItems[0] = "changed"
	cloned.TemplateSnapshot.SoulBindings[0].SoulID = "changed"
	cloned.TemplateSnapshot.ContextHints.Fields[0].Value.Object[0].Value.String = "changed"
	cloned.TemplateSnapshot.EvidencePolicies[0].Fields[0].Value.Array[0].String = "changed"
	cloned.DerivationRequests[1].Template.SoulBindings[0].SoulID = "changed"
	cloned.DerivationRequests[1].Resources[0].Content = "changed"
	cloned.DerivationRequests[1].Proposal.AdditionalItems[0] = "changed"
	cloned.SpecificationRequests[0].FailureCode = "provider_failed"
	cloned.SaveReceipts[0].RequestID = "changed"
	if !reflect.DeepEqual(dto, DefinitionAuthoringRecordDTO(record)) {
		t.Fatal("DTO Clone 保留可变别名")
	}
	model.TemplateSnapshot.SoulBindings["planning"] = "model-changed"
	model.DerivationRequests[1].Resources[0].ContentHash = "model-changed"
	model.DerivationRequests[1].Proposal.AdditionalItems[0] = "model-changed"
	if !reflect.DeepEqual(dto, DefinitionAuthoringRecordDTO(record)) {
		t.Fatal("DTO 转 model 保留可变别名")
	}
	// 同一次有效查询不能授权以后被篡改的 DTO。
	cloned = dto.Clone()
	cloned.DerivationRequests[0].PermitID = "forged"
	if cloned.Validate() == nil {
		t.Fatal("未选中的失败请求未重新校验")
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: record.Input.WorkspaceID, WorkspaceRevision: 2, Record: &cloned}
	if query.ValidateFor(workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: record.Input.WorkspaceID}) == nil {
		t.Fatal("查询忽略 DTO ledger 篡改")
	}
}

func TestAuthoringDTORejectsBindingCardinalityLossBeforeModelConversion(t *testing.T) {
	record := authoringMappingFixture(t)
	for _, nested := range []bool{false, true} {
		for _, binding := range []soulbinding.Binding{{Stage: "planning", SoulID: "planner"}, {Stage: " planning ", SoulID: "other"}, {Stage: "", SoulID: "other"}, {Stage: "extra"}} {
			dto := DefinitionAuthoringRecordDTO(record)
			template := &dto.TemplateSnapshot
			if nested {
				template = &dto.DerivationRequests[0].Template
			}
			template.SoulBindings = append(template.SoulBindings, binding)
			if _, err := dto.Model(); err == nil || dto.Validate() == nil {
				t.Fatalf("重复或空 binding 被 map 转换掩盖: nested=%t binding=%+v", nested, binding)
			}
			query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: record.Input.WorkspaceID, WorkspaceRevision: 2, Record: &dto}
			if query.ValidateFor(workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: record.Input.WorkspaceID}) == nil {
				t.Fatal("查询接受丢失 cardinality 的快照")
			}
		}
	}
}

func TestAuthoringHistoryDTOConversionPreservesAfterImagesAndIsolation(t *testing.T) {
	record := authoringMappingFixture(t)
	history := []workspace.DefinitionAuthoringConfirmedRevision{{Record: record, Confirmation: workspace.DefinitionAuthoringConfirmation{FormalSequence: 1, DoD: workspace.DoD{Items: []string{"正式验收"}}}}}
	dto := DefinitionAuthoringHistoryDTOs(history)
	expected := make([]workspace.DefinitionAuthoringConfirmedRevision, len(history))
	for i := range history {
		expected[i] = history[i].Clone()
	}
	for i := range expected {
		for j := range expected[i].Record.DerivationRequests {
			expected[i].Record.DerivationRequests[j] = expected[i].Record.DerivationRequests[j].Metadata()
		}
	}
	models, err := workspaceevents.AuthoringHistoryModels(dto)
	if err != nil || !reflect.DeepEqual(expected, models) {
		t.Fatalf("历史往返改变 after-image: %v", err)
	}
	clone := dto[0].Clone()
	clone.Confirmation.DoD.Items[0] = "clone-changed"
	models[0].Record.DerivationRequests[1].Template.SoulBindings["planning"] = "changed"
	models[0].Confirmation.DoD.Items[0] = "changed"
	if !reflect.DeepEqual(dto, DefinitionAuthoringHistoryDTOs(history)) {
		t.Fatal("历史 DTO 与 model 共享可变字段")
	}
	binding := dto[0].Record.TemplateSnapshot.SoulBindings[0]
	dto[0].Record.TemplateSnapshot.SoulBindings = append(dto[0].Record.TemplateSnapshot.SoulBindings, binding)
	if _, err := workspaceevents.AuthoringHistoryModels(dto); err == nil {
		t.Fatal("坏历史 binding 在转换时被折叠")
	}
}
