package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// 文档观测只描述本次同步调用实际收到的材料，不提供重试或执行权限。
const ProviderDocumentObservationLimit = 256 * 1024

type ProviderObservedContent struct {
	Content       []byte `json:"-"`
	ObservedBytes uint64
	SHA256        string
	Complete      bool
	Observed      bool
	Saved         bool
	Reason        string
}

func NewProviderObservedContent(content []byte, complete bool, reason string) ProviderObservedContent {
	hash := sha256.Sum256(content)
	observation := ProviderObservedContent{Observed: true, ObservedBytes: uint64(len(content)), SHA256: hex.EncodeToString(hash[:]), Complete: complete, Reason: reason}
	if len(content) <= ProviderDocumentObservationLimit {
		observation.Content = append([]byte{}, content...)
		observation.Saved = true
	} else {
		observation.Reason = "observation_storage_limit"
	}
	return observation
}

func (o ProviderObservedContent) Clone() ProviderObservedContent {
	if o.Content != nil {
		o.Content = append([]byte{}, o.Content...)
	}
	return o
}

func (o ProviderObservedContent) Validate() error {
	if !o.Observed {
		if o.Content != nil || o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == "" {
			return fmt.Errorf("unobserved provider content has conflicting facts")
		}
		return nil
	}
	decoded, err := hex.DecodeString(o.SHA256)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != o.SHA256 {
		return fmt.Errorf("provider observation hash invalid")
	}
	if !o.Complete && o.Reason == "" {
		return fmt.Errorf("partial provider observation requires reason")
	}
	if o.Saved {
		if o.Content == nil || o.ObservedBytes != uint64(len(o.Content)) || len(o.Content) > ProviderDocumentObservationLimit {
			return fmt.Errorf("provider observation bytes invalid")
		}
		hash := sha256.Sum256(o.Content)
		if hex.EncodeToString(hash[:]) != o.SHA256 {
			return fmt.Errorf("provider observation content hash mismatch")
		}
	} else if o.Content != nil || o.ObservedBytes <= ProviderDocumentObservationLimit || o.Reason != "observation_storage_limit" {
		return fmt.Errorf("provider observation not-saved fact invalid")
	}
	return nil
}
