package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeCandidateCorrectionDisposition is the only same-contract correction
// outcome a complete candidate rejection may carry. SM-3 consumes it; no other
// component reinterprets raw reason text to choose retry or terminal.
type RuntimeCandidateCorrectionDisposition string

const (
	// RuntimeCandidateCorrectionCorrectableOnce means there is exactly one safe
	// structural correction under the same sealed manifest. Attempt 1 may open
	// only when a non-empty CorrectionDirective can be verified from the
	// rejection + manifest.
	RuntimeCandidateCorrectionCorrectableOnce RuntimeCandidateCorrectionDisposition = "correctable_once"
	// RuntimeCandidateCorrectionRejectedFinal means no safe same-contract
	// correction exists. SM-3 must form a typed terminal with zero correction
	// provider call.
	RuntimeCandidateCorrectionRejectedFinal RuntimeCandidateCorrectionDisposition = "rejected_final"
)

// RuntimeCandidateCorrectionWitnessDimension identifies one concrete,
// execution-semantic difference that the sealed manifest still permits.  It
// is deliberately narrower than a correction objective: objectives explain
// what was wrong, while a witness proves that attempt 1 has somewhere legal
// to go without treating operation IDs or display summaries as progress.
type RuntimeCandidateCorrectionWitnessDimension string

const (
	RuntimeCandidateCorrectionWitnessParameterCardinality    RuntimeCandidateCorrectionWitnessDimension = "parameter_cardinality"
	RuntimeCandidateCorrectionWitnessOperationKind           RuntimeCandidateCorrectionWitnessDimension = "operation_kind"
	RuntimeCandidateCorrectionWitnessTarget                  RuntimeCandidateCorrectionWitnessDimension = "target"
	RuntimeCandidateCorrectionWitnessVariant                 RuntimeCandidateCorrectionWitnessDimension = "variant"
	RuntimeCandidateCorrectionWitnessValidationRecipe        RuntimeCandidateCorrectionWitnessDimension = "validation_recipe"
	RuntimeCandidateCorrectionWitnessClosedParameter         RuntimeCandidateCorrectionWitnessDimension = "closed_semantic_parameter"
	RuntimeCandidateCorrectionWitnessEvidenceBinding         RuntimeCandidateCorrectionWitnessDimension = "evidence_binding"
	RuntimeCandidateCorrectionWitnessSchemaField             RuntimeCandidateCorrectionWitnessDimension = "schema_field"
	RuntimeCandidateCorrectionWitnessParameterEntryEncoding  RuntimeCandidateCorrectionWitnessDimension = "parameter_entry_encoding"
	RuntimeCandidateCorrectionWitnessOwnerConstraint         RuntimeCandidateCorrectionWitnessDimension = "owner_candidate_constraint"
	RuntimeCandidateCorrectionWitnessOperationValueRule      RuntimeCandidateCorrectionWitnessDimension = "operation_value_rule"
	RuntimeCandidateCorrectionWitnessOperationSetCardinality RuntimeCandidateCorrectionWitnessDimension = "operation_set_cardinality"
)

const (
	RuntimeCandidateParameterEncodingInlineSiblings = "inline_declared_parameter_siblings"
	RuntimeCandidateParameterEncodingNamedEntries   = "independent_named_typed_parameter_entries"
)

// RuntimeCandidateCorrectionWitness is the bounded proof carried by a
// correctable_once directive.  RejectedValue/AlternativeValue come from the
// rejected document and the sealed manifest respectively.  Semantic shape
// hashes are required for operation-level differences and intentionally omit
// operation IDs and display-only summary parameters.
type RuntimeCandidateCorrectionWitness struct {
	ParameterCardinality    *RuntimeCandidateParameterCardinalityViolation `json:",omitempty"`
	Dimension               RuntimeCandidateCorrectionWitnessDimension
	OperationKind           string
	Path                    string
	RejectedValue           string
	AlternativeValue        string
	RejectedShapeHash       string
	AlternativeShapeHash    string
	ConstraintRef           string
	ParameterEntryEncodings []RuntimeCandidateParameterEntryEncodingViolation
	OperationSetViolation   *RuntimeCandidateOperationSetViolation
}

func (w RuntimeCandidateCorrectionWitness) Normalized() RuntimeCandidateCorrectionWitness {
	w.Dimension = RuntimeCandidateCorrectionWitnessDimension(strings.TrimSpace(string(w.Dimension)))
	w.OperationKind = strings.TrimSpace(w.OperationKind)
	w.Path = strings.TrimSpace(w.Path)
	w.RejectedValue = strings.TrimSpace(w.RejectedValue)
	w.AlternativeValue = strings.TrimSpace(w.AlternativeValue)
	w.RejectedShapeHash = strings.TrimSpace(w.RejectedShapeHash)
	w.AlternativeShapeHash = strings.TrimSpace(w.AlternativeShapeHash)
	w.ConstraintRef = strings.TrimSpace(w.ConstraintRef)
	w.ParameterEntryEncodings = cloneRuntimeCandidateParameterEntryEncodings(w.ParameterEntryEncodings)
	w.ParameterCardinality = w.ParameterCardinality.Clone()
	w.OperationSetViolation = w.OperationSetViolation.Clone()
	return w
}

func (w RuntimeCandidateCorrectionWitness) Validate() error {
	w = w.Normalized()
	if w.Dimension != RuntimeCandidateCorrectionWitnessParameterEntryEncoding && len(w.ParameterEntryEncodings) != 0 {
		return fmt.Errorf("candidate correction witness dimension %q cannot carry parameter-entry encoding facts", w.Dimension)
	}
	if w.Dimension != RuntimeCandidateCorrectionWitnessOperationSetCardinality && w.OperationSetViolation != nil {
		return fmt.Errorf("candidate correction witness dimension %q cannot carry an operation-set violation", w.Dimension)
	}
	if w.Dimension != RuntimeCandidateCorrectionWitnessParameterCardinality && w.ParameterCardinality != nil {
		return fmt.Errorf("unrelated correction witness carries parameter cardinality")
	}
	switch w.Dimension {
	case RuntimeCandidateCorrectionWitnessParameterCardinality:
		if w.OperationKind != "" || w.Path != "$.operations" || w.RejectedValue != RuntimeCandidateParameterNamesRepeated || w.AlternativeValue != RuntimeCandidateParameterNamesUnique || w.RejectedShapeHash != "" || w.AlternativeShapeHash != "" || w.ConstraintRef != "" {
			return fmt.Errorf("invalid parameter cardinality witness")
		}
		if err := w.ParameterCardinality.Validate(); err != nil {
			return err
		}
	case RuntimeCandidateCorrectionWitnessOperationKind,
		RuntimeCandidateCorrectionWitnessTarget,
		RuntimeCandidateCorrectionWitnessVariant,
		RuntimeCandidateCorrectionWitnessValidationRecipe,
		RuntimeCandidateCorrectionWitnessClosedParameter,
		RuntimeCandidateCorrectionWitnessEvidenceBinding:
		if w.OperationKind == "" || w.Path == "" || w.AlternativeValue == "" || w.RejectedValue == w.AlternativeValue {
			return fmt.Errorf("semantic candidate correction witness requires kind, path and a concrete different alternative")
		}
		if w.RejectedShapeHash == "" || w.AlternativeShapeHash == "" || w.RejectedShapeHash == w.AlternativeShapeHash {
			return fmt.Errorf("semantic candidate correction witness requires distinct rejected and alternative shape hashes")
		}
	case RuntimeCandidateCorrectionWitnessSchemaField:
		if w.Path == "" || w.AlternativeValue == "" || w.RejectedValue == w.AlternativeValue {
			return fmt.Errorf("schema-field candidate correction witness requires path and a concrete different alternative")
		}
		if (w.RejectedShapeHash == "") != (w.AlternativeShapeHash == "") ||
			(w.RejectedShapeHash != "" && w.RejectedShapeHash == w.AlternativeShapeHash) {
			return fmt.Errorf("schema-field candidate correction witness shape hashes must be absent together or present and distinct")
		}
	case RuntimeCandidateCorrectionWitnessParameterEntryEncoding:
		if w.OperationKind != "" || w.Path != "$.operations" ||
			w.RejectedValue != RuntimeCandidateParameterEncodingInlineSiblings ||
			w.AlternativeValue != RuntimeCandidateParameterEncodingNamedEntries ||
			w.RejectedShapeHash != "" || w.AlternativeShapeHash != "" {
			return fmt.Errorf("parameter-entry candidate correction witness requires only the bounded encoding alternative")
		}
		if err := validateRuntimeCandidateParameterEntryEncodingFacts(w.ParameterEntryEncodings); err != nil {
			return err
		}
	case RuntimeCandidateCorrectionWitnessOwnerConstraint:
		if w.OperationKind == "" || w.Path == "" || w.ConstraintRef == "" || w.RejectedValue == "" || w.AlternativeValue == "" || w.RejectedValue == w.AlternativeValue || w.RejectedShapeHash != "" || w.AlternativeShapeHash != "" {
			return fmt.Errorf("owner-constraint candidate correction witness requires kind, path, constraint and a bounded different shape")
		}
		switch RuntimeOperationCandidateActualShape(w.RejectedValue) {
		case RuntimeOperationCandidateActualShapeInvalidSyntax,
			RuntimeOperationCandidateActualShapeTargetMismatch,
			RuntimeOperationCandidateActualShapeTargetModeMismatch,
			RuntimeOperationCandidateActualShapeObservedTargetContextMismatch:
		default:
			return fmt.Errorf("owner-constraint candidate correction witness has undeclared actual shape %q", w.RejectedValue)
		}
		switch RuntimeOperationCandidateRequiredShape(w.AlternativeValue) {
		case RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile,
			RuntimeOperationCandidateRequiredShapeUnifiedDiffAuthorizedTargetMode,
			RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget,
			RuntimeOperationCandidateRequiredShapeTextReplaceApplicableToObservedTarget:
		default:
			return fmt.Errorf("owner-constraint candidate correction witness has undeclared required shape %q", w.AlternativeValue)
		}
	case RuntimeCandidateCorrectionWitnessOperationValueRule:
		if w.OperationKind == "" || w.Path == "" || w.ConstraintRef == "" || w.RejectedValue == "" || w.AlternativeValue == "" || w.RejectedValue == w.AlternativeValue || w.RejectedShapeHash != "" || w.AlternativeShapeHash != "" {
			return fmt.Errorf("operation-value-rule correction witness requires kind, path, rule and bounded value shapes")
		}
		if !runtimeOperationValueShapesCompatible(RuntimeOperationValueActualShape(w.RejectedValue), RuntimeOperationValueRequiredShape(w.AlternativeValue)) {
			return fmt.Errorf("operation-value-rule correction witness has incompatible shapes %q -> %q", w.RejectedValue, w.AlternativeValue)
		}
	case RuntimeCandidateCorrectionWitnessOperationSetCardinality:
		if w.OperationKind != "" || w.Path != "$.operations" || w.AlternativeValue != "one_operation_per_owner_target" ||
			w.RejectedValue == "" || w.RejectedShapeHash == "" || w.AlternativeShapeHash != "" || w.ConstraintRef != "" ||
			w.OperationSetViolation == nil {
			return fmt.Errorf("operation-set candidate correction witness requires the exact owner/target violation and consolidation alternative")
		}
		if err := w.OperationSetViolation.Validate(); err != nil || w.RejectedValue != w.OperationSetViolation.OwnerModuleID+"|"+w.OperationSetViolation.TargetRef {
			return fmt.Errorf("operation-set candidate correction witness differs from its typed violation")
		}
	default:
		return fmt.Errorf("candidate correction witness dimension %q is not declared", w.Dimension)
	}
	if w.Dimension != RuntimeCandidateCorrectionWitnessOwnerConstraint && w.Dimension != RuntimeCandidateCorrectionWitnessOperationValueRule && w.ConstraintRef != "" {
		return fmt.Errorf("candidate correction witness dimension %q cannot carry a constraint ref", w.Dimension)
	}
	return nil
}

func (w *RuntimeCandidateCorrectionWitness) Clone() *RuntimeCandidateCorrectionWitness {
	if w == nil {
		return nil
	}
	value := w.Normalized()
	return &value
}

// RuntimeCandidateCorrectionDirective is the pure structural delta for the one
// permitted same-manifest correction. It is deliberately free of raw
// provider/decoder/owner error text and of Runtime action selection.
type RuntimeCandidateCorrectionDirective struct {
	Disposition  RuntimeCandidateCorrectionDisposition
	Objectives   []RuntimeCandidateCorrectionObjective
	ManifestHash string
	// Witness is required for correctable_once. It is persisted with attempt 0
	// and revalidated against the same manifest before attempt 1 may perform any
	// provider or capability-owner call.
	Witness *RuntimeCandidateCorrectionWitness
	// TerminalCode is required when Disposition is rejected_final and empty
	// otherwise. It is the stable terminal code SM-3 must seal; it is not a
	// free-form reason.
	TerminalCode string
}

func (d RuntimeCandidateCorrectionDirective) Normalized() RuntimeCandidateCorrectionDirective {
	d.Disposition = RuntimeCandidateCorrectionDisposition(strings.TrimSpace(string(d.Disposition)))
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.TerminalCode = strings.TrimSpace(d.TerminalCode)
	seen := map[RuntimeCandidateCorrectionObjective]struct{}{}
	objectives := make([]RuntimeCandidateCorrectionObjective, 0, len(d.Objectives))
	for _, objective := range d.Objectives {
		objective = RuntimeCandidateCorrectionObjective(strings.TrimSpace(string(objective)))
		if objective == "" {
			continue
		}
		if _, found := seen[objective]; found {
			continue
		}
		seen[objective] = struct{}{}
		objectives = append(objectives, objective)
	}
	// Stable sort via the existing Normalized path on a diagnostic shell.
	d.Objectives = (&RuntimeCandidateFailureDiagnostic{Objectives: objectives}).Normalized().Objectives
	d.Witness = d.Witness.Clone()
	return d
}

func (d *RuntimeCandidateCorrectionDirective) Clone() *RuntimeCandidateCorrectionDirective {
	if d == nil {
		return nil
	}
	value := d.Normalized()
	return &value
}

func (d RuntimeCandidateCorrectionDirective) Validate() error {
	d = d.Normalized()
	switch d.Disposition {
	case RuntimeCandidateCorrectionCorrectableOnce:
		if len(d.Objectives) == 0 {
			return fmt.Errorf("correctable_once directive requires at least one objective")
		}
		if d.ManifestHash == "" {
			return fmt.Errorf("correctable_once directive requires manifest identity")
		}
		if d.Witness == nil {
			return fmt.Errorf("correctable_once directive requires a concrete correction witness")
		}
		if err := d.Witness.Validate(); err != nil {
			return err
		}
		if d.TerminalCode != "" {
			return fmt.Errorf("correctable_once directive must not carry a terminal code")
		}
		return nil
	case RuntimeCandidateCorrectionRejectedFinal:
		if d.ManifestHash == "" {
			return fmt.Errorf("rejected_final directive requires manifest identity")
		}
		if d.TerminalCode == "" {
			return fmt.Errorf("rejected_final directive requires a stable terminal code")
		}
		if len(d.Objectives) != 0 {
			return fmt.Errorf("rejected_final directive must not carry correction objectives")
		}
		if d.Witness != nil {
			return fmt.Errorf("rejected_final directive must not carry a correction witness")
		}
		return nil
	default:
		return fmt.Errorf("candidate correction disposition %q is not declared", d.Disposition)
	}
}
