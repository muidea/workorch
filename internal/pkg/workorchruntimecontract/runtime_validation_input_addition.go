package workorchruntimecontract

import (
	"fmt"
	"path"
	"strings"
)

// RuntimeValidationInputAddition is a bounded owner observation of one missing
// conjunctive validation input. It grants no target/effect authority: a Profile
// route, prospective resource-owner feasibility and SM-2 seal are still required.
// The original native check and immutable Definition remain required.
type RuntimeValidationInputAddition struct {
	RequiredCheckRef       string `json:"requiredCheckRef"`
	OriginalNativeCheckRef string `json:"originalNativeCheckRef"`
	NativeCheckRef         string `json:"nativeCheckRef"`
	TargetRef              string `json:"targetRef"`
}

func (v RuntimeValidationInputAddition) Normalized() RuntimeValidationInputAddition {
	v.RequiredCheckRef = strings.TrimSpace(v.RequiredCheckRef)
	v.OriginalNativeCheckRef = strings.TrimSpace(v.OriginalNativeCheckRef)
	v.NativeCheckRef = strings.TrimSpace(v.NativeCheckRef)
	v.TargetRef = strings.TrimSpace(v.TargetRef)
	return v
}

func (v RuntimeValidationInputAddition) Validate() error {
	v = v.Normalized()
	for _, ref := range []string{v.RequiredCheckRef, v.OriginalNativeCheckRef, v.NativeCheckRef, v.TargetRef} {
		if ref == "" || len(ref) > 512 || strings.IndexFunc(ref, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return fmt.Errorf("validation input addition requires bounded exact check and target identities")
		}
	}
	if v.NativeCheckRef == v.OriginalNativeCheckRef || path.IsAbs(v.TargetRef) || path.Clean(v.TargetRef) != v.TargetRef || v.TargetRef == "." || v.TargetRef == ".." || strings.HasPrefix(v.TargetRef, "../") || strings.ContainsAny(v.TargetRef, "\\:") {
		return fmt.Errorf("validation input addition must preserve the original check and identify one relative new input")
	}
	return nil
}

func (v RuntimeValidationInputAddition) MatchesBinding(binding CoverageEvidenceBinding) bool {
	if v.Validate() != nil {
		return false
	}
	for _, proof := range binding.Proofs {
		for _, check := range proof.ValidationCheckAttestations {
			if check.RequiredCheckRef == v.RequiredCheckRef && check.NativeCheckRef == v.OriginalNativeCheckRef && check.SupplementalNativeCheckRef == "" {
				return true
			}
		}
	}
	return false
}

func writeRuntimeValidationInputAdditionHash(h interface{ Write([]byte) (int, error) }, v RuntimeValidationInputAddition) {
	if v == (RuntimeValidationInputAddition{}) {
		return
	}
	v = v.Normalized()
	writeHashString(h, "validation-input-addition/v1")
	writeHashStrings(h, []string{v.RequiredCheckRef, v.OriginalNativeCheckRef, v.NativeCheckRef, v.TargetRef})
}
