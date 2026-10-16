package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeHistoricalAcceptanceSource is an immutable Definition declaration,
// not a receipt, completion state or cross-CWI execution grant. Its consumer
// must independently prove both original endpoints and all current checks.
type RuntimeHistoricalAcceptanceSource struct {
	ContractRef                  string
	InventoryHash                string
	HistoricalItemRef            string
	AcceptanceItemRef            string
	HistoricalCoverageWorkItemID common.CoverageWorkItemID
	AcceptanceCoverageWorkItemID common.CoverageWorkItemID
	HistoricalTargetTextHash     string
	AcceptanceTargetTextHash     string
	AcceptanceChecksHash         string
}

func cloneRuntimeHistoricalAcceptanceSource(value *RuntimeHistoricalAcceptanceSource) *RuntimeHistoricalAcceptanceSource {
	if value == nil {
		return nil
	}
	ret := *value
	return &ret
}

func sameRuntimeHistoricalAcceptanceSource(left, right *RuntimeHistoricalAcceptanceSource) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func (value RuntimeHistoricalAcceptanceSource) Validate(coverageID common.CoverageWorkItemID) error {
	if (value.ContractRef != RuntimeCrossCWIEvidencePurposeV1 && value.ContractRef != RuntimeProfileDeliverableEvidencePurposeV2) || value.HistoricalCoverageWorkItemID != coverageID ||
		value.HistoricalCoverageWorkItemID == value.AcceptanceCoverageWorkItemID || value.HistoricalItemRef == value.AcceptanceItemRef ||
		!runtimeSourceIdentity(value.InventoryHash, 256) || !runtimeSourceIdentity(value.HistoricalItemRef, 1024) ||
		!runtimeSourceIdentity(value.AcceptanceItemRef, 1024) || !runtimeSourceIdentity(string(value.HistoricalCoverageWorkItemID), 1024) ||
		!runtimeSourceIdentity(string(value.AcceptanceCoverageWorkItemID), 1024) {
		return fmt.Errorf("historical acceptance requires exact distinct confirmed original endpoints")
	}
	for _, digest := range []string{value.HistoricalTargetTextHash, value.AcceptanceTargetTextHash, value.AcceptanceChecksHash} {
		bytes, err := hex.DecodeString(digest)
		if err != nil || len(bytes) != sha256.Size || hex.EncodeToString(bytes) != digest {
			return fmt.Errorf("historical acceptance requires complete original text/check identity")
		}
	}
	return nil
}

func runtimeHistoricalAcceptanceTextHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func runtimeHistoricalAcceptanceChecksHash(checks []RuntimeSourceObligationCheck) string {
	h := newRuntimeHashWriter()
	writeRuntimeSourceObligationChecksHash(h, checks)
	return hex.EncodeToString(h.Sum(nil))
}

func writeRuntimeHistoricalAcceptanceSourceHash(h hashStringWriter, value *RuntimeHistoricalAcceptanceSource) {
	if value == nil {
		return // Preserve the exact legacy empty-extension hash.
	}
	writeHashString(h, "historical-acceptance-source")
	writeHashString(h, value.ContractRef)
	writeHashString(h, value.InventoryHash)
	writeHashString(h, value.HistoricalItemRef)
	writeHashString(h, value.AcceptanceItemRef)
	writeHashString(h, string(value.HistoricalCoverageWorkItemID))
	writeHashString(h, string(value.AcceptanceCoverageWorkItemID))
	writeHashString(h, value.HistoricalTargetTextHash)
	writeHashString(h, value.AcceptanceTargetTextHash)
	writeHashString(h, value.AcceptanceChecksHash)
}
