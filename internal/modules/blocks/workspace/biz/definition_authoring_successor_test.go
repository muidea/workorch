package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func reviewedAuthoringSuccessor(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, closeProofGap bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	t.Helper()
	ctx := context.Background()
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if current.DefinitionAuthoring == nil || current.DefinitionAuthoringConfirmation == nil || len(current.DefinitionAdmissionCandidates) == 0 {
		t.Fatal("blocked predecessor missing")
	}
	if closeProofGap {
		// 用新模板的 exact proof 补齐闭包；保留原必需交付物，不降低义务。
		template, err := owners.repositories.WorkspaceTemplates.Get(ctx, current.TemplateID)
		if err != nil {
			t.Fatal(err)
		}
		hints := template.ContextHints.Map()
		generic := hints["profileContracts"].(map[string]any)["generic"].(map[string]any)
		declarations := generic["evidenceRequirements"].([]any)
		generic["evidenceRequirements"] = append(declarations, map[string]any{"id": "required-delivery", "scope": "deliverable", "requiredEvidenceKinds": []any{"state_change"}, "acceptedStepKinds": []any{"file/write"}})
		template.ContextHints, err = workorcheventpayload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
		if err := owners.repositories.WorkspaceTemplates.Update(ctx, template); err != nil {
			t.Fatal(err)
		}
	}
	input := current.DefinitionAuthoring.Input.Clone()
	input.Revision++
	input.GoalText = "修订后的交付目标"
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-successor/" + input.InputHash(), ExpectedInputHash: current.DefinitionAuthoring.InputHash, Input: input, ExpectedWorkspaceRevision: current.Revision}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	record := current.DefinitionAuthoring.Clone()
	reserveDerivation := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: id, InputHash: record.InputHash, RequestID: "derive-successor/" + record.InputHash, ProviderBindingHash: "provider", ResourceSnapshotHash: workspacemodel.DefinitionAuthoringResourcesHash(nil)}
	reservedDerivation, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserveDerivation})
	if err != nil || reservedDerivation.Derivation == nil || reservedDerivation.Derivation.FreshPermitID == "" {
		t.Fatalf("reserve successor derivation: %v", err)
	}
	derivation := reservedDerivation.Derivation.Request
	derived := workspacemodel.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	completeDerivation := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: id, PermitID: derivation.PermitID, Proposal: &derived}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &completeDerivation}); err != nil {
		t.Fatal(err)
	}
	proposal := authoringOwnerProposal(t, input, record.TemplateSnapshot, derived)
	for index := range proposal.Verifications {
		proof := &proposal.Verifications[index]
		proof.Status = workspacemodel.GoalVerificationDeclared
		proof.ProfileID = "generic"
		proof.EvidenceRequirementID = "*"
		proof.Scope = "dodItem"
	}
	reserveProposal := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: record.InputHash, RequestID: "proposal-successor/" + record.InputHash, DerivationPermitID: derivation.PermitID, ProviderBindingHash: "provider"}
	reservedProposal, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &reserveProposal})
	if err != nil || reservedProposal.AuthoringSpecification == nil || reservedProposal.AuthoringSpecification.FreshPermitID == "" {
		t.Fatalf("reserve successor proposal: %v", err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	completeProposal := authoringOwnerCompletion(t, *current, reservedProposal.AuthoringSpecification.FreshPermitID, proposal)
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecComplete: &completeProposal}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		reserveReview := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: record.InputHash, RequestID: "successor-" + kind + "/" + record.InputHash, DerivationPermitID: derivation.PermitID, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"}
		reservedReview, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &reserveReview})
		if err != nil || reservedReview.AuthoringSpecification == nil || reservedReview.AuthoringSpecification.FreshPermitID == "" {
			t.Fatalf("reserve successor review %s: %v", kind, err)
		}
		review := workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "逐项覆盖", EvidenceRefs: []string{workspacemodel.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{authoringGoalRequirementID(t, proposal)}}
		completeReview := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: id, PermitID: reservedReview.AuthoringSpecification.FreshPermitID, Review: &review}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecComplete: &completeReview}); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, reviewed, err := updated.DefinitionAuthoring.ConfirmableSpecification(derivation.PermitID, proposal.ContentHash())
	if err != nil {
		t.Fatal(err)
	}
	return workspaceevents.ConfirmDefinitionAuthoringCommand{WorkspaceID: id, InputHash: record.InputHash, DerivationPermitID: derivation.PermitID, ProposalHash: proposal.ContentHash(), SpecificationHash: reviewed.AdmissionHash(), ExpectedWorkspaceRevision: updated.Revision}
}

func TestAuthoringMultipleBlockedSuccessorsRetainCumulativeBudget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	command := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-multiple-successors")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command); err != nil {
		t.Fatal(err)
	}
	for revision := 2; revision <= 3; revision++ {
		command = reviewedAuthoringSuccessor(t, owners, command.WorkspaceID, false)
		if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command); err != nil {
			t.Fatalf("revision %d: %v", revision, err)
		}
		stored, err := owners.repositories.Workspaces.Get(ctx, command.WorkspaceID)
		if err != nil || len(stored.DefinitionAuthoringHistory) != revision-1 || len(stored.DefinitionAdmissionCandidates) != revision || len(stored.DefinitionAuthoring.DerivationRequests) != revision || len(stored.DefinitionAuthoring.SpecificationRequests) != 7*revision || stored.ValidateDefinitionAuthoringHistory() != nil || stored.StartupInput != nil || stored.ActiveRunID != "" {
			t.Fatalf("revision %d lost history or budget: %+v %v", revision, stored, err)
		}
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command); err != nil {
		t.Fatalf("revision 3 reopen: %v", err)
	}
}

func TestAuthoringBlockedSuccessorReconcilesTicketAndRetainsLineage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	firstCommand := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-authoring-successor-blocked")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, firstCommand)
	if err != nil || first.BlockingTicketID == "" {
		t.Fatalf("initial clarification: %+v %v", first, err)
	}
	secondCommand := reviewedAuthoringSuccessor(t, owners, firstCommand.WorkspaceID, false)
	second, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, secondCommand)
	if err != nil || second.BlockingTicketID == "" || second.BlockingTicketID == first.BlockingTicketID || second.PriorClarification == nil || second.StartupInput != nil {
		t.Fatalf("successor clarification: %+v %v", second, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, firstCommand.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 2 || len(stored.DefinitionAuthoringHistory) != 1 || stored.StartupInput != nil || stored.ActiveRunID != "" || stored.DefinitionAdmissionCandidates[0].CandidateID != stored.DefinitionAdmissionCandidates[1].CandidateID || stored.DefinitionAdmissionCandidates[1].CandidateRevision != 2 {
		t.Fatalf("lineage not durable: %+v %v", stored, err)
	}
	tickets, err := owners.repositories.Tickets.ListByWorkspaceID(common.ContextWithRuntimeWorkspaceID(ctx, firstCommand.WorkspaceID), firstCommand.WorkspaceID)
	if err != nil || len(tickets) != 2 {
		t.Fatalf("ticket reconciliation did not preserve both facts: %+v %v", tickets, err)
	}
	oldResolved, newOpen := false, false
	for _, ticket := range tickets {
		switch ticket.ID {
		case first.BlockingTicketID:
			oldResolved = ticket.State == common.DecisionTicketStateResolved
		case second.BlockingTicketID:
			newOpen = ticket.State == common.DecisionTicketStateOpen
		}
	}
	if !oldResolved || !newOpen {
		t.Fatalf("reconciled ticket lifecycle is invalid: %+v", tickets)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	replayed, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, secondCommand)
	if err != nil || !reflect.DeepEqual(second, replayed) {
		t.Fatalf("successor response-loss replay: %+v %+v %v", second, replayed, err)
	}
}

func TestAuthoringBlockedSuccessorPromotesOnlyAfterDecisionResolution(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	firstCommand := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-authoring-successor-resolution")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, firstCommand)
	if err != nil || first.BlockingTicketID == "" {
		t.Fatalf("initial blocker: %+v %v", first, err)
	}
	secondCommand := reviewedAuthoringSuccessor(t, owners, firstCommand.WorkspaceID, true)
	_, err = owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: secondCommand})
	if err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, secondCommand.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(current)}
	firstCut, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || firstCut.PriorClarification == nil || firstCut.StartupInput != nil || firstCut.ActivationAdmission != nil {
		t.Fatalf("candidate must wait for persisted ticket: %+v %v", firstCut, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" || len(stored.DefinitionAdmissionCandidates) != 2 {
		t.Fatalf("candidate append sealed executable authority: %+v %v", stored, err)
	}
	owners.decision.Teardown(ctx)
	if _, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate); err == nil {
		t.Fatal("missing Decision owner admitted successor")
	}
	stored, err = owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("Decision failure promoted candidate: %+v %v", stored, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	identity, err := firstCut.PriorClarification.DecisionRequestIdentity()
	if err != nil {
		t.Fatal(err)
	}
	opened, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, owners.hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(identity))
	if err != nil {
		t.Fatal(err)
	}
	command.ClarificationTicket = &workspaceevents.WorkspaceDefinitionClarificationTicketFact{RequestIdentity: identity, TicketBinding: opened.BindingValue()}
	pending, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || pending.ActivationAdmission == nil || pending.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		t.Fatalf("old ticket did not seal pending startup: %+v %v", pending, err)
	}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: current.ID})
	if err != nil || view.ActivationAdmission == nil || view.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		t.Fatalf("pending startup is not recoverable: %+v %v", view, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	admitted, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, secondCommand)
	if err != nil || admitted.ActivationAdmission == nil || admitted.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || admitted.ActivationAdmission.BlockingTicketID != first.BlockingTicketID {
		t.Fatalf("resolution did not promote successor: %+v %v", admitted, err)
	}
	stored, err = owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.ActivationAdmission == nil || stored.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || stored.ActiveRunID != "" || len(stored.DefinitionAdmissionCandidates) != 2 {
		t.Fatalf("eligible successor has invalid durable state: %+v %v", stored, err)
	}
	replay, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, secondCommand)
	if err != nil || !reflect.DeepEqual(admitted, replay) {
		t.Fatalf("resolved successor replay drifted: %+v %+v %v", admitted, replay, err)
	}
}

func TestAuthoringSuccessorConfirmationRecoversFileCuts(t *testing.T) {
	for _, cut := range []string{"prepared", "goal", "dod", "materialized"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			initial := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-successor-cuts")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			application := appbiz.New()
			application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
			if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, initial); err != nil {
				t.Fatal(err)
			}
			command := reviewedAuthoringSuccessor(t, owners, initial.WorkspaceID, false)
			current, err := owners.repositories.Workspaces.Get(ctx, initial.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, current, command)
			if err != nil {
				t.Fatal(err)
			}
			current.DefinitionAuthoringConfirmation = &journal
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			if cut != "prepared" {
				if err := owners.repositories.Goals.Create(ctx, &journal.Goal); err != nil {
					t.Fatal(err)
				}
			}
			if cut == "dod" || cut == "materialized" {
				if err := owners.repositories.DoDs.Create(ctx, &journal.DoD); err != nil {
					t.Fatal(err)
				}
			}
			if cut == "materialized" {
				journal.Phase = workspacemodel.DefinitionAuthoringConfirmationMaterialized
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
					t.Fatal(err)
				}
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
			first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command)
			if err != nil || first.Candidate.CandidateRevision != 2 || first.BlockingTicketID == "" {
				t.Fatalf("successor confirmation cut %s: %+v %v", cut, first, err)
			}
			second, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("successor duplicate after cut %s: %v", cut, err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, initial.WorkspaceID)
			if err != nil || current.StartupInput != nil || current.ActiveRunID != "" || len(current.DefinitionAuthoringHistory) != 1 || len(current.DefinitionAdmissionCandidates) != 2 {
				t.Fatalf("successor cut changed authority: %+v %v", current, err)
			}
		})
	}
}

func TestAuthoringEligibleStartupRejectsDraftRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	firstCommand := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-authoring-successor-resolution", true)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, firstCommand)
	if err != nil || first.ActivationAdmission == nil || first.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		t.Fatalf("initial proof should be eligible: %+v %v", first, err)
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: firstCommand.WorkspaceID, Authoring: &workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: workspacemodel.DefinitionAuthoringInput{Version: 1, WorkspaceID: firstCommand.WorkspaceID, Revision: 2, GoalText: "new", TemplateID: "template"}, ExpectedWorkspaceRevision: first.WorkspaceRevision}}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("eligible startup was edited as a draft: %v", err)
	}
}
