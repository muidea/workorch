package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func reserveAuthoringProposalForCompletion(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) (workspace.Workspace, workspaceevents.CompleteAuthoringSpecificationCommand) {
	t.Helper()
	ctx := context.Background()
	confirmation := seedReviewedAuthoringConfirmationWithProof(t, owners, id, true)
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var proposal workspace.GoalSpecification
	for _, request := range current.DefinitionAuthoring.SpecificationRequests {
		if request.Proposal != nil {
			proposal = request.Proposal.Clone()
			break
		}
	}
	reserve := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: confirmation.InputHash, DerivationPermitID: confirmation.DerivationPermitID, RequestID: "independent-completion", ProviderBindingHash: "provider"}
	reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &reserve})
	if err != nil || reserved.AuthoringSpecification == nil || reserved.AuthoringSpecification.FreshPermitID == "" {
		t.Fatalf("reserve completion: %+v %v", reserved, err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return *current, workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: id, PermitID: reserved.AuthoringSpecification.FreshPermitID, Proposal: &proposal}
}

func TestAuthoringOwnerCompletionRechecksSealedProfileApplicability(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*workspace.GoalSpecification)
	}{
		{"wrong-profile", func(p *workspace.GoalSpecification) { p.Verifications[0].ProfileID = "invented" }},
		{"wrong-declaration", func(p *workspace.GoalSpecification) { p.Verifications[0].EvidenceRequirementID = "invented" }},
		{"wrong-scope", func(p *workspace.GoalSpecification) { p.Verifications[0].Scope = "deliverable" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			before, command := reserveAuthoringProposalForCompletion(t, owners, "ws-owner-completion")
			test.change(command.Proposal)
			// DTO 与来源图仍然合法；拒绝必须来自实际封存 Profile，而非 Application parser。
			if err := command.Validate(); err != nil {
				t.Fatal(err)
			}
			derivation := before.DefinitionAuthoring.DerivationRequests[0]
			if err := workspace.ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, *command.Proposal); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); err == nil {
				t.Fatal("typed completion bypassed sealed Profile applicability")
			}
			after, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil || !reflect.DeepEqual(&before, after) {
				t.Fatalf("invalid completion changed persisted Workspace: %v", err)
			}
		})
	}
}

func TestAuthoringKnownFailureCASPreservesOriginalDiagnosticForExactReplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	before, command := reserveAuthoringProposalForCompletion(t, owners, "ws-known-failure-cas")
	original := []byte("\xef\xbb\xbf  {\"invalid\":\"原文\\r\\nsynthetic_marker\"}\r\n")
	diagnostic := workspace.NewDefinitionAuthoringDiagnostic("原始解析失败\r\n第二行", original)
	command.Proposal, command.FailureCode, command.Diagnostic = nil, "invalid_proposal", &diagnostic
	cause := errors.New("synthetic diagnostic completion CAS failure")
	failing := &failPreparationDiagnosticCASRepository{WorkspaceRepository: owners.workspace.workspaces, cause: cause}
	owners.workspace.workspaces = failing
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); !errors.Is(err, cause) {
		owners.close()
		t.Fatalf("known failure CAS error = %v", err)
	}
	if failing.calls != 1 {
		owners.close()
		t.Fatalf("known failure CAS attempts = %d", failing.calls)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	after, err := owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil || !reflect.DeepEqual(&before, after) {
		t.Fatalf("failed diagnostic settlement changed persisted reservation: %v", err)
	}
	settled, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command})
	if err != nil || settled.AuthoringSpecification == nil || settled.AuthoringSpecification.FreshPermitID != "" || !reflect.DeepEqual(settled.AuthoringSpecification.Request.Diagnostic, &diagnostic) || settled.AuthoringSpecification.Request.FailureCode != "invalid_proposal" {
		t.Fatalf("exact failed completion replay lost original bytes: %+v %v", settled, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	revision := stored.Revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); err != nil {
		t.Fatal(err)
	}
	rewritten := workspace.NewDefinitionAuthoringDiagnostic("改写解析错误", original)
	command.Diagnostic = &rewritten
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); err == nil {
		t.Fatal("changed diagnostic replaced immutable known failure")
	}
	stored, err = owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil || stored.Revision != revision || stored.ActiveRunID != "" {
		t.Fatalf("replay changed owner revision or started Run: %+v %v", stored, err)
	}
}

func TestAuthoringKnownProposalCASFailurePreservesReservedIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	before, command := reserveAuthoringProposalForCompletion(t, owners, "ws-known-proposal-cas")
	cause := errors.New("synthetic known proposal CAS failure")
	failing := &failPreparationDiagnosticCASRepository{WorkspaceRepository: owners.workspace.workspaces, cause: cause}
	owners.workspace.workspaces = failing
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); !errors.Is(err, cause) {
		owners.close()
		t.Fatalf("known proposal CAS error = %v", err)
	}
	if failing.calls != 1 {
		owners.close()
		t.Fatalf("known proposal CAS attempts = %d", failing.calls)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	after, err := owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil || !reflect.DeepEqual(&before, after) {
		t.Fatalf("failed settlement altered durable reservation: %v", err)
	}
	// 重启只能读取原许可未决事实；测试显式重放原已知完成，不授予新 provider 许可。
	settled, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command})
	if err != nil || settled.AuthoringSpecification == nil || settled.AuthoringSpecification.FreshPermitID != "" || settled.AuthoringSpecification.Request.PermitID != command.PermitID || settled.AuthoringSpecification.Request.Proposal == nil {
		t.Fatalf("exact known completion replay: %+v %v", settled, err)
	}
}
