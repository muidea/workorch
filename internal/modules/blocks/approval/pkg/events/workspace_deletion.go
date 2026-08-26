package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func approvalDeletionHash(kind string, scope deletioncontract.Scope, extra string) string {
	scope = scope.Normalize()
	sum := sha256.Sum256([]byte(fmt.Sprintf("approval-workspace-deletion\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", kind, scope.WorkspaceID, scope.DeletionGeneration, scope.DeletionID, scope.Mode, scope.QuiescenceID, scope.RequestHash+"\x00"+extra)))
	return "approval-workspace-deletion-sha256:" + hex.EncodeToString(sum[:])
}
func StableApprovalDeletionFenceID(s deletioncontract.Scope) string {
	return approvalDeletionHash("fence", s, "")
}
func StableApprovalDeletionPurgeID(s deletioncontract.Scope) string {
	return approvalDeletionHash("purge", s, StableApprovalDeletionFenceID(s))
}
func StableApprovalDeletionSealAckHash(s deletioncontract.Scope) string {
	return approvalDeletionHash("seal-ack", s, StableApprovalDeletionFenceID(s))
}
func StableApprovalDeletionPurgeProofHash(s deletioncontract.Scope) string {
	return approvalDeletionHash("purge-proof-empty", s, StableApprovalDeletionFenceID(s))
}
func StableApprovalDeletionPurgeAckHash(s deletioncontract.Scope) string {
	return approvalDeletionHash("purge-ack", s, StableApprovalDeletionPurgeProofHash(s))
}
func (r SealWorkspaceApprovalDeletionResult) ValidateFor(c SealWorkspaceApprovalDeletionCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s := c.Scope()
	if r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableApprovalDeletionSealAckHash(s) {
		return common.ErrConflict
	}
	return nil
}
func (r PurgeWorkspaceApprovalsResult) ValidateFor(c PurgeWorkspaceApprovalsCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s := c.Scope()
	if r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.PurgeID != StableApprovalDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableApprovalDeletionPurgeAckHash(s) || r.PurgeProofHash != StableApprovalDeletionPurgeProofHash(s) {
		return common.ErrConflict
	}
	return nil
}
