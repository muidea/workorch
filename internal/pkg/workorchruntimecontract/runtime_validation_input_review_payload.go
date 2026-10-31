package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
)

func validateRuntimeValidationInputReviewPayload(path string, raw json.RawMessage) error {
	var review map[string]json.RawMessage
	if err := json.Unmarshal(raw, &review); err != nil || review == nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: path, Actual: "non_object", Required: []string{"object"}}
	}
	if err := validateRuntimeSemanticObjectFields(path, review, []string{"subjectHash", "checks", "diagnosticSources"}, []string{"subjectHash", "checks"}); err != nil {
		return err
	}
	if raw, exists := review["diagnosticSources"]; exists {
		var sources []json.RawMessage
		if json.Unmarshal(raw, &sources) != nil || len(sources) == 0 || len(sources) > RuntimeValidationDiagnosticSourceLimit {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".diagnosticSources", Actual: "empty_or_unbounded_mapping", Required: []string{"complete bounded diagnostic source array"}}
		}
		for index, raw := range sources {
			var source map[string]json.RawMessage
			p := fmt.Sprintf("%s.diagnosticSources[%d]", path, index)
			if json.Unmarshal(raw, &source) != nil || source == nil {
				return &RuntimeCandidatePayloadDiagnostic{Path: p, Actual: "non_object", Required: []string{"object"}}
			}
			fields := []string{"sourceRef", "conclusion", "reason", "inputTargetRefs"}
			if err := validateRuntimeSemanticObjectFields(p, source, fields, fields); err != nil {
				return err
			}
		}
	}
	var checks []json.RawMessage
	if err := json.Unmarshal(review["checks"], &checks); err != nil || len(checks) == 0 {
		return &RuntimeCandidatePayloadDiagnostic{Path: path + ".checks", Actual: "empty_or_non_array", Required: []string{"complete check array"}}
	}
	for i, rawCheck := range checks {
		var check map[string]json.RawMessage
		checkPath := fmt.Sprintf("%s.checks[%d]", path, i)
		if err := json.Unmarshal(rawCheck, &check); err != nil || check == nil {
			return &RuntimeCandidatePayloadDiagnostic{Path: checkPath, Actual: "non_object", Required: []string{"object"}}
		}
		fields := []string{"requiredCheckRef", "conclusion", "reason", "inputTargetRefs"}
		if err := validateRuntimeSemanticObjectFields(checkPath, check, append(append([]string(nil), fields...), "obligations"), fields); err != nil {
			return err
		}
		if raw, exists := check["obligations"]; exists {
			var obligations []json.RawMessage
			if err := json.Unmarshal(raw, &obligations); err != nil || len(obligations) == 0 || len(obligations) > RuntimeValidationInputReviewMaxObligations {
				return &RuntimeCandidatePayloadDiagnostic{Path: checkPath + ".obligations", Actual: "empty_or_unbounded_mapping", Required: []string{"complete bounded obligation array"}}
			}
			for j, rawObligation := range obligations {
				var obligation map[string]json.RawMessage
				path := fmt.Sprintf("%s.obligations[%d]", checkPath, j)
				if err := json.Unmarshal(rawObligation, &obligation); err != nil || obligation == nil {
					return &RuntimeCandidatePayloadDiagnostic{Path: path, Actual: "non_object", Required: []string{"object"}}
				}
				fields := []string{"criterionText", "finding", "reason", "inputTargetRefs"}
				if err := validateRuntimeSemanticObjectFields(path, obligation, append(append([]string(nil), fields...), "proofConsumerRefs"), fields); err != nil {
					return err
				}
				if raw, exists := obligation["proofConsumerRefs"]; exists {
					var refs []string
					if err := json.Unmarshal(raw, &refs); err != nil || len(refs) == 0 || len(refs) > MaxRuntimeLineageAssertions+1 {
						return &RuntimeCandidatePayloadDiagnostic{Path: path + ".proofConsumerRefs", Actual: "empty_or_unbounded_mapping", Required: []string{"complete bounded consumer array"}}
					}
				}
			}
		}
	}
	return nil
}

func addRuntimeSemanticInputReviewSchema(proposal map[string]any, manifest RuntimeSemanticPlanManifest) {
	var applicable []any
	var rules []any
	for _, catalog := range manifest.OwnerCatalogs {
		for _, surface := range catalog.ValidationSurfaces {
			for _, binding := range surface.ProofRequirementBindings {
				if binding.InputReview == nil {
					continue
				}
				condition := map[string]any{
					"required": []string{"coverageWorkItemId", "ownerModuleId", "operationKind", "validationRecipeId", "requiredEvidenceKinds"},
					"properties": map[string]any{
						"coverageWorkItemId":    map[string]any{"const": string(binding.CoverageWorkItemID)},
						"ownerModuleId":         map[string]any{"const": catalog.OwnerModuleID},
						"operationKind":         map[string]any{"const": surface.OperationKind},
						"validationRecipeId":    map[string]any{"const": binding.ValidationRecipeID},
						"requiredEvidenceKinds": map[string]any{"contains": map[string]any{"const": EvidenceKindValidation}},
					},
				}
				applicable = append(applicable, condition)
				refs := make([]string, 0, len(binding.InputReview.Files))
				for _, file := range binding.InputReview.Files {
					refs = append(refs, file.TargetRef)
				}
				checkSchema := map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"requiredCheckRef", "conclusion", "reason", "inputTargetRefs"},
					"properties": map[string]any{
						"requiredCheckRef": stringEnumSchema(binding.RequiredCheckRefs),
						"conclusion":       stringEnumSchema([]string{RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive}),
						"reason":           map[string]any{"type": "string", "minLength": 1, "maxLength": RuntimeValidationInputReviewMaxReasonBytes},
						"inputTargetRefs":  stringEnumArraySchemaWithMaximum(refs, len(refs), 1),
					},
				}

				checkSchema["required"] = append(checkSchema["required"].([]string), "obligations")
				checkSchema["properties"].(map[string]any)["obligations"] = map[string]any{
					"type": "array", "minItems": 1, "maxItems": RuntimeValidationInputReviewMaxObligations,
					"items": map[string]any{
						"type": "object", "additionalProperties": false,
						"required": []string{"criterionText", "finding", "reason", "inputTargetRefs"},
						"properties": map[string]any{
							"criterionText":   map[string]any{"type": "string", "minLength": 1, "maxLength": RuntimeValidationInputReviewMaxReasonBytes},
							"finding":         stringEnumSchema([]string{RuntimeValidationInputCovered, RuntimeValidationInputMissing, RuntimeValidationInputConflicting, RuntimeValidationInputUnknown}),
							"reason":          map[string]any{"type": "string", "minLength": 1, "maxLength": RuntimeValidationInputReviewMaxReasonBytes},
							"inputTargetRefs": stringEnumArraySchemaWithMaximum(refs, len(refs), 1),
						},
					},
				}

				if len(binding.InputReview.LineageAssertions) != 0 {

					item := checkSchema["properties"].(map[string]any)["obligations"].(map[string]any)["items"].(map[string]any)
					item["required"] = append(item["required"].([]string), "proofConsumerRefs")
					consumerRefs := RuntimeValidationProofConsumerRefs(*binding.InputReview)
					item["properties"].(map[string]any)["proofConsumerRefs"] = stringEnumArraySchemaWithMaximum(consumerRefs, len(consumerRefs), 1)

				}

				var allChecks []any
				for _, check := range binding.RequiredCheckRefs {
					allChecks = append(allChecks, map[string]any{"contains": map[string]any{"required": []string{"requiredCheckRef"}, "properties": map[string]any{"requiredCheckRef": map[string]any{"const": check}}}})
				}
				reviewSchema := map[string]any{
					"type": "object", "additionalProperties": false, "required": []string{"subjectHash", "checks"},
					"properties": map[string]any{
						"subjectHash": map[string]any{"const": binding.InputReview.Hash},
						"checks":      map[string]any{"type": "array", "minItems": len(binding.RequiredCheckRefs), "maxItems": len(binding.RequiredCheckRefs), "items": checkSchema, "allOf": allChecks},
					},
				}
				if len(binding.InputReview.DiagnosticSources) != 0 {
					var sourceRefs []string
					var sourceRules []any
					for _, source := range binding.InputReview.DiagnosticSources {
						sourceRefs = append(sourceRefs, source.SourceRef)
						sourceRules = append(sourceRules, map[string]any{"contains": map[string]any{"required": []string{"sourceRef"}, "properties": map[string]any{"sourceRef": map[string]any{"const": source.SourceRef}}}})
					}
					reviewSchema["required"] = append(reviewSchema["required"].([]string), "diagnosticSources")
					reviewSchema["properties"].(map[string]any)["diagnosticSources"] = map[string]any{"type": "array", "minItems": len(sourceRefs), "maxItems": len(sourceRefs), "allOf": sourceRules, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"sourceRef", "conclusion", "reason", "inputTargetRefs"}, "properties": map[string]any{"sourceRef": stringEnumSchema(sourceRefs), "conclusion": stringEnumSchema([]string{RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive}), "reason": map[string]any{"type": "string", "minLength": 1, "maxLength": RuntimeValidationInputReviewMaxReasonBytes}, "inputTargetRefs": stringEnumArraySchemaWithMaximum(refs, len(refs), 1)}}}
				}
				rules = append(rules, map[string]any{"if": condition, "then": map[string]any{"required": []string{"validationInputReview"}, "properties": map[string]any{"validationInputReview": reviewSchema}}})
			}
		}
	}
	if len(applicable) == 0 {
		return
	}
	proposal["properties"].(map[string]any)["validationInputReview"] = map[string]any{"type": "object"}
	rules = append(rules, map[string]any{"if": map[string]any{"required": []string{"validationInputReview"}}, "then": map[string]any{"anyOf": applicable}})
	proposal["allOf"] = append(proposal["allOf"].([]any), rules...)
}
