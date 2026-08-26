package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func decisionDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	sum := sha256.Sum256([]byte(fmt.Sprintf("decision-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)))
	return "decision-workspace-deletion-sha256:" + hex.EncodeToString(sum[:])
}
func StableDecisionDeletionFenceID(s deletioncontract.Scope) string {
	return decisionDeletionHash("fence", s, "")
}
func StableDecisionDeletionPurgeID(s deletioncontract.Scope) string {
	return decisionDeletionHash("purge", s, StableDecisionDeletionFenceID(s))
}
func StableDecisionDeletionSealAckHash(s deletioncontract.Scope) string {
	return decisionDeletionHash("seal-ack", s, StableDecisionDeletionFenceID(s))
}
func StableDecisionDeletionPurgeProofHash(s deletioncontract.Scope) string {
	return decisionDeletionHash("purge-proof-empty", s, StableDecisionDeletionFenceID(s))
}
func StableDecisionDeletionPurgeAckHash(s deletioncontract.Scope) string {
	return decisionDeletionHash("purge-ack", s, StableDecisionDeletionPurgeProofHash(s))
}
func (r SealWorkspaceDecisionDeletionResult) ValidateFor(c SealWorkspaceDecisionDeletionCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s := c.Scope()
	if r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableDecisionDeletionSealAckHash(s) {
		return common.ErrConflict
	}
	return nil
}
func (r PurgeWorkspaceDecisionsResult) ValidateFor(c PurgeWorkspaceDecisionsCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s := c.Scope()
	if r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.PurgeID != StableDecisionDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableDecisionDeletionPurgeAckHash(s) || r.PurgeProofHash != StableDecisionDeletionPurgeProofHash(s) {
		return common.ErrConflict
	}
	return nil
}
