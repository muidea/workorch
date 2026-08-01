package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
)

// These are opaque-content helpers for the persisted C4 request-content
// carrier. They intentionally contain no continuation state or provider flow.
func runtimeCandidateOutputHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func runtimeCandidateOutputTail(value string, max int) string {
	if max <= 0 || len(value) == 0 {
		return ""
	}
	if len(value) > max {
		return value[len(value)-max:]
	}
	return value
}
