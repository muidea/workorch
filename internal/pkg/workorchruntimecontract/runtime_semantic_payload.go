package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

var (
	runtimeSemanticTopLevelFields = []string{"definitionSeedHash", "documentKind", "manifestHash", "proposals", "schemaVersion"}
	runtimeSemanticProposalFields = []string{"artifactRefs", "coverageWorkItemId", "declarationRef", "metricConstraints", "operationKind", "operationVariantId", "ownerModuleId", "requiredCheckRefs", "requiredEffectScopes", "requiredEvidenceKinds", "target", "validationRecipeId"}
	runtimeSemanticTargetFields   = []string{"exactTargetRefs", "ownerScopeTargetRefs", "ownerSelectorRefs", "policy", "prospectiveConstraintRefs", "prospectiveTargetRef"}
)

type runtimeSemanticPlanPayload struct {
	DocumentKind       RuntimeProviderDocumentKind      `json:"documentKind"`
	SchemaVersion      string                           `json:"schemaVersion"`
	ManifestHash       string                           `json:"manifestHash"`
	DefinitionSeedHash string                           `json:"definitionSeedHash"`
	Proposals          []runtimeSemanticProposalPayload `json:"proposals"`
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
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: wire.SchemaVersion, ManifestHash: wire.ManifestHash, DefinitionSeedHash: wire.DefinitionSeedHash,
		Proposals: make([]RuntimeSemanticProofProposal, 0, len(wire.Proposals)),
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
		Proposals: make([]runtimeSemanticProposalPayload, 0, len(document.Proposals)),
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
	if err := validateRuntimeSemanticObjectFields("$", envelope, runtimeSemanticTopLevelFields, []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "proposals"}); err != nil {
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
		"required": []string{"documentKind", "schemaVersion", "manifestHash", "definitionSeedHash", "proposals"},
		"properties": map[string]any{
			"documentKind":       map[string]any{"const": string(RuntimeProviderDocumentSemanticPlan)},
			"schemaVersion":      map[string]any{"const": RuntimeSemanticPlanDocumentSchemaCurrent},
			"manifestHash":       map[string]any{"const": manifest.Hash},
			"definitionSeedHash": map[string]any{"const": manifest.DefinitionSeedHash},
			"proposals": map[string]any{"type": "array", "minItems": len(coverageIDs), "maxItems": len(coverageIDs), "items": map[string]any{
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

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func stringEnumArraySchema(values []string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": normalizeStrings(values)}}
}
