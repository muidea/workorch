package workorchruntimecontract

import (
	"fmt"
	"slices"
)

const RuntimeCandidatePayloadViolationRequiredOperations RuntimeCandidatePayloadViolationKind = "required_operations_missing"
const RuntimeCandidateCorrectionWitnessRequiredOperations RuntimeCandidateCorrectionWitnessDimension = "required_operations"
const RuntimeCandidateCorrectionSupplyRequiredOperations RuntimeCandidateCorrectionObjective = "supply_required_operations"

// The decoder records a complete omitted-slot set and freezes every existing
// operation. A correction may supply only those slots, never rewrite retained
// choices. Document identity refers to the unmodified original provider bytes.
type RuntimeCandidateRequiredOperationsViolation struct {
	ManifestHash   string                              `json:"manifestHash"`
	DocumentSHA256 string                              `json:"documentSha256"`
	DocumentBytes  int                                 `json:"documentBytes"`
	MissingRefs    []string                            `json:"missingRefs"`
	Retained       []RuntimeCandidateRetainedOperation `json:"retained"`
}
type RuntimeCandidateRetainedOperation struct {
	RequiredOperationRef string `json:"requiredOperationRef,omitempty"`
	ValueHash            string `json:"valueHash"`
}

func (v *RuntimeCandidateRequiredOperationsViolation) Clone() *RuntimeCandidateRequiredOperationsViolation {
	if v == nil {
		return nil
	}
	out := *v
	out.MissingRefs = slices.Clone(v.MissingRefs)
	out.Retained = slices.Clone(v.Retained)
	return &out
}
func (v *RuntimeCandidateRequiredOperationsViolation) Validate() error {
	if v == nil || v.ManifestHash == "" || !cardinalityDigestValid(v.DocumentSHA256) || v.DocumentBytes <= 0 || len(v.MissingRefs) == 0 || len(v.MissingRefs)+len(v.Retained) > RuntimeCandidateOperationMaxCount {
		return fmt.Errorf("required operation omission lacks complete bounded source")
	}
	seen := map[string]bool{}
	for _, ref := range v.MissingRefs {
		if ref == "" || seen[ref] {
			return fmt.Errorf("invalid missing required operation set")
		}
		seen[ref] = true
	}
	for _, op := range v.Retained {
		if !cardinalityDigestValid(op.ValueHash) || (op.RequiredOperationRef != "" && seen[op.RequiredOperationRef]) {
			return fmt.Errorf("invalid retained operation")
		}
		if op.RequiredOperationRef != "" {
			seen[op.RequiredOperationRef] = true
		}
	}
	return nil
}
func (v *RuntimeCandidateRequiredOperationsViolation) MatchesDiagnostic(d *RuntimeCandidateFailureDiagnostic) bool {
	if v.Validate() != nil || d == nil || d.Path != "$.operations" || d.Actual != "missing_required_operation_"+v.MissingRefs[0] || d.ManifestHash != v.ManifestHash {
		return false
	}
	refs := slices.Clone(v.MissingRefs)
	for _, op := range v.Retained {
		if op.RequiredOperationRef != "" {
			refs = append(refs, op.RequiredOperationRef)
		}
	}
	return slices.Equal(normalizeStrings(refs), normalizeStrings(d.Required))
}

func ValidateRuntimeCandidateRequiredOperations(m RuntimeCandidateContractManifest, v *RuntimeCandidateRequiredOperationsViolation) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ManifestHash != m.Hash || len(v.MissingRefs)+len(v.Retained) > m.MaxOperationCount {
		return fmt.Errorf("required operation correction differs from manifest")
	}
	missing := []string{}
	retained := map[string]bool{}
	for _, op := range v.Retained {
		if op.RequiredOperationRef != "" {
			retained[op.RequiredOperationRef] = true
		}
	}
	for _, r := range m.RequiredOperationSequence {
		if !retained[r.RequirementID] {
			missing = append(missing, r.RequirementID)
		}
		delete(retained, r.RequirementID)
	}
	if len(retained) > 0 || !slices.Equal(missing, v.MissingRefs) {
		return fmt.Errorf("required operation omission is not exact manifest complement")
	}
	return nil
}
func requiredOperationValueHash(op RuntimeOperation) string {
	op = op.Normalized()
	op.ID = ""
	op.DependsOn = nil
	return cardinalityValueHash(op)
}
func ValidateRuntimeCandidateRequiredOperationsCorrection(m RuntimeCandidateContractManifest, v *RuntimeCandidateRequiredOperationsViolation, ops []RuntimeOperation) error {
	if err := ValidateRuntimeCandidateRequiredOperations(m, v); err != nil {
		return err
	}
	if len(ops) != len(v.Retained)+len(v.MissingRefs) {
		return fmt.Errorf("required operation correction changed complete set size")
	}
	// Required slot ordering is derived from the manifest, so freeze values
	// as a complete multiset rather than trusting provider array order.
	remaining := map[string]int{}
	for _, original := range v.Retained {
		remaining[original.RequiredOperationRef+"|"+original.ValueHash]++
	}
	added := map[string]bool{}
	for _, op := range ops {
		if slices.Contains(v.MissingRefs, op.RequiredOperationRef) {
			if added[op.RequiredOperationRef] {
				return fmt.Errorf("duplicate supplied slot")
			}
			added[op.RequiredOperationRef] = true
			continue
		}
		key := op.RequiredOperationRef + "|" + requiredOperationValueHash(op)
		if remaining[key] == 0 {
			return fmt.Errorf("required operation correction changed retained operation")
		}
		remaining[key]--
	}
	for _, count := range remaining {
		if count != 0 {
			return fmt.Errorf("required operation correction omitted retained operation")
		}
	}
	if len(added) != len(v.MissingRefs) {
		return fmt.Errorf("required operation correction is incomplete")
	}
	return nil
}
func writeHashRuntimeCandidateRequiredOperations(h hashStringWriter, v *RuntimeCandidateRequiredOperationsViolation) {
	if v != nil {
		writeHashString(h, "required_operations")
		writeHashString(h, cardinalityValueHash(v))
	}
}
