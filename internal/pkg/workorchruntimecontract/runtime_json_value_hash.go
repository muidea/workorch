package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
)

// Encode retains Marshal's default JSON representation but writes its private
// encoder buffer directly to the digest instead of making a full output copy.
// Only Encode's final newline is withheld; internal bytes are never rewritten.
func runtimeJSONValueHash(prefix string, value any) string {
	w := runtimeJSONHashWriter{digest: sha256.New()}
	if json.NewEncoder(&w).Encode(value) != nil || !w.hasTail || w.tail[0] != '\n' {
		return ""
	}
	return prefix + hex.EncodeToString(w.digest.Sum(nil))
}

// Keep one trailing byte across arbitrary Write boundaries, not just the
// current encoder's single-write implementation. The digest is call-local.
type runtimeJSONHashWriter struct {
	digest  hash.Hash
	tail    [1]byte
	hasTail bool
}

func (w *runtimeJSONHashWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.hasTail {
		_, _ = w.digest.Write(w.tail[:])
	}
	_, _ = w.digest.Write(p[:len(p)-1])
	w.tail[0], w.hasTail = p[len(p)-1], true
	return len(p), nil
}
