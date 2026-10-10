package workspace

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestAuthoringCurrentMappingFull37ConfirmsEligibleWithoutRunAfterReopen(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-full37-owner", Revision: 1, GoalText: "保留独立 Goal，完整确认 37 项义务", TemplateID: "full37"}
	for n := 1; n <= 5; n++ {
		input.AcceptanceRequirements = append(input.AcceptanceRequirements, fmt.Sprintf("用户义务 %02d", n))
	}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID}
	for n := 1; n <= 13; n++ {
		template.DoDItems = append(template.DoDItems, fmt.Sprintf("模板义务 %02d", n))
	}
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	template.ContextHints = hints
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-full37", Input: input, ExpectedWorkspaceRevision: current.Revision}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	reserveDerive := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, InputHash: input.InputHash(), RequestID: "derive-full37", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	derived, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserveDerive})
	if err != nil || derived.Derivation == nil || derived.Derivation.FreshPermitID == "" {
		t.Fatalf("reserve derivation: %v", err)
	}
	acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	for n := 1; n <= 19; n++ {
		acceptance.AdditionalItems = append(acceptance.AdditionalItems, fmt.Sprintf("派生义务 %02d", n))
	}
	completeDerive := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, PermitID: derived.Derivation.FreshPermitID, Proposal: &acceptance}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &completeDerive}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	derivation := current.DefinitionAuthoring.DerivationRequests[0]
	mapping, audit, proposal, err := testkit.Proposal(derivation, workspace.DefinitionAuthoringVerificationMapping{Declaration: "按封存模板验证", Status: workspace.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "*", Scope: "dodItem"})
	if err != nil || len(proposal.Obligations) != 37 || len(proposal.Verifications) != 37 || len(proposal.Requirements) != 38 {
		t.Fatalf("完整 current mapping 编译: %v", err)
	}
	reserveProposal := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, InputHash: input.InputHash(), RequestID: "proposal-full37", DerivationPermitID: derivation.PermitID, ProviderBindingHash: "provider"}
	reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecReserve: &reserveProposal})
	if err != nil {
		t.Fatal(err)
	}
	completeProposal := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, PermitID: reserved.AuthoringSpecification.FreshPermitID, Mapping: &mapping, SuccessAudit: &audit, Proposal: &proposal}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completeProposal}); err != nil {
		t.Fatal(err)
	}
	items, err := acceptance.CandidateItems(input, &template)
	if err != nil || len(items) != 37 {
		t.Fatalf("候选基数: %v", err)
	}
	var evidence, requirements []string
	for _, item := range items {
		evidence = append(evidence, item.CandidateItemRef)
	}
	for _, requirement := range proposal.Requirements {
		requirements = append(requirements, requirement.ID)
	}
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		reserve := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, InputHash: input.InputHash(), RequestID: "review-full37/" + kind, DerivationPermitID: derivation.PermitID, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"}
		result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecReserve: &reserve})
		if err != nil {
			t.Fatal(err)
		}
		review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立评审完整 37 项封存来源", EvidenceRefs: evidence, RequirementIDs: requirements}
		complete := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, PermitID: result.AuthoringSpecification.FreshPermitID, Review: &review}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &complete}); err != nil {
			t.Fatal(err)
		}
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil || validateWorkspaceAuthoringMappingOutcomes(current) != nil {
		t.Fatalf("重开原始 mapping 身份: %v", err)
	}
	confirmation := authoringConfirmationCommandFromPersistedLedger(t, current)
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err != nil {
		t.Fatal(err)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil || before.ActivationAdmission == nil || before.StartupInput == nil || before.ActiveRunID != "" {
		t.Fatalf("确认未生成独立 eligible admission：%v", err)
	}
	dods, err := owners.repositories.DoDs.ListByWorkspaceID(ctx, input.WorkspaceID)
	if err != nil || len(dods) != 1 || len(dods[0].Items) != 37 {
		t.Fatalf("正式 DoD 未保留完整 37 项：%v", err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err != nil {
		t.Fatal(err)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	runs, runErr := owners.repositories.Runs.List(ctx)
	if err != nil || runErr != nil || len(runs) != 0 || !reflect.DeepEqual(before, after) || len(after.DefinitionAuthoring.DerivationRequests) != 1 || len(after.DefinitionAuthoring.SpecificationRequests) != 7 {
		t.Fatalf("重启精确重放增加预算、CAS 或自动 Run：%v/%v", err, runErr)
	}
}
