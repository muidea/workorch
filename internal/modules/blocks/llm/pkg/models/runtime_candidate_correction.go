package models

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCandidateCorrectionFieldAbsent  = runtimecontract.RuntimeCandidateCorrectionFieldAbsent
	runtimeCandidateCorrectionFieldPresent = runtimecontract.RuntimeCandidateCorrectionFieldPresent
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
	if observation.Diagnostic.WireRule != nil {
		if err := runtimecontract.ValidateRuntimeCandidateFailureDiagnosticWireRule(manifest, observation.Diagnostic); err != nil {
			return fmt.Errorf("runtime candidate correction wire diagnostic: %w", err)
		}
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
		return validateRuntimeCandidateSchemaWitnessManifest(manifest, observation.Diagnostic, c.Witness)
	}
	if c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding {
		return runtimecontract.ValidateRuntimeCandidateParameterEntryEncodings(manifest, c.Witness.ParameterEntryEncodings)
	}
	if c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint {
		constraint, found := runtimecontract.RuntimeCandidateConstraintForOperationRef(manifest, c.Witness.OperationKind, c.Witness.ConstraintRef)
		if !found || constraint.ParameterName != runtimeCandidateCorrectionPathField(c.Witness.Path) || string(constraint.RequiredShape) != c.Witness.AlternativeValue {
			return fmt.Errorf("runtime candidate owner-constraint witness is not sealed in the manifest")
		}
	}
	return nil
}

func validateRuntimeCandidateSchemaWitnessManifest(manifest runtimecontract.RuntimeCandidateContractManifest, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, witness runtimecontract.RuntimeCandidateCorrectionWitness) error {
	field := runtimeCandidateCorrectionPathField(witness.Path)
	switch {
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldPresent:
		if !runtimeCandidateManifestRequiresCorrectionField(manifest, diagnostic, witness.Path, field) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a required field")
		}
	case witness.AlternativeValue == runtimeCandidateCorrectionFieldAbsent:
		if !runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic, witness.Path, field) {
			return fmt.Errorf("runtime candidate schema correction witness does not prove a forbidden field")
		}
	case strings.HasPrefix(witness.AlternativeValue, "value_absent:"):
		if !runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic, witness.Path, field) {
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

func runtimeCandidateManifestRequiresCorrectionField(manifest runtimecontract.RuntimeCandidateContractManifest, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, path, field string) bool {
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
	rule, found := runtimeCandidateWireRuleForCorrectionDiagnostic(manifest, diagnostic)
	if !found {
		return false
	}
	if strings.Contains(path, ".parameters[name=") {
		return runtimeCandidateCorrectionDiagnosticContains(rule.RequiredProviderParameterNames, field) ||
			runtimeCandidateCorrectionDiagnosticContains(rule.RequiredNonEmptyParameterNames, field) ||
			runtimeCandidateCorrectionDiagnosticContains(rule.RequireOneOfProviderParameters, field)
	}
	return runtimeCandidateCorrectionDiagnosticContains(rule.RequiredProviderFields, field)
}

func runtimeCandidateManifestForbidsCorrectionField(manifest runtimecontract.RuntimeCandidateContractManifest, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, path, field string) bool {
	if strings.HasPrefix(path, "$.") && !strings.HasPrefix(path, "$.operations[") {
		return runtimeCandidateCorrectionDiagnosticContains(manifest.ForbiddenTopLevelFields, field) ||
			!runtimeCandidateCorrectionDiagnosticContains(manifest.AllowedTopLevelFields, field)
	}
	rule, found := runtimeCandidateWireRuleForCorrectionDiagnostic(manifest, diagnostic)
	if !found {
		return false
	}
	if strings.Contains(path, ".parameters[name=") {
		if runtimeCandidateCorrectionDiagnosticContains(rule.ProviderParameterNames, field) {
			return false
		}
		if runtimeCandidateDynamicParameterAllowedForCorrection(field, rule.DynamicParameterPrefixes) {
			return false
		}
		return true
	}
	return !runtimeCandidateCorrectionDiagnosticContains(rule.ProviderFields, field)
}

func runtimeCandidateWireRuleForCorrectionDiagnostic(manifest runtimecontract.RuntimeCandidateContractManifest, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic) (runtimecontract.RuntimeCandidateWireOperationRule, bool) {
	if diagnostic == nil || diagnostic.WireRule == nil ||
		runtimecontract.ValidateRuntimeCandidateFailureDiagnosticWireRule(manifest, diagnostic) != nil {
		return runtimecontract.RuntimeCandidateWireOperationRule{}, false
	}
	rule, err := diagnostic.WireRule.RuleAgainstManifest(manifest)
	if err != nil {
		return runtimecontract.RuntimeCandidateWireOperationRule{}, false
	}
	return rule, true
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
