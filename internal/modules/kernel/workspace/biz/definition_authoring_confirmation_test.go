package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func seedReviewedAuthoringConfirmation(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithProof(t, owners, id, true)
}

// 声明适用且六审通过，但必需交付物缺少 exact proof 合同，正式准入仍须澄清。
func seedReviewedAuthoringConfirmationWithAdmissionBlocker(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithTemplate(t, owners, id, true, true, "*", "dodItem", func(template *workspace.WorkspaceTemplate) {
		hints := template.ContextHints.Map()
		generic := hints["profileContracts"].(map[string]any)["generic"].(map[string]any)
		generic["deliverableExpectations"] = []any{map[string]any{"id": "required-delivery", "required": true}}
		var err error
		template.ContextHints, err = workorcheventpayload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func seedReviewedAuthoringConfirmationWithProof(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, declared bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithReview(t, owners, id, declared, declared)
}

func seedReviewedAuthoringConfirmationWithReview(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, templateDeclared, proposalDeclared bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithDeclaration(t, owners, id, templateDeclared, proposalDeclared, "*", "dodItem")
}

func seedReviewedAuthoringConfirmationWithDeclaration(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, templateDeclared, proposalDeclared bool, declarationID, declarationScope string) workspaceevents.ConfirmDefinitionAuthoringCommand {
	return seedReviewedAuthoringConfirmationWithTemplate(t, owners, id, templateDeclared, proposalDeclared, declarationID, declarationScope, nil)
}

func seedReviewedAuthoringConfirmationWithTemplate(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, templateDeclared, proposalDeclared bool, declarationID, declarationScope string, decorate func(*workspace.WorkspaceTemplate)) workspaceevents.ConfirmDefinitionAuthoringCommand {
	t.Helper()
	ctx := context.Background()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, GoalText: "交付目标", TemplateID: "template", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, Name: "模板", DoDItems: []string{"模板验收"}}
	if templateDeclared {
		hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": declarationID, "scope": declarationScope, "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		template.ContextHints = hints
	}
	if decorate != nil {
		decorate(&template)
	}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	var err error
	var derivation workspace.DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
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
			verification.EvidenceRequirementID = declarationID
			// 三个 exact 声明分别绑定对应 obligation 的确认后 DoD 身份，随后才生成六审。
			profiles := template.ContextHints.Map()["profileContracts"].(map[string]any)
			declarations := profiles["generic"].(map[string]any)["evidenceRequirements"].([]any)
			if len(declarations) > 1 {
				sourceByRef := map[string]string{"derived-dod:user:000001": "dod-001", "derived-dod:template:000001": "dod-002", "derived-dod:generated:000001": "dod-003"}
				for _, obligation := range proposal.Obligations {
					if obligation.ID == verification.ObligationID && len(obligation.DoDItemRefs) == 1 {
						verification.EvidenceRequirementID = sourceByRef[obligation.DoDItemRefs[0]]
					}
				}
			}
			verification.Scope = declarationScope
		}
	}
	var reserved workspace.DefinitionAuthoringSpecificationRequest
	record, reserved, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	mapping, audit := authoringOwnerMapping(t, record.DerivationRequests[0], proposal)
	record, err = record.CompleteSpecificationMapping(reserved.PermitID, mapping, audit)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		record, reserved, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review-" + kind, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "逐项覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: authoringRequiredRequirementIDs(t, proposal, kind)}
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
	return authoringConfirmationCommandFromPersistedLedger(t, w)
}

// 构造客户端提交身份而不调用确认门禁，负例才能证明 receiving owner 独立拒绝。
func authoringConfirmationCommandFromPersistedLedger(t *testing.T, current *workspace.Workspace) workspaceevents.ConfirmDefinitionAuthoringCommand {
	t.Helper()
	record := current.DefinitionAuthoring
	var derivationPermitID, proposalHash string
	var reviewed workspace.GoalSpecification
	var derivation *workspace.DefinitionDerivationRequest
	for index := range record.DerivationRequests {
		request := &record.DerivationRequests[index]
		if request.Input.InputHash() == record.InputHash && request.Proposal != nil {
			derivation = request
			derivationPermitID = request.PermitID
		}
	}
	if derivation == nil {
		t.Fatal("current completed derivation missing")
	}
	for _, request := range record.SpecificationRequests {
		if request.Proposal != nil && request.DerivationPermitID == derivationPermitID {
			reviewed = request.Proposal.Clone()
			proposalHash = reviewed.ContentHash()
			break
		}
	}
	for _, request := range record.SpecificationRequests {
		if request.Review != nil && request.DerivationPermitID == derivationPermitID && request.ProposalHash == proposalHash {
			reviewed.Reviews = append(reviewed.Reviews, *request.Review)
		}
	}
	if err := workspace.ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, reviewed); err != nil {
		t.Fatal(err)
	}
	return workspaceevents.ConfirmDefinitionAuthoringCommand{WorkspaceID: current.ID, InputHash: record.InputHash, DerivationPermitID: derivationPermitID, ProposalHash: proposalHash, SpecificationHash: reviewed.AdmissionHash(), ExpectedWorkspaceRevision: current.Revision}
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
			if cut != "before-prepare" {
				observed, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: cmd.WorkspaceID})
				if err != nil || observed.Confirmation == nil || observed.WorkspaceRevision <= cmd.ExpectedWorkspaceRevision {
					t.Fatalf("prepared confirmation was not queryable after reopen: %+v %v", observed, err)
				}
				cmd.ExpectedWorkspaceRevision = observed.WorkspaceRevision
			}
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
			// 页面刷新不保留客户端请求；读取 owner 后以当前 revision 恢复同一 journal。
			cmd.ExpectedWorkspaceRevision = current.Revision
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
			originalTemplate := template.Clone()
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
			if err := owners.repositories.WorkspaceTemplates.Update(ctx, &originalTemplate); err != nil {
				t.Fatal(err)
			}
			first, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			if err != nil {
				t.Fatalf("original template snapshot did not restore exact journal replay: %v", err)
			}
			second, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd})
			stored, readErr := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || !reflect.DeepEqual(first, second) || readErr != nil || stored.DefinitionAuthoringConfirmation.InputHash != cmd.InputHash || stored.DefinitionAuthoringConfirmation.SpecificationHash != cmd.SpecificationHash || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
				t.Fatalf("restored snapshot changed identity or granted activation: %+v %v %v", stored, err, readErr)
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
			template, err := owners.repositories.WorkspaceTemplates.Get(ctx, current.TemplateID)
			if err != nil {
				t.Fatal(err)
			}
			original := template.Clone()
			template.DoDItems = []string{"drifted"}
			if err := owners.repositories.WorkspaceTemplates.Update(ctx, template); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("drift with conflicting row was accepted: %v", err)
			}
			if err := owners.repositories.WorkspaceTemplates.Update(ctx, &original); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("conflicting %s was accepted: %v", cut, err)
			}
			// Restoring the exact template does not grant permission to overwrite
			// a conflicting formal row, change the hash, or abandon the journal.
			rows, rowErr := owners.repositories.DoDs.ListByWorkspaceID(ctx, current.ID)
			if rowErr != nil || cut == "dod" && (len(rows) != 1 || rows[0].Items[0] != "被改写的验收") {
				t.Fatalf("restoration overwrote conflicting formal row: %v", rowErr)
			}
			goal, goalErr := owners.repositories.Goals.Get(ctx, journal.Goal.ID)
			if goalErr != nil || cut == "goal" && goal.Text != "被改写的目标" {
				t.Fatalf("restoration overwrote conflicting Goal: %+v %v", goal, goalErr)
			}

			unchanged, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
			if err != nil || unchanged.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationPrepared || unchanged.DefinitionAuthoringConfirmation.InputHash != cmd.InputHash || unchanged.DefinitionAuthoringConfirmation.SpecificationHash != cmd.SpecificationHash || unchanged.StartupInput != nil || unchanged.ActivationAdmission != nil || unchanged.ActiveRunID != "" {
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
