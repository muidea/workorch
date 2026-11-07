package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestRuntimeValidationOutputChunkBoundsHashesAndOffsets(t *testing.T) {
	raw := bytes.Repeat([]byte("original\x00\xff\r\n"), 11000)
	hash := func(body []byte) string { v := sha256.Sum256(body); return hex.EncodeToString(v[:]) }
	saved := hash(raw)
	identity := RuntimeValidationOutputIdentity{Bytes: int64(len(raw)), SavedBytes: int64(len(raw)), SHA256: saved, Complete: true, Storage: &RuntimeValidationOutputStorage{Reference: "owner-ref"}}
	for _, offset := range []int64{0, 65530, 65536, int64(len(raw) - 1), int64(len(raw))} {
		page := raw[offset:min(int64(len(raw)), offset+RuntimeValidationOutputChunkMaxBytes)]
		if err := ValidateRuntimeValidationOutputChunk(identity, saved, offset, hash(page), page, offset); err != nil {
			t.Fatal(err)
		}
		if ValidateRuntimeValidationOutputChunk(identity, saved, offset, hash(page), page, offset+1) == nil {
			t.Fatal("wrong offset accepted")
		}
		if len(page) > 0 && ValidateRuntimeValidationOutputChunk(identity, saved, offset, hash(page[:len(page)-1]), page[:len(page)-1], offset) == nil {
			t.Fatal("short page accepted")
		}
		if ValidateRuntimeValidationOutputChunk(identity, saved, offset, saved, page, offset) == nil && hash(page) != saved {
			t.Fatal("wrong page digest accepted")
		}
	}
	bad := identity.Clone()
	bad.SavedBytes = RuntimeValidationOutputSavedMaxBytes + 1
	bad.Bytes = bad.SavedBytes
	if ValidateRuntimeValidationOutputChunk(bad, saved, 0, hash(raw[:65536]), raw[:65536], 0) == nil {
		t.Fatal("saved storage bound bypassed")
	}
	partial := identity.Clone()
	partial.Complete = false
	partial.Bytes++
	partial.OmittedReason = "explicit storage capacity"
	partial.SHA256 = hash([]byte("full stream"))
	if err := ValidateRuntimeValidationOutputChunk(partial, saved, 0, hash(raw[:65536]), raw[:65536], 0); err != nil {
		t.Fatal("explicit partial lost", err)
	}
}
