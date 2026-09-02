package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const RuntimeSemanticCorrectionObservationSchemaCurrent = "workorch.runtime_semantic_correction_observation"

const maxRuntimeCorrectionObservationItems = 32
const maxRuntimeCorrectionObservationTextBytes = 4096

func validateRuntimeCorrectionObservationText(field, value string) error {
	if len(strings.TrimSpace(value)) > maxRuntimeCorrectionObservationTextBytes {
		return fmt.Errorf("runtime correction observation %s exceeds bounded text", field)
	}
	return validateRuntimeFeedbackSafeText(field, value)
}

func validateRuntimeCorrectionObservationTexts(field string, values []string) error {
	for _, value := range values {
		if err := validateRuntimeCorrectionObservationText(field, value); err != nil {
			return err
		}
	}
	return nil
}

// RuntimeCandidateCorrectionObservation is the complete, bounded candidate
// rejection surface that may cross from the Orchestrator SM-3 owner to the
// LLM owner. Persisted/audit Reason and validation output are deliberately not
// representable here.
type RuntimeCandidateCorrectionObservation struct {
	Class      RuntimeCandidateFailureClass       `json:"class"`
	Code       string                             `json:"code"`
	Signature  string                             `json:"signature"`
	TargetRefs []string                           `json:"targetRefs,omitempty"`
	Diagnostic *RuntimeCandidateFailureDiagnostic `json:"diagnostic"`
	// SealedCorrectionHash is the Orchestrator-authored canonical hash of the
	// exact manifest, objectives and witness authorized for attempt 1. It does
	// not carry disposition, terminal state or audit text, and lets the LLM
	// owner reject a detached/mis-mapped correction without recreating SM-3's
	// private candidate frontier.
	SealedCorrectionHash string `json:"sealedCorrectionHash"`
}

func (o RuntimeCandidateCorrectionObservation) Normalized() RuntimeCandidateCorrectionObservation {
	o.Class = RuntimeCandidateFailureClass(strings.TrimSpace(string(o.Class)))
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	o.Diagnostic = o.Diagnostic.Clone()
	o.SealedCorrectionHash = strings.TrimSpace(o.SealedCorrectionHash)
	return o
}

func (o RuntimeCandidateCorrectionObservation) Clone() RuntimeCandidateCorrectionObservation {
	o = o.Normalized()
	o.TargetRefs = append([]string(nil), o.TargetRefs...)
	o.Diagnostic = o.Diagnostic.Clone()
	return o
}

func (o RuntimeCandidateCorrectionObservation) Validate() error {
	o = o.Normalized()
	if o.Class != RuntimeCandidateFailureClassSchema && o.Class != RuntimeCandidateFailureClassContract {
		return fmt.Errorf("runtime candidate correction observation requires schema or contract class")
	}
	if o.Code == "" || o.Signature == "" || o.Diagnostic == nil || o.SealedCorrectionHash == "" {
		return fmt.Errorf("runtime candidate correction observation requires code, signature, typed diagnostic and sealed correction hash")
	}
	if len(o.TargetRefs) > maxRuntimeCorrectionObservationItems {
		return fmt.Errorf("runtime candidate correction observation exceeds target bound")
	}
	if err := validateRuntimeCorrectionObservationText("candidate correction code", o.Code); err != nil {
		return err
	}
	if err := validateRuntimeCorrectionObservationText("candidate correction signature", o.Signature); err != nil {
		return err
	}
	if err := validateRuntimeCorrectionObservationText("candidate sealed correction hash", o.SealedCorrectionHash); err != nil {
		return err
	}
	if err := validateRuntimeCorrectionObservationTexts("candidate correction target", o.TargetRefs); err != nil {
		return err
	}
	diagnostic := o.Diagnostic.Normalized()
	if diagnostic.ManifestHash == "" || len(diagnostic.Objectives) == 0 {
		return fmt.Errorf("runtime candidate correction observation requires manifest-bound diagnostic objectives")
	}
	if RuntimeCandidateFailureDiagnosticRequiresWireRule(o.Class, &diagnostic) && diagnostic.WireRule == nil {
		return fmt.Errorf("runtime candidate operation-scoped schema correction requires exact wire rule identity")
	}
	if len(diagnostic.Allowed) > maxRuntimeCorrectionObservationItems || len(diagnostic.Required) > maxRuntimeCorrectionObservationItems || len(diagnostic.Forbidden) > maxRuntimeCorrectionObservationItems || len(diagnostic.Objectives) > maxRuntimeCorrectionObservationItems || len(diagnostic.ParameterEntryEncodings) > maxRuntimeCorrectionObservationItems {
		return fmt.Errorf("runtime candidate correction diagnostic exceeds bounded values")
	}
	if diagnostic.OperationID == "" && diagnostic.Path == "" && diagnostic.Actual == "" && len(diagnostic.Allowed) == 0 && len(diagnostic.Required) == 0 && len(diagnostic.Forbidden) == 0 && diagnostic.WireRule == nil && diagnostic.ViolationKind == "" && len(diagnostic.ParameterEntryEncodings) == 0 && diagnostic.ConstraintRef == "" && diagnostic.RequiredShape == "" && diagnostic.ManifestHash == "" && diagnostic.RejectedShapeHash == "" && len(diagnostic.Objectives) == 0 {
		return fmt.Errorf("runtime candidate correction observation requires a non-empty typed diagnostic")
	}
	if diagnostic.WireRule != nil {
		if err := diagnostic.WireRule.Validate(); err != nil {
			return err
		}
		operationIndex, found := runtimeCandidateDiagnosticOperationIndex(diagnostic.Path)
		if !found || operationIndex != diagnostic.WireRule.OperationIndex {
			return fmt.Errorf("runtime candidate correction diagnostic wire rule does not match its operation path")
		}
	}
	switch diagnostic.ViolationKind {
	case "":
		if len(diagnostic.ParameterEntryEncodings) != 0 {
			return fmt.Errorf("runtime candidate correction diagnostic carries parameter-entry facts without a violation kind")
		}
	case RuntimeCandidatePayloadViolationParameterEntryEncoding:
		if err := validateRuntimeCandidateParameterEntryEncodingFacts(diagnostic.ParameterEntryEncodings); err != nil {
			return err
		}
		if !runtimeCandidateCorrectionHasObjective(diagnostic.Objectives, RuntimeCandidateCorrectionReencodeParameterEntries) {
			return fmt.Errorf("runtime candidate parameter-entry diagnostic requires re-encode objective")
		}
	default:
		return fmt.Errorf("runtime candidate correction diagnostic has unsupported violation kind %q", diagnostic.ViolationKind)
	}
	for _, value := range []struct{ name, text string }{
		{"candidate correction operation id", diagnostic.OperationID}, {"candidate correction path", diagnostic.Path},
		{"candidate correction actual", diagnostic.Actual}, {"candidate correction manifest hash", diagnostic.ManifestHash},
		{"candidate correction rejected shape hash", diagnostic.RejectedShapeHash}, {"candidate correction constraint ref", diagnostic.ConstraintRef},
		{"candidate correction required shape", diagnostic.RequiredShape},
	} {
		if err := validateRuntimeCorrectionObservationText(value.name, value.text); err != nil {
			return err
		}
	}
	if diagnostic.WireRule != nil {
		for _, value := range []struct{ name, text string }{
			{"candidate correction wire contract hash", diagnostic.WireRule.WireContractHash},
			{"candidate correction wire required operation ref", diagnostic.WireRule.RequiredOperationRef},
			{"candidate correction wire operation kind", diagnostic.WireRule.OperationKind},
			{"candidate correction wire variant id", diagnostic.WireRule.VariantID},
		} {
			if err := validateRuntimeCorrectionObservationText(value.name, value.text); err != nil {
				return err
			}
		}
	}
	for _, values := range []struct {
		name   string
		values []string
	}{{"candidate correction allowed", diagnostic.Allowed}, {"candidate correction required", diagnostic.Required}, {"candidate correction forbidden", diagnostic.Forbidden}} {
		if err := validateRuntimeCorrectionObservationTexts(values.name, values.values); err != nil {
			return err
		}
	}
	for _, objective := range diagnostic.Objectives {
		switch objective {
		case RuntimeCandidateCorrectionRemoveForbiddenField,
			RuntimeCandidateCorrectionSupplyRequiredField,
			RuntimeCandidateCorrectionReplaceTarget,
			RuntimeCandidateCorrectionSelectOperationKind,
			RuntimeCandidateCorrectionSelectDurableEffect,
			RuntimeCandidateCorrectionChangeReplayedShape,
			RuntimeCandidateCorrectionSatisfyCoverage,
			RuntimeCandidateCorrectionSatisfyDependency,
			RuntimeCandidateCorrectionSelectEvidenceBinding,
			RuntimeCandidateCorrectionReencodeParameterEntries,
			RuntimeCandidateCorrectionConformOwnerConstraint,
			RuntimeCandidateCorrectionConformExpectedDocument:
		default:
			return fmt.Errorf("runtime candidate correction observation has unsupported objective %q", objective)
		}
	}
	return nil
}

// StableRuntimeCandidateSealedCorrectionHash is the bounded SM-3 authority
// fence carried by RuntimeCandidateCorrectionObservation. The hash excludes
// disposition, terminal state, rejection reason and raw provider content. It
// is intentionally independent of the observation itself: the observation
// explains the rejection while this value proves the exact detached
// correction input that Orchestrator sealed from its private frontier.
func StableRuntimeCandidateSealedCorrectionHash(manifestHash string, objectives []RuntimeCandidateCorrectionObjective, witness *RuntimeCandidateCorrectionWitness) string {
	manifestHash = strings.TrimSpace(manifestHash)
	objectives = (&RuntimeCandidateFailureDiagnostic{Objectives: append([]RuntimeCandidateCorrectionObjective(nil), objectives...)}).Normalized().Objectives
	if manifestHash == "" || len(objectives) == 0 || witness == nil {
		return ""
	}
	witnessValue := witness.Normalized()
	if err := witnessValue.Validate(); err != nil {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "runtime-candidate-sealed-correction/v3")
	writeHashString(h, manifestHash)
	for _, objective := range objectives {
		writeHashString(h, string(objective))
	}
	writeHashString(h, string(witnessValue.Dimension))
	writeHashString(h, witnessValue.OperationKind)
	writeHashString(h, witnessValue.Path)
	writeHashString(h, witnessValue.RejectedValue)
	writeHashString(h, witnessValue.AlternativeValue)
	writeHashString(h, witnessValue.RejectedShapeHash)
	writeHashString(h, witnessValue.AlternativeShapeHash)
	writeHashString(h, witnessValue.ConstraintRef)
	writeHashRuntimeCandidateParameterEntryEncodings(h, witnessValue.ParameterEntryEncodings)
	return hex.EncodeToString(h.Sum(nil))
}

// RuntimeSemanticCorrectionObservation is the independent bounded semantic
// rejection surface rendered in C4. OriginalMessage, details/raw content,
// validation observations and governance dependencies cannot cross this type.
type RuntimeSemanticCorrectionObservation struct {
	SchemaVersion      string                             `json:"schemaVersion"`
	OwnerModuleID      string                             `json:"ownerModuleId"`
	Class              string                             `json:"class"`
	Code               string                             `json:"code"`
	Signature          string                             `json:"signature"`
	OperationID        string                             `json:"operationId,omitempty"`
	TargetRefs         []string                           `json:"targetRefs,omitempty"`
	ExpectedRevision   string                             `json:"expectedRevision,omitempty"`
	ObservedRevision   string                             `json:"observedRevision,omitempty"`
	FieldViolations    []RuntimeFieldViolation            `json:"fieldViolations,omitempty"`
	SemanticViolations []RuntimeSemanticContractViolation `json:"semanticViolations"`
	TargetObservation  *RuntimeFailureTargetObservation   `json:"targetObservation,omitempty"`
}

func (o RuntimeSemanticCorrectionObservation) Normalized() RuntimeSemanticCorrectionObservation {
	o.SchemaVersion = strings.TrimSpace(o.SchemaVersion)
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.Class = strings.TrimSpace(o.Class)
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	o.OperationID = strings.TrimSpace(o.OperationID)
	o.TargetRefs = normalizeOrderedStrings(o.TargetRefs)
	for idx := range o.FieldViolations {
		o.FieldViolations[idx] = o.FieldViolations[idx].Normalized()
	}
	o.SemanticViolations = normalizeRuntimeSemanticContractViolations(o.SemanticViolations)
	if o.TargetObservation != nil {
		value := o.TargetObservation.Normalized()
		o.TargetObservation = &value
	}
	o.ExpectedRevision = strings.TrimSpace(o.ExpectedRevision)
	o.ObservedRevision = strings.TrimSpace(o.ObservedRevision)
	return o
}

func (o RuntimeSemanticCorrectionObservation) Clone() RuntimeSemanticCorrectionObservation {
	o = o.Normalized()
	o.TargetRefs = append([]string(nil), o.TargetRefs...)
	o.FieldViolations = CloneRuntimeFieldViolations(o.FieldViolations)
	o.SemanticViolations = cloneRuntimeSemanticContractViolations(o.SemanticViolations)
	if o.TargetObservation != nil {
		value := o.TargetObservation.Clone()
		o.TargetObservation = &value
	}
	return o
}

func (o RuntimeSemanticCorrectionObservation) Validate() error {
	o = o.Normalized()
	if o.SchemaVersion != RuntimeSemanticCorrectionObservationSchemaCurrent || o.OwnerModuleID == "" || o.Class == "" || o.Code == "" || o.Signature == "" {
		return fmt.Errorf("runtime semantic correction observation requires current typed identity")
	}
	if len(o.TargetRefs) > maxRuntimeCorrectionObservationItems || len(o.FieldViolations) > maxRuntimeCorrectionObservationItems || len(o.SemanticViolations) == 0 || len(o.SemanticViolations) > maxRuntimeCorrectionObservationItems {
		return fmt.Errorf("runtime semantic correction observation exceeds bounded violations")
	}
	for _, value := range []struct{ name, text string }{
		{"semantic correction owner", o.OwnerModuleID}, {"semantic correction class", o.Class},
		{"semantic correction code", o.Code}, {"semantic correction signature", o.Signature},
		{"semantic correction operation id", o.OperationID}, {"semantic correction expected revision", o.ExpectedRevision},
		{"semantic correction observed revision", o.ObservedRevision},
	} {
		if err := validateRuntimeCorrectionObservationText(value.name, value.text); err != nil {
			return err
		}
	}
	if err := validateRuntimeCorrectionObservationTexts("semantic correction target", o.TargetRefs); err != nil {
		return err
	}
	for _, violation := range o.FieldViolations {
		if err := violation.Validate(); err != nil {
			return err
		}
		if len(violation.Allowed) > maxRuntimeCorrectionObservationItems || len(violation.Required) > maxRuntimeCorrectionObservationItems || len(violation.Forbidden) > maxRuntimeCorrectionObservationItems {
			return fmt.Errorf("runtime semantic correction field violation exceeds bounded values")
		}
		if err := validateRuntimeCorrectionObservationText("semantic field path", violation.Path); err != nil {
			return err
		}
		if err := validateRuntimeCorrectionObservationText("semantic field actual", violation.Actual); err != nil {
			return err
		}
		for _, values := range [][]string{violation.Allowed, violation.Required, violation.Forbidden} {
			if err := validateRuntimeCorrectionObservationTexts("semantic field value", values); err != nil {
				return err
			}
		}
	}
	for idx := range o.SemanticViolations {
		if err := o.SemanticViolations[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic correction observation violation %d: %w", idx, err)
		}
		violation := o.SemanticViolations[idx].Normalized()
		if len(violation.Required) > maxRuntimeCorrectionObservationItems || len(violation.Allowed) > maxRuntimeCorrectionObservationItems || len(violation.RequiredCoverageWorkItemIDs) > maxRuntimeCorrectionObservationItems {
			return fmt.Errorf("runtime semantic correction violation %d exceeds bounded values", idx)
		}
		if err := validateRuntimeCorrectionObservationText("semantic violation path", violation.Path); err != nil {
			return err
		}
		if err := validateRuntimeCorrectionObservationText("semantic violation actual", violation.Actual); err != nil {
			return err
		}
		if err := validateRuntimeCorrectionObservationTexts("semantic violation value", append(append([]string(nil), violation.Required...), violation.Allowed...)); err != nil {
			return err
		}
		for _, value := range []struct{ name, text string }{
			{"semantic violation required-before id", string(violation.RequiredBeforeID)},
			{"semantic violation required-after id", string(violation.RequiredAfterID)},
			{"semantic violation coverage id", string(violation.CoverageWorkItemID)},
		} {
			if err := validateRuntimeCorrectionObservationText(value.name, value.text); err != nil {
				return err
			}
		}
		for _, id := range violation.RequiredCoverageWorkItemIDs {
			if err := validateRuntimeCorrectionObservationText("semantic violation required coverage id", string(id)); err != nil {
				return err
			}
		}
	}
	if o.TargetObservation != nil {
		if err := o.TargetObservation.Validate(); err != nil {
			return fmt.Errorf("runtime semantic correction target observation: %w", err)
		}
		if o.TargetObservation.OwnerModuleID != o.OwnerModuleID {
			return fmt.Errorf("runtime semantic correction target observation owner mismatch")
		}
		if err := validateRuntimeSemanticCorrectionTargetObservation(*o.TargetObservation); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeSemanticCorrectionTargetObservation(observation RuntimeFailureTargetObservation) error {
	observation = observation.Normalized()
	if len(observation.TargetRefs) > maxRuntimeCorrectionObservationItems || len(observation.EvidenceRefs) > maxRuntimeCorrectionObservationItems {
		return fmt.Errorf("runtime semantic correction target observation exceeds bounded references")
	}
	for _, value := range []struct{ name, text string }{
		{"semantic target owner", observation.OwnerModuleID},
		{"semantic target subject", observation.SubjectRef},
		{"semantic target role", observation.TargetRole},
		{"semantic target observed revision", observation.ObservedRevision},
		{"semantic target diagnostic class", observation.DiagnosticClass},
		{"semantic target content hash", observation.ContentHash},
	} {
		if err := validateRuntimeCorrectionObservationText(value.name, value.text); err != nil {
			return err
		}
	}
	if err := validateRuntimeCorrectionObservationTexts("semantic target ref", observation.TargetRefs); err != nil {
		return err
	}
	return validateRuntimeCorrectionObservationTexts("semantic target evidence ref", observation.EvidenceRefs)
}
