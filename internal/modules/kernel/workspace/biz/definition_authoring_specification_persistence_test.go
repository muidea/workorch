package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
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
	current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	complete := authoringOwnerCompletion(t, *current, first.AuthoringSpecification.FreshPermitID, spec)
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

func authoringOwnerProposal(t *testing.T, input workspace.DefinitionAuthoringInput, template workspace.WorkspaceTemplate, derived workspace.DefinitionDerivedAcceptance, snapshots ...[]workspace.DefinitionAuthoringResourceSnapshot) workspace.GoalSpecification {
	t.Helper()
	var resources []workspace.DefinitionAuthoringResourceSnapshot
	if len(snapshots) > 0 {
		resources = snapshots[0]
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	record, d, _, err := record.ReserveDerivation("synthetic/current", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(d.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, proposal, err := testkit.Proposal(record.DerivationRequests[0], workspace.DefinitionAuthoringVerificationMapping{Declaration: "待评估", Status: workspace.GoalVerificationUnverified})
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

// 从 synthetic canonical 样本中只提取声明；生产路径不接受 canonical 成功回退。
func authoringOwnerMapping(t *testing.T, d workspace.DefinitionDerivationRequest, proposal workspace.GoalSpecification) (workspace.DefinitionAuthoringMapping, workspace.DefinitionAuthoringSuccessByteAudit) {
	t.Helper()
	mapping, err := testkit.Mapping(d, workspace.DefinitionAuthoringVerificationMapping{Declaration: "待评估", Status: workspace.GoalVerificationUnverified})
	if err != nil {
		t.Fatal(err)
	}
	for i := range mapping.Obligations {
		for _, obligation := range proposal.Obligations {
			if len(obligation.DoDItemRefs) != 1 || obligation.DoDItemRefs[0] != mapping.Obligations[i].CandidateItemRef {
				continue
			}
			for _, verification := range proposal.Verifications {
				if verification.ObligationID == obligation.ID {
					mapping.Obligations[i].Verification = workspace.DefinitionAuthoringVerificationMapping{Declaration: verification.Declaration, Status: verification.Status, ProfileID: verification.ProfileID, EvidenceRequirementID: verification.EvidenceRequirementID, Scope: verification.Scope}
				}
			}
		}
	}
	audit, err := testkit.Audit(mapping)
	if err != nil {
		t.Fatal(err)
	}
	return mapping, audit
}

func authoringOwnerCompletion(t *testing.T, w workspace.Workspace, permit string, proposal workspace.GoalSpecification) workspaceevents.CompleteAuthoringSpecificationCommand {
	t.Helper()
	for _, request := range w.DefinitionAuthoring.SpecificationRequests {
		if request.PermitID != permit {
			continue
		}
		for _, d := range w.DefinitionAuthoring.DerivationRequests {
			if d.PermitID == request.DerivationPermitID {
				mapping, audit := authoringOwnerMapping(t, d, proposal)
				return workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: w.ID, PermitID: permit, Mapping: &mapping, SuccessAudit: &audit, Proposal: &proposal}
			}
		}
	}
	t.Fatal("synthetic completion has no sealed reservation")
	return workspaceevents.CompleteAuthoringSpecificationCommand{}
}

func authoringGoalRequirementID(t *testing.T, proposal workspace.GoalSpecification) string {
	t.Helper()
	for _, requirement := range proposal.Requirements {
		if len(requirement.SourceItemRefs) == 1 && requirement.SourceItemRefs[0] == workspace.DefinitionAuthoringGoalItemRef {
			return requirement.ID
		}
	}
	t.Fatal("independent Goal requirement missing")
	return ""
}
