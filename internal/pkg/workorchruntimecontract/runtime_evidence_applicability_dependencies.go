package workorchruntimecontract

import "fmt"

const RuntimeEvidenceApplicabilityDependenciesV1 = "evidence-applicability-dependencies/v1"
const RuntimeEvidenceApplicabilityDependenciesV2 = "evidence-applicability-dependencies/v2"
const RuntimeHistoricalEffectProofV1 = "historical-effect-proof/v1"
const RuntimeFileHistoricalEffectProofV1 = "file/mutation-historical-effect/v1"

// RuntimeEvidenceApplicabilityDependencies is the owner-observed dependency
// snapshot against which SM-2 must test applicability. It is not an assertion
// that the resources are still current. Historical OwnerRevisionSet reads stay
// intact on the same receipt/fact; neither collection can stand in for the other.
type RuntimeEvidenceApplicabilityDependencies struct {
	ProtocolRef      string
	OwnerRevisionSet RuntimeOwnerRevisionSet
	// Explicit receiving-owner declaration. Empty fields preserve the v1 wire;
	// old receipts are never upgraded by normalization or by their operation kind.
	PurposeRef       string `json:",omitempty"`
	OwnerModuleID    string `json:",omitempty"`
	OperationKind    string `json:",omitempty"`
	OwnerContractRef string `json:",omitempty"`
}

func CloneRuntimeEvidenceApplicabilityDependencies(value *RuntimeEvidenceApplicabilityDependencies) *RuntimeEvidenceApplicabilityDependencies {
	if value == nil {
		return nil
	}
	ret := *value
	ret.OwnerRevisionSet = *CloneRuntimeOwnerRevisionSet(&value.OwnerRevisionSet)
	return &ret
}

// ValidateRuntimeEvidenceDependencyPair does not normalize observations, query
// owners, infer absent observations, or choose a stale/reopen transition.
func ValidateRuntimeEvidenceDependencyPair(reads *RuntimeOwnerRevisionSet, applicability *RuntimeEvidenceApplicabilityDependencies, definition string) error {
	if err := validateRuntimeOwnerRevisionSetRaw(reads); err != nil {
		return err
	}
	if reads != nil && definition != "" && reads.DefinitionRevision != definition {
		return fmt.Errorf("historical dependency definition mismatch")
	}
	if applicability == nil {
		return nil
	}
	if reads == nil {
		return fmt.Errorf("applicability dependencies require exact protocol and original observed reads")
	}
	switch applicability.ProtocolRef {
	case RuntimeEvidenceApplicabilityDependenciesV1:
		if applicability.PurposeRef != "" || applicability.OwnerModuleID != "" || applicability.OperationKind != "" || applicability.OwnerContractRef != "" {
			return fmt.Errorf("legacy applicability cannot declare a proof purpose")
		}
	case RuntimeEvidenceApplicabilityDependenciesV2:
		descriptor, found := RuntimeOperationDescriptorForKind(applicability.OperationKind)
		if !found || descriptor.Kind != applicability.OperationKind || descriptor.OwnerModuleID != applicability.OwnerModuleID ||
			applicability.PurposeRef != RuntimeHistoricalEffectProofV1 || applicability.OwnerContractRef == "" ||
			descriptor.HistoricalEffectProofContractRef != applicability.OwnerContractRef {
			return fmt.Errorf("historical effect requires exact published owner operation purpose")
		}
	default:
		return fmt.Errorf("unsupported applicability dependencies protocol")
	}
	set := &applicability.OwnerRevisionSet
	if err := validateRuntimeOwnerRevisionSetRaw(set); err != nil {
		return err
	}
	if len(set.Entries) == 0 || set.DefinitionRevision != reads.DefinitionRevision || (definition != "" && set.DefinitionRevision != definition) {
		return fmt.Errorf("applicability dependencies require nonempty exact-definition owner observations")
	}
	return nil
}

// This is behavioral proof eligibility, never resource currentness. Callers
// must still reverse-prove the original receipt, binding and execution lineage.
func RuntimeEvidenceIsHistoricalEffect(fact EvidenceFact) bool {
	value := fact.ApplicabilityDependencies
	return fact.Kind == EvidenceKindStateChange && value != nil && value.ProtocolRef == RuntimeEvidenceApplicabilityDependenciesV2 &&
		value.PurposeRef == RuntimeHistoricalEffectProofV1 && ValidateRuntimeEvidenceDependencyPair(fact.OwnerRevisionSet, value, "") == nil
}

func ValidateRuntimeEvidencePurposeForOperation(value *RuntimeEvidenceApplicabilityDependencies, kind, evidenceKind string) error {
	if value != nil && value.ProtocolRef == RuntimeEvidenceApplicabilityDependenciesV2 &&
		(value.OperationKind != kind || evidenceKind != EvidenceKindStateChange) {
		return fmt.Errorf("historical effect declaration differs from sealed operation/evidence purpose")
	}
	return nil
}

// A sealed ordinary binding requires either same-binding validation or an
// explicitly compiled original cross-CWI acceptance declaration. Unmapped
// state-change-only/acquisition bindings retain their current-resource contract.
func RuntimeHistoricalEffectProofContractForBindings(bindings []CoverageEvidenceBinding, operation RuntimeOperation) string {
	descriptor, found := RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.HistoricalEffectProofContractRef == "" || len(operation.BindingRefs) == 0 {
		return ""
	}
	seen := map[string]bool{}
	for _, ref := range operation.BindingRefs {
		if ref == "" || seen[ref] {
			return ""
		}
		seen[ref] = true
		matches := 0
		for _, binding := range bindings {
			if binding.ID != ref {
				continue
			}
			if binding.Validate() != nil {
				return ""
			}
			proof, effect := CoverageEvidenceProofForKind(binding, EvidenceKindStateChange)
			_, acceptance := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
			acceptance = acceptance || binding.HistoricalAcceptance != nil
			if effect && acceptance && containsString(proof.AllowedOperationKinds, operation.Kind) {
				matches++
			}
		}
		if matches != 1 {
			return ""
		}
	}
	return descriptor.HistoricalEffectProofContractRef
}

// A repair/work dispatch has an independent causation/completion contract.
// Merely retaining its original ordinary binding must not upgrade that
// intermediate receipt to a Definition historical-effect acceptance proof.
func RuntimeHistoricalEffectProofContractForDispatch(dispatch RuntimeDispatchRecord, operation RuntimeOperation) string {
	if dispatch.Transaction.State != ConvergenceStateNeedStateChange || dispatch.RepairCommitment != nil ||
		dispatch.RepairCommitmentHash != "" || dispatch.ImplementationWorkCommitment != nil {
		return ""
	}
	return RuntimeHistoricalEffectProofContractForBindings(dispatch.CoverageBindings, operation)
}

// A mapped historical source cannot silently downgrade to current-resource
// v1 (or omit its observed dependency pair). The original mapping remains in
// the sealed binding/manifest; this gate checks purpose only, never credit.
func ValidateRuntimeHistoricalEffectReceiptPurpose(dispatch RuntimeDispatchRecord, operation RuntimeOperation, value *RuntimeEvidenceApplicabilityDependencies) error {
	expected := RuntimeHistoricalEffectProofContractForDispatch(dispatch, operation)
	mapped := false
	for _, binding := range dispatch.CoverageBindings {
		if containsString(operation.BindingRefs, binding.ID) && binding.HistoricalAcceptance != nil {
			mapped = true
		}
	}
	if mapped && (expected == "" || value == nil || value.ProtocolRef != RuntimeEvidenceApplicabilityDependenciesV2 || value.PurposeRef != RuntimeHistoricalEffectProofV1) {
		return fmt.Errorf("mapped historical source requires original owner-declared historical receipt purpose")
	}
	if value != nil && value.ProtocolRef == RuntimeEvidenceApplicabilityDependenciesV2 && value.OwnerContractRef != expected {
		return fmt.Errorf("historical effect has no original sealed ordinary acceptance source")
	}
	return nil
}

func ValidateRuntimeEvidencePurposeKind(value *RuntimeEvidenceApplicabilityDependencies, kind string) error {
	if value == nil {
		return nil
	}
	return ValidateRuntimeEvidencePurposeForOperation(value, value.OperationKind, kind)
}

func RuntimeEvidenceHasProofApplicabilityAtRevision(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, revision string) bool {
	return RuntimeEvidenceIsHistoricalEffect(fact) || RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, observations, revision)
}

func RuntimeEvidenceHasFreshProofApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, revision, generation string) bool {
	return RuntimeEvidenceIsHistoricalEffect(fact) || RuntimeEvidenceHasFreshApplicability(fact, observations, revision, generation)
}

// Input must contain only exact-binding ordinary proof candidates already
// validated for source and fresh sampling. A historical effect never supplies
// the independently observed current acceptance resources of that binding.
func RuntimeHistoricalEffectsHaveCurrentAcceptanceProof(selected []EvidenceFact) bool {
	historical, current := false, false
	for _, fact := range selected {
		historical = historical || RuntimeEvidenceIsHistoricalEffect(fact)
		value := fact.ApplicabilityDependencies
		current = current || (fact.Kind == EvidenceKindValidation && fact.Validation != nil && value != nil &&
			value.ProtocolRef == RuntimeEvidenceApplicabilityDependenciesV1 && len(value.OwnerRevisionSet.Entries) > 0 &&
			ValidateRuntimeEvidenceDependencyPair(fact.OwnerRevisionSet, value, "") == nil)
	}
	return !historical || current
}
