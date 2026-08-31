package models

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCandidateCorrectionFieldAbsent  = "field_absent"
	runtimeCandidateCorrectionFieldPresent = "field_present"
)

// ValidateAgainstObservation proves that a detached attempt-1 correction is
// the exact bounded authority sealed by Orchestrator for this rejection. It
// validates identity and the directly observable document/schema relation;
// it deliberately does not recreate Orchestrator's private replay frontier.
func (c RuntimeCandidateCorrectionInput) ValidateAgainstObservation(manifest runtimecontract.RuntimeCandidateContractManifest, observation runtimecontract.RuntimeCandidateCorrectionObservation) error {
	manifest = manifest.Normalized()
	if manifest.Revision != runtimecontract.RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != runtimecontract.StableRuntimeCandidateContractManifestHash(manifest) {
		return fmt.Errorf("runtime candidate correction requires a sealed candidate manifest")
	}
	if err := runtimecontract.ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err != nil {
		return fmt.Errorf("runtime candidate correction manifest metadata: %w", err)
	}
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return err
	}
	c = c.Normalized()
	if err := c.Validate(manifest.Hash); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeCandidateCorrectionAgainstObservation(
		observation, c.ManifestHash, c.Objectives, c.Witness,
	); err != nil {
		return err
	}
	if c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField {
		return validateRuntimeCandidateSchemaWitnessManifest(manifest, c.Witness)
	}
	if c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding {
		return runtimecontract.ValidateRuntimeCandidateParameterEntryEncodings(manifest, c.Witness.ParameterEntryEncodings)
	}
	return nil
}

func validateRuntimeCandidateSchemaWitnessManifest(manifest runtimecontract.RuntimeCandidateContractManifest, witness runtimecontract.RuntimeCandidateCorrectionWitness) error {
	field := runtimeCandidateCorrectionPathField(witness.Path)
	switch {
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldPresent:
		if !runtimeCandidateManifestRequiresCorrectionField(manifest, witness.Path, field) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a required field")
		}
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldAbsent:
		if !runtimeCandidateManifestForbidsCorrectionField(manifest, witness.Path, field) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a forbidden field")
		}
	case strings.HasPrefix(witness.AlternativeValue, "value_absent:"):
		if !runtimeCandidateManifestForbidsCorrectionField(manifest, witness.Path, field) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove removed value")
		}
	}
	return nil
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

func runtimeCandidateManifestRequiresCorrectionField(manifest runtimecontract.RuntimeCandidateContractManifest, path, field string) bool {
	if strings.HasPrefix(path, "$.") && !strings.HasPrefix(path, "$.operations[") {
		if runtimeCandidateCorrectionDiagnosticContains(manifest.RequiredTopLevelFields, field) {
			return true
		}
		// Assessment evidence is conditionally required when the provider chose
		// the sealed satisfied disposition. The bounded observation supplies that
		// exact missing-field fact; no other merely allowed top-level field gains
		// correction authority here.
		return manifest.State == runtimecontract.ConvergenceStateNeedAssessment && field == "evidenceRefs" &&
			runtimeCandidateCorrectionDiagnosticContains(manifest.AllowedTopLevelFields, field)
	}
	if len(manifest.Operations) == 0 {
		return false
	}
	// The bounded observation does not carry provider-authored operation
	// identity. When more than one operation kind is authorized, a field is
	// direct manifest proof only if every possible kind requires it; borrowing a
	// requirement from one unrelated kind would let a forged pair cross C4.
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		required := false
		if strings.Contains(path, ".parameters[name=") {
			for _, parameter := range operation.Parameters {
				if parameter.Name == field && runtimeCandidateManifestParameterMayBeRequired(operation, field, parameter.Required) {
					required = true
					break
				}
			}
		} else {
			required = runtimeCandidateCorrectionDiagnosticContains(operation.RequiredOperationFields, field) ||
				(field == "targetRefs" && operation.RequireTargetRefs)
		}
		if !required {
			return false
		}
	}
	return true
}

func runtimeCandidateManifestParameterMayBeRequired(operation runtimecontract.RuntimeCandidateOperationManifest, field string, required bool) bool {
	if required {
		return true
	}
	if runtimeCandidateCorrectionDiagnosticContains(operation.RequireOneOfParameters, field) {
		return true
	}
	for _, variant := range operation.Variants {
		if runtimeCandidateCorrectionDiagnosticContains(variant.RequiredParameters, field) ||
			runtimeCandidateCorrectionDiagnosticContains(variant.RequiredNonEmpty, field) {
			return true
		}
	}
	return false
}

func runtimeCandidateManifestForbidsCorrectionField(manifest runtimecontract.RuntimeCandidateContractManifest, path, field string) bool {
	if strings.HasPrefix(path, "$.") && !strings.HasPrefix(path, "$.operations[") {
		return runtimeCandidateCorrectionDiagnosticContains(manifest.ForbiddenTopLevelFields, field) ||
			!runtimeCandidateCorrectionDiagnosticContains(manifest.AllowedTopLevelFields, field)
	}
	if len(manifest.Operations) == 0 {
		return false
	}
	if field == "id" || field == "dependsOn" || field == "coverageWorkItemIds" {
		for _, operation := range manifest.Operations {
			if runtimeCandidateCorrectionDiagnosticContains(operation.AllowedOperationFields, field) {
				return false
			}
		}
		return true
	}
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		if strings.Contains(path, ".parameters[name=") {
			declared := false
			for _, parameter := range operation.Parameters {
				if parameter.Name == field {
					declared = true
					if !runtimeCandidateCorrectionDiagnosticContains(operation.ForbiddenParameters, field) {
						return false
					}
					break
				}
			}
			if !declared && operation.AllowsDynamicParameters && runtimeCandidateDynamicParameterAllowedForCorrection(field, operation.DynamicParameterPrefixes) {
				return false
			}
			continue
		}
		if runtimeCandidateCorrectionDiagnosticContains(operation.AllowedOperationFields, field) {
			return false
		}
	}
	return len(manifest.Operations) != 0
}

func runtimeCandidateDynamicParameterAllowedForCorrection(name string, prefixes []string) bool {
	name = strings.TrimSpace(name)
	for _, prefix := range prefixes {
		if prefix = strings.TrimSpace(prefix); prefix != "" && strings.HasPrefix(name, prefix) && strings.TrimSpace(strings.TrimPrefix(name, prefix)) != "" {
			return true
		}
	}
	return false
}
