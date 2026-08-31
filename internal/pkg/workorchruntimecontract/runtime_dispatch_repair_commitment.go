package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDispatchRepairPurpose identifies an immutable, admission-time repair
// purpose.  It is dispatch authority, not a reducer state or a capability-owner
// instruction.
type RuntimeDispatchRepairPurpose string

const (
	RuntimeDispatchRepairPurposeConfirmedValidation RuntimeDispatchRepairPurpose = "confirmed_validation_repair"
)

// RuntimeDispatchRepairCommitment is the immutable SM-3 projection of one
// validation rejection -> resource-owner confirmation -> owner repair closure. The source
// confirmation, rejected evidence and owner receipt remain their own ledgers;
// this value makes their mandatory relationship non-erasable from a completed
// dispatch and lets fresh-load require every reverse edge exactly.
//
// OriginDispatchID remains unchanged across operation-governance resume hops.
// No field is derived from current Focus, FailureTarget or RepairDirective.
type RuntimeDispatchRepairCommitment struct {
	Purpose             RuntimeDispatchRepairPurpose
	RepairOwnerModuleID string
	OriginDispatchID    string
	FocusID             string
	Closure             RuntimeRepairClosure

	ConfirmationIdentityHash string
	ConfirmationHash         string

	SourceDispatchID          string
	SourceAuthoritySnapshotID string
	SourceOperationID         string
	SourceOperationHash       string
	SourceEvidenceFactID      common.EvidenceRecordID
	SourceEvidenceFactHash    string
	SourceReceiptExecutionKey string
	SourceReceiptFence        uint64
	SourceReceiptHash         string
}

func (c RuntimeDispatchRepairCommitment) Normalized() RuntimeDispatchRepairCommitment {
	c.Purpose = RuntimeDispatchRepairPurpose(strings.TrimSpace(string(c.Purpose)))
	c.RepairOwnerModuleID = strings.TrimSpace(c.RepairOwnerModuleID)
	c.OriginDispatchID = strings.TrimSpace(c.OriginDispatchID)
	c.FocusID = strings.TrimSpace(c.FocusID)
	c.Closure = c.Closure.Normalized()
	c.ConfirmationIdentityHash = strings.TrimSpace(c.ConfirmationIdentityHash)
	c.ConfirmationHash = strings.TrimSpace(c.ConfirmationHash)
	c.SourceDispatchID = strings.TrimSpace(c.SourceDispatchID)
	c.SourceAuthoritySnapshotID = strings.TrimSpace(c.SourceAuthoritySnapshotID)
	c.SourceOperationID = strings.TrimSpace(c.SourceOperationID)
	c.SourceOperationHash = strings.TrimSpace(c.SourceOperationHash)
	c.SourceEvidenceFactID = common.EvidenceRecordID(strings.TrimSpace(string(c.SourceEvidenceFactID)))
	c.SourceEvidenceFactHash = strings.TrimSpace(c.SourceEvidenceFactHash)
	c.SourceReceiptExecutionKey = strings.TrimSpace(c.SourceReceiptExecutionKey)
	c.SourceReceiptHash = strings.TrimSpace(c.SourceReceiptHash)
	return c
}

func (c RuntimeDispatchRepairCommitment) Clone() RuntimeDispatchRepairCommitment {
	c = c.Normalized()
	c.Closure = c.Closure.Clone()
	return c
}

func (c RuntimeDispatchRepairCommitment) Validate() error {
	c = c.Normalized()
	if c.Purpose != RuntimeDispatchRepairPurposeConfirmedValidation {
		return fmt.Errorf("runtime dispatch repair commitment has invalid purpose %q", c.Purpose)
	}
	if c.RepairOwnerModuleID == "" || c.OriginDispatchID == "" || c.FocusID == "" || c.ConfirmationIdentityHash == "" || c.ConfirmationHash == "" ||
		c.SourceDispatchID == "" || c.SourceAuthoritySnapshotID == "" || c.SourceOperationID == "" ||
		c.SourceOperationHash == "" || c.SourceEvidenceFactID == "" || c.SourceEvidenceFactHash == "" ||
		c.SourceReceiptExecutionKey == "" || c.SourceReceiptFence == 0 || c.SourceReceiptHash == "" {
		return fmt.Errorf("runtime dispatch repair commitment requires complete origin, confirmation, evidence and receipt lineage")
	}
	if err := c.Closure.Validate(); err != nil {
		return fmt.Errorf("runtime dispatch repair commitment closure: %w", err)
	}
	if !c.Closure.RequireValidationAfterStateChange ||
		!sameStrings(c.Closure.RequiredEvidenceKinds, []string{EvidenceKindStateChange, EvidenceKindValidation}) {
		return fmt.Errorf("runtime dispatch repair commitment requires state-change followed by fresh validation")
	}
	return nil
}

func StableRuntimeDispatchRepairCommitmentHash(value RuntimeDispatchRepairCommitment) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-dispatch-repair-commitment/v1")
	writeHashString(h, string(value.Purpose))
	writeHashString(h, value.RepairOwnerModuleID)
	writeHashString(h, value.OriginDispatchID)
	writeHashString(h, value.FocusID)
	writeHashString(h, value.Closure.ID)
	writeHashString(h, StableRuntimeRepairClosureID(value.Closure))
	writeHashString(h, value.ConfirmationIdentityHash)
	writeHashString(h, value.ConfirmationHash)
	writeHashString(h, value.SourceDispatchID)
	writeHashString(h, value.SourceAuthoritySnapshotID)
	writeHashString(h, value.SourceOperationID)
	writeHashString(h, value.SourceOperationHash)
	writeHashString(h, string(value.SourceEvidenceFactID))
	writeHashString(h, value.SourceEvidenceFactHash)
	writeHashString(h, value.SourceReceiptExecutionKey)
	writeHashString(h, fmt.Sprintf("%d", value.SourceReceiptFence))
	writeHashString(h, value.SourceReceiptHash)
	return hex.EncodeToString(h.Sum(nil))
}

func SameRuntimeDispatchRepairCommitment(left, right *RuntimeDispatchRepairCommitment) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.Purpose == r.Purpose && l.RepairOwnerModuleID == r.RepairOwnerModuleID &&
		l.OriginDispatchID == r.OriginDispatchID && l.FocusID == r.FocusID &&
		SameRuntimeRepairClosure(&l.Closure, &r.Closure) &&
		l.ConfirmationIdentityHash == r.ConfirmationIdentityHash && l.ConfirmationHash == r.ConfirmationHash &&
		l.SourceDispatchID == r.SourceDispatchID && l.SourceAuthoritySnapshotID == r.SourceAuthoritySnapshotID &&
		l.SourceOperationID == r.SourceOperationID && l.SourceOperationHash == r.SourceOperationHash &&
		l.SourceEvidenceFactID == r.SourceEvidenceFactID && l.SourceEvidenceFactHash == r.SourceEvidenceFactHash &&
		l.SourceReceiptExecutionKey == r.SourceReceiptExecutionKey && l.SourceReceiptFence == r.SourceReceiptFence &&
		l.SourceReceiptHash == r.SourceReceiptHash
}

// ValidateRuntimeDispatchRepairCommitmentProjection validates the optional
// commitment/hash pair without inferring whether a dispatch ought to carry it.
// That mandatory-purpose decision is proved by accepted admission lineage.
func ValidateRuntimeDispatchRepairCommitmentProjection(hash string, commitment *RuntimeDispatchRepairCommitment) error {
	canonicalHash := strings.TrimSpace(hash)
	if hash != canonicalHash {
		return fmt.Errorf("runtime dispatch repair commitment hash is not raw-canonical")
	}
	if commitment == nil {
		if canonicalHash != "" {
			return fmt.Errorf("runtime dispatch repair commitment hash has no projection")
		}
		return nil
	}
	value := commitment.Normalized()
	if !reflect.DeepEqual(*commitment, value) {
		return fmt.Errorf("runtime dispatch repair commitment projection is not raw-canonical")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if canonicalHash == "" || canonicalHash != StableRuntimeDispatchRepairCommitmentHash(value) {
		return fmt.Errorf("runtime dispatch repair commitment hash mismatch")
	}
	return nil
}
