package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// RuntimeVerificationPolicy is an optional, versioned minimum for SM-2 closure.
// Levels follow the evidence taxonomy: 0 Claim, 1 Inspection, 2 Static
// Validation, 3 Executed Validation, 4 Behavioral Validation, and
// 5 Independent Validation. Grade 2-4 requires an exact sealed operation,
// committed capability-owner receipt and typed native observation; grade 5
// requires independent environment/external/human proof not yet supported.
// MinIndependentSources is separate from evidence grade.
const RuntimeVerificationPolicyVersion = "runtime-verification-policy/v1"

type RuntimeVerificationPolicy struct {
	Version               string
	MinimumEvidenceLevel  int
	MinIndependentSources int
	RequireBoundContext   bool
}

func (p RuntimeVerificationPolicy) Normalized() RuntimeVerificationPolicy {
	p.Version = strings.TrimSpace(p.Version)
	return p
}

func (p RuntimeVerificationPolicy) Validate() error {
	p = p.Normalized()
	if p.Version != RuntimeVerificationPolicyVersion || p.MinimumEvidenceLevel < 0 || p.MinimumEvidenceLevel > 5 ||
		p.MinIndependentSources < 0 || p.MinIndependentSources > 6 {
		return fmt.Errorf("verification policy requires version v1, level 0-5 and 0-6 independent sources")
	}
	return nil
}

func cloneRuntimeVerificationPolicy(p *RuntimeVerificationPolicy) *RuntimeVerificationPolicy {
	if p == nil {
		return nil
	}
	value := *p
	return &value
}

func SameRuntimeVerificationPolicy(a, b *RuntimeVerificationPolicy) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Normalized() == b.Normalized()
}

func StableRuntimeVerificationPolicyHash(p RuntimeVerificationPolicy) string {
	p = p.Normalized()
	h := sha256.New()
	writeHashString(h, p.Version)
	writeHashString(h, fmt.Sprintf("%d", p.MinimumEvidenceLevel))
	writeHashString(h, fmt.Sprintf("%d", p.MinIndependentSources))
	writeHashString(h, fmt.Sprintf("%t", p.RequireBoundContext))
	return "runtime-verification-policy-" + hex.EncodeToString(h.Sum(nil))
}
