package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
)

// RuntimeValidationInputLocation is a bounded compiler-location fact, never a
// writable target. Code and Symbol are owner-normalized vocabulary, not output.
type RuntimeValidationInputLocation struct {
	TargetRef string `json:"targetRef"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Code      string `json:"code"`
	Symbol    string `json:"symbol,omitempty"`
}

// RuntimeValidationInputRepair describes one criterion-preserving input delta
// observed by the validation owner. It is not write authority: a sealed Profile
// route, resource-owner confirmation and exact pre/postimage feasibility proof
// are all required before SM-3 can seal an operation. The original check and
// its conjunctive supplement remain mandatory after the change.
type RuntimeValidationInputRepair struct {
	ContractRef                string                                   `json:"contractRef"`
	RequiredCheckRef           string                                   `json:"requiredCheckRef"`
	ObservationProtocolRef     string                                   `json:"observationProtocolRef"`
	OriginalNativeCheckRef     string                                   `json:"originalNativeCheckRef"`
	SupplementalNativeCheckRef string                                   `json:"supplementalNativeCheckRef"`
	TargetRef                  string                                   `json:"targetRef"`
	BeforeHash                 string                                   `json:"beforeHash"`
	AfterHash                  string                                   `json:"afterHash"`
	Corrections                []RuntimeValidationInputSymbolCorrection `json:"corrections"`
}

type RuntimeValidationInputSymbolCorrection struct {
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Before string `json:"before"`
	After  string `json:"after"`
}

const RuntimeValidationInputDetailLimit = 8

func (v *RuntimeValidationInputRepair) Clone() *RuntimeValidationInputRepair {
	if v == nil {
		return nil
	}
	out := *v
	out.Corrections = slices.Clone(v.Corrections)
	return &out
}

func (v RuntimeValidationInputRepair) Validate() error {
	for _, value := range []string{v.ContractRef, v.RequiredCheckRef, v.ObservationProtocolRef, v.OriginalNativeCheckRef, v.SupplementalNativeCheckRef, v.TargetRef} {
		if value == "" || value != strings.TrimSpace(value) || len(value) > 512 || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return fmt.Errorf("validation input repair requires bounded canonical identities")
		}
	}
	if path.IsAbs(v.TargetRef) || path.Clean(v.TargetRef) != v.TargetRef || v.TargetRef == "." || v.TargetRef == ".." || strings.HasPrefix(v.TargetRef, "../") || strings.ContainsAny(v.TargetRef, "\\:") {
		return fmt.Errorf("validation input repair requires one canonical relative input")
	}
	if v.OriginalNativeCheckRef == v.SupplementalNativeCheckRef || v.BeforeHash == v.AfterHash {
		return fmt.Errorf("validation input repair requires distinct checks and content revisions")
	}
	for _, digest := range []string{v.BeforeHash, v.AfterHash} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) {
			return fmt.Errorf("validation input repair requires canonical sha256 pre/postimages")
		}
	}
	if len(v.Corrections) == 0 || len(v.Corrections) > RuntimeValidationInputDetailLimit {
		return fmt.Errorf("validation input repair requires bounded exact corrections")
	}
	previousLine, previousColumn := 0, 0
	for _, correction := range v.Corrections {
		if correction.Line < 1 || correction.Column < 1 || correction.Line < previousLine ||
			(correction.Line == previousLine && correction.Column <= previousColumn) ||
			!runtimeValidationInputSymbol(correction.Before) || !runtimeValidationInputSymbol(correction.After) || correction.Before == correction.After {
			return fmt.Errorf("validation input repair has invalid or duplicate symbol correction")
		}
		previousLine, previousColumn = correction.Line, correction.Column
	}
	return nil
}

func (v *RuntimeValidationInputRepair) MatchesBinding(binding CoverageEvidenceBinding) bool {
	if v == nil || v.Validate() != nil {
		return false
	}
	for _, proof := range binding.Normalized().Proofs {
		for _, check := range proof.ValidationCheckAttestations {
			if check.ObservationProtocolRef == v.ObservationProtocolRef && check.RequiredCheckRef == v.RequiredCheckRef && check.NativeCheckRef == v.OriginalNativeCheckRef &&
				check.SupplementalNativeCheckRef == v.SupplementalNativeCheckRef {
				return true
			}
		}
	}
	return false
}

func (v *RuntimeValidationInputRepair) MatchesReplayProof(proof *RuntimeOperationReplayProof) bool {
	return v != nil && v.Validate() == nil && proof != nil && proof.Validate() == nil &&
		len(proof.TargetRefs) == 1 && proof.TargetRefs[0] == v.TargetRef && proof.BeforeHash == v.BeforeHash && proof.AfterHash == v.AfterHash
}

func SameRuntimeValidationInputRepair(left, right *RuntimeValidationInputRepair) bool {
	return reflect.DeepEqual(left, right)
}

func runtimeValidationInputSymbol(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if r != '_' && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validateRuntimeValidationInputDiagnostic(d ValidationFailureDiagnostic) error {
	if d.InputRepair != nil {
		if err := d.InputRepair.Validate(); err != nil {
			return err
		}
		if d.InputAddition != (RuntimeValidationInputAddition{}) || len(d.InputLocations) == 0 {
			return fmt.Errorf("validation input repair requires compiler locations and excludes input addition")
		}
		for _, correction := range d.InputRepair.Corrections {
			found := false
			for _, location := range d.InputLocations {
				if location.TargetRef == d.InputRepair.TargetRef && location.Line == correction.Line && location.Column == correction.Column && location.Symbol == correction.Before {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("validation input repair correction has no exact diagnostic location")
			}
		}
	}
	if len(d.InputLocations) > RuntimeValidationInputDetailLimit {
		return fmt.Errorf("validation input diagnostic exceeds detail limit")
	}
	seen := map[string]bool{}
	for _, location := range d.InputLocations {
		key := fmt.Sprintf("%s:%d:%d", location.TargetRef, location.Line, location.Column)
		if seen[key] || location.TargetRef == "" || len(location.TargetRef) > 512 || location.TargetRef != strings.TrimSpace(location.TargetRef) ||
			strings.ContainsAny(location.TargetRef, "\x00\r\n") || location.Line < 1 || location.Column < 1 ||
			!runtimeValidationInputSymbol(location.Code) || (location.Symbol != "" && !runtimeValidationInputSymbol(location.Symbol)) {
			return fmt.Errorf("validation input diagnostic has invalid location")
		}
		seen[key] = true
	}
	return nil
}

func writeRuntimeValidationInputDiagnosticHash(h interface{ Write([]byte) (int, error) }, d ValidationFailureDiagnostic) {
	if len(d.InputLocations) != 0 {
		writeHashString(h, "validation-input-locations/v1")
		for _, location := range d.InputLocations {
			writeHashString(h, location.TargetRef)
			writeHashString(h, fmt.Sprintf("%d:%d", location.Line, location.Column))
			writeHashString(h, location.Code)
			writeHashString(h, location.Symbol)
		}
	}
	writeRuntimeValidationInputRepairHash(h, d.InputRepair)
}

func writeRuntimeValidationInputRepairHash(h interface{ Write([]byte) (int, error) }, v *RuntimeValidationInputRepair) {
	if v == nil {
		return
	}
	writeHashString(h, "validation-input-repair/v1")
	for _, value := range []string{v.ContractRef, v.RequiredCheckRef, v.ObservationProtocolRef, v.OriginalNativeCheckRef, v.SupplementalNativeCheckRef, v.TargetRef, v.BeforeHash, v.AfterHash} {
		writeHashString(h, value)
	}
	for _, correction := range v.Corrections {
		writeHashString(h, fmt.Sprintf("%d:%d", correction.Line, correction.Column))
		writeHashString(h, correction.Before)
		writeHashString(h, correction.After)
	}
}

// AcceptsInputDiagnostic is the exact declaration/binding gate shared by all
// repair admission paths. An observed input delta cannot borrow an ordinary
// implementation route, even when a custom route reuses the same trigger.
func (r RuntimeValidationFailureRepairRoute) AcceptsInputDiagnostic(d ValidationFailureDiagnostic, binding CoverageEvidenceBinding) bool {
	if r.RequireInputAddition && !d.InputAddition.MatchesBinding(binding) {
		return false
	}
	if r.RequireInputRepair != (d.InputRepair != nil) {
		return false
	}
	return !r.RequireInputRepair || (d.InputRepair.MatchesBinding(binding) && len(d.TargetRefs) == 1 && d.TargetRefs[0] == d.InputRepair.TargetRef)
}
