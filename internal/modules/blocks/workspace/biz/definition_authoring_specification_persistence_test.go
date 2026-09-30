package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringSpecificationPermitAndReviewSurviveFileReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-authoring-spec", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", Name: "模板", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DefinitionAuthoring: &record}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	reserveDerivation := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: "derive-1", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	derived, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserveDerivation})
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	acceptance := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	completeDerivation := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, PermitID: derived.Derivation.FreshPermitID, Proposal: acceptance}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &completeDerivation}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	proposalRequest := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal-1", DerivationPermitID: derived.Derivation.FreshPermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	reserve := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: proposalRequest.RequestID, DerivationPermitID: proposalRequest.DerivationPermitID, ProviderBindingHash: proposalRequest.ProviderBindingHash}
	if err := reserve.Validate(); err != nil {
		t.Fatalf("reserve command: %v", err)
	}
	if current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID); err != nil || current.DefinitionAuthoring.Validate() != nil {
		t.Fatalf("stored record: %+v %v", current, err)
	} else {
		if current.TemplateID != input.TemplateID || current.StartupInput != nil || current.ActivationAdmission != nil || current.ActiveRunID != "" {
			t.Fatalf("workspace authoring fence: %+v", current)
		}
		if observed, observeErr := owners.workspace.resolvedTemplate(ctx, current.TemplateID); observeErr != nil || workspace.WorkspaceTemplateSnapshotHash(observed) != record.TemplateSnapshotHash {
			t.Fatalf("template fence: %+v %v", observed, observeErr)
		}
		if _, _, _, reserveErr := current.DefinitionAuthoring.ReserveSpecification(proposalRequest); reserveErr != nil {
			t.Fatalf("model reserve: %v", reserveErr)
		}
	}
	first, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecReserve: &reserve})
	if err != nil || first.AuthoringSpecification == nil || first.AuthoringSpecification.FreshPermitID == "" {
		owners.close()
		t.Fatalf("reserve: %+v %v", first, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	replay, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecReserve: &reserve})
	if err != nil || replay.AuthoringSpecification == nil || replay.AuthoringSpecification.FreshPermitID != "" || replay.AuthoringSpecification.Request.PermitID != first.AuthoringSpecification.Request.PermitID {
		t.Fatalf("reopen granted second physical permit: %+v %v", replay, err)
	}
	spec := authoringOwnerProposal(t, input, template, *acceptance)
	complete := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, PermitID: first.AuthoringSpecification.FreshPermitID, Proposal: &spec}
	done, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &complete})
	if err != nil || done.AuthoringSpecification.Request.Proposal == nil {
		t.Fatalf("complete proposal: %+v %v", done, err)
	}
	reviewRequest := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review-1", DerivationPermitID: derived.Derivation.FreshPermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: workspace.GoalReviewTraceability, ProposalHash: spec.ContentHash(), ProviderBindingHash: "provider"}
	reserveReview := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: reviewRequest.RequestID, DerivationPermitID: reviewRequest.DerivationPermitID, ReviewKind: reviewRequest.ReviewKind, ProposalHash: reviewRequest.ProposalHash, ProviderBindingHash: reviewRequest.ProviderBindingHash}
	critic, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecReserve: &reserveReview})
	if err != nil || critic.AuthoringSpecification.FreshPermitID == "" {
		t.Fatalf("critic reserve: %+v %v", critic, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	review := workspace.GoalSpecificationCompletenessReview{Kind: workspace.GoalReviewTraceability, Status: workspace.GoalReviewUnknown, SpecificationHash: spec.ContentHash()}
	completeReview := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, PermitID: critic.AuthoringSpecification.FreshPermitID, Review: &review}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completeReview}); err != nil {
		t.Fatal(err)
	}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: input.WorkspaceID})
	if err != nil || view.Record == nil || len(view.Record.SpecificationRequests) != 2 || view.Record.SpecificationRequests[1].Review == nil || view.Record.InputHash != input.InputHash() {
		t.Fatalf("file-backed specification not restored: %+v %v", view, err)
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, AuthoringSpecComplete: &completeReview}); err != nil {
		t.Fatalf("duplicate completion changed result: %v", err)
	}
}

func authoringOwnerProposal(t *testing.T, input workspace.DefinitionAuthoringInput, template workspace.WorkspaceTemplate, derived workspace.DefinitionDerivedAcceptance) workspace.GoalSpecification {
	t.Helper()
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef, GoalTextHash: workspace.StableDefinitionTextHash(input.GoalText), Dimensions: []workspace.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}}
	spec.Requirements = append(spec.Requirements, workspace.GoalSpecificationRequirement{ID: "goal-req", DimensionID: "dimension", Required: true, SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{spec.GoalItemRef}, Text: input.GoalText})
	for n, item := range items {
		kind := workspace.GoalRequirementDerived
		if n == 0 {
			kind = workspace.GoalRequirementUserExplicit
		} else if n == 1 {
			kind = workspace.GoalRequirementTemplateRequired
		}
		id := item.CandidateItemRef
		spec.Requirements = append(spec.Requirements, workspace.GoalSpecificationRequirement{ID: "req-" + id, DimensionID: "dimension", Required: true, SourceKind: kind, SourceItemRefs: []string{id}, Text: item.Text})
		spec.Obligations = append(spec.Obligations, workspace.GoalSpecificationObligation{ID: "obl-" + id, RequirementID: "req-" + id, DoDItemRefs: []string{id}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, workspace.GoalSpecificationVerification{ID: "verification-" + id, ObligationID: "obl-" + id, Declaration: "待评估", Status: workspace.GoalVerificationUnverified})
	}
	spec.Obligations = append(spec.Obligations, workspace.GoalSpecificationObligation{ID: "goal-obligation", RequirementID: "goal-req", DoDItemRefs: []string{items[0].CandidateItemRef}, Text: "目标验收"})
	spec.Verifications = append(spec.Verifications, workspace.GoalSpecificationVerification{ID: "goal-verification", ObligationID: "goal-obligation", Declaration: "待评估", Status: workspace.GoalVerificationUnverified})
	return spec
}
