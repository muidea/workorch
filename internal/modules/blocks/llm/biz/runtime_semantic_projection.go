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
	ProofObligations          []runtimeSemanticProofObligationProjection     `json:"proofObligations,omitempty"`
	OwnerCatalogs             []runtimeSemanticOwnerCatalogProjection        `json:"ownerCatalogs,omitempty"`
}

type runtimeSemanticProofObligationProjection struct {
	CoverageWorkItemID                  common.CoverageWorkItemID                                `json:"coverageWorkItemId"`
	EvidencePurpose                     runtimecontract.RuntimeSemanticEvidenceObligationPurpose `json:"evidencePurpose"`
	TerminalRequiredEvidenceKinds       []string                                                 `json:"terminalRequiredEvidenceKinds"`
	CurrentRequiredEvidenceKinds        []string                                                 `json:"currentRequiredEvidenceKinds"`
	DeclarationTargetSelectorRef        string                                                   `json:"declarationTargetSelectorRef,omitempty"`
	DeclarationTargetSelectorOwnerRef   string                                                   `json:"declarationTargetSelectorOwnerRef,omitempty"`
	DeclarationTargetSelectorSubjectRef string                                                   `json:"declarationTargetSelectorSubjectRef,omitempty"`
	DeclarationTargetSelectorCandidates []string                                                 `json:"declarationTargetSelectorCandidateRefs,omitempty"`
	OwnerFactAcquisition                *runtimeSemanticOwnerFactAcquisitionProjection           `json:"ownerFactAcquisition,omitempty"`
	ValidationFailureRepair             *runtimeSemanticValidationFailureRepairProjection        `json:"validationFailureRepair,omitempty"`
}

// runtimeSemanticValidationFailureRepairProjection is the provider-safe C3
// view of one Profile-sealed repair generation. It exposes the typed failure
// identity and exact repair vocabulary needed to propose a relevant proof, but
// carries no raw diagnostic, Runtime action, retry, terminal, receipt or owner
// observation.
type runtimeSemanticValidationFailureRepairProjection struct {
	FailedFocusID                                string                                       `json:"failedFocusId"`
	ConvergenceID                                string                                       `json:"convergenceId"`
	FailureSignature                             string                                       `json:"failureSignature"`
	TriggerOwnerModuleID                         string                                       `json:"triggerOwnerModuleId"`
	TriggerOperationKind                         string                                       `json:"triggerOperationKind"`
	TriggerDiagnosticClass                       string                                       `json:"triggerDiagnosticClass"`
	RepairOwnerModuleID                          string                                       `json:"repairOwnerModuleId"`
	RepairOperationKinds                         []string                                     `json:"repairOperationKinds"`
	RepairTargetPolicy                           runtimecontract.RuntimeOperationTargetPolicy `json:"repairTargetPolicy"`
	RepairTargetRole                             string                                       `json:"repairTargetRole"`
	RepairDeclarationTargetSelectorRef           string                                       `json:"repairDeclarationTargetSelectorRef"`
	RepairDeclarationTargetSelectorOwnerRef      string                                       `json:"repairDeclarationTargetSelectorOwnerRef"`
	RepairDeclarationTargetSelectorSubjectRef    string                                       `json:"repairDeclarationTargetSelectorSubjectRef"`
	RepairDeclarationTargetSelectorCandidateRefs []string                                     `json:"repairDeclarationTargetSelectorCandidateRefs,omitempty"`
	RepairOperationTargetSelectorRef             string                                       `json:"repairOperationTargetSelectorRef"`
	RepairConstraintRefs                         []string                                     `json:"repairConstraintRefs,omitempty"`
	RequireFreshValidation                       bool                                         `json:"requireFreshValidation"`
	MaxAttempts                                  int                                          `json:"maxAttempts"`
}

type runtimeSemanticOwnerFactAcquisitionProjection struct {
	DefinitionPrerequisiteDeclared     bool                                                  `json:"definitionPrerequisiteDeclared"`
	TerminalEvidenceCredit             bool                                                  `json:"terminalEvidenceCredit"`
	RequiredOwnerModuleID              string                                                `json:"requiredOwnerModuleId"`
	RequiredFactKind                   runtimecontract.RuntimeOwnerFactKind                  `json:"requiredFactKind"`
	RequiredOperationKind              string                                                `json:"requiredOperationKind"`
	AcquisitionOwnerModuleID           string                                                `json:"acquisitionOwnerModuleId"`
	AcquisitionOperationKind           string                                                `json:"acquisitionOperationKind"`
	AcquisitionOperationVariantID      string                                                `json:"acquisitionOperationVariantId,omitempty"`
	AcquisitionTargetPolicy            runtimecontract.RuntimeOperationTargetPolicy          `json:"acquisitionTargetPolicy"`
	AcquisitionTargetSelectorRef       string                                                `json:"acquisitionTargetSelectorRef"`
	AcquisitionConstraintRefs          []string                                              `json:"acquisitionConstraintRefs,omitempty"`
	ProspectiveSatisfactionContractRef string                                                `json:"prospectiveSatisfactionContractRef,omitempty"`
	ProspectiveTargetConstraint        *runtimeSemanticProspectiveTargetConstraintProjection `json:"prospectiveTargetConstraint,omitempty"`
	OwnerCatalogSnapshotID             string                                                `json:"ownerCatalogSnapshotId"`
	OwnerCatalogObservedRevision       string                                                `json:"ownerCatalogObservedRevision"`
	ReobserveOwnerModuleID             string                                                `json:"reobserveOwnerModuleId"`
	ReobserveFactKind                  runtimecontract.RuntimeOwnerFactKind                  `json:"reobserveFactKind"`
	MaxAcquisitionAttempts             int                                                   `json:"maxAcquisitionAttempts"`
}

// runtimeSemanticProspectiveTargetConstraintProjection is the provider-safe
// C3 view of one required-owner-authored prospective target predicate. It
// preserves the exact conjunctive predicate set and its owner/catalog
// provenance, while deliberately omitting the surface/recipe identity,
// command and args used by Shell to derive the fact. Those remain dependency
// facts and are not provider-selectable.
type runtimeSemanticProspectiveTargetConstraintProjection struct {
	SchemaVersion                      string                                             `json:"schemaVersion"`
	OwnerModuleID                      string                                             `json:"ownerModuleId"`
	ProspectiveSatisfactionContractRef string                                             `json:"prospectiveSatisfactionContractRef"`
	InputSchemaHash                    string                                             `json:"inputSchemaHash"`
	CatalogObservedRevision            string                                             `json:"catalogObservedRevision"`
	AllOf                              []runtimeSemanticProspectiveTargetClauseProjection `json:"allOf"`
}

type runtimeSemanticProspectiveTargetClauseProjection struct {
	ID    string                                                `json:"id"`
	AllOf []runtimeSemanticProspectiveTargetPredicateProjection `json:"allOf"`
}

type runtimeSemanticProspectiveTargetPredicateProjection struct {
	Kind  runtimecontract.RuntimeProspectiveTargetPredicateKind `json:"kind"`
	Value string                                                `json:"value"`
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

func runtimeSemanticManifestForProvider(manifest runtimecontract.RuntimeSemanticPlanManifest, step runtimecontract.RuntimeSemanticPlanStep, batchCoverageIDs []common.CoverageWorkItemID) (runtimeSemanticManifestProjection, error) {
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
		var err error
		projection.ProofObligations, err = runtimeSemanticProofObligationsForProvider(manifest.ProofObligations, batchCoverageIDs)
		if err != nil {
			return runtimeSemanticManifestProjection{}, err
		}
		projection.OwnerCatalogs = runtimeSemanticOwnerCatalogsForProvider(manifest.OwnerCatalogs)
	}
	return projection, nil
}

func runtimeSemanticProofObligationsForProvider(values []runtimecontract.RuntimeSemanticProofObligation, visibleCoverageIDs []common.CoverageWorkItemID) ([]runtimeSemanticProofObligationProjection, error) {
	selected := make(map[common.CoverageWorkItemID]struct{}, len(visibleCoverageIDs))
	for _, id := range visibleCoverageIDs {
		selected[id] = struct{}{}
	}
	ret := make([]runtimeSemanticProofObligationProjection, 0, len(visibleCoverageIDs))
	for _, raw := range values {
		value := raw.Normalized()
		if _, found := selected[value.CoverageWorkItemID]; !found {
			continue
		}
		evidenceObligation, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(value)
		if err != nil {
			return nil, err
		}
		projection := runtimeSemanticProofObligationProjection{
			CoverageWorkItemID:                  value.CoverageWorkItemID,
			EvidencePurpose:                     evidenceObligation.Purpose,
			TerminalRequiredEvidenceKinds:       append([]string(nil), evidenceObligation.TerminalRequiredEvidenceKinds...),
			CurrentRequiredEvidenceKinds:        append([]string(nil), evidenceObligation.CurrentRequiredEvidenceKinds...),
			DeclarationTargetSelectorRef:        value.DeclarationTargetSelectorRef,
			DeclarationTargetSelectorOwnerRef:   value.DeclarationTargetSelectorOwnerRef,
			DeclarationTargetSelectorSubjectRef: value.DeclarationTargetSelectorSubjectRef,
			DeclarationTargetSelectorCandidates: append([]string(nil), value.DeclarationTargetSelectorCandidateRefs...),
		}
		if value.OwnerFactAcquisition != nil {
			acquisition := value.OwnerFactAcquisition.Normalized()
			prospectiveConstraint, err := runtimeSemanticProspectiveTargetConstraintForProvider(acquisition.ProspectiveTargetSatisfactionConstraint)
			if err != nil {
				return nil, err
			}
			projection.OwnerFactAcquisition = &runtimeSemanticOwnerFactAcquisitionProjection{
				DefinitionPrerequisiteDeclared:     true,
				TerminalEvidenceCredit:             false,
				RequiredOwnerModuleID:              acquisition.Requirement.OwnerModuleID,
				RequiredFactKind:                   acquisition.Requirement.FactKind,
				RequiredOperationKind:              acquisition.Requirement.OperationKind,
				AcquisitionOwnerModuleID:           acquisition.AcquisitionOwnerModuleID,
				AcquisitionOperationKind:           acquisition.AcquisitionOperationKind,
				AcquisitionOperationVariantID:      acquisition.AcquisitionOperationVariantID,
				AcquisitionTargetPolicy:            acquisition.AcquisitionTargetPolicy,
				AcquisitionTargetSelectorRef:       acquisition.AcquisitionTargetSelectorRef,
				AcquisitionConstraintRefs:          append([]string(nil), acquisition.AcquisitionConstraintRefs...),
				ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
				ProspectiveTargetConstraint:        prospectiveConstraint,
				OwnerCatalogSnapshotID:             acquisition.OwnerCatalogSnapshotID,
				OwnerCatalogObservedRevision:       acquisition.OwnerCatalogObservedRevision,
				ReobserveOwnerModuleID:             acquisition.ReobserveOwnerModuleID,
				ReobserveFactKind:                  acquisition.ReobserveFactKind,
				MaxAcquisitionAttempts:             acquisition.MaxAcquisitionAttempts,
			}
		}
		if value.ValidationFailureRepair != nil {
			repair := value.ValidationFailureRepair.Normalized()
			projection.ValidationFailureRepair = &runtimeSemanticValidationFailureRepairProjection{
				FailedFocusID:                                repair.FailedFocusID,
				ConvergenceID:                                repair.ConvergenceID,
				FailureSignature:                             repair.FailureSignature,
				TriggerOwnerModuleID:                         repair.TriggerOwnerModuleID,
				TriggerOperationKind:                         repair.TriggerOperationKind,
				TriggerDiagnosticClass:                       repair.TriggerDiagnosticClass,
				RepairOwnerModuleID:                          repair.RepairOwnerModuleID,
				RepairOperationKinds:                         append([]string(nil), repair.RepairOperationKinds...),
				RepairTargetPolicy:                           repair.RepairTargetPolicy,
				RepairTargetRole:                             repair.RepairTargetRole,
				RepairDeclarationTargetSelectorRef:           repair.RepairDeclarationTargetSelectorRef,
				RepairDeclarationTargetSelectorOwnerRef:      repair.RepairDeclarationTargetSelectorOwnerRef,
				RepairDeclarationTargetSelectorSubjectRef:    repair.RepairDeclarationTargetSelectorSubjectRef,
				RepairDeclarationTargetSelectorCandidateRefs: append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...),
				RepairOperationTargetSelectorRef:             repair.RepairOperationTargetSelectorRef,
				RepairConstraintRefs:                         append([]string(nil), repair.RepairConstraintRefs...),
				RequireFreshValidation:                       repair.RequireFreshValidation,
				MaxAttempts:                                  repair.MaxAttempts,
			}
		}
		ret = append(ret, projection)
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func runtimeSemanticProspectiveTargetConstraintForProvider(value *runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint) (*runtimeSemanticProspectiveTargetConstraintProjection, error) {
	if value == nil {
		return nil, nil
	}
	constraint := value.Normalized()
	if err := constraint.Validate(); err != nil {
		return nil, err
	}
	projection := &runtimeSemanticProspectiveTargetConstraintProjection{
		SchemaVersion:                      constraint.SchemaVersion,
		OwnerModuleID:                      constraint.OwnerModuleID,
		ProspectiveSatisfactionContractRef: constraint.ProspectiveSatisfactionContractRef,
		InputSchemaHash:                    constraint.InputSchemaHash,
		CatalogObservedRevision:            constraint.CatalogObservedRevision,
		AllOf:                              make([]runtimeSemanticProspectiveTargetClauseProjection, 0, len(constraint.AllOf)),
	}
	for _, clause := range constraint.AllOf {
		clauseProjection := runtimeSemanticProspectiveTargetClauseProjection{
			ID: clause.ID, AllOf: make([]runtimeSemanticProspectiveTargetPredicateProjection, 0, len(clause.AllOf)),
		}
		for _, predicate := range clause.AllOf {
			clauseProjection.AllOf = append(clauseProjection.AllOf, runtimeSemanticProspectiveTargetPredicateProjection{
				Kind: predicate.Kind, Value: predicate.Value,
			})
		}
		projection.AllOf = append(projection.AllOf, clauseProjection)
	}
	return projection, nil
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
