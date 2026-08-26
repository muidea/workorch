package events

import (
	"testing"

	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestDecisionWorkspaceDeletionResultsValidateExactCommand(t *testing.T) {
	scope := deletioncontract.Scope{WorkspaceID: "ws-decision-contract", DeletionID: "delete-1", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-1"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	sealCommand := SealWorkspaceDecisionDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	seal := SealWorkspaceDecisionDeletionResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, FenceID: StableDecisionDeletionFenceID(scope), Revision: 1, AckHash: StableDecisionDeletionSealAckHash(scope)}
	purgeCommand := PurgeWorkspaceDecisionsCommand{WorkspaceID: sealCommand.WorkspaceID, DeletionID: sealCommand.DeletionID, DeletionGeneration: sealCommand.DeletionGeneration, QuiescenceID: sealCommand.QuiescenceID, RequestHash: sealCommand.RequestHash}
	purge := PurgeWorkspaceDecisionsResult{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, FenceID: StableDecisionDeletionFenceID(scope), PurgeID: StableDecisionDeletionPurgeID(scope), Revision: 2, AckHash: StableDecisionDeletionPurgeAckHash(scope), PurgeProofHash: StableDecisionDeletionPurgeProofHash(scope)}
	if err := seal.ValidateFor(sealCommand); err != nil {
		t.Fatal(err)
	}
	if err := purge.ValidateFor(purgeCommand); err != nil {
		t.Fatal(err)
	}
	retainScope := scope
	retainScope.Mode = deletioncontract.ModeRetain
	retainScope.RequestHash = deletioncontract.StableRequestHash(retainScope)
	if err := (PurgeWorkspaceDecisionsCommand{
		WorkspaceID: retainScope.WorkspaceID, DeletionID: retainScope.DeletionID,
		DeletionGeneration: retainScope.DeletionGeneration, QuiescenceID: retainScope.QuiescenceID, RequestHash: retainScope.RequestHash,
	}).Validate(); err == nil {
		t.Fatal("retain deletion intent was escalated by a purge command")
	}
	for _, tc := range []struct {
		name   string
		mutate func()
	}{
		{name: "seal ack", mutate: func() { seal.AckHash = "forged" }},
		{name: "purge id", mutate: func() { purge.PurgeID = "forged" }},
		{name: "purge proof", mutate: func() { purge.PurgeProofHash = "forged" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sealCopy, purgeCopy := seal, purge
			t.Cleanup(func() { seal, purge = sealCopy, purgeCopy })
			tc.mutate()
			if tc.name == "seal ack" {
				if seal.ValidateFor(sealCommand) == nil {
					t.Fatal("forged seal accepted")
				}
			} else if purge.ValidateFor(purgeCommand) == nil {
				t.Fatal("forged purge accepted")
			}
		})
	}
}
