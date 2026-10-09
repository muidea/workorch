package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/testkit"
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
	return *current, authoringOwnerCompletion(t, *current, reserved.AuthoringSpecification.FreshPermitID, proposal)
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
			mapping, audit := authoringOwnerMapping(t, before.DefinitionAuthoring.DerivationRequests[0], *command.Proposal)
			command.Mapping, command.SuccessAudit, command.Proposal = &mapping, &audit, nil
			// shape 合法但 Profile 声明无效，owner 必须同 CAS 结算已知 invalid_proposal。
			if err := command.Validate(); err != nil {
				t.Fatal(err)
			}
			settled, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command})
			if err != nil || settled.AuthoringSpecification == nil || settled.AuthoringSpecification.Request.FailureCode != "invalid_proposal" {
				t.Fatalf("known semantic failure was not settled: %+v %v", settled, err)
			}
			after, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil || after.Revision != before.Revision+1 || len(after.DefinitionAuthoring.SpecificationRequests) != len(before.DefinitionAuthoring.SpecificationRequests) {
				t.Fatalf("semantic settlement changed request budget: %v", err)
			}
			if err := validateWorkspaceAuthoringMappingOutcomes(after); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command}); err != nil {
				t.Fatal(err)
			}
			replayed, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil || replayed.Revision != after.Revision {
				t.Fatal("exact semantic completion replay wrote again")
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
	parseErr := errors.New("placeholder")
	if _, err := workspace.DecodeDefinitionAuthoringMapping(original); err != nil {
		parseErr = err
	} else {
		t.Fatal("synthetic malformed document decoded")
	}
	diagnostic := workspace.NewDefinitionAuthoringDiagnostic(parseErr.Error(), original)
	observationAudit, err := testkit.Audit(*command.Mapping)
	if err != nil {
		t.Fatal(err)
	}
	observationAudit.Document = originalAuthoringObservation(original)
	observationAudit.Wire = observationAudit.Document.Clone()
	diagnostic.ProviderDocument, diagnostic.ProviderWire = &observationAudit.Document, &observationAudit.Wire
	command.Proposal, command.Mapping, command.SuccessAudit, command.FailureCode, command.Diagnostic = nil, nil, nil, "invalid_proposal", &diagnostic
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
	if err != nil || settled.AuthoringSpecification == nil || settled.AuthoringSpecification.FreshPermitID != "" || !reflect.DeepEqual(settled.AuthoringSpecification.Request.Diagnostic, diagnosticMetadataPointer(diagnostic)) || settled.AuthoringSpecification.Request.FailureCode != "invalid_proposal" {
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

func originalAuthoringObservation(payload []byte) workspace.DefinitionProviderObservation {
	sum := sha256.Sum256(payload)
	return workspace.DefinitionProviderObservation{Content: append([]byte{}, payload...), ObservedBytes: uint64(len(payload)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
}

func diagnosticMetadataPointer(d workspace.DefinitionAuthoringDiagnostic) *workspace.DefinitionAuthoringDiagnostic {
	metadata := d.Metadata()
	return &metadata
}

func TestAuthoringMappingFailureCannotBeForgedOrDowngradedOnOwnerReload(t *testing.T) {
	for _, variant := range []string{"valid-semantic-failure", "valid-parse-failure", "erase-identity", "coordinated-error-hash", "coordinated-raw-error-hash"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			before, command := reserveAuthoringProposalForCompletion(t, owners, "ws-forged-mapping")
			mapping, audit := command.Mapping.Clone(), command.SuccessAudit.Clone()
			var semanticErr error
			if variant != "valid-semantic-failure" && variant != "valid-parse-failure" {
				mapping.Obligations[0].Verification.ProfileID = "invented"
				var err error
				audit, err = testkit.Audit(mapping)
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := workspace.CompileDefinitionAuthoringMapping(before.DefinitionAuthoring.DerivationRequests[0], mapping)
				if err != nil {
					t.Fatal(err)
				}
				semanticErr = validateAuthoringVerificationAgainstSealedTemplate(before.DefinitionAuthoring.TemplateSnapshotHash, before.DefinitionAuthoring.DerivationRequests[0], proposal)
				if semanticErr == nil {
					t.Fatal("synthetic Profile defect was not invalid")
				}
			} else {
				semanticErr = errors.New("forged semantic failure")
			}
			diagnostic := workspace.MappingFailureDiagnostic(semanticErr, audit)
			next, err := before.DefinitionAuthoring.CompleteMappingFailure(command.PermitID, mapping, audit, diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			r := &next.SpecificationRequests[len(next.SpecificationRequests)-1]
			switch variant {
			case "valid-parse-failure", "erase-identity":
				r.CompletionIdentity = ""
			case "coordinated-error-hash":
				changed := workspace.MappingFailureDiagnostic(errors.New("coordinated forged error"), audit)
				r.Diagnostic = &changed
			case "coordinated-raw-error-hash":
				mapping.Obligations[0].Verification.ProfileID = "generic"
				audit, err = testkit.Audit(mapping)
				if err != nil {
					t.Fatal(err)
				}
				changed := workspace.MappingFailureDiagnostic(semanticErr, audit)
				r.Diagnostic = &changed
				r.CompletionIdentity = workspace.MappingCompletionIdentity(r.PermitID, mapping, audit)
			}
			if err := next.Validate(); err != nil {
				t.Fatalf("negative fixture fails identity framing before owner semantic check: %v", err)
			}
			if err := validateAuthoringMappingOutcomes(next); err == nil {
				t.Fatal("forged failure passed owner semantic re-evaluation")
			}
			updated := before
			updated.DefinitionAuthoring = &next
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &updated, before.Revision); err != nil {
				t.Fatal(err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			if _, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID}); err == nil {
				t.Fatal("ordinary query accepted persisted forged outcome after reopen")
			}
			if _, err := owners.workspace.queryAuthoringDiagnostic(ctx, workspaceevents.QueryAuthoringDiagnosticCommand{WorkspaceID: before.ID, PermitID: command.PermitID}); err == nil {
				t.Fatal("restricted diagnostic query accepted persisted forged outcome after reopen")
			}
		})
	}
}

func TestAuthoringMultiErrorMappingFailureReevaluatesDeterministicallyAfterReopen(t *testing.T) {
	for _, variant := range []string{"unused-dimensions", "invalid-verifications", "invalid-profiles"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			before, command := reserveAuthoringProposalForCompletion(t, owners, "ws-multifault-mapping")
			switch variant {
			case "unused-dimensions":
				command.Mapping.Dimensions = append(command.Mapping.Dimensions, workspace.DefinitionAuthoringDimensionMapping{Ref: "unused-b", ResourceIDs: []string{}}, workspace.DefinitionAuthoringDimensionMapping{Ref: "unused-a", ResourceIDs: []string{}})
			case "invalid-verifications":
				for i := range command.Mapping.Obligations {
					command.Mapping.Obligations[i].Verification.Status = "INVALID"
				}
			case "invalid-profiles":
				for i := range command.Mapping.Obligations {
					command.Mapping.Obligations[i].Verification.ProfileID = "invented"
				}
			}
			audit, err := testkit.Audit(*command.Mapping)
			if err != nil {
				t.Fatal(err)
			}
			command.SuccessAudit, command.Proposal = &audit, nil
			settled, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, AuthoringSpecComplete: &command})
			if err != nil || settled.AuthoringSpecification.Request.FailureCode != "invalid_proposal" {
				t.Fatalf("multi-error settlement: %+v %v", settled, err)
			}
			var original *workspace.DefinitionAuthoringDiagnostic
			for iteration := 0; iteration < 64; iteration++ {
				if iteration%16 == 0 {
					owners.close()
					owners = openFileDefinitionAdmissionOwners(t, root)
				}
				if _, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID}); err != nil {
					t.Fatalf("load %d: %v", iteration, err)
				}
				result, err := owners.workspace.queryAuthoringDiagnostic(ctx, workspaceevents.QueryAuthoringDiagnosticCommand{WorkspaceID: before.ID, PermitID: command.PermitID})
				if err != nil {
					t.Fatalf("restricted load %d: %v", iteration, err)
				}
				if original == nil {
					copy := result.Diagnostic.Clone()
					original = &copy
				} else if !reflect.DeepEqual(*original, result.Diagnostic) {
					t.Fatal("multi-error selection changed original diagnostic")
				}
			}
			owners.close()
		})
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
