package events

import (
	"testing"

	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestMemoryWorkspaceDeletionCommandsSeparateIntentFromOwnerTransition(t *testing.T) {
	scope := deletioncontract.Scope{WorkspaceID: "ws-memory-contract", DeletionID: "delete-1", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-1"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	seal := SealWorkspaceMemoryDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if err := seal.Validate(); err != nil || seal.Scope() != scope {
		t.Fatalf("seal scope = %+v err=%v", seal.Scope(), err)
	}
	purge := PurgeWorkspaceMemoryCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	if err := purge.Validate(); err != nil || purge.Scope() != scope {
		t.Fatalf("purge scope = %+v err=%v", purge.Scope(), err)
	}
	retain := scope
	retain.Mode = deletioncontract.ModeRetain
	retain.RequestHash = deletioncontract.StableRequestHash(retain)
	purge.RequestHash = retain.RequestHash
	if err := purge.Validate(); err == nil {
		t.Fatal("retain deletion intent was escalated by a purge command")
	}
}
