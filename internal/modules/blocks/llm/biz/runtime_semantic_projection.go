package llm

import (
	"encoding/json"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticCapabilityProjection struct {
	CapabilityKinds   []string                                 `json:"capabilityKinds"`
	ExecutionContract runtimecontract.RuntimeExecutionContract `json:"executionContract"`
}

type runtimeSemanticAuthorityProjection struct {
	WorkspaceID       string                                  `json:"workspaceId"`
	RunID             string                                  `json:"runId"`
	Soul              *runtimecontract.RuntimeSoulContext     `json:"soul,omitempty"`
	DefinitionSeed    runtimeSemanticDefinitionSeedProjection `json:"definitionSeed"`
	Manifest          runtimeSemanticManifestProjection       `json:"manifest"`
	Schema            json.RawMessage                         `json:"schema"`
	OutputInstruction string                                  `json:"outputInstruction"`
	PlanStep          runtimecontract.RuntimeSemanticPlanStep `json:"planStep"`
	BatchID           string                                  `json:"batchId,omitempty"`
	BatchSequence     *int                                    `json:"batchSequence,omitempty"`
	BatchCoverageIDs  []common.CoverageWorkItemID             `json:"batchCoverageWorkItemIds,omitempty"`
}

type runtimeSemanticDefinitionSeedProjection struct {
	SchemaVersion      string `json:"schemaVersion"`
	Hash               string `json:"hash"`
	DefinitionRevision string `json:"definitionRevision"`
	PolicyHash         string `json:"policyHash"`
}

type runtimeSemanticManifestProjection struct {
	SchemaVersion             string                                         `json:"schemaVersion"`
	Hash                      string                                         `json:"hash"`
	DefinitionRevision        string                                         `json:"definitionRevision"`
	DefinitionSeedHash        string                                         `json:"definitionSeedHash"`
	OperationContractRevision string                                         `json:"operationContractRevision"`
	CoverageItems             []runtimecontract.RuntimeCoverageItem          `json:"coverageItems"`
	AllowedOwnerModuleIDs     []string                                       `json:"allowedOwnerModuleIds,omitempty"`
	AllowedOperationKinds     []string                                       `json:"allowedOperationKinds,omitempty"`
	AllowedTargetPolicies     []runtimecontract.RuntimeOperationTargetPolicy `json:"allowedTargetPolicies,omitempty"`
	OwnerCatalogs             []runtimeSemanticOwnerCatalogProjection        `json:"ownerCatalogs,omitempty"`
}

// runtimeSemanticOwnerCatalogProjection is the provider-safe vocabulary from
// one sealed owner catalog. Non-ready validation surfaces, owner-private input
// bindings, executable Shell recipes and dynamic MCP effect metadata remain in
// Runtime/audit state; the provider receives only identities it may select.
type runtimeSemanticOwnerCatalogProjection struct {
	PrerequisiteID       string                                                 `json:"prerequisiteId"`
	SnapshotID           string                                                 `json:"snapshotId"`
	OwnerModuleID        string                                                 `json:"ownerModuleId"`
	OperationKinds       []string                                               `json:"operationKinds"`
	OperationVariants    []runtimecontract.RuntimeOperationAdmissibilityVariant `json:"operationVariants,omitempty"`
	TargetRefs           []string                                               `json:"targetRefs,omitempty"`
	SelectorRefs         []string                                               `json:"selectorRefs,omitempty"`
	ConstraintRefs       []string                                               `json:"constraintRefs,omitempty"`
	ValidationRecipeRefs []string                                               `json:"validationRecipeRefs,omitempty"`
	ObservedRevision     string                                                 `json:"observedRevision"`
}

type runtimeSemanticSliceProjection struct {
	ID                    string                      `json:"id"`
	Title                 string                      `json:"title,omitempty"`
	Scope                 string                      `json:"scope,omitempty"`
	Priority              int                         `json:"priority,omitempty"`
	CoverageWorkItemIDs   []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	RequiredEvidenceKinds []string                    `json:"requiredEvidenceKinds,omitempty"`
	CompletionPolicy      string                      `json:"completionPolicy,omitempty"`
	ResultPolicy          string                      `json:"resultPolicy,omitempty"`
}

func runtimeSemanticDefinitionSeedForProvider(seed runtimecontract.RuntimeDefinitionSeed) runtimeSemanticDefinitionSeedProjection {
	seed = seed.Normalized()
	return runtimeSemanticDefinitionSeedProjection{
		SchemaVersion: seed.SchemaVersion, Hash: seed.Hash,
		DefinitionRevision: seed.DefinitionRevision, PolicyHash: seed.PolicyHash,
	}
}

func runtimeSemanticManifestForProvider(manifest runtimecontract.RuntimeSemanticPlanManifest, step runtimecontract.RuntimeSemanticPlanStep, batchCoverageIDs []common.CoverageWorkItemID) runtimeSemanticManifestProjection {
	manifest = manifest.Normalized()
	selected := make(map[common.CoverageWorkItemID]struct{}, len(batchCoverageIDs))
	for _, id := range batchCoverageIDs {
		selected[id] = struct{}{}
	}
	items := make([]runtimecontract.RuntimeCoverageItem, 0, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		if step == runtimecontract.RuntimeSemanticPlanStepProofBatch {
			if _, found := selected[item.ID]; !found {
				continue
			}
		}
		items = append(items, item)
	}
	projection := runtimeSemanticManifestProjection{
		SchemaVersion: manifest.SchemaVersion, Hash: manifest.Hash,
		DefinitionRevision: manifest.DefinitionRevision, DefinitionSeedHash: manifest.DefinitionSeedHash,
		OperationContractRevision: manifest.OperationContractRevision, CoverageItems: items,
	}
	if step != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		projection.AllowedOwnerModuleIDs = append([]string(nil), manifest.AllowedOwnerModuleIDs...)
		projection.AllowedOperationKinds = append([]string(nil), manifest.AllowedOperationKinds...)
		projection.AllowedTargetPolicies = append([]runtimecontract.RuntimeOperationTargetPolicy(nil), manifest.AllowedTargetPolicies...)
		projection.OwnerCatalogs = runtimeSemanticOwnerCatalogsForProvider(manifest.OwnerCatalogs)
	}
	return projection
}

func runtimeSemanticOwnerCatalogsForProvider(values []runtimecontract.RuntimeOperationAdmissibility) []runtimeSemanticOwnerCatalogProjection {
	values = runtimecontract.NormalizeRuntimeOperationAdmissibilities(values)
	ret := make([]runtimeSemanticOwnerCatalogProjection, 0, len(values))
	for _, catalog := range values {
		catalog = catalog.Normalized()
		readyRecipes := append([]string(nil), catalog.ValidationRecipeRefs...)
		if len(catalog.ValidationSurfaces) != 0 {
			// Fail closed at the final rendering boundary as well as catalog
			// validation: a non-ready identity must never become a model hint.
			readyRecipes = runtimecontract.RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
		}
		ret = append(ret, runtimeSemanticOwnerCatalogProjection{
			PrerequisiteID: catalog.PrerequisiteID, SnapshotID: catalog.SnapshotID, OwnerModuleID: catalog.OwnerModuleID,
			OperationKinds: append([]string(nil), catalog.OperationKinds...), OperationVariants: append([]runtimecontract.RuntimeOperationAdmissibilityVariant(nil), catalog.OperationVariants...),
			TargetRefs: append([]string(nil), catalog.TargetRefs...), SelectorRefs: append([]string(nil), catalog.SelectorRefs...),
			ConstraintRefs: append([]string(nil), catalog.ConstraintRefs...), ValidationRecipeRefs: readyRecipes,
			ObservedRevision: catalog.ObservedRevision,
		})
	}
	return ret
}

func runtimeSemanticSlicesForProvider(slices []runtimecontract.RuntimeAcceptanceSlice, visibleCoverageIDs []common.CoverageWorkItemID) []runtimeSemanticSliceProjection {
	selected := make(map[common.CoverageWorkItemID]struct{}, len(visibleCoverageIDs))
	for _, id := range visibleCoverageIDs {
		selected[id] = struct{}{}
	}
	ret := make([]runtimeSemanticSliceProjection, 0, len(slices))
	for _, value := range runtimecontract.NormalizeRuntimeAcceptanceSlices(slices) {
		coverageIDs := make([]common.CoverageWorkItemID, 0, len(value.CoverageWorkItemIDs))
		for _, id := range value.CoverageWorkItemIDs {
			if _, found := selected[id]; found {
				coverageIDs = append(coverageIDs, id)
			}
		}
		if len(coverageIDs) == 0 {
			continue
		}
		ret = append(ret, runtimeSemanticSliceProjection{
			ID: value.ID, Title: value.Title, Scope: value.Scope, Priority: value.Priority,
			CoverageWorkItemIDs: coverageIDs, RequiredEvidenceKinds: append([]string(nil), value.RequiredEvidenceKinds...),
			CompletionPolicy: value.CompletionPolicy, ResultPolicy: value.ResultPolicy,
		})
	}
	return ret
}
