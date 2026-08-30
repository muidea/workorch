package workorchruntimecontract

import (
	"fmt"
	"strings"
)

const (
	runtimeCandidateCorrectionFieldAbsent  = "field_absent"
	runtimeCandidateCorrectionFieldPresent = "field_present"
	runtimeCandidateCorrectionValueAbsent  = "value_absent:"
)

// RuntimeCandidateCorrectionObjectivesEqual compares the normalized bounded
// objectives carried by a failure diagnostic and its detached correction.
// It is shared by both sides of the EventHub boundary so producer and consumer
// cannot silently adopt different lineage rules.
func RuntimeCandidateCorrectionObjectivesEqual(left, right []RuntimeCandidateCorrectionObjective) bool {
	left = (&RuntimeCandidateFailureDiagnostic{Objectives: append([]RuntimeCandidateCorrectionObjective(nil), left...)}).Normalized().Objectives
	right = (&RuntimeCandidateFailureDiagnostic{Objectives: append([]RuntimeCandidateCorrectionObjective(nil), right...)}).Normalized().Objectives
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// ValidateRuntimeCandidateCorrectionAgainstObservation validates the complete
// cross-owner binding for one SM-3 attempt-1 correction. The Orchestrator owns
// disposition and witness selection; this pure contract proves that the LLM
// owner received exactly the manifest-bound objectives/witness sealed for the
// observed rejection. Manifest-specific field authorization remains an
// additional consumer check because the observation intentionally contains no
// private candidate frontier.
func ValidateRuntimeCandidateCorrectionAgainstObservation(
	observation RuntimeCandidateCorrectionObservation,
	manifestHash string,
	objectives []RuntimeCandidateCorrectionObjective,
	witness RuntimeCandidateCorrectionWitness,
) error {
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return err
	}
	manifestHash = strings.TrimSpace(manifestHash)
	objectives = (&RuntimeCandidateFailureDiagnostic{Objectives: append([]RuntimeCandidateCorrectionObjective(nil), objectives...)}).Normalized().Objectives
	witness = witness.Normalized()
	if manifestHash == "" || len(objectives) == 0 {
		return fmt.Errorf("runtime candidate correction requires manifest-bound objectives")
	}
	if err := witness.Validate(); err != nil {
		return err
	}
	diagnostic := observation.Diagnostic.Normalized()
	if diagnostic.ManifestHash != manifestHash {
		return fmt.Errorf("runtime candidate correction observation manifest identity is stale")
	}
	if !RuntimeCandidateCorrectionObjectivesEqual(objectives, diagnostic.Objectives) {
		return fmt.Errorf("runtime candidate correction objectives differ from observation")
	}
	expectedHash := StableRuntimeCandidateSealedCorrectionHash(manifestHash, objectives, &witness)
	if expectedHash == "" || expectedHash != observation.SealedCorrectionHash {
		return fmt.Errorf("runtime candidate correction differs from sealed observation authority")
	}
	if err := validateRuntimeCandidateCorrectionWitnessAgainstObservation(observation, witness); err != nil {
		return err
	}
	return validateRuntimeCandidateCorrectionObjectiveWitnessCompatibility(objectives, witness)
}

// validateRuntimeCandidateCorrectionObjectiveWitnessCompatibility closes the
// "both sides made the same mapping error" gap. Objective equality proves
// lineage, while this matrix proves that the bounded objective vocabulary can
// actually be witnessed by the selected structural/semantic dimension.
func validateRuntimeCandidateCorrectionObjectiveWitnessCompatibility(objectives []RuntimeCandidateCorrectionObjective, witness RuntimeCandidateCorrectionWitness) error {
	objectives = (&RuntimeCandidateFailureDiagnostic{Objectives: append([]RuntimeCandidateCorrectionObjective(nil), objectives...)}).Normalized().Objectives
	hasAny := func(wanted ...RuntimeCandidateCorrectionObjective) bool {
		for _, objective := range objectives {
			for _, candidate := range wanted {
				if objective == candidate {
					return true
				}
			}
		}
		return false
	}
	allowsOnly := func(allowed ...RuntimeCandidateCorrectionObjective) bool {
		for _, objective := range objectives {
			found := false
			for _, candidate := range allowed {
				if objective == candidate {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	compatible := false
	switch witness.Dimension {
	case RuntimeCandidateCorrectionWitnessDocumentShape:
		compatible = len(objectives) == 1 && objectives[0] == RuntimeCandidateCorrectionConformExpectedDocument
	case RuntimeCandidateCorrectionWitnessSchemaField:
		compatible = hasAny(RuntimeCandidateCorrectionSupplyRequiredField, RuntimeCandidateCorrectionRemoveForbiddenField) &&
			allowsOnly(
				RuntimeCandidateCorrectionSupplyRequiredField,
				RuntimeCandidateCorrectionRemoveForbiddenField,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	case RuntimeCandidateCorrectionWitnessOperationKind:
		compatible = hasAny(RuntimeCandidateCorrectionSelectOperationKind, RuntimeCandidateCorrectionChangeReplayedShape, RuntimeCandidateCorrectionSelectDurableEffect) &&
			allowsOnly(
				RuntimeCandidateCorrectionSelectOperationKind,
				RuntimeCandidateCorrectionChangeReplayedShape,
				RuntimeCandidateCorrectionSelectDurableEffect,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	case RuntimeCandidateCorrectionWitnessTarget:
		compatible = hasAny(RuntimeCandidateCorrectionReplaceTarget, RuntimeCandidateCorrectionChangeReplayedShape) &&
			allowsOnly(
				RuntimeCandidateCorrectionReplaceTarget,
				RuntimeCandidateCorrectionChangeReplayedShape,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	case RuntimeCandidateCorrectionWitnessEvidenceBinding:
		compatible = hasAny(RuntimeCandidateCorrectionSelectEvidenceBinding, RuntimeCandidateCorrectionChangeReplayedShape) &&
			allowsOnly(
				RuntimeCandidateCorrectionSelectEvidenceBinding,
				RuntimeCandidateCorrectionChangeReplayedShape,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	case RuntimeCandidateCorrectionWitnessVariant, RuntimeCandidateCorrectionWitnessClosedParameter:
		compatible = hasAny(RuntimeCandidateCorrectionChangeReplayedShape, RuntimeCandidateCorrectionSelectDurableEffect) &&
			allowsOnly(
				RuntimeCandidateCorrectionChangeReplayedShape,
				RuntimeCandidateCorrectionSelectDurableEffect,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	case RuntimeCandidateCorrectionWitnessValidationRecipe:
		compatible = hasAny(RuntimeCandidateCorrectionChangeReplayedShape) &&
			allowsOnly(
				RuntimeCandidateCorrectionChangeReplayedShape,
				RuntimeCandidateCorrectionSatisfyCoverage,
				RuntimeCandidateCorrectionSatisfyDependency,
			)
	}
	if !compatible {
		return fmt.Errorf("runtime candidate correction objectives are incompatible with witness dimension %q", witness.Dimension)
	}
	return nil
}

// ValidateRuntimeCandidateFailureCorrectionPair is the persisted SM-3 fence.
// It rejects a structurally valid directive whose witness describes a
// different failure before an attempt record can survive CAS/recovery.
func ValidateRuntimeCandidateFailureCorrectionPair(failure *RuntimeCandidateFailure, correction *RuntimeCandidateCorrectionDirective) error {
	if failure == nil || correction == nil {
		return fmt.Errorf("runtime candidate failure and correction must be paired")
	}
	sealed := correction.Normalized()
	if err := sealed.Validate(); err != nil {
		return err
	}
	if sealed.Disposition == RuntimeCandidateCorrectionRejectedFinal {
		return nil
	}
	if sealed.Witness == nil {
		return fmt.Errorf("runtime candidate correctable failure requires a witness")
	}
	observation := RuntimeCandidateCorrectionObservation{
		Class:      failure.Class,
		Code:       failure.Code,
		Signature:  failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
		Diagnostic: failure.Diagnostic.Clone(),
		SealedCorrectionHash: StableRuntimeCandidateSealedCorrectionHash(
			sealed.ManifestHash, sealed.Objectives, sealed.Witness,
		),
	}
	return ValidateRuntimeCandidateCorrectionAgainstObservation(
		observation, sealed.ManifestHash, sealed.Objectives, *sealed.Witness,
	)
}

func validateRuntimeCandidateCorrectionWitnessAgainstObservation(observation RuntimeCandidateCorrectionObservation, witness RuntimeCandidateCorrectionWitness) error {
	diagnostic := observation.Diagnostic.Normalized()
	witness = witness.Normalized()
	if runtimeCandidateCorrectionDiagnosticReportsFieldAbsence(diagnostic) {
		if witness.Dimension != RuntimeCandidateCorrectionWitnessSchemaField || witness.Path != diagnostic.Path ||
			witness.RejectedValue != runtimeCandidateCorrectionFieldAbsent || witness.AlternativeValue != runtimeCandidateCorrectionFieldPresent ||
			!RuntimeCandidateCorrectionObjectivesEqual(diagnostic.Objectives, []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredField}) {
			return fmt.Errorf("runtime candidate missing-field correction requires schema-only authority")
		}
	}
	if diagnostic.RejectedShapeHash != "" && witness.RejectedShapeHash != diagnostic.RejectedShapeHash {
		return fmt.Errorf("runtime candidate correction witness rejected shape %q differs from observation %q", witness.RejectedShapeHash, diagnostic.RejectedShapeHash)
	}
	switch witness.Dimension {
	case RuntimeCandidateCorrectionWitnessDocumentShape:
		if observation.Code != "candidate_document_nonconformant" || diagnostic.Path != "$" ||
			diagnostic.Actual != witness.RejectedValue ||
			!runtimeCandidateCorrectionHasObjective(diagnostic.Objectives, RuntimeCandidateCorrectionConformExpectedDocument) {
			return fmt.Errorf("runtime candidate document correction witness differs from observation")
		}
	case RuntimeCandidateCorrectionWitnessSchemaField:
		return validateRuntimeCandidateSchemaWitnessAgainstObservation(diagnostic, witness)
	default:
		// Semantic alternatives come from the Orchestrator-private candidate
		// frontier. The observation can nevertheless prove that the rejected
		// path, value and shape are the same facts used to select the witness.
		if diagnostic.Path != "" && witness.Path != diagnostic.Path {
			return fmt.Errorf("runtime candidate correction witness path differs from observation")
		}
		if diagnostic.Actual != "" && witness.RejectedValue != diagnostic.Actual {
			return fmt.Errorf("runtime candidate correction witness rejected value differs from observation")
		}
		if diagnostic.RejectedShapeHash == "" || witness.RejectedShapeHash != diagnostic.RejectedShapeHash {
			return fmt.Errorf("runtime candidate semantic correction witness lacks observed rejected shape")
		}
	}
	return nil
}

func runtimeCandidateCorrectionDiagnosticReportsFieldAbsence(diagnostic RuntimeCandidateFailureDiagnostic) bool {
	field := runtimeCandidateCorrectionPathField(diagnostic.Path)
	if field == "" {
		return false
	}
	actual := strings.TrimSpace(diagnostic.Actual)
	return (actual == "" || strings.HasPrefix(actual, "missing_") || strings.HasPrefix(actual, "empty_")) &&
		runtimeCandidateCorrectionDiagnosticRequiresField(diagnostic, field)
}

func validateRuntimeCandidateSchemaWitnessAgainstObservation(diagnostic RuntimeCandidateFailureDiagnostic, witness RuntimeCandidateCorrectionWitness) error {
	if witness.OperationKind != "" || witness.Path != diagnostic.Path || witness.Path == "" {
		return fmt.Errorf("runtime candidate schema correction witness path differs from observation")
	}
	field := runtimeCandidateCorrectionPathField(witness.Path)
	if field == "" {
		return fmt.Errorf("runtime candidate schema correction witness has no typed field")
	}
	requires := runtimeCandidateCorrectionDiagnosticRequiresField(diagnostic, field)
	forbids := runtimeCandidateCorrectionDiagnosticContains(diagnostic.Forbidden, field) ||
		(diagnostic.Actual == field && len(diagnostic.Allowed) != 0 && !runtimeCandidateCorrectionDiagnosticContains(diagnostic.Allowed, field))
	switch {
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldPresent:
		if witness.RejectedValue != runtimeCandidateCorrectionFieldAbsent || !requires ||
			!runtimeCandidateCorrectionDiagnosticReportsFieldAbsence(diagnostic) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a required field")
		}
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldAbsent:
		if !forbids {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a forbidden field")
		}
		if diagnostic.Actual != "" && witness.RejectedValue != diagnostic.Actual && witness.RejectedValue != field {
			return fmt.Errorf("runtime candidate schema correction rejected value differs from observation")
		}
	case strings.HasPrefix(witness.AlternativeValue, runtimeCandidateCorrectionValueAbsent):
		removed := strings.TrimPrefix(witness.AlternativeValue, runtimeCandidateCorrectionValueAbsent)
		if removed == "" || diagnostic.Actual != removed || witness.RejectedValue != diagnostic.Actual || !forbids {
			return fmt.Errorf("runtime candidate schema correction witness does not prove removed value")
		}
	default:
		return fmt.Errorf("runtime candidate schema correction witness has undeclared alternative")
	}
	if witness.RejectedShapeHash != "" && witness.RejectedShapeHash != diagnostic.RejectedShapeHash {
		return fmt.Errorf("runtime candidate schema correction rejected shape %q differs from observation %q", witness.RejectedShapeHash, diagnostic.RejectedShapeHash)
	}
	return nil
}

func runtimeCandidateCorrectionDiagnosticRequiresField(diagnostic RuntimeCandidateFailureDiagnostic, field string) bool {
	if runtimeCandidateCorrectionDiagnosticContains(diagnostic.Required, field) {
		return true
	}
	actual := strings.TrimSpace(diagnostic.Actual)
	for _, prefix := range []string{"missing_", "empty_"} {
		if !strings.HasPrefix(actual, prefix) {
			continue
		}
		left := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimPrefix(actual, prefix)))
		right := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(field)))
		if left != "" && left == right {
			return true
		}
	}
	return false
}

func runtimeCandidateCorrectionHasObjective(values []RuntimeCandidateCorrectionObjective, wanted RuntimeCandidateCorrectionObjective) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func runtimeCandidateCorrectionDiagnosticContains(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func runtimeCandidateCorrectionPathField(path string) string {
	path = strings.TrimSpace(path)
	if marker := strings.LastIndex(path, "[name="); marker >= 0 {
		value := path[marker+len("[name="):]
		if end := strings.Index(value, "]"); end >= 0 {
			return strings.TrimSpace(value[:end])
		}
	}
	if dot := strings.LastIndex(path, "."); dot >= 0 && dot+1 < len(path) {
		return strings.TrimSpace(path[dot+1:])
	}
	return ""
}
