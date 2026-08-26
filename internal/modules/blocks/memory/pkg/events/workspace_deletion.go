package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func StableMemoryDeletionFenceID(scope deletioncontract.Scope) string {
	return memoryDeletionHash("fence", scope, "")
}

func StableMemoryDeletionSealAckHash(scope deletioncontract.Scope) string {
	return memoryDeletionHash("seal-ack", scope, StableMemoryDeletionFenceID(scope))
}

func StableMemoryDeletionPurgeProofHash(scope deletioncontract.Scope) string {
	return memoryDeletionHash("purge-proof-empty", scope, StableMemoryDeletionFenceID(scope))
}

func StableMemoryDeletionPurgeID(scope deletioncontract.Scope) string {
	return memoryDeletionHash("purge", scope, StableMemoryDeletionFenceID(scope))
}

func StableMemoryDeletionPurgeAckHash(scope deletioncontract.Scope) string {
	return memoryDeletionHash("purge-ack", scope, StableMemoryDeletionPurgeProofHash(scope))
}

func memoryDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("memory-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)
	digest := sha256.Sum256([]byte(material))
	return "memory-workspace-deletion-sha256:" + hex.EncodeToString(digest[:])
}

func (r SealWorkspaceMemoryDeletionResult) ValidateFor(command SealWorkspaceMemoryDeletionCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableMemoryDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableMemoryDeletionSealAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}

func (r PurgeWorkspaceMemoryResult) ValidateFor(command PurgeWorkspaceMemoryCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableMemoryDeletionFenceID(scope) || r.PurgeID != StableMemoryDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableMemoryDeletionPurgeProofHash(scope) || r.AckHash != StableMemoryDeletionPurgeAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}
