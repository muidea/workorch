package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func seedReviewedAuthoringConfirmation(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithProof(t, owners, id, false)
}

func seedReviewedAuthoringConfirmationWithProof(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, declared bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithReview(t, owners, id, declared, declared)
}

func seedReviewedAuthoringConfirmationWithReview(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, templateDeclared, proposalDeclared bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	t.Helper()
	ctx := context.Background()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, GoalText: "交付目标", TemplateID: "template", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, Name: "模板", DoDItems: []string{"模板验收"}}
	if templateDeclared {
		hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "proof", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		template.ContextHints = hints
	}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	var err error
	var derivation workspace.DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	proposal := authoringOwnerProposal(t, input, template, derived)
	if proposalDeclared {
		for index := range proposal.Verifications {
			verification := &proposal.Verifications[index]
			verification.Status = workspace.GoalVerificationDeclared
			verification.ProfileID = "generic"
			verification.EvidenceRequirementID = "proof"
			verification.Scope = "dodItem"
		}
	}
	var reserved workspace.DefinitionAuthoringSpecificationRequest
	record, reserved, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteSpecification(reserved.PermitID, &proposal, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		record, reserved, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review-" + kind, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "逐项覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal-req"}}
		record, err = record.CompleteSpecification(reserved.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: id, TemplateID: input.TemplateID, DeliverableDirectory: "delivery", DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	w, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, reviewed, err := record.ConfirmableSpecification(derivation.PermitID, proposal.ContentHash())
	if err != nil {
		t.Fatal(err)
	}
	return workspaceevents.ConfirmDefinitionAuthoringCommand{WorkspaceID: id, InputHash: record.InputHash, DerivationPermitID: derivation.PermitID, ProposalHash: proposal.ContentHash(), SpecificationHash: reviewed.AdmissionHash(), ExpectedWorkspaceRevision: w.Revision}
}

func TestAuthoringConfirmationRecoversEveryFileBackedCutpoint(t *testing.T) {
	for _, cut := range []string{"before-prepare", "prepared", "goal", "dod", "materialized"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm")
			w, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if cut != "before-prepare" {
				journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, w, cmd)
				if err != nil {
					t.Fatal(err)
				}
				next := *w
				next.DefinitionAuthoringConfirmation = &journal
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &next, w.Revision); err != nil {
					t.Fatal(err)
				}
				if cut == "goal" || cut == "dod" || cut == "materialized" {
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
					journal.Phase = workspace.DefinitionAuthoringConfirmationMaterialized
					next.DefinitionAuthoringConfirmation = &journal
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &next, next.Revision); err != nil {
						t.Fatal(err)
					}
				}
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			first, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if err != nil {
				t.Fatal(err)
			}
			if first.ValidateFor(cmd) != nil {
				t.Fatalf("invalid confirmation receipt: %+v", first)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || current.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || current.StartupInput != nil || current.ActivationAdmission != nil || current.ActiveRunID != "" {
				t.Fatalf("partial confirmation gained admission or did not finish: %+v %v", current, err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			second, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("replay was not idempotent: %+v %+v %v", first, second, err)
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: workspaceevents.ConfirmDefinitionAuthoringCommand{WorkspaceID: cmd.WorkspaceID, InputHash: cmd.InputHash, DerivationPermitID: cmd.DerivationPermitID, ProposalHash: cmd.ProposalHash, SpecificationHash: "forged", ExpectedWorkspaceRevision: cmd.ExpectedWorkspaceRevision}}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("stale confirmation accepted: %v", err)
			}
		})
	}
}

func TestAuthoringConfirmationEventHubRequiresApplicationSource(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-event")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"caller", "blocks/file"} {
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringConfirm, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
		result := owners.hub.Send(ev)
		if result == nil || result.Error() == nil {
			t.Fatalf("source %q bypassed confirmation provenance", source)
		}
	}
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil || current.DefinitionAuthoringConfirmation != nil {
		t.Fatalf("unauthorized event mutated confirmation: %+v %v", current, err)
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringConfirm, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	result := owners.hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("authorized confirmation failed: %+v", result)
	}
	value, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	confirmation, ok := value.(workspaceevents.ConfirmDefinitionAuthoringResult)
	if !ok || confirmation.ValidateFor(cmd) != nil {
		t.Fatalf("invalid confirmation response: %+v", value)
	}
}

func TestAuthoringConfirmationRejectsTemplateAndConfigurationDrift(t *testing.T) {
	for _, cut := range []string{"before-prepare", "prepared", "materialized"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-drift")
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if cut == "prepared" {
				journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, current, cmd)
				if err != nil {
					t.Fatal(err)
				}
				prepared := *current
				prepared.DefinitionAuthoringConfirmation = &journal
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &prepared, current.Revision); err != nil {
					t.Fatal(err)
				}
			}
			if cut == "materialized" {
				if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); err != nil {
					t.Fatal(err)
				}
			}
			template, err := owners.repositories.WorkspaceTemplates.Get(ctx, current.TemplateID)
			if err != nil {
				t.Fatal(err)
			}
			template.DoDItems = []string{"修改后的模板"}
			if err := owners.repositories.WorkspaceTemplates.Update(ctx, template); err != nil {
				t.Fatal(err)
			}
			_, err = owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if cut != "materialized" && !errors.Is(err, common.ErrConflict) {
				t.Fatalf("template drift was accepted at %s: %v", cut, err)
			}
			if cut == "materialized" && err != nil {
				t.Fatalf("completed materialization could not be replayed: %v", err)
			}
			if cut == "before-prepare" {
				unchanged, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
				if err != nil || unchanged.DefinitionAuthoringConfirmation != nil {
					t.Fatalf("template drift prepared a journal: %+v %v", unchanged, err)
				}
			}
		})
	}
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-config")
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current.Title = "另一个编辑"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale Workspace revision confirmed: %v", err)
	}
}

func TestAuthoringConfirmationRejectsConflictingFormalRows(t *testing.T) {
	for _, cut := range []string{"goal", "dod"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-conflict")
			current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, current, cmd)
			if err != nil {
				t.Fatal(err)
			}
			next := *current
			next.DefinitionAuthoringConfirmation = &journal
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &next, current.Revision); err != nil {
				t.Fatal(err)
			}
			if cut == "goal" {
				conflict := journal.Goal
				conflict.Text = "被改写的目标"
				if err := owners.repositories.Goals.Create(ctx, &conflict); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := owners.repositories.Goals.Create(ctx, &journal.Goal); err != nil {
					t.Fatal(err)
				}
				conflict := workspace.CloneDoD(&journal.DoD)
				conflict.Items[0] = "被改写的验收"
				if err := owners.repositories.DoDs.Create(ctx, conflict); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("conflicting %s was accepted: %v", cut, err)
			}
			unchanged, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || unchanged.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationPrepared || unchanged.ActivationAdmission != nil {
				t.Fatalf("conflicting row completed journal: %+v %v", unchanged, err)
			}
		})
	}
}

func TestAuthoringConfirmationHidesPartialFormalDefinitions(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-reads")
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, current, cmd)
	if err != nil {
		t.Fatal(err)
	}
	prepared := *current
	prepared.DefinitionAuthoringConfirmation = &journal
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &prepared, current.Revision); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Goals.Create(ctx, &journal.Goal); err != nil {
		t.Fatal(err)
	}
	goalRead := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetGoalCommand{WorkspaceID: cmd.WorkspaceID, GoalID: journal.Goal.ID})
	goalList := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListGoalsByWS, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: cmd.WorkspaceID})
	dodList := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListDoDsByWS, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListDoDsByWorkspaceCommand{WorkspaceID: cmd.WorkspaceID})
	if _, err := owners.workspace.executeGoalCommand(ctx, goalRead); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("prepared Goal was exposed: %v", err)
	}
	listed, err := owners.workspace.executeGoalCommand(ctx, goalList)
	if err != nil || len(listed.Goals) != 0 {
		t.Fatalf("prepared Goal list exposed rows: %+v %v", listed, err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); err != nil {
		t.Fatal(err)
	}
	listed, err = owners.workspace.executeGoalCommand(ctx, goalList)
	if err != nil || len(listed.Goals) != 1 || !reflect.DeepEqual(*listed.Goals[0], journal.Goal) {
		t.Fatalf("materialized Goal was not visible: %+v %v", listed, err)
	}
	listed, err = owners.workspace.executeDoDCommand(ctx, dodList)
	if err != nil || len(listed.DoDs) != 1 || !reflect.DeepEqual(*listed.DoDs[0], journal.DoD) {
		t.Fatalf("materialized DoD was not visible: %+v %v", listed, err)
	}
}

func TestAuthoringConfirmationRejectsIncompleteAndDriftedInputWithoutRows(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-confirm-reject")
	cmd.SpecificationHash = "wrong"
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched reviewed content accepted: %v", err)
	}
	cmd.SpecificationHash = ""
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty content accepted: %v", err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil || current.DefinitionAuthoringConfirmation != nil {
		t.Fatalf("invalid confirmation wrote a journal: %+v %v", current, err)
	}
}
