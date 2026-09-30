package events

import (
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"testing"
)

func TestDerivationResultRejectsForgedPermitBudgetAndCompletion(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	request := workspace.DefinitionDerivationRequest{RequestID: "r", Input: input, Template: workspace.WorkspaceTemplate{ID: "t"}, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	request.PermitID = request.ExpectedPermitID()
	command := ReserveDefinitionDerivationCommand{WorkspaceID: "ws", InputHash: input.InputHash(), RequestID: "r", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	result := DefinitionDerivationRequestResult{Request: request, FreshPermitID: request.PermitID, Consumed: 1, Remaining: workspace.DefinitionDerivationRequestLimit - 1}
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
