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
	RuntimeCandidateCorrectionWitnessOperationKind    RuntimeCandidateCorrectionWitnessDimension = "operation_kind"
	RuntimeCandidateCorrectionWitnessTarget           RuntimeCandidateCorrectionWitnessDimension = "target"
	RuntimeCandidateCorrectionWitnessVariant          RuntimeCandidateCorrectionWitnessDimension = "variant"
	RuntimeCandidateCorrectionWitnessValidationRecipe RuntimeCandidateCorrectionWitnessDimension = "validation_recipe"
	RuntimeCandidateCorrectionWitnessClosedParameter  RuntimeCandidateCorrectionWitnessDimension = "closed_semantic_parameter"
	RuntimeCandidateCorrectionWitnessSchemaField      RuntimeCandidateCorrectionWitnessDimension = "schema_field"
	RuntimeCandidateCorrectionWitnessDocumentShape    RuntimeCandidateCorrectionWitnessDimension = "document_shape"
)

// RuntimeCandidateCorrectionConformExpectedDocument is the bounded objective
// for replacing an empty/invalid/trailing provider body with exactly one
// manifest-conformant JSON document. It grants no operation authority.
const RuntimeCandidateCorrectionConformExpectedDocument RuntimeCandidateCorrectionObjective = "conform_expected_document"

const (
	RuntimeCandidateDocumentShapeEmpty              = "empty_document"
	RuntimeCandidateDocumentShapeInvalid            = "invalid_provider_output"
	RuntimeCandidateDocumentShapeTrailingContent    = "trailing_content"
	RuntimeCandidateDocumentShapeMultipleJSONValues = "multiple_json_values"
	RuntimeCandidateDocumentShapeNonObjectJSONValue = "non_object_json_value"
	RuntimeCandidateDocumentShapeConformant         = "single_manifest_conformant_json_object"
)

// RuntimeCandidateCorrectionWitness is the bounded proof carried by a
// correctable_once directive.  RejectedValue/AlternativeValue come from the
// rejected document and the sealed manifest respectively.  Semantic shape
// hashes are required for operation-level differences and intentionally omit
// operation IDs and display-only summary parameters.
type RuntimeCandidateCorrectionWitness struct {
	Dimension            RuntimeCandidateCorrectionWitnessDimension
	OperationKind        string
	Path                 string
	RejectedValue        string
	AlternativeValue     string
	RejectedShapeHash    string
	AlternativeShapeHash string
}

func (w RuntimeCandidateCorrectionWitness) Normalized() RuntimeCandidateCorrectionWitness {
	w.Dimension = RuntimeCandidateCorrectionWitnessDimension(strings.TrimSpace(string(w.Dimension)))
	w.OperationKind = strings.TrimSpace(w.OperationKind)
	w.Path = strings.TrimSpace(w.Path)
	w.RejectedValue = strings.TrimSpace(w.RejectedValue)
	w.AlternativeValue = strings.TrimSpace(w.AlternativeValue)
	w.RejectedShapeHash = strings.TrimSpace(w.RejectedShapeHash)
	w.AlternativeShapeHash = strings.TrimSpace(w.AlternativeShapeHash)
	return w
}

func (w RuntimeCandidateCorrectionWitness) Validate() error {
	w = w.Normalized()
	switch w.Dimension {
	case RuntimeCandidateCorrectionWitnessOperationKind,
		RuntimeCandidateCorrectionWitnessTarget,
		RuntimeCandidateCorrectionWitnessVariant,
		RuntimeCandidateCorrectionWitnessValidationRecipe,
		RuntimeCandidateCorrectionWitnessClosedParameter:
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
	case RuntimeCandidateCorrectionWitnessDocumentShape:
		if w.OperationKind != "" || w.Path != "$" || w.AlternativeValue != RuntimeCandidateDocumentShapeConformant ||
			w.RejectedShapeHash != "" || w.AlternativeShapeHash != "" {
			return fmt.Errorf("document-shape candidate correction witness requires only the root path and conformant-document alternative")
		}
		switch w.RejectedValue {
		case RuntimeCandidateDocumentShapeEmpty,
			RuntimeCandidateDocumentShapeInvalid,
			RuntimeCandidateDocumentShapeTrailingContent,
			RuntimeCandidateDocumentShapeMultipleJSONValues,
			RuntimeCandidateDocumentShapeNonObjectJSONValue:
		default:
			return fmt.Errorf("document-shape candidate correction witness has undeclared rejected shape %q", w.RejectedValue)
		}
	default:
		return fmt.Errorf("candidate correction witness dimension %q is not declared", w.Dimension)
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
