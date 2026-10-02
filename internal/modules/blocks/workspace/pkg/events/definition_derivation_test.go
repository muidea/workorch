package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
)

func TestDerivationCompletionPreservesProposalSliceShape(t *testing.T) {
	for _, items := range [][]string{nil, {}, {"验收"}} {
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
		template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
		record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		reserved, request, _, err := record.ReserveDerivation("derive", "provider", nil)
		if err != nil {
			t.Fatal(err)
		}
		proposal := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: items}
		completed, err := reserved.CompleteDerivation(request.PermitID, proposal, "")
		if err != nil {
			t.Fatal(err)
		}
		stored := completed.DerivationRequests[0]
		if (stored.Proposal.AdditionalItems == nil) != (items == nil) {
			t.Fatalf("model clone changed proposal shape: %#v", items)
		}
		if _, err := completed.CompleteDerivation(request.PermitID, proposal, ""); err != nil {
			t.Fatalf("completion replay: %v", err)
		}
		dto := DefinitionDerivationRequest{RequestID: stored.RequestID, Input: stored.Input, Template: WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}, Resources: stored.Resources, ResourceSnapshotHash: stored.ResourceSnapshotHash, ProviderBindingHash: stored.ProviderBindingHash, PermitID: stored.PermitID, Proposal: stored.Proposal}.Clone()
		model, err := dto.Model()
		if err != nil {
			t.Fatal(err)
		}
		if (model.Proposal.AdditionalItems == nil) != (items == nil) {
			t.Fatalf("DTO round trip changed proposal shape: %#v", items)
		}
		result := DefinitionDerivationRequestResult{Request: dto, Consumed: 1, Remaining: workspace.DefinitionDerivationRequestLimit - 1}
		command := CompleteDefinitionDerivationCommand{WorkspaceID: "ws", PermitID: request.PermitID, Proposal: proposal}
		if err := result.ValidateCompletion(command); err != nil {
			t.Fatalf("completion shape %#v: %v", items, err)
		}
		if len(items) > 0 {
			proposal.AdditionalItems[0] = "篡改"
			if dto.Proposal.AdditionalItems[0] != "验收" || model.Proposal.AdditionalItems[0] != "验收" {
				t.Fatal("clone aliases proposal")
			}
			if result.ValidateCompletion(command) == nil {
				t.Fatal("tampered completion accepted")
			}
		}
	}
}

func TestDerivationResultRejectsForgedPermitBudgetAndCompletion(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	request := workspace.DefinitionDerivationRequest{RequestID: "r", Input: input, Template: workspace.WorkspaceTemplate{ID: "t"}, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	request.PermitID = request.ExpectedPermitID()
	command := ReserveDefinitionDerivationCommand{WorkspaceID: "ws", InputHash: input.InputHash(), RequestID: "r", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	result := DefinitionDerivationRequestResult{Request: DefinitionDerivationRequest{RequestID: request.RequestID, Input: input, Template: WorkspaceTemplate{ID: "t"}, ResourceSnapshotHash: request.ResourceSnapshotHash, ProviderBindingHash: request.ProviderBindingHash, PermitID: request.PermitID}, FreshPermitID: request.PermitID, Consumed: 1, Remaining: workspace.DefinitionDerivationRequestLimit - 1}
	if err := result.ValidateReserve(command); err != nil {
		t.Fatal(err)
	}
	bad := result
	bad.FreshPermitID = "forged"
	if bad.ValidateReserve(command) == nil {
		t.Fatal("forged permit accepted")
	}
	bad = result
	bad.Remaining++
	if bad.ValidateReserve(command) == nil {
		t.Fatal("budget drift accepted")
	}
	bad = result
	bad.Request.FailureCode = "provider_failed"
	if bad.ValidateReserve(command) == nil {
		t.Fatal("completed request granted permit")
	}
	bad = result
	bad.Request.Template.SoulBindings = []soulbinding.Binding{{Stage: "planning", SoulID: "planner"}, {Stage: "planning", SoulID: "planner"}}
	if bad.ValidateReserve(command) == nil {
		t.Fatal("重复 binding 被折叠后授予派生调用许可")
	}
	bad.FreshPermitID = ""
	bad.Request.FailureCode = "provider_failed"
	if bad.ValidateCompletion(CompleteDefinitionDerivationCommand{WorkspaceID: "ws", PermitID: request.PermitID, FailureCode: "provider_failed"}) == nil {
		t.Fatal("完成回执忽略重复 binding")
	}
	result.FreshPermitID = ""
	result.Request.FailureCode = "provider_failed"
	complete := CompleteDefinitionDerivationCommand{WorkspaceID: "ws", PermitID: request.PermitID, FailureCode: "provider_failed"}
	if err := result.ValidateCompletion(complete); err != nil {
		t.Fatal(err)
	}
	complete.FailureCode = "invalid_proposal"
	if result.ValidateCompletion(complete) == nil {
		t.Fatal("different persisted result accepted")
	}
}
