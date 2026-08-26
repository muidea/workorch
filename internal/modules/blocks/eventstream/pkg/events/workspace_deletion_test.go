package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func eventStreamDeletionTestScope(mode deletioncontract.Mode) deletioncontract.Scope {
	scope := deletioncontract.Scope{
		WorkspaceID: "ws-eventstream-delete", DeletionID: "deletion-eventstream-1", DeletionGeneration: 1,
		Mode: mode, QuiescenceID: "quiescence-eventstream-1",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return scope
}

func TestEventStreamWorkspaceDeletionResultsValidateExactStableIdentity(t *testing.T) {
	scope := eventStreamDeletionTestScope(deletioncontract.ModePurge)
	seal := SealWorkspaceEventStreamDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	sealResult := SealWorkspaceEventStreamDeletionResult{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		FenceID: StableEventStreamDeletionFenceID(scope), Revision: 1, AckHash: StableEventStreamDeletionSealAckHash(scope),
	}
	if err := sealResult.ValidateFor(seal); err != nil {
		t.Fatalf("validate seal result: %v", err)
	}
	changedSeal := seal
	changedSeal.QuiescenceID = "quiescence-other"
	changedSeal.RequestHash = deletioncontract.StableRequestHash(changedSeal.Scope())
	if err := sealResult.ValidateFor(changedSeal); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed seal validation = %v, want conflict", err)
	}

	purge := PurgeWorkspaceCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	purgeResult := PurgeWorkspaceResult{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		FenceID: StableEventStreamDeletionFenceID(scope), PurgeID: StableEventStreamDeletionPurgeID(scope), Revision: 2,
		AckHash: StableEventStreamDeletionPurgeAckHash(scope), PurgeProofHash: StableEventStreamDeletionPurgeProofHash(scope),
	}
	if err := purgeResult.ValidateFor(purge); err != nil {
		t.Fatalf("validate purge result: %v", err)
	}
	changedResult := purgeResult
	changedResult.PurgeProofHash = "different"
	if err := changedResult.ValidateFor(purge); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed purge validation = %v, want conflict", err)
	}
}

func TestEventStreamWorkspaceDeletionRetainCannotPurge(t *testing.T) {
	scope := eventStreamDeletionTestScope(deletioncontract.ModeRetain)
	command := PurgeWorkspaceCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if err := command.Validate(); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("retain purge validation = %v, want request-hash conflict", err)
	}
}
