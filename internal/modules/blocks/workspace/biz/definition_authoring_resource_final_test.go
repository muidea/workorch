package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestResourceBoundSpecificationReservationUsesSealedDerivation(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	content := "验收资源"
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners, file := openResourceAdmissionOwners(t, storage)
	defer func() { closeResourceAdmissionOwners(owners, file) }()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-resource-spec-reserve", Revision: 1, GoalText: "交付目标", TemplateID: "resource-spec-template", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, Name: "模板", DoDItems: []string{"模板验收"}}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	sum := sha256.Sum256([]byte(content))
	resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: content, ContentHash: "sha256:" + hex.EncodeToString(sum[:])}}
	var derivation workspace.DefinitionDerivationRequest
	var err error
	record, derivation, _, err = record.ReserveDerivation("derive", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: input.TemplateID, DeliverableDirectory: directory, DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: "proposal/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, ProviderBindingHash: derivation.ProviderBindingHash}
	send := func() workspaceevents.AuthoringSpecificationRequestResult {
		t.Helper()
		response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, definitionSpecificationCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
		if response == nil || response.Error() != nil {
			t.Fatalf("resource-bound specification reserve failed: %+v", response)
		}
		value, err := response.Get()
		if err != nil {
			t.Fatal(err)
		}
		result, ok := value.(workspaceevents.AuthoringSpecificationRequestResult)
		if !ok || result.ValidateReserve(command) != nil {
			t.Fatalf("invalid reserve response: %+v", value)
		}
		return result
	}
	first := send()
	if first.FreshPermitID == "" || first.Request.DerivationPermitID != derivation.PermitID {
		t.Fatalf("persisted resource derivation did not grant first specification permit: %+v", first)
	}
	closeResourceAdmissionOwners(owners, file)
	owners, file = openResourceAdmissionOwners(t, storage)
	second := send()
	if second.FreshPermitID != "" || second.Request.PermitID != first.Request.PermitID {
		t.Fatalf("reopen granted second physical specification call: %+v", second)
	}
}

func seedResourceBoundAuthoringConfirmation(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID, directory, content string, declared ...bool) workspaceevents.ConfirmDefinitionAuthoringCommand {
	t.Helper()
	ctx := context.Background()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, GoalText: "交付目标", TemplateID: "resource-template", AcceptanceRequirements: []string{"用户验收"}, ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, Name: "模板", DoDItems: []string{"模板验收"}}
	eligible := len(declared) > 0 && declared[0]
	hints, hintErr := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
	if hintErr != nil {
		t.Fatal(hintErr)
	}
	template.ContextHints = hints
	if !eligible {
		hints := template.ContextHints.Map()
		generic := hints["profileContracts"].(map[string]any)["generic"].(map[string]any)
		generic["deliverableExpectations"] = []any{map[string]any{"id": "required-delivery", "required": true}}
		var err error
		template.ContextHints, err = workorcheventpayload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	sum := sha256.Sum256([]byte(content))
	resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: input.ResourceRefs[0], Content: content, ContentHash: "sha256:" + hex.EncodeToString(sum[:])}}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	var err error
	var derivation workspace.DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	proposal := authoringOwnerProposal(t, input, template, derived, resources)
	for index := range proposal.Verifications {
		verification := &proposal.Verifications[index]
		verification.Status = workspace.GoalVerificationDeclared
		verification.ProfileID = "generic"
		verification.EvidenceRequirementID = "*"
		verification.Scope = "dodItem"
	}
	proposal.Resources, err = workspace.AuthoringSpecificationResources(input, resources)
	if err != nil {
		t.Fatal(err)
	}
	record, request, _, err := record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: "provider"})
	if err != nil {
		t.Fatal(err)
	}
	mapping, audit := authoringOwnerMapping(t, record.DerivationRequests[0], proposal)
	record, err = record.CompleteSpecificationMapping(request.PermitID, mapping, audit)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		record, request, _, err = record.ReserveSpecification(workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review-" + kind, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
		if err != nil {
			t.Fatal(err)
		}
		review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "逐项覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: authoringRequiredRequirementIDs(t, proposal, kind)}
		record, err = record.CompleteSpecification(request.PermitID, nil, &review, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: id, TemplateID: input.TemplateID, DeliverableDirectory: directory, DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, reviewed, err := record.ConfirmableSpecification(derivation.PermitID, proposal.ContentHash())
	if err != nil {
		t.Fatal(err)
	}
	return workspaceevents.ConfirmDefinitionAuthoringCommand{WorkspaceID: id, InputHash: record.InputHash, DerivationPermitID: derivation.PermitID, ProposalHash: proposal.ContentHash(), SpecificationHash: reviewed.AdmissionHash(), ExpectedWorkspaceRevision: current.Revision}
}

func sendResourceConfirmation(t *testing.T, owners *fileDefinitionAdmissionOwners, command workspaceevents.ConfirmDefinitionAuthoringCommand) (workspaceevents.ConfirmDefinitionAuthoringResult, error) {
	t.Helper()
	response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringConfirm, definitionSpecificationCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), command))
	if response == nil {
		t.Fatal("confirmation response missing")
	}
	if err := response.Error(); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	value, err := response.Get()
	if err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	result, ok := value.(workspaceevents.ConfirmDefinitionAuthoringResult)
	if !ok {
		t.Fatalf("unexpected confirmation response %T", value)
	}
	return result, nil
}

func sendResourceAdmission(t *testing.T, owners *fileDefinitionAdmissionOwners, command workspaceevents.AdmitDefinitionAuthoringCommand) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	t.Helper()
	response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringAdmit, definitionSpecificationCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), command))
	if response == nil {
		t.Fatal("admission response missing")
	}
	if err := response.Error(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	value, err := response.Get()
	if err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	result, ok := value.(workspaceevents.AdmitDefinitionAuthoringResult)
	if !ok {
		t.Fatalf("unexpected admission response %T", value)
	}
	return result, nil
}

func openResourceAdmissionOwners(t *testing.T, root string) (*fileDefinitionAdmissionOwners, *fileowner.Biz) {
	t.Helper()
	owners := openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		owners.close()
		t.Fatal(err)
	}
	file, err := fileowner.New(owners.hub, nil)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		file.Teardown(context.Background())
		owners.close()
		t.Fatal(err)
	}
	return owners, file
}

func closeResourceAdmissionOwners(owners *fileDefinitionAdmissionOwners, file *fileowner.Biz) {
	if file != nil {
		file.Teardown(context.Background())
	}
	if owners != nil {
		owners.close()
	}
}

func TestResourceBoundAuthoringConfirmationAndAdmissionRecheckFileAfterReopen(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	path := filepath.Join(directory, "requirements.txt")
	content := "已审查的资源正文\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners, file := openResourceAdmissionOwners(t, storage)
	defer func() { closeResourceAdmissionOwners(owners, file) }()
	confirmation := seedResourceBoundAuthoringConfirmation(t, owners, "ws-resourced-admission", directory, content)
	first, err := sendResourceConfirmation(t, owners, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || current.DefinitionAuthoringConfirmation == nil || current.DefinitionAuthoringConfirmation.ResourceSnapshotHash == "" || current.StartupInput != nil {
		t.Fatalf("confirmation did not seal resource provenance: %+v %v", current, err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	closeResourceAdmissionOwners(owners, file)
	owners, file = openResourceAdmissionOwners(t, storage)
	if err := os.WriteFile(path, []byte("已修改的资源正文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sendResourceAdmission(t, owners, command); err == nil {
		t.Fatal("drift after materialized confirmation obtained admission")
	}
	if _, err := sendResourceConfirmation(t, owners, confirmation); err == nil {
		t.Fatal("drift after materialization replayed confirmation")
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := sendResourceConfirmation(t, owners, confirmation)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("resource confirmation response-loss replay changed after reopen: %+v %+v %v", first, second, err)
	}
	admitted, err := sendResourceAdmission(t, owners, command)
	if err != nil || admitted.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired || admitted.StartupInput != nil {
		t.Fatalf("resource admission did not preserve reviewed blocking result: %+v %v", admitted, err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || current.ActiveRunID != "" || current.StartupInput != nil || current.ActivationAdmission != nil || len(current.DefinitionAdmissionCandidates) != 1 {
		t.Fatalf("resource admission created startup or lost blocked candidate: %+v %v", current, err)
	}
	if err := os.WriteFile(path, []byte("new drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sendResourceAdmission(t, owners, command); err == nil {
		t.Fatal("response-loss replay bypassed File recheck")
	}
}

func TestResourceBoundConfirmationRejectsDriftBeforePrepareAndAtPreparedCut(t *testing.T) {
	ctx := context.Background()
	for _, cut := range []string{"before-prepare", "prepared", "goal"} {
		t.Run(cut, func(t *testing.T) {
			storage, directory := t.TempDir(), t.TempDir()
			path := filepath.Join(directory, "requirements.txt")
			content := "原始资源"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			owners, file := openResourceAdmissionOwners(t, storage)
			defer func() { closeResourceAdmissionOwners(owners, file) }()
			command := seedResourceBoundAuthoringConfirmation(t, owners, common.WorkspaceID("ws-resource-"+cut), directory, content)
			current, err := owners.repositories.Workspaces.Get(ctx, command.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if cut != "before-prepare" {
				proof, err := owners.workspace.verifyCurrentAuthoringResources(ctx, command.WorkspaceID, command.DerivationPermitID)
				if err != nil {
					t.Fatal(err)
				}
				journal, err := owners.workspace.prepareDefinitionAuthoringConfirmationWithProof(ctx, current, command, &proof)
				if err != nil {
					t.Fatal(err)
				}
				prepared := *current
				prepared.DefinitionAuthoringConfirmation = &journal
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &prepared, current.Revision); err != nil {
					t.Fatal(err)
				}
				if cut == "goal" {
					if err := owners.repositories.Goals.Create(ctx, &journal.Goal); err != nil {
						t.Fatal(err)
					}
				}
				closeResourceAdmissionOwners(owners, file)
				owners, file = openResourceAdmissionOwners(t, storage)
			}
			if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := sendResourceConfirmation(t, owners, command); err == nil {
				t.Fatal("resource drift continued a confirmation journal")
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, command.WorkspaceID)
			if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
				t.Fatalf("drift produced admission: %+v %v", stored, err)
			}
			if cut == "before-prepare" && stored.DefinitionAuthoringConfirmation != nil {
				t.Fatal("drift prepared a journal")
			}
			if cut != "before-prepare" && (stored.DefinitionAuthoringConfirmation == nil || stored.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationPrepared) {
				t.Fatal("drift materialized a prepared journal")
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			first, err := sendResourceConfirmation(t, owners, command)
			if err != nil {
				t.Fatalf("reopening a prepared journal at same snapshot failed: %v", err)
			}
			closeResourceAdmissionOwners(owners, file)
			owners, file = openResourceAdmissionOwners(t, storage)
			second, err := sendResourceConfirmation(t, owners, command)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("restored snapshot replay changed original after-image: %+v %+v %v", first, second, err)
			}
			stored, err = owners.repositories.Workspaces.Get(ctx, command.WorkspaceID)
			if err != nil || stored.DefinitionAuthoringConfirmation.InputHash != command.InputHash || stored.DefinitionAuthoringConfirmation.SpecificationHash != command.SpecificationHash || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
				t.Fatalf("restoration replaced confirmation identity or started execution: %+v %v", stored, err)
			}
		})
	}
}

func TestResourceBoundAdmissionPersistsEligibleAfterFileRecheck(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	content := "资源正文"
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners, file := openResourceAdmissionOwners(t, storage)
	defer func() { closeResourceAdmissionOwners(owners, file) }()
	confirmation := seedResourceBoundAuthoringConfirmation(t, owners, "ws-resource-eligible", directory, content, true)
	if _, err := sendResourceConfirmation(t, owners, confirmation); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	admitted, err := sendResourceAdmission(t, owners, command)
	if err != nil || admitted.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || admitted.StartupInput == nil || admitted.ActivationAdmission == nil {
		t.Fatalf("File-verified exact six-review candidate was not eligible: %+v %v", admitted, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.ActiveRunID != "" || stored.StartupInput == nil || stored.ActivationAdmission == nil {
		t.Fatalf("eligible admission started a Run or lost startup: %+v %v", stored, err)
	}
	if _, present, err := stored.ValidateActivationAdmissionPersistence(); err != nil || !present {
		t.Fatalf("eligible resource admission lost review provenance: %v", err)
	}
	closeResourceAdmissionOwners(owners, file)
	owners, file = openResourceAdmissionOwners(t, storage)
	replayed, err := sendResourceAdmission(t, owners, command)
	if err != nil || !reflect.DeepEqual(admitted, replayed) {
		t.Fatalf("resource admission response-loss replay changed eligible result: %+v %+v %v", admitted, replayed, err)
	}
}

func TestResourceBoundAdmissionRejectsStaleOwnerProof(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	content := "资源正文"
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners, file := openResourceAdmissionOwners(t, storage)
	defer func() { closeResourceAdmissionOwners(owners, file) }()
	confirmation := seedResourceBoundAuthoringConfirmation(t, owners, "ws-resource-admission-proof", directory, content)
	if _, err := sendResourceConfirmation(t, owners, confirmation); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("direct admission bypassed File owner: %v", err)
	}
	proof, err := owners.workspace.verifyCurrentAuthoringResources(ctx, current.ID, confirmation.DerivationPermitID)
	if err != nil {
		t.Fatal(err)
	}
	current.Description = "new revision"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringAdmissionWithProof(ctx, command, &proof); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale admission File fact survived Workspace CAS fence: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 0 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("stale File fact mutated admission: %+v %v", stored, err)
	}
}

func TestResourceBoundConfirmationRejectsMissingOrStaleOwnerProof(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	content := "资源正文"
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners, file := openResourceAdmissionOwners(t, storage)
	defer func() { closeResourceAdmissionOwners(owners, file) }()
	cmd := seedResourceBoundAuthoringConfirmation(t, owners, "ws-resource-proof", directory, content)
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("direct confirmation bypassed File owner: %v", err)
	}
	proof, err := owners.workspace.verifyCurrentAuthoringResources(ctx, cmd.WorkspaceID, cmd.DerivationPermitID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.prepareDefinitionAuthoringConfirmationWithProof(ctx, current, cmd, &proof); err != nil {
		t.Fatalf("verified proof did not match revision: %v", err)
	}
	current.Description = "new revision"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd, VerifiedResources: &proof}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale verification passed Workspace revision fence: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil || stored.DefinitionAuthoringConfirmation != nil || stored.StartupInput != nil {
		t.Fatalf("stale proof wrote confirmation: %+v %v", stored, err)
	}
}
