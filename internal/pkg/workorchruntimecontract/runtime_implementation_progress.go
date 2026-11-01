package workorchruntimecontract

import "fmt"

// A review is a model proposal against an original source, never readiness or
// completion credit. Owners verify exact set coverage and purpose consistency.
type RuntimeImplementationRequirementReview struct {
	RequirementRef string `json:"requirementRef"`
	WorkRequired   bool   `json:"workRequired"`
}

// RuntimeImplementationRequirementRefs uses only sealed original identities.
// A declaration without finer source checks remains one indivisible obligation.
func RuntimeImplementationRequirementRefs(m RuntimeSemanticPlanManifest, w RuntimeSemanticImplementationWork, declaration string) []string {
	for _, item := range m.CoverageItems {
		if item.ID != w.Scope.Request.CoverageWorkItemID {
			continue
		}
		if len(item.SourceObligationChecks) != 0 {
			refs := make([]string, len(item.SourceObligationChecks))
			for n, check := range item.SourceObligationChecks {
				refs[n] = check.CheckRef
			}
			return refs
		}
	}
	return []string{declaration}
}

func validateRuntimeImplementationProgress(d RuntimeSemanticPlanDocument, m RuntimeSemanticPlanManifest, w *RuntimeSemanticImplementationWork, declaration string) error {
	i := d.ImplementationIntent
	if w == nil {
		if i != nil && (len(i.RequirementReviews) != 0 || len(i.SelectedRequirementRefs) != 0) {
			return fmt.Errorf("implementation progress requires an implementation work scope")
		}
		return nil
	}
	if w.Scope.Content == nil || i == nil {
		return fmt.Errorf("implementation progress requires exact current content")
	}
	refs := RuntimeImplementationRequirementRefs(m, *w, declaration)
	if len(i.RequirementReviews) != len(refs) {
		return fmt.Errorf("implementation progress requires every original requirement exactly once")
	}
	gaps := make(map[string]bool, len(refs))
	for n, review := range i.RequirementReviews {
		if review.RequirementRef != refs[n] {
			return fmt.Errorf("implementation progress has missing, reordered or foreign original requirement")
		}
		if review.WorkRequired {
			gaps[review.RequirementRef] = true
		}
	}
	if i.PreviousRoundReview != nil && i.PreviousRoundReview.Outcome == "inconclusive" && d.ImplementationPurpose != RuntimeImplementationWorkPurposeInconclusive {
		return fmt.Errorf("inconclusive previous work requires an explicit inconclusive observation")
	}
	if d.ImplementationPurpose == RuntimeImplementationWorkPurposeTerminal {
		if len(gaps) != 0 || len(i.SelectedRequirementRefs) != 0 || (i.PreviousRoundReview != nil && i.PreviousRoundReview.Outcome != "addressed") {
			return fmt.Errorf("terminal proof purpose contradicts proposed remaining implementation")
		}
		return nil
	}
	if d.ImplementationPurpose == RuntimeImplementationWorkPurposeIncomplete {
		if w.EffectAvailable || len(gaps) == 0 || len(i.SelectedRequirementRefs) != 0 {
			return fmt.Errorf("incomplete observation requires exhausted work budget and original gaps without effect selection")
		}
		return nil
	}
	if d.ImplementationPurpose == RuntimeImplementationWorkPurposeInconclusive {
		if len(i.SelectedRequirementRefs) != 0 {
			return fmt.Errorf("inconclusive observation cannot select implementation effects")
		}
		return nil
	}

	if d.ImplementationPurpose != RuntimeImplementationWorkPurposeEffect || len(i.SelectedRequirementRefs) == 0 {
		return fmt.Errorf("implementation effect requires a selected original implementation gap")
	}
	seen := make(map[string]bool, len(i.SelectedRequirementRefs))
	for _, ref := range i.SelectedRequirementRefs {
		if !gaps[ref] || seen[ref] {
			return fmt.Errorf("implementation selection must uniquely reference proposed original gaps")
		}
		seen[ref] = true
	}
	return nil
}

// This grammar mirrors the deterministic owner checks. The model selects only
// semantic booleans/remaining work; exact original identities are code-derived.
func addRuntimeImplementationProgressSchema(schema, intent map[string]any, refs []string, work *RuntimeSemanticImplementationWork) {
	fields := intent["properties"].(map[string]any)
	review := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"requirementRef", "workRequired"}, "properties": map[string]any{"requirementRef": map[string]any{"type": "string", "enum": refs}, "workRequired": map[string]any{"type": "boolean"}}}
	positions := make([]any, len(refs))
	for n, ref := range refs {
		positions[n] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"requirementRef", "workRequired"}, "properties": map[string]any{"requirementRef": map[string]any{"const": ref}, "workRequired": map[string]any{"type": "boolean"}}}
	}
	fields["requirementReviews"] = map[string]any{"type": "array", "minItems": len(refs), "maxItems": len(refs), "items": review, "prefixItems": positions}
	fields["selectedRequirementRefs"] = map[string]any{"type": "array", "maxItems": len(refs), "uniqueItems": true, "items": map[string]any{"type": "string", "enum": refs}}
	intent["required"] = append(intent["required"].([]string), "requirementReviews")
	if p := work.PreviousRound; p != nil {
		fields["previousRoundReview"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"dispatchId", "receiptSetHash", "outcome", "reason"}, "properties": map[string]any{
			"dispatchId": map[string]any{"const": p.DispatchID}, "receiptSetHash": map[string]any{"const": p.ReceiptSetHash}, "outcome": map[string]any{"enum": []string{"addressed", "partial", "diverged", "inconclusive"}}, "reason": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
		}}
		intent["required"] = append(intent["required"].([]string), "previousRoundReview")
	}
	allOf, _ := schema["allOf"].([]any)
	for _, purpose := range []RuntimeImplementationWorkPurpose{RuntimeImplementationWorkPurposeEffect, RuntimeImplementationWorkPurposeTerminal, RuntimeImplementationWorkPurposeIncomplete, RuntimeImplementationWorkPurposeInconclusive} {
		constraints := map[string]any{}
		required := []string{"requirementReviews"}
		if purpose == RuntimeImplementationWorkPurposeEffect {
			required = append(required, "selectedRequirementRefs")
			constraints["selectedRequirementRefs"] = map[string]any{"minItems": 1}
			if work.PreviousRound != nil {
				constraints["previousRoundReview"] = map[string]any{"properties": map[string]any{"outcome": map[string]any{"enum": []string{"addressed", "partial", "diverged"}}}}
			}
		} else if purpose == RuntimeImplementationWorkPurposeTerminal {
			constraints["selectedRequirementRefs"] = map[string]any{"maxItems": 0}
			constraints["requirementReviews"] = map[string]any{"items": map[string]any{"properties": map[string]any{"workRequired": map[string]any{"const": false}}}}
			if work.PreviousRound != nil {
				constraints["previousRoundReview"] = map[string]any{"properties": map[string]any{"outcome": map[string]any{"const": "addressed"}}}
			}
		} else {
			constraints["selectedRequirementRefs"] = map[string]any{"maxItems": 0}
			if purpose == RuntimeImplementationWorkPurposeIncomplete {
				if work.PreviousRound != nil {
					constraints["previousRoundReview"] = map[string]any{"properties": map[string]any{"outcome": map[string]any{"enum": []string{"addressed", "partial", "diverged"}}}}
				}
				constraints["requirementReviews"] = map[string]any{"contains": map[string]any{"required": []string{"workRequired"}, "properties": map[string]any{"workRequired": map[string]any{"const": true}}}}
			}
		}
		allOf = append(allOf, map[string]any{"if": map[string]any{"required": []string{"implementationPurpose"}, "properties": map[string]any{"implementationPurpose": map[string]any{"const": purpose}}}, "then": map[string]any{"properties": map[string]any{"implementationIntent": map[string]any{"required": required, "properties": constraints}}}})
	}
	for _, ref := range refs {
		allOf = append(allOf, map[string]any{
			"if":   map[string]any{"required": []string{"implementationIntent"}, "properties": map[string]any{"implementationIntent": map[string]any{"required": []string{"selectedRequirementRefs"}, "properties": map[string]any{"selectedRequirementRefs": map[string]any{"contains": map[string]any{"const": ref}}}}}},
			"then": map[string]any{"properties": map[string]any{"implementationIntent": map[string]any{"properties": map[string]any{"requirementReviews": map[string]any{"contains": map[string]any{"required": []string{"requirementRef", "workRequired"}, "properties": map[string]any{"requirementRef": map[string]any{"const": ref}, "workRequired": map[string]any{"const": true}}}}}}}},
		})
	}
	if work.PreviousRound != nil {
		retained := []any{}
		for _, ref := range work.PreviousRound.IntentBinding.Intent.SelectedRequirementRefs {
			retained = append(retained, map[string]any{"properties": map[string]any{"requirementReviews": map[string]any{"contains": map[string]any{"required": []string{"requirementRef", "workRequired"}, "properties": map[string]any{"requirementRef": map[string]any{"const": ref}, "workRequired": map[string]any{"const": true}}}}}})
			retained = append(retained, map[string]any{"if": map[string]any{"required": []string{"selectedRequirementRefs"}, "properties": map[string]any{"selectedRequirementRefs": map[string]any{"minItems": 1}}}, "then": map[string]any{"properties": map[string]any{"selectedRequirementRefs": map[string]any{"contains": map[string]any{"const": ref}}}}})
		}
		allOf = append(allOf, map[string]any{"if": map[string]any{"properties": map[string]any{"implementationIntent": map[string]any{"properties": map[string]any{"previousRoundReview": map[string]any{"properties": map[string]any{"outcome": map[string]any{"enum": []string{"partial", "diverged"}}}}}}}}, "then": map[string]any{"properties": map[string]any{"implementationIntent": map[string]any{"allOf": retained}}}})
	}
	schema["allOf"] = allOf
}
