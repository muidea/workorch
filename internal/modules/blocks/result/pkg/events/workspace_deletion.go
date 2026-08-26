package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func StableResultDeletionFenceID(scope deletioncontract.Scope) string {
	return resultDeletionHash("fence", scope, "")
}

func StableResultDeletionSealAckHash(scope deletioncontract.Scope) string {
	return resultDeletionHash("seal-ack", scope, StableResultDeletionFenceID(scope))
}

func StableResultDeletionPurgeProofHash(scope deletioncontract.Scope) string {
	return resultDeletionHash("purge-proof-empty", scope, StableResultDeletionFenceID(scope))
}

func StableResultDeletionPurgeID(scope deletioncontract.Scope) string {
	return resultDeletionHash("purge", scope, StableResultDeletionFenceID(scope))
}

func StableResultDeletionPurgeAckHash(scope deletioncontract.Scope) string {
	return resultDeletionHash("purge-ack", scope, StableResultDeletionPurgeProofHash(scope))
}

func resultDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	material := fmt.Sprintf("result-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)
	digest := sha256.Sum256([]byte(material))
	return "result-workspace-deletion-sha256:" + hex.EncodeToString(digest[:])
}

func (r SealWorkspaceResultDeletionResult) ValidateFor(command SealWorkspaceResultDeletionCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableResultDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableResultDeletionSealAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}

func (r PurgeWorkspaceResultsResult) ValidateFor(command PurgeWorkspaceResultsCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	scope := command.Scope()
	if r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration ||
		r.FenceID != StableResultDeletionFenceID(scope) || r.PurgeID != StableResultDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableResultDeletionPurgeProofHash(scope) || r.AckHash != StableResultDeletionPurgeAckHash(scope) {
		return common.ErrConflict
	}
	return nil
}
