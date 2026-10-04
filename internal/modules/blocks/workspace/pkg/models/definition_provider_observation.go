package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Provider 观察是受限诊断材料；hash 只覆盖实际观察 bytes，不代表未读取的响应。
type DefinitionProviderObservation struct {
	Content       []byte
	ObservedBytes uint64
	SHA256        string
	Observed      bool
	Complete      bool
	Saved         bool
	Reason        string
}

func (o DefinitionProviderObservation) Clone() DefinitionProviderObservation {
	if o.Content != nil {
		o.Content = append([]byte{}, o.Content...)
	}
	return o
}

func (o DefinitionProviderObservation) Validate() error {
	if !o.Observed {
		if o.Content != nil || o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == "" {
			return fmt.Errorf("unobserved provider diagnostic has conflicting facts")
		}
		return nil
	}
	hash, err := hex.DecodeString(o.SHA256)
	if err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != o.SHA256 {
		return fmt.Errorf("provider diagnostic hash invalid")
	}
	if !o.Complete && o.Reason == "" {
		return fmt.Errorf("partial provider diagnostic requires reason")
	}
	if o.Saved {
		if o.Content == nil || uint64(len(o.Content)) != o.ObservedBytes || len(o.Content) > DefinitionAuthoringDiagnosticDocumentLimit {
			return fmt.Errorf("provider diagnostic byte count invalid")
		}
		sum := sha256.Sum256(o.Content)
		if hex.EncodeToString(sum[:]) != o.SHA256 {
			return fmt.Errorf("provider diagnostic content hash mismatch")
		}
	} else if o.Content != nil || o.ObservedBytes <= DefinitionAuthoringDiagnosticDocumentLimit || o.Reason != "observation_storage_limit" {
		return fmt.Errorf("provider diagnostic missing storage reason")
	}
	return nil
}
