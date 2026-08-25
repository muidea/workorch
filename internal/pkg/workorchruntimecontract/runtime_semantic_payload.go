package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

var (
	runtimeSemanticTopLevelFields = []string{"batchCoverageWorkItemIds", "batchId", "batchSequence", "coverageOrder", "definitionSeedHash", "diagnosticMessage", "documentKind", "gapKind", "manifestHash", "offendingItemRefs", "planStep", "proposals", "resultKind", "schemaVersion"}
	runtimeSemanticProposalFields = []string{"coverageWorkItemId", "declarationRef", "operationKind", "operationVariantId", "ownerModuleId", "requiredEvidenceKinds", "target", "validationRecipeId"}
	runtimeSemanticTargetFields   = []string{"exactTargetRefs", "ownerScopeTargetRefs", "ownerSelectorRefs", "policy", "prospectiveConstraintRefs", "prospectiveTargetRef"}
)

type runtimeSemanticPlanPayload struct {
	DocumentKind             RuntimeProviderDocumentKind      `json:"documentKind"`
	SchemaVersion            string                           `json:"schemaVersion"`
	ManifestHash             string                           `json:"manifestHash"`
	DefinitionSeedHash       string                           `json:"definitionSeedHash"`
	PlanStep                 RuntimeSemanticPlanStep          `json:"planStep,omitempty"`
	BatchID                  string                           `json:"batchId,omitempty"`
	BatchSequence            *int                             `json:"batchSequence,omitempty"`
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID      `json:"batchCoverageWorkItemIds,omitempty"`
	CoverageOrder            []common.CoverageWorkItemID      `json:"coverageOrder,omitempty"`
	Proposals                []runtimeSemanticProposalPayload `json:"proposals"`
}

type runtimeSemanticProposalPayload struct {
	CoverageWorkItemID    common.CoverageWorkItemID    `json:"coverageWorkItemId"`
	OwnerModuleID         string                       `json:"ownerModuleId"`
	OperationKind         string                       `json:"operationKind"`
	OperationVariantID    string                       `json:"operationVariantId,omitempty"`
	Target                runtimeSemanticTargetPayload `json:"target"`
	RequiredEvidenceKinds []string                     `json:"requiredEvidenceKinds"`
	ValidationRecipeID    string                       `json:"validationRecipeId,omitempty"`
	DeclarationRef        string                       `json:"declarationRef"`
}

type runtimeSemanticTargetPayload struct {
	Policy                    RuntimeOperationTargetPolicy `json:"policy"`
	ExactTargetRefs           []string                     `json:"exactTargetRefs,omitempty"`
	OwnerScopeTargetRefs      []string                     `json:"ownerScopeTargetRefs,omitempty"`
	OwnerSelectorRefs         []string                     `json:"ownerSelectorRefs,omitempty"`
	ProspectiveConstraintRefs []string                     `json:"prospectiveConstraintRefs,omitempty"`
	ProspectiveTargetRef      string                       `json:"prospectiveTargetRef,omitempty"`
}

// deriveRuntimeSemanticProposalEffects compiles the internal proposal effect
// from sealed owner facts. It is deliberately used only after strict wire
// decoding; the provider never submits this value.
func deriveRuntimeSemanticProposalEffects(manifest RuntimeSemanticPlanManifest, value runtimeSemanticProposalPayload) []RuntimeEffectScope {
	proposal := RuntimeSemanticProofProposal{
		OwnerModuleID: value.OwnerModuleID, OperationKind: value.OperationKind,
		OperationVariantID: value.OperationVariantID, ValidationRecipeID: value.ValidationRecipeID,
		RequiredEvidenceKinds: value.RequiredEvidenceKinds,
	}
	for _, catalog := range manifest.OwnerCatalogs {
		if catalog.OwnerModuleID != proposal.OwnerModuleID {
			continue
		}
		if proposal.ValidationRecipeID != "" {
			if surface, ok := RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, proposal.ValidationRecipeID); ok {
				effect := NormalizeRuntimeEffectScope(surface.EffectScope)
				if effect != RuntimeEffectUnknown {
					return []RuntimeEffectScope{effect}
				}
			}
		}
		if proposal.OperationVariantID != "" {
			if contract, ok := RuntimeOperationContractForKind(proposal.OperationKind); ok {
				for _, variant := range contract.Variants {
					if variant.ID == proposal.OperationVariantID {
						effect := NormalizeRuntimeEffectScope(variant.EffectScope)
						if effect != RuntimeEffectUnknown {
							return []RuntimeEffectScope{effect}
						}
					}
				}
			}
		}
		break
	}
	for _, kind := range value.RequiredEvidenceKinds {
		if kind == EvidenceKindStateChange {
			return []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}
		}
	}
	return []RuntimeEffectScope{RuntimeEffectNone}
}

// DecodeRuntimeSemanticPlanPayloadWithManifest is the sole strict semantic
// document decoder. It is pure and returns proposal facts only; no binding,
// retry, action or terminal decision can be produced here.
func DecodeRuntimeSemanticPlanPayloadWithManifest(manifest RuntimeSemanticPlanManifest, payload []byte) (RuntimeSemanticPlanDocument, error) {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: "unsealed_semantic_manifest", Cause: err}
	}
	if err := validateRuntimeSemanticPayloadFields(payload); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	var wire runtimeSemanticPlanPayload
	if err := strictDecodeRuntimeCandidatePayload(payload, &wire); err != nil {
		field := runtimeCandidatePayloadUnknownField(err)
		path := "$"
		if field != "" {
			path = "$." + field
		}
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: path, Actual: strings.TrimSpace(err.Error()), Allowed: runtimeSemanticTopLevelFields, Cause: err}
	}
	if wire.DocumentKind != RuntimeProviderDocumentSemanticPlan {
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.documentKind", Actual: string(wire.DocumentKind), Allowed: []string{string(RuntimeProviderDocumentSemanticPlan)}, Cause: fmt.Errorf("runtime semantic document kind is invalid")}
	}
	if wire.PlanStep == RuntimeSemanticPlanStepCoverageOrder && len(wire.CoverageOrder) == 0 {
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.coverageOrder", Actual: "missing_or_empty", Allowed: []string{"complete ordered CWI list"}, Cause: fmt.Errorf("runtime semantic plan must include provider-authored coverage order")}
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: wire.SchemaVersion, ManifestHash: wire.ManifestHash, DefinitionSeedHash: wire.DefinitionSeedHash,
		BatchID: wire.BatchID, BatchSequence: runtimeSemanticBatchSequence(wire.BatchSequence), BatchCoverageWorkItemIDs: wire.BatchCoverageWorkItemIDs,
		PlanStep:      wire.PlanStep,
		CoverageOrder: wire.CoverageOrder,
		Proposals:     make([]RuntimeSemanticProofProposal, 0, len(wire.Proposals)),
	}
	for _, value := range wire.Proposals {
		document.Proposals = append(document.Proposals, RuntimeSemanticProofProposal{
			CoverageWorkItemID: value.CoverageWorkItemID, OwnerModuleID: value.OwnerModuleID,
			OperationKind: value.OperationKind, OperationVariantID: value.OperationVariantID,
			Target: RuntimeOperationTargetAuthorization{
				Policy: value.Target.Policy, ExactTargetRefs: value.Target.ExactTargetRefs,
				OwnerScopeTargetRefs: value.Target.OwnerScopeTargetRefs, OwnerSelectorRefs: value.Target.OwnerSelectorRefs,
				ProspectiveConstraintRefs: value.Target.ProspectiveConstraintRefs, ProspectiveTargetRef: value.Target.ProspectiveTargetRef,
			},
			RequiredEvidenceKinds: value.RequiredEvidenceKinds, RequiredEffectScopes: deriveRuntimeSemanticProposalEffects(manifest, value),
			ValidationRecipeID: value.ValidationRecipeID,
			DeclarationRef:     value.DeclarationRef,
		})
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		var dependencyOrder *RuntimeSemanticDependencyOrderError
		if errors.As(err, &dependencyOrder) {
			violation := dependencyOrder.Violation()
			return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{
				Path:               violation.Path,
				Actual:             fmt.Sprintf("%s at %d before %s at %d", violation.RequiredAfterID, violation.ActualAfterPosition, violation.RequiredBeforeID, violation.ActualBeforePosition),
				Required:           []string{fmt.Sprintf("%s before %s", violation.RequiredBeforeID, violation.RequiredAfterID)},
				SemanticViolations: []RuntimeSemanticContractViolation{violation},
				Cause:              err,
			}
		}
		var proposalConstraint *RuntimeSemanticProposalViolationError
		if errors.As(err, &proposalConstraint) {
			violation := proposalConstraint.Violation()
			return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{
				Path: violation.Path, Actual: violation.Actual, Allowed: append([]string(nil), violation.Allowed...),
				Required: append([]string(nil), violation.Required...), SemanticViolations: []RuntimeSemanticContractViolation{violation}, Cause: err,
			}
		}
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.proposals", Actual: "contract_violation", Allowed: runtimeSemanticProposalFields, Cause: err}
	}
	return document, nil
}

// EncodeRuntimeSemanticPlanPayloadWithManifest is used by deterministic
// fixtures and mock providers. Production decoding always uses the strict path
// above and never trusts this encoder as authority.
func EncodeRuntimeSemanticPlanPayloadWithManifest(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument) ([]byte, error) {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return nil, err
	}
	wire := runtimeSemanticPlanPayload{
		DocumentKind: RuntimeProviderDocumentSemanticPlan, SchemaVersion: document.SchemaVersion,
		ManifestHash: document.ManifestHash, DefinitionSeedHash: document.DefinitionSeedHash,
		BatchID: document.BatchID, BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), document.BatchCoverageWorkItemIDs...),
		PlanStep:  document.PlanStep,
		Proposals: make([]runtimeSemanticProposalPayload, 0, len(document.Proposals)),
	}
	if document.PlanStep == RuntimeSemanticPlanStepCoverageOrder {
		wire.CoverageOrder = append([]common.CoverageWorkItemID(nil), document.CoverageOrder...)
	} else if document.PlanStep == RuntimeSemanticPlanStepProofBatch {
		sequence := document.BatchSequence
		wire.BatchSequence = &sequence
	}
	for _, value := range document.Proposals {
		wire.Proposals = append(wire.Proposals, runtimeSemanticProposalPayload{
			CoverageWorkItemID: value.CoverageWorkItemID, OwnerModuleID: value.OwnerModuleID,
			OperationKind: value.OperationKind, OperationVariantID: value.OperationVariantID,
			Target: runtimeSemanticTargetPayload{
				Policy: value.Target.Policy, ExactTargetRefs: value.Target.ExactTargetRefs,
				OwnerScopeTargetRefs: value.Target.OwnerScopeTargetRefs, OwnerSelectorRefs: value.Target.OwnerSelectorRefs,
				ProspectiveConstraintRefs: value.Target.ProspectiveConstraintRefs, ProspectiveTargetRef: value.Target.ProspectiveTargetRef,
			},
			RequiredEvidenceKinds: value.RequiredEvidenceKinds,
			ValidationRecipeID:    value.ValidationRecipeID,
			DeclarationRef:        value.DeclarationRef,
		})
	}
	return json.Marshal(wire)
}

func runtimeSemanticBatchSequence(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func validateRuntimeSemanticPayloadFields(payload []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: strings.TrimSpace(err.Error()), Allowed: runtimeSemanticTopLevelFields, Cause: err}
	}
	var planStep RuntimeSemanticPlanStep
	if raw, found := envelope["planStep"]; found {
		_ = json.Unmarshal(raw, &planStep)
	}
	allowed := runtimeSemanticTopLevelFields
	required := []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "proposals"}
	switch planStep {
	case RuntimeSemanticPlanStepCoverageOrder:
		required = append(required, "coverageOrder")
	case RuntimeSemanticPlanStepProofBatch:
		allowed = []string{"batchCoverageWorkItemIds", "batchId", "batchSequence", "definitionSeedHash", "documentKind", "manifestHash", "planStep", "proposals", "schemaVersion"}
		required = append(required, "batchId", "batchSequence", "batchCoverageWorkItemIds")
	}
	if err := validateRuntimeSemanticObjectFields("$", envelope, allowed, required); err != nil {
		return err
	}
	var proposals []json.RawMessage
	if err := json.Unmarshal(envelope["proposals"], &proposals); err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$.proposals", Actual: "non_array", Allowed: []string{"array"}, Cause: err}
	}
	for idx, raw := range proposals {
		var proposal map[string]json.RawMessage
		if err := json.Unmarshal(raw, &proposal); err != nil {
			return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("$.proposals[%d]", idx), Actual: "non_object", Allowed: []string{"object"}, Cause: err}
		}
		if err := validateRuntimeSemanticObjectFields(fmt.Sprintf("$.proposals[%d]", idx), proposal, runtimeSemanticProposalFields, []string{"coverageWorkItemId", "ownerModuleId", "operationKind", "target", "requiredEvidenceKinds", "declarationRef"}); err != nil {
			return err
		}
		var target map[string]json.RawMessage
		if err := json.Unmarshal(proposal["target"], &target); err != nil {
			return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("$.proposals[%d].target", idx), Actual: "non_object", Allowed: []string{"object"}, Cause: err}
		}
		if err := validateRuntimeSemanticObjectFields(fmt.Sprintf("$.proposals[%d].target", idx), target, runtimeSemanticTargetFields, []string{"policy"}); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeSemanticObjectFields(path string, object map[string]json.RawMessage, allowed, required []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range object {
		if _, found := allowedSet[field]; !found {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return &RuntimeCandidatePayloadDiagnostic{Path: path + "." + unknown[0], Actual: "unknown_field", Allowed: append([]string(nil), allowed...), Cause: fmt.Errorf("runtime semantic document contains unknown field %q", unknown[0])}
	}
	missing := make([]string, 0)
	for _, field := range required {
		if _, found := object[field]; !found {
			missing = append(missing, field)
		}
	}
	if len(missing) != 0 {
		return &RuntimeCandidatePayloadDiagnostic{Path: path, Actual: "missing_required", Allowed: append([]string(nil), allowed...), Required: missing, Cause: fmt.Errorf("runtime semantic document is missing required field(s)")}
	}
	return nil
}

// RuntimeSemanticPlanJSONSchema renders the request-specific semantic schema
// exclusively from the sealed manifest.
func RuntimeSemanticPlanJSONSchema(manifest RuntimeSemanticPlanManifest) (string, error) {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	coverageIDs := make([]string, 0, len(manifest.CoverageItems))
	definitionRefs := make([]string, 0, len(manifest.CoverageItems)*2)
	for _, item := range manifest.CoverageItems {
		coverageIDs = append(coverageIDs, string(item.ID))
		definitionRefs = append(definitionRefs, string(item.ID), item.SourceID)
	}
	definitionRefs = normalizeStrings(definitionRefs)
	policies := make([]string, 0, len(manifest.AllowedTargetPolicies))
	for _, policy := range manifest.AllowedTargetPolicies {
		policies = append(policies, string(policy))
	}
	targetRefs := make([]string, 0)
	selectorRefs := make([]string, 0)
	constraintRefs := make([]string, 0)
	recipeIDs := make([]string, 0)
	for _, catalog := range manifest.OwnerCatalogs {
		targetRefs = append(targetRefs, catalog.TargetRefs...)
		selectorRefs = append(selectorRefs, catalog.SelectorRefs...)
		constraintRefs = append(constraintRefs, catalog.ConstraintRefs...)
		// Dynamic capability owners (for example MCP) publish opaque recipe
		// identities through ValidationRecipeRefs, while Shell may additionally
		// publish executable RuntimeValidationRecipe values.  Both forms are
		// sealed owner facts and must be represented in the bounded provider
		// enum; omitting refs leaves the top-level enum empty and makes the
		// owner-scoped rule unsatisfiable for dynamic recipes.
		recipeIDs = append(recipeIDs, catalog.ValidationRecipeRefs...)
		for _, recipe := range catalog.ValidationRecipes {
			recipeIDs = append(recipeIDs, recipe.ID)
		}
	}
	targetRefs = normalizeStrings(targetRefs)
	selectorRefs = normalizeStrings(selectorRefs)
	constraintRefs = normalizeStrings(constraintRefs)
	recipeIDs = normalizeStrings(recipeIDs)
	if targetRefs == nil {
		targetRefs = []string{}
	}
	if selectorRefs == nil {
		selectorRefs = []string{}
	}
	if constraintRefs == nil {
		constraintRefs = []string{}
	}
	if recipeIDs == nil {
		recipeIDs = []string{}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "proposals"},
		"properties": map[string]any{
			"documentKind":             map[string]any{"const": string(RuntimeProviderDocumentSemanticPlan)},
			"schemaVersion":            map[string]any{"const": RuntimeSemanticPlanDocumentSchemaCurrent},
			"manifestHash":             map[string]any{"const": manifest.Hash},
			"definitionSeedHash":       map[string]any{"const": manifest.DefinitionSeedHash},
			"resultKind":               map[string]any{"const": RuntimeDefinitionGapResultKind},
			"gapKind":                  map[string]any{"type": "string", "enum": RuntimeDefinitionClarificationGapKinds()},
			"offendingItemRefs":        stringEnumArraySchemaWithMaximum(definitionRefs, RuntimeDefinitionClarificationMaxOffendingRefs, 1),
			"diagnosticMessage":        map[string]any{"type": "string", "minLength": 1, "maxLength": RuntimeDefinitionClarificationMaxDiagnosticBytes},
			"planStep":                 map[string]any{"enum": []string{string(RuntimeSemanticPlanStepCoverageOrder), string(RuntimeSemanticPlanStepProofBatch)}},
			"batchId":                  map[string]any{"type": "string"},
			"batchSequence":            map[string]any{"type": "integer", "minimum": 0},
			"batchCoverageWorkItemIds": map[string]any{"type": "array", "maxItems": len(coverageIDs), "items": map[string]any{"type": "string", "enum": coverageIDs}},
			"coverageOrder":            map[string]any{"type": "array", "minItems": len(coverageIDs), "maxItems": len(coverageIDs), "items": map[string]any{"type": "string", "enum": coverageIDs}},
			"proposals": map[string]any{"type": "array", "minItems": 0, "maxItems": len(coverageIDs) * 2, "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"coverageWorkItemId", "ownerModuleId", "operationKind", "target", "requiredEvidenceKinds", "declarationRef"},
				"properties": map[string]any{
					"coverageWorkItemId": map[string]any{"type": "string", "enum": coverageIDs},
					"ownerModuleId":      map[string]any{"type": "string", "enum": manifest.AllowedOwnerModuleIDs},
					"operationKind":      map[string]any{"type": "string", "enum": manifest.AllowedOperationKinds},
					"operationVariantId": map[string]any{"type": "string", "maxLength": 65536},
					"target": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy"}, "properties": map[string]any{
						"policy": map[string]any{"type": "string", "enum": policies}, "exactTargetRefs": stringEnumArraySchema(targetRefs),
						"ownerScopeTargetRefs": stringEnumArraySchema(targetRefs), "ownerSelectorRefs": stringEnumArraySchema(selectorRefs),
						"prospectiveConstraintRefs": stringEnumArraySchema(constraintRefs), "prospectiveTargetRef": map[string]any{"type": "string", "minLength": 1, "maxLength": 65536},
					}},
					"requiredEvidenceKinds": stringEnumArraySchemaWithMinimum([]string{EvidenceKindStateChange, EvidenceKindValidation}, 1),
					"validationRecipeId":    stringEnumSchema(recipeIDs),
					"declarationRef":        map[string]any{"type": "string", "minLength": 1, "maxLength": 65536},
				},
			}},
		},
	}
	// Conditional constraints are kept beside the canonical proposal schema so
	// validation-only proofs cannot omit their recipe/check facts while state
	// change proofs remain free of validation-only requirements.
	proposalItems := schema["properties"].(map[string]any)["proposals"].(map[string]any)["items"].(map[string]any)
	targetSchema := proposalItems["properties"].(map[string]any)["target"].(map[string]any)
	targetSchema["oneOf"] = []any{
		map[string]any{
			"properties": map[string]any{"policy": map[string]any{"const": string(RuntimeOperationTargetPolicyExactTarget)}, "exactTargetRefs": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": targetRefs}}},
			"required":   []string{"policy", "exactTargetRefs"},
		},
		map[string]any{
			"properties": map[string]any{
				"policy":               map[string]any{"const": string(RuntimeOperationTargetPolicyExistingOwnerScope)},
				"ownerScopeTargetRefs": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": targetRefs}},
				"ownerSelectorRefs":    map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": selectorRefs}},
			},
			"required": []string{"policy"},
			"anyOf":    []any{map[string]any{"required": []string{"ownerScopeTargetRefs"}}, map[string]any{"required": []string{"ownerSelectorRefs"}}},
		},
		map[string]any{
			"properties": map[string]any{
				"policy":                    map[string]any{"const": string(RuntimeOperationTargetPolicyProspectiveOwnerTarget)},
				"ownerSelectorRefs":         map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": selectorRefs}},
				"prospectiveConstraintRefs": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": constraintRefs}},
				// A prospective target is intentionally not enum-bound to the
				// current target catalog: it denotes the resource to be created
				// under the sealed selector/constraint facts.
				"prospectiveTargetRef": map[string]any{"type": "string", "minLength": 1},
			},
			"required": []string{"policy", "ownerSelectorRefs", "prospectiveConstraintRefs", "prospectiveTargetRef"},
		},
	}
	// Validation is an owner capability, not a global property. The owner
	// catalog rules below add recipe/check requirements only for owners that
	// actually publish validation recipes; owners without recipes cannot borrow
	// another owner's recipe merely because a CWI is in a validation slice.
	proposalItems["allOf"] = []any{}
	// Only owners with a typed validation observation may produce validation
	// proof. A plain shell command is an executable operation, not a validation
	// receipt, even when its command happens to run a test.
	proposalItems["allOf"] = append(proposalItems["allOf"].([]any), map[string]any{
		"if": map[string]any{"properties": map[string]any{
			"requiredEvidenceKinds": map[string]any{"contains": map[string]any{"const": EvidenceKindValidation}},
		}},
		"then": map[string]any{"properties": map[string]any{
			"operationKind": map[string]any{"enum": []string{StepKindMCPToolCall, RuntimeBlockKindManagedValidationSession}},
		}},
	})
	// Dynamic capability operations do not have static RuntimeOperationVariant
	// contracts. Their owner-scoped identity is carried by validationRecipeId
	// (for MCP tools) or the owner feasibility facts, never by operationVariantId.
	for _, kind := range []string{StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet, StepKindShellCommand, StepKindFileList, StepKindFileRead, StepKindFileWrite, StepKindFileEdit} {
		proposalItems["allOf"] = append(proposalItems["allOf"].([]any), map[string]any{
			"if":   map[string]any{"properties": map[string]any{"operationKind": map[string]any{"const": kind}}},
			"then": map[string]any{"not": map[string]any{"required": []string{"operationVariantId"}}},
		})
	}
	// Every owner receives only its own sealed catalog values. The manifest is
	// the bounded per-run authority; no process-global registry is introduced.
	for _, catalog := range manifest.OwnerCatalogs {
		catalog = catalog.Normalized()
		ownerRule := map[string]any{
			"if": map[string]any{"properties": map[string]any{"ownerModuleId": map[string]any{"const": catalog.OwnerModuleID}}},
			"then": map[string]any{"properties": map[string]any{
				"operationKind":      map[string]any{"type": "string", "enum": normalizeStrings(catalog.OperationKinds)},
				"validationRecipeId": stringEnumSchema(catalog.ValidationRecipeRefs),
				"target": map[string]any{"properties": map[string]any{
					"exactTargetRefs":           stringEnumArraySchema(catalog.TargetRefs),
					"ownerScopeTargetRefs":      stringEnumArraySchema(catalog.TargetRefs),
					"ownerSelectorRefs":         stringEnumArraySchema(catalog.SelectorRefs),
					"prospectiveConstraintRefs": stringEnumArraySchema(catalog.ConstraintRefs),
				}},
			}},
		}
		then := ownerRule["then"].(map[string]any)
		if len(catalog.ValidationRecipeRefs) == 0 {
			then["allOf"] = []any{map[string]any{
				"if": map[string]any{"properties": map[string]any{
					"requiredEvidenceKinds": map[string]any{"contains": map[string]any{"const": EvidenceKindValidation}},
				}},
				"then": map[string]any{"properties": map[string]any{
					"requiredEvidenceKinds": map[string]any{"not": map[string]any{"contains": map[string]any{"const": EvidenceKindValidation}}},
				}},
			}}
		} else {
			then["allOf"] = []any{map[string]any{
				"if": map[string]any{"properties": map[string]any{
					"requiredEvidenceKinds": map[string]any{"contains": map[string]any{"const": EvidenceKindValidation}},
				}},
				"then": map[string]any{
					"required": []string{"validationRecipeId"},
					"properties": map[string]any{
						"validationRecipeId": stringEnumSchemaWithMinimum(catalog.ValidationRecipeRefs, 1),
					},
				},
			}}
		}
		proposalItems["allOf"] = append(proposalItems["allOf"].([]any), ownerRule)
		// Narrow target policy and selector/constraint enums by the concrete
		// owner operation fact. The broad owner rule above remains a vocabulary
		// envelope; these conditions prevent a selector from one File variant (or
		// another owner) from authorizing a different variant.
		for _, kind := range catalog.OperationKinds {
			kind = CanonicalRuntimeBlockKind(kind)
			contract, contractFound := RuntimeOperationContractForKind(kind)
			if !contractFound || len(contract.Variants) == 0 {
				allowedPolicies := runtimeSemanticOwnerTargetPolicies(catalog, kind, "")
				policyValues := make([]string, 0, len(allowedPolicies))
				for _, policy := range allowedPolicies {
					policyValues = append(policyValues, string(policy))
				}
				proposalItems["allOf"] = append(proposalItems["allOf"].([]any), map[string]any{
					"if": map[string]any{"properties": map[string]any{
						"ownerModuleId": map[string]any{"const": catalog.OwnerModuleID},
						"operationKind": map[string]any{"const": kind},
					}},
					"then": map[string]any{"properties": map[string]any{
						"target": map[string]any{"properties": map[string]any{"policy": stringEnumSchema(policyValues)}},
					}},
				})
				continue
			}
			// A static operation must carry an owner-admissible concrete variant.
			// Do not first enumerate the wider technical operation contract: a
			// provider correction must never be told to choose a variant that this
			// sealed owner catalog did not publish.
			variantIDs := runtimeSemanticCatalogVariantIDs(catalog, kind)
			proposalItems["allOf"] = append(proposalItems["allOf"].([]any), map[string]any{
				"if": map[string]any{"properties": map[string]any{
					"ownerModuleId": map[string]any{"const": catalog.OwnerModuleID},
					"operationKind": map[string]any{"const": kind},
				}},
				"then": map[string]any{
					"required": []string{"operationVariantId"},
					"properties": map[string]any{
						"operationVariantId": stringEnumSchemaWithMinimum(variantIDs, 1),
					},
				},
			})
			for _, rawVariant := range catalog.OperationVariants {
				variant := rawVariant.Normalized()
				if variant.Kind != kind || variant.VariantID == "" {
					continue
				}
				policyValues := make([]string, 0, len(variant.AllowedTargetPolicies))
				for _, policy := range variant.AllowedTargetPolicies {
					policyValues = append(policyValues, string(policy))
				}
				proposalItems["allOf"] = append(proposalItems["allOf"].([]any), map[string]any{
					"if": map[string]any{"allOf": []any{
						map[string]any{"properties": map[string]any{
							"ownerModuleId": map[string]any{"const": catalog.OwnerModuleID},
							"operationKind": map[string]any{"const": kind},
						}},
						map[string]any{"required": []string{"operationVariantId"}},
						map[string]any{"properties": map[string]any{"operationVariantId": map[string]any{"const": variant.VariantID}}},
					}},
					"then": map[string]any{"properties": map[string]any{
						"target": map[string]any{"properties": map[string]any{
							"policy":                    stringEnumSchema(policyValues),
							"ownerSelectorRefs":         stringEnumArraySchema(variant.SelectorRefs),
							"prospectiveConstraintRefs": stringEnumArraySchema(variant.ConstraintRefs),
						}},
					}},
				})
			}
		}
	}
	encoded, err := json.Marshal(schema)
	return string(encoded), err
}

// RuntimeSemanticPlanJSONSchemaForRequest narrows the sealed manifest schema
// to one provider transaction. The full manifest remains the authority used by
// the evaluator; this projection only prevents an order or proof-batch request
// from authorizing fields and coverage outside its current step.
func RuntimeSemanticPlanJSONSchemaForRequest(manifest RuntimeSemanticPlanManifest, step RuntimeSemanticPlanStep, batchID string, batchSequence int, batchCoverageIDs []common.CoverageWorkItemID) (string, error) {
	full, err := RuntimeSemanticPlanJSONSchema(manifest)
	if err != nil || step == "" {
		return full, err
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(full), &schema); err != nil {
		return "", err
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("runtime semantic schema properties are missing")
	}
	properties["planStep"] = map[string]any{"const": string(step)}
	switch step {
	case RuntimeSemanticPlanStepCoverageOrder:
		normalRequired := []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "coverageOrder", "proposals"}
		delete(properties, "batchId")
		delete(properties, "batchSequence")
		delete(properties, "batchCoverageWorkItemIds")
		properties["proposals"] = map[string]any{"type": "array", "maxItems": 0}
		attachRuntimeDefinitionGapSchemaForRequest(schema, normalRequired, nil)
	case RuntimeSemanticPlanStepProofBatch:
		batchID = strings.TrimSpace(batchID)
		batchCoverageIDs = normalizeOrderedCoverageWorkItemIDs(batchCoverageIDs)
		if batchID == "" || batchSequence < 0 || len(batchCoverageIDs) == 0 {
			return "", fmt.Errorf("runtime semantic proof-batch schema requires batch identity and coverage")
		}
		manifestIDs := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
		for _, item := range manifest.CoverageItems {
			manifestIDs[item.ID] = struct{}{}
		}
		coverageIDs := make([]string, 0, len(batchCoverageIDs))
		for _, id := range batchCoverageIDs {
			if _, found := manifestIDs[id]; !found {
				return "", fmt.Errorf("runtime semantic proof-batch coverage %q is outside manifest", id)
			}
			coverageIDs = append(coverageIDs, string(id))
		}
		properties["batchId"] = map[string]any{"const": batchID}
		properties["batchSequence"] = map[string]any{"const": batchSequence}
		properties["batchCoverageWorkItemIds"] = map[string]any{"type": "array", "minItems": len(coverageIDs), "maxItems": len(coverageIDs), "uniqueItems": true, "items": map[string]any{"type": "string", "enum": coverageIDs}}
		delete(properties, "coverageOrder")
		normalRequired := []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "batchId", "batchSequence", "batchCoverageWorkItemIds", "proposals"}
		proposals, ok := properties["proposals"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposals are missing")
		}
		proposals["minItems"] = len(coverageIDs)
		proposals["maxItems"] = len(coverageIDs) * 2
		items, ok := proposals["items"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposal items are missing")
		}
		itemProperties, ok := items["properties"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposal properties are missing")
		}
		itemProperties["coverageWorkItemId"] = map[string]any{"type": "string", "enum": coverageIDs}
		if err := attachRuntimeSemanticProofObligationSchema(items, manifest, batchCoverageIDs); err != nil {
			return "", err
		}
		attachRuntimeDefinitionGapSchemaForRequest(schema, normalRequired, []string{"batchId", "batchSequence", "batchCoverageWorkItemIds"})
	default:
		return "", fmt.Errorf("runtime semantic plan has invalid request step %q", step)
	}
	encoded, err := json.Marshal(schema)
	return string(encoded), err
}

// attachRuntimeSemanticProofObligationSchema narrows each CWI to the exact
// missing-owner-fact acquisition sealed by the manifest. The Definition
// selector remains visible as lineage in the manifest projection, while only
// the capability owner's operation selector is valid in a provider proposal.
func attachRuntimeSemanticProofObligationSchema(items map[string]any, manifest RuntimeSemanticPlanManifest, coverageIDs []common.CoverageWorkItemID) error {
	allOf, ok := items["allOf"].([]any)
	if !ok {
		return fmt.Errorf("runtime semantic proposal conditional schema is missing")
	}
	obligations := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, raw := range manifest.ProofObligations {
		value := raw.Normalized()
		obligations[value.CoverageWorkItemID] = value
	}
	for _, coverageID := range normalizeOrderedCoverageWorkItemIDs(coverageIDs) {
		obligation, found := obligations[coverageID]
		if !found {
			return fmt.Errorf("runtime semantic proof-batch coverage %q has no sealed proof obligation", coverageID)
		}
		if obligation.OwnerFactAcquisition == nil {
			continue
		}
		evidenceObligation, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
		if err != nil {
			return err
		}
		acquisition := obligation.OwnerFactAcquisition.Normalized()
		targetRequired := []string{"policy", "ownerSelectorRefs"}
		targetProperties := map[string]any{
			"policy":            map[string]any{"const": string(acquisition.AcquisitionTargetPolicy)},
			"ownerSelectorRefs": exactStringArraySchema([]string{acquisition.AcquisitionTargetSelectorRef}),
		}
		if acquisition.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
			targetRequired = append(targetRequired, "prospectiveConstraintRefs", "prospectiveTargetRef")
			targetProperties["prospectiveConstraintRefs"] = exactStringArraySchema(acquisition.AcquisitionConstraintRefs)
			targetProperties["prospectiveTargetRef"] = map[string]any{"const": acquisition.ProspectiveTargetRef}
		}
		proposalProperties := map[string]any{
			"ownerModuleId":         map[string]any{"const": acquisition.AcquisitionOwnerModuleID},
			"operationKind":         map[string]any{"const": acquisition.AcquisitionOperationKind},
			"requiredEvidenceKinds": exactStringArraySchema(evidenceObligation.CurrentRequiredEvidenceKinds),
			"target": map[string]any{
				"required":   targetRequired,
				"properties": targetProperties,
			},
		}
		then := map[string]any{"properties": proposalProperties}
		if acquisition.AcquisitionOperationVariantID != "" {
			then["required"] = []string{"operationVariantId"}
			proposalProperties["operationVariantId"] = map[string]any{"const": acquisition.AcquisitionOperationVariantID}
		} else {
			then["not"] = map[string]any{"required": []string{"operationVariantId"}}
		}
		allOf = append(allOf, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"coverageWorkItemId": map[string]any{"const": string(coverageID)}}},
			"then": then,
		})
	}
	items["allOf"] = allOf
	return nil
}

func attachRuntimeDefinitionGapSchemaForRequest(schema map[string]any, normalRequired, gapBatchRequired []string) {
	commonRequired := []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep"}
	gapRequired := append(append([]string(nil), commonRequired...), "resultKind", "gapKind", "offendingItemRefs", "diagnosticMessage")
	gapRequired = append(gapRequired, gapBatchRequired...)
	schema["required"] = commonRequired
	schema["anyOf"] = []any{
		map[string]any{
			"required": normalRequired,
			"not":      map[string]any{"required": []string{"resultKind"}},
		},
		map[string]any{
			"required": gapRequired,
			"properties": map[string]any{
				"resultKind": map[string]any{"const": RuntimeDefinitionGapResultKind},
			},
			"not": map[string]any{"anyOf": []any{
				map[string]any{"required": []string{"coverageOrder"}},
				map[string]any{"required": []string{"proposals"}},
			}},
		},
	}
}

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func stringEnumSchema(values []string) map[string]any {
	values = normalizeStrings(values)
	if values == nil {
		values = []string{}
	}
	return map[string]any{"type": "string", "maxLength": 65536, "enum": values}
}

func stringEnumSchemaWithMinimum(values []string, minimum int) map[string]any {
	schema := stringEnumSchema(values)
	schema["minLength"] = minimum
	return schema
}

func stringEnumArraySchema(values []string) map[string]any {
	values = normalizeStrings(values)
	if values == nil {
		values = []string{}
	}
	return map[string]any{"type": "array", "maxItems": len(values), "uniqueItems": true, "items": map[string]any{"type": "string", "maxLength": 65536, "enum": values}}
}

func stringEnumArraySchemaWithMinimum(values []string, minimum int) map[string]any {
	schema := stringEnumArraySchema(values)
	schema["minItems"] = minimum
	schema["maxItems"] = minimum
	schema["uniqueItems"] = true
	return schema
}

func exactStringArraySchema(values []string) map[string]any {
	values = normalizeStrings(values)
	if values == nil {
		values = []string{}
	}
	item := map[string]any{"type": "string", "maxLength": 65536, "enum": values}
	if len(values) == 1 {
		item = map[string]any{"const": values[0]}
	}
	return map[string]any{
		"type": "array", "minItems": len(values), "maxItems": len(values), "uniqueItems": true,
		"items": item,
	}
}

func stringEnumArraySchemaWithMaximum(values []string, maximum, minimum int) map[string]any {
	schema := stringEnumArraySchema(values)
	if maximum > len(values) {
		maximum = len(values)
	}
	schema["minItems"] = minimum
	schema["maxItems"] = maximum
	schema["uniqueItems"] = true
	return schema
}
