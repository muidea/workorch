package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
)

// The provider supplies semantic observations, not redundant owner-derived
// reference sets. The persisted report retains the complete, exact typed scope.
type runtimeValidationInputReviewPayload struct {
	SubjectHash       string                                     `json:"subjectHash"`
	Checks            []runtimeValidationInputCheckReviewPayload `json:"checks"`
	DiagnosticSources []runtimeValidationDiagnosticSourcePayload `json:"diagnosticSources,omitempty"`
}

type runtimeValidationInputCheckReviewPayload struct {
	RequiredCheckRef     string                                   `json:"requiredCheckRef"`
	Conclusion           string                                   `json:"conclusion"`
	Reason               string                                   `json:"reason"`
	UncertaintyKind      string                                   `json:"uncertaintyKind,omitempty"`
	DependencyTargetRefs []string                                 `json:"dependencyTargetRefs,omitempty"`
	Obligations          []RuntimeValidationInputObligationReview `json:"obligations"`
}

type runtimeValidationDiagnosticSourcePayload struct {
	SourceRef                 string   `json:"sourceRef"`
	Conclusion                string   `json:"conclusion"`
	Reason                    string   `json:"reason"`
	SupportingInputTargetRefs []string `json:"supportingInputTargetRefs,omitempty"`
	UncertaintyKind           string   `json:"uncertaintyKind,omitempty"`
}

func runtimeInputObligationRefs(obligations []RuntimeValidationInputObligationReview) []string {
	var refs []string
	for _, obligation := range obligations {
		refs = append(refs, obligation.InputTargetRefs...)
	}
	return normalizeStrings(refs)
}

// Binding selection consumes the already admitted manifest. It supplies only
// the declared source->file relation; complete report admission still runs once
// at document admission, and no consistency judgment is compiled here.
func (w *runtimeValidationInputReviewPayload) compile(p RuntimeSemanticProofProposal, catalogs []RuntimeOperationAdmissibility) (*RuntimeValidationInputReview, error) {
	if w == nil {
		return nil, nil
	}
	binding, found := RuntimeSemanticValidationInputReviewBinding(p, catalogs)
	if !found {
		return nil, runtimeInputViolation("", "undeclared_input_review", "review bound to the exact selected owner/recipe/CWI declaration", "semantic proposal cannot introduce an undeclared input review")
	}
	r := &RuntimeValidationInputReview{SubjectHash: w.SubjectHash}
	for _, check := range w.Checks {
		r.Checks = append(r.Checks, RuntimeValidationInputCheckReview{
			RequiredCheckRef: check.RequiredCheckRef, Conclusion: check.Conclusion, Reason: check.Reason,
			UncertaintyKind: check.UncertaintyKind, DependencyTargetRefs: slices.Clone(check.DependencyTargetRefs),
			Obligations: check.Obligations, InputTargetRefs: runtimeInputObligationRefs(check.Obligations),
		})
	}
	for i, source := range w.DiagnosticSources {
		path := fmt.Sprintf(".diagnosticSources[%d]", i)
		var input string
		for _, declared := range binding.InputReview.DiagnosticSources {
			if declared.SourceRef == source.SourceRef {
				input = declared.InputTargetRef
				break
			}
		}
		if input == "" {
			return nil, runtimeInputViolation(path+".sourceRef", "undeclared_diagnostic_source", "exact sourceRef from the selected owner subject", "diagnostic source has no declared input")
		}
		if !reflect.DeepEqual(source.SupportingInputTargetRefs, normalizeStrings(source.SupportingInputTargetRefs)) || slices.Contains(source.SupportingInputTargetRefs, input) {
			return nil, runtimeInputViolation(path+".supportingInputTargetRefs", "noncanonical_supporting_refs", "omit or supply nonempty unique sorted observed refs excluding the owner-bound source input", "supporting refs cannot repeat the fixed source input or hide noncanonical references")
		}
		refs := append([]string{input}, source.SupportingInputTargetRefs...)
		r.DiagnosticSources = append(r.DiagnosticSources, RuntimeValidationDiagnosticSourceReview{
			SourceRef: source.SourceRef, Conclusion: source.Conclusion, Reason: source.Reason,
			InputTargetRefs: normalizeStrings(refs), UncertaintyKind: source.UncertaintyKind,
		})
	}
	detached := r.Clone()
	return &detached, nil
}

func runtimeValidationInputReviewToWire(r *RuntimeValidationInputReview, p RuntimeSemanticProofProposal, catalogs []RuntimeOperationAdmissibility) (*runtimeValidationInputReviewPayload, error) {
	if r == nil {
		return nil, nil
	}
	binding, found := RuntimeSemanticValidationInputReviewBinding(p, catalogs)
	if !found {
		return nil, fmt.Errorf("input review wire has no selected owner binding")
	}
	w := &runtimeValidationInputReviewPayload{SubjectHash: r.SubjectHash}
	for _, check := range r.Checks {
		if !reflect.DeepEqual(check.InputTargetRefs, runtimeInputObligationRefs(check.Obligations)) {
			return nil, fmt.Errorf("input review check refs must equal its complete obligation reference union")
		}
		w.Checks = append(w.Checks, runtimeValidationInputCheckReviewPayload{
			RequiredCheckRef: check.RequiredCheckRef, Conclusion: check.Conclusion, Reason: check.Reason,
			UncertaintyKind: check.UncertaintyKind, DependencyTargetRefs: check.DependencyTargetRefs, Obligations: check.Obligations,
		})
	}
	for _, source := range r.DiagnosticSources {
		var input string
		for _, declared := range binding.InputReview.DiagnosticSources {
			if declared.SourceRef == source.SourceRef {
				input = declared.InputTargetRef
				break
			}
		}
		if input == "" || !slices.Contains(source.InputTargetRefs, input) || !reflect.DeepEqual(source.InputTargetRefs, normalizeStrings(source.InputTargetRefs)) {
			return nil, fmt.Errorf("input review wire has no exact diagnostic source input")
		}
		var supporting []string
		for _, ref := range source.InputTargetRefs {
			if ref != input {
				supporting = append(supporting, ref)
			}
		}
		w.DiagnosticSources = append(w.DiagnosticSources, runtimeValidationDiagnosticSourcePayload{
			SourceRef: source.SourceRef, Conclusion: source.Conclusion, Reason: source.Reason,
			UncertaintyKind: source.UncertaintyKind, SupportingInputTargetRefs: supporting,
		})
	}
	return w, nil
}
