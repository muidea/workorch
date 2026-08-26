package governanceports

import (
	"context"
	"errors"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func clarificationOpenCommandForPortTest() decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand {
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:   "workspace-1", CandidateID: "workspace-definition", CandidateRevision: 1,
		CandidateHash: "candidate-hash-1", ViolationCode: "definition_one_of_unsupported", OffendingRefs: []string{"relation-1"},
		CanonicalSignatureVersion: "workspace-definition-clarification-v1", CanonicalSignature: "one-of:relation-1",
		RequiredActionKind: decisionevents.DefinitionClarificationActionRewriteAllOf,
	}
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}

func TestDefinitionClarificationPortRejectsMismatchedDecisionOwnerResult(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	command := clarificationOpenCommandForPortTest()
	testHub.subscribeDecision(decisionevents.TopicOpenWorkspaceDefinitionClarification, func(any) any {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{
			TicketID: "unrelated-ticket", WorkspaceID: command.WorkspaceID, CandidateID: command.CandidateID, CandidateRevision: command.CandidateRevision,
			Disposition: decisionevents.DefinitionClarificationCreated, ClarificationRequestBindingHash: command.BindingHash(),
		}
	})
	if _, err := OpenWorkspaceDefinitionClarification(context.Background(), testHub.rawHub, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched Decision result err=%v, want conflict", err)
	}
}

func TestDefinitionClarificationResolutionPortRejectsMismatchedDecisionOwnerResult(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	open := clarificationOpenCommandForPortTest()
	command := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: open.TicketID, WorkspaceID: open.WorkspaceID,
		OldCandidateID: open.CandidateID, OldCandidateRevision: open.CandidateRevision, OldCandidateHash: open.CandidateHash,
		NewCandidateID: open.CandidateID, NewCandidateRevision: 2, NewCandidateHash: "candidate-hash-2",
		AdmissionResultHash: "admission-result-hash", DefinitionRevision: "definition-r2", StartupInputHash: "startup-r2",
	}
	seed := command.AdmissionResultHash + "\x00" + command.StartupInputHash
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(command.TicketID, seed)
	testHub.subscribeDecision(decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission, func(any) any {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{
			TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, ResolutionID: resolutionID,
			ResolutionType: decisionevents.DefinitionClarificationResolutionCandidateAdjusted, Disposition: decisionevents.DefinitionClarificationResolved,
			ResolutionBindingHash: "forged-binding",
		}
	})
	if _, err := ResolveWorkspaceDefinitionClarificationByAdmission(context.Background(), testHub.rawHub, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched Decision resolution err=%v, want conflict", err)
	}
}

func TestDefinitionClarificationReconciliationPortRejectsMismatchedDecisionOwnerResult(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	oldRequest := clarificationOpenCommandForPortTest()
	newRequest := oldRequest
	newRequest.CandidateRevision = 2
	newRequest.CandidateHash = "candidate-hash-2"
	newRequest.CanonicalSignature = "one-of:relation-2"
	newRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", newRequest.CanonicalSignatureVersion, newRequest.CanonicalSignature)
	newRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(newRequest)
	command := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldRequest, NewRequest: newRequest}
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(oldRequest.TicketID, newRequest.BindingHash())
	testHub.subscribeDecision(decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, func(any) any {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{
			WorkspaceID: oldRequest.WorkspaceID, OldTicketID: oldRequest.TicketID, OldResolutionID: resolutionID,
			NewTicketID: "unrelated-ticket", NewTicketDisposition: decisionevents.DefinitionClarificationCreated,
			NewRequestBindingHash: newRequest.BindingHash(), ReconciliationBindingHash: command.BindingHash(resolutionID),
		}
	})
	if _, err := ReconcileWorkspaceDefinitionClarificationCandidate(context.Background(), testHub.rawHub, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched Decision reconciliation err=%v, want conflict", err)
	}
}
