package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// The provider supplies semantic observations, not redundant owner-derived
// reference sets. The persisted report retains the complete, exact typed scope.
type runtimeValidationInputReviewPayload struct {
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
	SourceIndex               *int     `json:"sourceIndex"`
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
	r := &RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
	var violations []error
	for _, check := range w.Checks {
		obligations := make([]RuntimeValidationInputObligationReview, len(check.Obligations))
		copy(obligations, check.Obligations)
		checkPath := fmt.Sprintf(".checks[%d]", len(r.Checks))
		for i := range obligations {
			path := fmt.Sprintf("%s.obligations[%d]", checkPath, i)
			refs, err := compileRuntimeInputRefSet(obligations[i].InputTargetRefs, path+".inputTargetRefs", false)
			if err != nil {
				violations = append(violations, err)
			} else {
				obligations[i].InputTargetRefs = refs
			}
			consumers, err := compileRuntimeInputRefSet(obligations[i].ProofConsumerRefs, path+".proofConsumerRefs", true)
			if err != nil {
				violations = append(violations, err)
			} else {
				obligations[i].ProofConsumerRefs = consumers
			}
		}
		dependencies, err := compileRuntimeInputRefSet(check.DependencyTargetRefs, checkPath+".dependencyTargetRefs", true)
		if err != nil {
			violations = append(violations, err)
		}
		r.Checks = append(r.Checks, RuntimeValidationInputCheckReview{
			RequiredCheckRef: check.RequiredCheckRef, Conclusion: check.Conclusion, Reason: check.Reason,
			UncertaintyKind: check.UncertaintyKind, DependencyTargetRefs: dependencies,
			Obligations: obligations, InputTargetRefs: runtimeInputObligationRefs(obligations),
		})
	}
	for i, source := range w.DiagnosticSources {
		path := fmt.Sprintf(".diagnosticSources[%d]", i)
		if source.SourceIndex == nil || *source.SourceIndex < 0 || *source.SourceIndex >= len(binding.InputReview.DiagnosticSources) {
			violations = append(violations, runtimeInputViolation(path+".sourceIndex", "undeclared_diagnostic_source", "zero-based diagnosticSources index from the selected frozen subject", "diagnostic source index has no declared input"))
			continue
		}
		declared := binding.InputReview.DiagnosticSources[*source.SourceIndex]
		input := declared.InputTargetRef
		supporting, err := compileRuntimeInputRefSet(source.SupportingInputTargetRefs, path+".supportingInputTargetRefs", true)
		if err != nil {
			violations = append(violations, err)
		}
		if slices.Contains(supporting, input) {
			violations = append(violations, runtimeInputViolation(path+".supportingInputTargetRefs", "repeated_source_anchor", "supporting refs excluding the owner-bound source input", "supporting refs repeat the fixed source input"))
		}

		refs := append([]string{input}, supporting...)
		r.DiagnosticSources = append(r.DiagnosticSources, RuntimeValidationDiagnosticSourceReview{
			SourceRef: declared.SourceRef, Conclusion: source.Conclusion, Reason: source.Reason,
			InputTargetRefs: normalizeStrings(refs), UncertaintyKind: source.UncertaintyKind,
		})
	}
	if err := joinRuntimeInputViolations(violations); err != nil {
		return nil, err
	}
	detached := r.Clone()
	return &detached, nil
}

func runtimeValidationInputReviewToWire(r *RuntimeValidationInputReview, p RuntimeSemanticProofProposal, catalogs []RuntimeOperationAdmissibility) (*runtimeValidationInputReviewPayload, error) {
	if r == nil {
		return nil, nil
	}
	detached := r.Clone()
	r = &detached
	binding, found := RuntimeSemanticValidationInputReviewBinding(p, catalogs)
	if !found {
		return nil, fmt.Errorf("input review wire has no selected owner binding")
	}
	if r.SubjectHash != binding.InputReview.Hash {
		return nil, fmt.Errorf("input review wire has a foreign subject")
	}
	w := &runtimeValidationInputReviewPayload{}
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
		sourceIndex := -1
		for i, declared := range binding.InputReview.DiagnosticSources {
			if declared.SourceRef == source.SourceRef {
				input = declared.InputTargetRef
				sourceIndex = i
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
			SourceIndex: &sourceIndex, Conclusion: source.Conclusion, Reason: source.Reason,
			UncertaintyKind: source.UncertaintyKind, SupportingInputTargetRefs: supporting,
		})
	}
	return w, nil
}

// The current provider wire represents sets without order semantics. The
// compiler rejects malformed or duplicate members before ordering a detached
// typed projection. Original provider bytes remain untouched.
func compileRuntimeInputRefSet(refs []string, path string, optional bool) ([]string, error) {
	if len(refs) == 0 {
		if optional {
			return nil, nil
		}
		return nil, runtimeInputViolation(path, "empty_input_refs", "nonempty unique observed reference set", "input reference set is empty")
	}
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref == "" || ref != strings.TrimSpace(ref) || seen[ref] {
			return nil, runtimeInputViolation(path, "invalid_reference_set", "nonempty unique exact refs; order is immaterial", "reference set contains an empty, padded or duplicate member")
		}
		seen[ref] = true
	}
	out := slices.Clone(refs)
	slices.Sort(out)
	return out, nil
}
