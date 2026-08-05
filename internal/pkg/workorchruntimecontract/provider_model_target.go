package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// BuiltinAIProxyProviderID is the reserved WorkOrch provider identity for the
// built-in ai-proxy endpoint. owned_by from ai-proxy catalog is display-only
// and never becomes a WorkOrch provider identity.
const BuiltinAIProxyProviderID = "ai-proxy"

// ProviderModelTarget is the durable provider/model identity used by activation,
// Runtime ceiling sealing, completion/embedding, and request audit.
// It must not carry API keys, base URLs, or dynamic client state.
type ProviderModelTarget struct {
	ProviderID string `json:"providerId" toml:"providerID"`
	ModelID    string `json:"modelId" toml:"modelID"`
}

func (t ProviderModelTarget) Normalized() ProviderModelTarget {
	t.ProviderID = strings.TrimSpace(t.ProviderID)
	t.ModelID = strings.TrimSpace(t.ModelID)
	return t
}

func (t ProviderModelTarget) Validate() error {
	t = t.Normalized()
	if t.ProviderID == "" {
		return fmt.Errorf("provider model target requires providerId")
	}
	if t.ModelID == "" {
		return fmt.Errorf("provider model target requires modelId")
	}
	return nil
}

func (t ProviderModelTarget) Same(other ProviderModelTarget) bool {
	left := t.Normalized()
	right := other.Normalized()
	return left.ProviderID == right.ProviderID && left.ModelID == right.ModelID
}

func (t ProviderModelTarget) Clone() ProviderModelTarget {
	return t.Normalized()
}

// CloneProviderModelTarget returns an independent normalized pointer while
// preserving nil. It is the canonical pointer-copy helper for contracts that
// carry an optional provider/model target.
func CloneProviderModelTarget(target *ProviderModelTarget) *ProviderModelTarget {
	if target == nil {
		return nil
	}
	cloned := target.Clone()
	return &cloned
}

func SameProviderModelTarget(left ProviderModelTarget, right ProviderModelTarget) bool {
	return left.Same(right)
}
