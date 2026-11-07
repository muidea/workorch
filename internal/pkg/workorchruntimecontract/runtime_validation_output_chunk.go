package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// The storage envelope and one EventHub response are separate bounds. A reader
// must verify the complete saved stream before exposing it as complete content.
const RuntimeValidationOutputChunkMaxBytes = 64 * 1024
const RuntimeValidationOutputSavedMaxBytes = 16 * 1024 * 1024

func ValidateRuntimeValidationOutputChunk(identity RuntimeValidationOutputIdentity, savedSHA256 string, offset int64, chunkSHA256 string, raw []byte, requestedOffset int64) error {
	if identity.Validate() != nil || identity.Storage == nil || identity.Storage.Reference == "" ||
		identity.SavedBytes > RuntimeValidationOutputSavedMaxBytes || offset != requestedOffset || offset < 0 || offset > identity.SavedBytes ||
		int64(len(raw)) != min(int64(RuntimeValidationOutputChunkMaxBytes), identity.SavedBytes-offset) {
		return fmt.Errorf("validation output chunk identity or bounds mismatch")
	}
	savedHash, err := hex.DecodeString(savedSHA256)
	if err != nil || len(savedHash) != sha256.Size || hex.EncodeToString(savedHash) != savedSHA256 || identity.Complete && identity.SHA256 != savedSHA256 {
		return fmt.Errorf("validation output saved stream hash mismatch")
	}
	digest := sha256.Sum256(raw)
	if chunkSHA256 != hex.EncodeToString(digest[:]) || offset == 0 && int64(len(raw)) == identity.SavedBytes && savedSHA256 != chunkSHA256 {
		return fmt.Errorf("validation output chunk bytes mismatch")
	}
	return nil
}
