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
	runtimeSemanticTopLevelFields = []string{"batchCoverageWorkItemIds", "batchId", "batchSequence", "coverageOrder", "definitionSeedHash", "documentKind", "manifestHash", "planStep", "proposals", "schemaVersion"}
	runtimeSemanticProposalFields = []string{"artifactRefs", "coverageWorkItemId", "declarationRef", "metricConstraints", "operationKind", "operationVariantId", "ownerModuleId", "requiredCheckRefs", "requiredEffectScopes", "requiredEvidenceKinds", "target", "validationRecipeId"}
	runtimeSemanticTargetFields   = []string{"exactTargetRefs", "ownerScopeTargetRefs", "ownerSelectorRefs", "policy", "prospectiveConstraintRefs", "prospectiveTargetRef"}
)

type runtimeSemanticPlanPayload struct {
	DocumentKind             RuntimeProviderDocumentKind      `json:"documentKind"`
	SchemaVersion            string                           `json:"schemaVersion"`
	ManifestHash             string                           `json:"manifestHash"`
	DefinitionSeedHash       string                           `json:"definitionSeedHash"`
	PlanStep                 RuntimeSemanticPlanStep          `json:"planStep,omitempty"`
	BatchID                  string                           `json:"batchId,omitempty"`
	BatchSequence            int                              `json:"batchSequence,omitempty"`
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID      `json:"batchCoverageWorkItemIds,omitempty"`
	CoverageOrder            []common.CoverageWorkItemID      `json:"coverageOrder"`
	Proposals                []runtimeSemanticProposalPayload `json:"proposals"`
}

type runtimeSemanticProposalPayload struct {
	CoverageWorkItemID    common.CoverageWorkItemID    `json:"coverageWorkItemId"`
	OwnerModuleID         string                       `json:"ownerModuleId"`
	OperationKind         string                       `json:"operationKind"`
	OperationVariantID    string                       `json:"operationVariantId,omitempty"`
	Target                runtimeSemanticTargetPayload `json:"target"`
	RequiredEvidenceKinds []string                     `json:"requiredEvidenceKinds"`
	RequiredEffectScopes  []RuntimeEffectScope         `json:"requiredEffectScopes"`
	ValidationRecipeID    string                       `json:"validationRecipeId,omitempty"`
	RequiredCheckRefs     []string                     `json:"requiredCheckRefs,omitempty"`
	MetricConstraints     []string                     `json:"metricConstraints,omitempty"`
	ArtifactRefs          []string                     `json:"artifactRefs,omitempty"`
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
	if len(wire.CoverageOrder) == 0 {
		return RuntimeSemanticPlanDocument{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.coverageOrder", Actual: "missing_or_empty", Allowed: []string{"complete ordered CWI list"}, Cause: fmt.Errorf("runtime semantic plan must include provider-authored coverage order")}
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: wire.SchemaVersion, ManifestHash: wire.ManifestHash, DefinitionSeedHash: wire.DefinitionSeedHash,
		BatchID: wire.BatchID, BatchSequence: wire.BatchSequence, BatchCoverageWorkItemIDs: wire.BatchCoverageWorkItemIDs,
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
			RequiredEvidenceKinds: value.RequiredEvidenceKinds, RequiredEffectScopes: value.RequiredEffectScopes,
			ValidationRecipeID: value.ValidationRecipeID, RequiredCheckRefs: value.RequiredCheckRefs,
			MetricConstraints: value.MetricConstraints, ArtifactRefs: value.ArtifactRefs, DeclarationRef: value.DeclarationRef,
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
		BatchID: document.BatchID, BatchSequence: document.BatchSequence, BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), document.BatchCoverageWorkItemIDs...),
		PlanStep:      document.PlanStep,
		CoverageOrder: append([]common.CoverageWorkItemID(nil), document.CoverageOrder...),
		Proposals:     make([]runtimeSemanticProposalPayload, 0, len(document.Proposals)),
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
			RequiredEvidenceKinds: value.RequiredEvidenceKinds, RequiredEffectScopes: value.RequiredEffectScopes,
			ValidationRecipeID: value.ValidationRecipeID, RequiredCheckRefs: value.RequiredCheckRefs,
			MetricConstraints: value.MetricConstraints, ArtifactRefs: value.ArtifactRefs, DeclarationRef: value.DeclarationRef,
		})
	}
	return json.Marshal(wire)
}

func validateRuntimeSemanticPayloadFields(payload []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: strings.TrimSpace(err.Error()), Allowed: runtimeSemanticTopLevelFields, Cause: err}
	}
	if err := validateRuntimeSemanticObjectFields("$", envelope, runtimeSemanticTopLevelFields, []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "coverageOrder", "proposals"}); err != nil {
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
		if err := validateRuntimeSemanticObjectFields(fmt.Sprintf("$.proposals[%d]", idx), proposal, runtimeSemanticProposalFields, []string{"coverageWorkItemId", "ownerModuleId", "operationKind", "target", "requiredEvidenceKinds", "requiredEffectScopes", "declarationRef"}); err != nil {
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
	for _, item := range manifest.CoverageItems {
		coverageIDs = append(coverageIDs, string(item.ID))
	}
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
		for _, recipe := range catalog.ValidationRecipes {
			recipeIDs = append(recipeIDs, recipe.ID)
		}
	}
	targetRefs = normalizeStrings(targetRefs)
	selectorRefs = normalizeStrings(selectorRefs)
	constraintRefs = normalizeStrings(constraintRefs)
	recipeIDs = normalizeStrings(recipeIDs)
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "coverageOrder", "proposals"},
		"properties": map[string]any{
			"documentKind":             map[string]any{"const": string(RuntimeProviderDocumentSemanticPlan)},
			"schemaVersion":            map[string]any{"const": RuntimeSemanticPlanDocumentSchemaCurrent},
			"manifestHash":             map[string]any{"const": manifest.Hash},
			"definitionSeedHash":       map[string]any{"const": manifest.DefinitionSeedHash},
			"planStep":                 map[string]any{"enum": []string{string(RuntimeSemanticPlanStepCoverageOrder), string(RuntimeSemanticPlanStepProofBatch)}},
			"batchId":                  map[string]any{"type": "string"},
			"batchSequence":            map[string]any{"type": "integer", "minimum": 0},
			"batchCoverageWorkItemIds": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": coverageIDs}},
			"coverageOrder":            map[string]any{"type": "array", "minItems": len(coverageIDs), "maxItems": len(coverageIDs), "items": map[string]any{"type": "string", "enum": coverageIDs}},
			"proposals": map[string]any{"type": "array", "minItems": 0, "maxItems": len(coverageIDs), "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"coverageWorkItemId", "ownerModuleId", "operationKind", "target", "requiredEvidenceKinds", "requiredEffectScopes", "declarationRef"},
				"properties": map[string]any{
					"coverageWorkItemId": map[string]any{"type": "string", "enum": coverageIDs},
					"ownerModuleId":      map[string]any{"type": "string", "enum": manifest.AllowedOwnerModuleIDs},
					"operationKind":      map[string]any{"type": "string", "enum": manifest.AllowedOperationKinds},
					"operationVariantId": map[string]any{"type": "string"},
					"target": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy"}, "properties": map[string]any{
						"policy": map[string]any{"type": "string", "enum": policies}, "exactTargetRefs": stringEnumArraySchema(targetRefs),
						"ownerScopeTargetRefs": stringEnumArraySchema(targetRefs), "ownerSelectorRefs": stringEnumArraySchema(selectorRefs),
						"prospectiveConstraintRefs": stringEnumArraySchema(constraintRefs), "prospectiveTargetRef": map[string]any{"type": "string", "enum": targetRefs},
					}},
					"requiredEvidenceKinds": stringArraySchema(), "requiredEffectScopes": stringArraySchema(),
					"validationRecipeId": map[string]any{"type": "string", "enum": recipeIDs}, "requiredCheckRefs": stringArraySchema(),
					"metricConstraints": stringArraySchema(), "artifactRefs": stringArraySchema(), "declarationRef": map[string]any{"type": "string"},
				},
			}},
		},
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
		schema["required"] = []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "coverageOrder", "proposals"}
		delete(properties, "batchId")
		delete(properties, "batchSequence")
		delete(properties, "batchCoverageWorkItemIds")
		properties["proposals"] = map[string]any{"type": "array", "maxItems": 0}
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
		schema["required"] = []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "planStep", "batchId", "batchSequence", "batchCoverageWorkItemIds", "coverageOrder", "proposals"}
		proposals, ok := properties["proposals"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposals are missing")
		}
		proposals["minItems"] = len(coverageIDs)
		proposals["maxItems"] = len(coverageIDs)
		items, ok := proposals["items"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposal items are missing")
		}
		itemProperties, ok := items["properties"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("runtime semantic proof-batch schema proposal properties are missing")
		}
		itemProperties["coverageWorkItemId"] = map[string]any{"type": "string", "enum": coverageIDs}
	default:
		return "", fmt.Errorf("runtime semantic plan has invalid request step %q", step)
	}
	encoded, err := json.Marshal(schema)
	return string(encoded), err
}

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func stringEnumArraySchema(values []string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": normalizeStrings(values)}}
}
