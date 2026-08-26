package events

import (
	"testing"

	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestApprovalWorkspaceDeletionResultsValidateExactCommand(t *testing.T) {
	scope := deletioncontract.Scope{WorkspaceID: "ws-approval-contract", DeletionID: "delete-1", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-1"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	sealCommand := SealWorkspaceApprovalDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	seal := SealWorkspaceApprovalDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, FenceID: StableApprovalDeletionFenceID(scope), Revision: 1, AckHash: StableApprovalDeletionSealAckHash(scope)}
	purgeCommand := PurgeWorkspaceApprovalsCommand{WorkspaceID: sealCommand.WorkspaceID, DeletionID: sealCommand.DeletionID, DeletionGeneration: sealCommand.DeletionGeneration, QuiescenceID: sealCommand.QuiescenceID, RequestHash: sealCommand.RequestHash}
	purge := PurgeWorkspaceApprovalsResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, FenceID: StableApprovalDeletionFenceID(scope), PurgeID: StableApprovalDeletionPurgeID(scope), Revision: 2, AckHash: StableApprovalDeletionPurgeAckHash(scope), PurgeProofHash: StableApprovalDeletionPurgeProofHash(scope)}
	if err := seal.ValidateFor(sealCommand); err != nil {
		t.Fatal(err)
	}
	if err := purge.ValidateFor(purgeCommand); err != nil {
		t.Fatal(err)
	}
	retainScope := scope
	retainScope.Mode = deletioncontract.ModeRetain
	retainScope.RequestHash = deletioncontract.StableRequestHash(retainScope)
	if err := (PurgeWorkspaceApprovalsCommand{
		WorkspaceID: retainScope.WorkspaceID, DeletionID: retainScope.DeletionID,
		DeletionGeneration: retainScope.DeletionGeneration, QuiescenceID: retainScope.QuiescenceID, RequestHash: retainScope.RequestHash,
	}).Validate(); err == nil {
		t.Fatal("retain deletion intent was escalated by a purge command")
	}
	seal.AckHash = "forged"
	if seal.ValidateFor(sealCommand) == nil {
		t.Fatal("forged seal accepted")
	}
	purge.PurgeProofHash = "forged"
	if purge.ValidateFor(purgeCommand) == nil {
		t.Fatal("forged purge accepted")
	}
}
