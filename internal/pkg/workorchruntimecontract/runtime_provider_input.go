package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RuntimeProviderInputRef binds restricted opaque request bytes. The aggregate
// stores only this integrity index; it never embeds prompts or credentials.
type RuntimeProviderInputRef struct {
	Hash  string
	Bytes int
}

func RuntimeProviderInputHash(raw []byte) string {
	h := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(h[:])
}
func (r *RuntimeProviderInputRef) Clone() *RuntimeProviderInputRef {
	if r == nil {
		return nil
	}
	v := *r
	return &v
}
func (r *RuntimeProviderInputRef) Validate() error {
	if r == nil {
		return nil
	}
	if r.Bytes <= 0 || len(r.Hash) != 71 || r.Hash[:7] != "sha256:" {
		return fmt.Errorf("provider input requires exact bytes/SHA256")
	}
	_, err := hex.DecodeString(r.Hash[7:])
	return err
}
func (r *RuntimeProviderInputRef) ValidateContent(raw []byte) error {
	if r == nil || r.Bytes != len(raw) || r.Hash != RuntimeProviderInputHash(raw) {
		return fmt.Errorf("provider request input missing or differs from sealed bytes/SHA256")
	}
	return r.Validate()
}
