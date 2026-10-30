package llm

import (
	"encoding/json"
	"fmt"

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
	ValidationGenerationContexts []runtimeValidationGenerationContextProjection `json:"validationGenerationContexts,omitempty"`
	SchemaVersion                string                                         `json:"schemaVersion"`
	Hash                         string                                         `json:"hash"`
	DefinitionRevision           string                                         `json:"definitionRevision"`
	DefinitionSeedHash           string                                         `json:"definitionSeedHash"`
	OperationContractRevision    string                                         `json:"operationContractRevision"`
	CoverageItems                []runtimecontract.RuntimeCoverageItem          `json:"coverageItems"`
	AllowedOwnerModuleIDs        []string                                       `json:"allowedOwnerModuleIds,omitempty"`
	AllowedOperationKinds        []string                                       `json:"allowedOperationKinds,omitempty"`
	AllowedTargetPolicies        []runtimecontract.RuntimeOperationTargetPolicy `json:"allowedTargetPolicies,omitempty"`
	ProofObligations             []runtimeSemanticProofObligationProjection     `json:"proofObligations,omitempty"`
	OwnerCatalogs                []runtimeSemanticOwnerCatalogProjection        `json:"ownerCatalogs,omitempty"`
	ImplementationPurpose        *runtimeSemanticImplementationWorkProjection   `json:"implementationPurpose,omitempty"`
}

// 工作 C3 只投影完整有界 vocabulary 与 exact owner scope；不携带 receipt、预算权限或原始诊断。
// policy 为 immutable Profile 声明，scope 只为提议约束，不是工作/终验 dispatch authority。
type runtimeSemanticImplementationWorkProjection struct {
	CoverageWorkItemID      common.CoverageWorkItemID                       `json:"coverageWorkItemId"`
	PurposeChoiceOnly       bool                                            `json:"purposeChoiceOnly"`
	EffectAvailable         bool                                            `json:"effectAvailable"`
	RoundHash               string                                          `json:"roundHash"`
	Ordinal                 int                                             `json:"ordinal"`
	PrecedingReceiptSetHash string                                          `json:"precedingReceiptSetHash,omitempty"`
	PolicyHash              string                                          `json:"policyHash"`
	Policy                  runtimecontract.RuntimeImplementationWorkPolicy `json:"policy"`
	Scope                   runtimeSemanticImplementationScopeProjection    `json:"scope"`
	ExcludedTargetRefs      []string                                        `json:"excludedTargetRefs,omitempty"`
	TerminalEvidenceCredit  bool                                            `json:"terminalEvidenceCredit"`
}

type runtimeSemanticImplementationScopeProjection struct {
	RequestID                            string                                            `json:"requestId"`
	SnapshotID                           string                                            `json:"snapshotId"`
	OwnerModuleID                        string                                            `json:"ownerModuleId"`
	TargetRole                           string                                            `json:"targetRole"`
	SelectorRef                          string                                            `json:"selectorRef"`
	SelectorOwnerRef                     string                                            `json:"selectorOwnerRef"`
	SelectorSubjectRef                   string                                            `json:"selectorSubjectRef"`
	SelectorCandidateRefs                []string                                          `json:"selectorCandidateRefs,omitempty"`
	ProspectiveTargetAllOfConstraintRefs []string                                          `json:"prospectiveTargetAllOfConstraintRefs,omitempty"`
	Disposition                          runtimecontract.RuntimeTargetSelectionDisposition `json:"disposition"`
	TargetRefs                           []string                                          `json:"targetRefs"`
	CandidateTargetRefs                  []string                                          `json:"candidateTargetRefs"`
	CandidateCount                       int                                               `json:"candidateCount"`
	ObservedRevision                     string                                            `json:"observedRevision"`
}

func runtimeSemanticImplementationWorkForProvider(work *runtimecontract.RuntimeSemanticImplementationWork) (*runtimeSemanticImplementationWorkProjection, error) {
	if work == nil {
		return nil, nil
	}
	if err := work.Validate(); err != nil {
		return nil, err
	}
	selection := work.Scope.Selection
	return &runtimeSemanticImplementationWorkProjection{
		CoverageWorkItemID: work.Scope.Request.CoverageWorkItemID,
		PurposeChoiceOnly:  work.PurposeChoiceOnly, EffectAvailable: work.EffectAvailable,
		RoundHash: work.RoundHash, Ordinal: work.Ordinal, PrecedingReceiptSetHash: work.PrecedingReceiptSetHash,
		PolicyHash: work.PolicyHash, Policy: work.Scope.Request.Policy.Clone(),
		ExcludedTargetRefs: append([]string(nil), work.ExcludedTargetRefs...),
		Scope: runtimeSemanticImplementationScopeProjection{
			RequestID: work.Scope.Request.RequestID, SnapshotID: selection.SnapshotID, OwnerModuleID: selection.OwnerModuleID,
			TargetRole: selection.TargetRole, SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.SelectorOwnerRef,
			SelectorSubjectRef: selection.SelectorSubjectRef, SelectorCandidateRefs: append([]string(nil), selection.SelectorCandidateRefs...),
			ProspectiveTargetAllOfConstraintRefs: append([]string(nil), selection.ProspectiveTargetAllOfConstraintRefs...),
			Disposition:                          selection.Disposition, TargetRefs: append([]string(nil), selection.TargetRefs...),
			CandidateTargetRefs: append([]string(nil), selection.CandidateTargetRefs...), CandidateCount: selection.CandidateCount,
			ObservedRevision: selection.ObservedRevision,
		},
	}, nil
}

type runtimeSemanticProofObligationProjection struct {
	CoverageWorkItemID                              common.CoverageWorkItemID                                `json:"coverageWorkItemId"`
	DeclarationRef                                  string                                                   `json:"declarationRef"`
	ValidationProofRequirement                      *runtimeSemanticValidationProofRequirementProjection     `json:"validationProofRequirement,omitempty"`
	EvidencePurpose                                 runtimecontract.RuntimeSemanticEvidenceObligationPurpose `json:"evidencePurpose"`
	TerminalRequiredEvidenceKinds                   []string                                                 `json:"terminalRequiredEvidenceKinds"`
	CurrentRequiredEvidenceKinds                    []string                                                 `json:"currentRequiredEvidenceKinds"`
	DeclarationTargetSelectorRef                    string                                                   `json:"declarationTargetSelectorRef,omitempty"`
	DeclarationTargetSelectorOwnerRef               string                                                   `json:"declarationTargetSelectorOwnerRef,omitempty"`
	DeclarationTargetSelectorSubjectRef             string                                                   `json:"declarationTargetSelectorSubjectRef,omitempty"`
	DeclarationTargetSelectorCandidates             []string                                                 `json:"declarationTargetSelectorCandidateRefs,omitempty"`
	DeclarationProspectiveTargetAllOfConstraintRefs []string                                                 `json:"declarationProspectiveTargetAllOfConstraintRefs,omitempty"`
	OwnerFactAcquisition                            *runtimeSemanticOwnerFactAcquisitionProjection           `json:"ownerFactAcquisition,omitempty"`
	ValidationFailureRepair                         *runtimeSemanticValidationFailureRepairProjection        `json:"validationFailureRepair,omitempty"`
	ImplementationWork                              *runtimeSemanticImplementationWorkProjection             `json:"implementationWork,omitempty"`
}

type runtimeSemanticValidationProofRequirementProjection struct {
	RequirementHash               string                                            `json:"requirementHash"`
	CoverageWorkItemID            common.CoverageWorkItemID                         `json:"coverageWorkItemId"`
	DeclarationRef                string                                            `json:"declarationRef"`
	ValidationRecipeID            string                                            `json:"validationRecipeId,omitempty"`
	SelectableValidationRecipeIDs []string                                          `json:"selectableValidationRecipeIds,omitempty"`
	RequiredCheckRefs             []string                                          `json:"requiredCheckRefs"`
	SourceObligationChecks        []runtimeSemanticSourceObligationCheckProjection  `json:"sourceObligationChecks,omitempty"`
	OwnerNativeCheckAttestations  []runtimeSemanticNativeCheckAttestationProjection `json:"ownerNativeCheckAttestations,omitempty"`
	InputReviews                  []runtimeSemanticInputReviewProjection            `json:"inputReviews,omitempty"`
}

type runtimeSemanticSourceObligationCheckProjection struct {
	InventoryHash  string `json:"inventoryHash"`
	ObligationID   string `json:"obligationId"`
	RequirementID  string `json:"requirementId"`
	VerificationID string `json:"verificationId"`
	Criterion      string `json:"criterion"`
	Declaration    string `json:"declaration"`
	CheckRef       string `json:"checkRef"`
}

func runtimeSemanticSourceObligationChecksForProvider(values []runtimecontract.RuntimeSourceObligationCheck) []runtimeSemanticSourceObligationCheckProjection {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimeSemanticSourceObligationCheckProjection, 0, len(values))
	for _, value := range values {
		ret = append(ret, runtimeSemanticSourceObligationCheckProjection{
			InventoryHash: value.InventoryHash, ObligationID: value.ObligationID, RequirementID: value.RequirementID,
			VerificationID: value.VerificationID, Criterion: value.Criterion, Declaration: value.Declaration, CheckRef: value.CheckRef,
		})
	}
	return ret
}

type runtimeSemanticInputReviewProjection struct {
	// Pure requirement projection, not a model conclusion or proof receipt.
	RequiredProofConsumerRefs   []string                                                  `json:"requiredProofConsumerRefs,omitempty"`
	OwnerModuleID               string                                                    `json:"ownerModuleId"`
	ValidationRecipeID          string                                                    `json:"validationRecipeId"`
	SubjectHash                 string                                                    `json:"subjectHash"`
	ProtocolRef                 string                                                    `json:"protocolRef"`
	ScopeContractRef            string                                                    `json:"scopeContractRef,omitempty"`
	ScopeRequirements           string                                                    `json:"scopeRequirements,omitempty"`
	Files                       []runtimecontract.RuntimeValidationInputReviewFile        `json:"files"`
	Information                 []runtimecontract.RuntimeValidationInputReviewInformation `json:"information,omitempty"`
	LineageAssertions           []runtimecontract.RuntimeLineageAssertion                 `json:"lineageAssertions,omitempty"`
	DiagnosticSourceContractRef string                                                    `json:"diagnosticSourceContractRef,omitempty"`
	DiagnosticSources           []runtimecontract.RuntimeValidationDiagnosticSource       `json:"diagnosticSources,omitempty"`
}

type runtimeSemanticNativeCheckAttestationProjection struct {
	SourceObligation           *runtimeSemanticSourceObligationCheckProjection `json:"sourceObligation,omitempty"`
	OwnerModuleID              string                                          `json:"ownerModuleId,omitempty"`
	CoverageWorkItemID         common.CoverageWorkItemID                       `json:"coverageWorkItemId,omitempty"`
	RequirementHash            string                                          `json:"requirementHash,omitempty"`
	ValidationRecipeID         string                                          `json:"validationRecipeId"`
	RequiredCheckRef           string                                          `json:"requiredCheckRef"`
	ObservationProtocolRef     string                                          `json:"observationProtocolRef"`
	SupplementalNativeCheckRef string                                          `json:"supplementalNativeCheckRef,omitempty"`
	NativeCheckRef             string                                          `json:"nativeCheckRef"`
	TargetRef                  string                                          `json:"targetRef,omitempty"`
	ContentContractRef         string                                          `json:"contentContractRef,omitempty"`
	ContentRequirements        string                                          `json:"contentRequirements,omitempty"`
}

// runtimeSemanticValidationFailureRepairProjection is the provider-safe C3
// view of one Profile-sealed repair generation. It exposes the typed failure
// identity and exact repair vocabulary needed to propose a relevant proof, but
// carries no raw diagnostic, Runtime action, retry, terminal, receipt or owner
// observation.
type runtimeSemanticValidationFailureRepairProjection struct {
	InputAddition                                runtimecontract.RuntimeValidationInputAddition `json:"inputAddition,omitzero"`
	FailedFocusID                                string                                         `json:"failedFocusId"`
	ConvergenceID                                string                                         `json:"convergenceId"`
	FailureSignature                             string                                         `json:"failureSignature"`
	TriggerOwnerModuleID                         string                                         `json:"triggerOwnerModuleId"`
	TriggerOperationKind                         string                                         `json:"triggerOperationKind"`
	TriggerDiagnosticClass                       string                                         `json:"triggerDiagnosticClass"`
	RepairOwnerModuleID                          string                                         `json:"repairOwnerModuleId"`
	RepairOperationKinds                         []string                                       `json:"repairOperationKinds"`
	RepairTargetPolicy                           runtimecontract.RuntimeOperationTargetPolicy   `json:"repairTargetPolicy"`
	RepairTargetRole                             string                                         `json:"repairTargetRole"`
	RepairDeclarationTargetSelectorRef           string                                         `json:"repairDeclarationTargetSelectorRef"`
	RepairDeclarationTargetSelectorOwnerRef      string                                         `json:"repairDeclarationTargetSelectorOwnerRef"`
	RepairDeclarationTargetSelectorSubjectRef    string                                         `json:"repairDeclarationTargetSelectorSubjectRef"`
	RepairDeclarationTargetSelectorCandidateRefs []string                                       `json:"repairDeclarationTargetSelectorCandidateRefs,omitempty"`
	RepairOperationTargetSelectorRef             string                                         `json:"repairOperationTargetSelectorRef"`
	RepairConstraintRefs                         []string                                       `json:"repairConstraintRefs,omitempty"`
	RequireFreshValidation                       bool                                           `json:"requireFreshValidation"`
	MaxAttempts                                  int                                            `json:"maxAttempts"`
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
	ValidationProofPreparations        []runtimeSemanticNativeCheckAttestationProjection  `json:"validationProofPreparations,omitempty"`
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
	if step == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		for _, obligation := range manifest.ProofObligations {
			if obligation.ImplementationWork == nil || !obligation.ImplementationWork.PurposeChoiceOnly {
				continue
			}
			if projection.ImplementationPurpose != nil {
				return runtimeSemanticManifestProjection{}, fmt.Errorf("implementation purpose projection requires one exact work scope")
			}
			var err error
			projection.ImplementationPurpose, err = runtimeSemanticImplementationWorkForProvider(obligation.ImplementationWork)
			if err != nil {
				return runtimeSemanticManifestProjection{}, err
			}
		}
	}
	if step != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		projection.ValidationGenerationContexts = runtimeValidationGenerationContexts(batchCoverageIDs, nil, append(append([]runtimecontract.RuntimeOperationAdmissibility(nil), manifest.OwnerCatalogs...), manifest.OwnerFactDependencyCatalogs...))
		projection.AllowedOwnerModuleIDs = append([]string(nil), manifest.AllowedOwnerModuleIDs...)
		projection.AllowedOperationKinds = append([]string(nil), manifest.AllowedOperationKinds...)
		projection.AllowedTargetPolicies = append([]runtimecontract.RuntimeOperationTargetPolicy(nil), manifest.AllowedTargetPolicies...)
		var err error
		projection.ProofObligations, err = runtimeSemanticProofObligationsForProvider(manifest.ProofObligations, manifest.OwnerCatalogs, manifest.OwnerFactDependencyCatalogs, batchCoverageIDs)
		if err != nil {
			return runtimeSemanticManifestProjection{}, err
		}
		for i := range projection.ProofObligations {
			requirement := projection.ProofObligations[i].ValidationProofRequirement
			if requirement == nil {
				continue
			}
			for j := range requirement.InputReviews {
				for _, information := range manifest.InputReviewInformation {
					if information.SubjectHash == requirement.InputReviews[j].SubjectHash {
						requirement.InputReviews[j].Information = append(requirement.InputReviews[j].Information, information)
					}
				}
			}
		}
		projection.OwnerCatalogs = runtimeSemanticOwnerCatalogsForProvider(manifest.OwnerCatalogs)
	}
	return projection, nil
}

func runtimeSemanticProofObligationsForProvider(values []runtimecontract.RuntimeSemanticProofObligation, ownerCatalogs, dependencyCatalogs []runtimecontract.RuntimeOperationAdmissibility, visibleCoverageIDs []common.CoverageWorkItemID) ([]runtimeSemanticProofObligationProjection, error) {
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
			CoverageWorkItemID:                              value.CoverageWorkItemID,
			DeclarationRef:                                  runtimecontract.RuntimeSemanticCurrentDeclarationRef(value),
			EvidencePurpose:                                 evidenceObligation.Purpose,
			TerminalRequiredEvidenceKinds:                   append([]string(nil), evidenceObligation.TerminalRequiredEvidenceKinds...),
			CurrentRequiredEvidenceKinds:                    append([]string(nil), evidenceObligation.CurrentRequiredEvidenceKinds...),
			DeclarationTargetSelectorRef:                    value.DeclarationTargetSelectorRef,
			DeclarationTargetSelectorOwnerRef:               value.DeclarationTargetSelectorOwnerRef,
			DeclarationTargetSelectorSubjectRef:             value.DeclarationTargetSelectorSubjectRef,
			DeclarationTargetSelectorCandidates:             append([]string(nil), value.DeclarationTargetSelectorCandidateRefs...),
			DeclarationProspectiveTargetAllOfConstraintRefs: append([]string(nil), value.DeclarationProspectiveTargetAllOfConstraintRefs...),
		}
		projection.ImplementationWork, err = runtimeSemanticImplementationWorkForProvider(value.ImplementationWork)
		if err != nil {
			return nil, err
		}
		if value.ValidationProofRequirement != nil {
			requirement := value.ValidationProofRequirement.Normalized()
			projection.ValidationProofRequirement = &runtimeSemanticValidationProofRequirementProjection{
				RequirementHash: requirement.Hash, CoverageWorkItemID: requirement.CoverageWorkItemID,
				DeclarationRef: requirement.DeclarationRef, ValidationRecipeID: requirement.ValidationRecipeID,
				SelectableValidationRecipeIDs: runtimecontract.RuntimeSelectableValidationRecipeRefsForProofRequirementInCatalogs(ownerCatalogs, requirement),
				RequiredCheckRefs:             append([]string(nil), requirement.RequiredCheckRefs...),
				SourceObligationChecks:        runtimeSemanticSourceObligationChecksForProvider(requirement.SourceObligationChecks),
				OwnerNativeCheckAttestations:  runtimeSemanticNativeCheckAttestationsForRequirement(ownerCatalogs, requirement),
				InputReviews:                  runtimeSemanticInputReviewsForRequirement(ownerCatalogs, requirement),
			}
		}
		if value.OwnerFactAcquisition != nil {
			acquisition := value.OwnerFactAcquisition.Normalized()
			var preparation *runtimecontract.RuntimeValidationProofPreparation
			if acquisition.Requirement.ValidationProofRequirementHash != "" {
				for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(dependencyCatalogs) {
					if catalog.OwnerModuleID != acquisition.Requirement.OwnerModuleID {
						continue
					}
					resolved, resolveErr := runtimecontract.ResolveRuntimeValidationProofPreparation(catalog, acquisition.Requirement, acquisition.ProspectiveSatisfactionContractRef)
					if resolveErr != nil {
						return nil, resolveErr
					}
					resolved = resolved.Clone()
					preparation = &resolved
					break
				}
			}
			prospectiveConstraint, err := runtimeSemanticProspectiveTargetConstraintForProvider(acquisition.ProspectiveTargetSatisfactionConstraint, preparation)
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
				InputAddition:                                repair.InputAddition,
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

func runtimeSemanticProspectiveTargetConstraintForProvider(value *runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint, preparation *runtimecontract.RuntimeValidationProofPreparation) (*runtimeSemanticProspectiveTargetConstraintProjection, error) {
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
	if preparation != nil {
		preparationValue := preparation.Clone()
		if err := preparationValue.Validate(); err != nil {
			return nil, err
		}
		if runtimecontract.StableRuntimeProspectiveTargetSatisfactionConstraintHash(preparationValue.ProspectiveTargetSatisfactionConstraint) !=
			runtimecontract.StableRuntimeProspectiveTargetSatisfactionConstraintHash(constraint) {
			return nil, fmt.Errorf("runtime semantic validation proof preparation does not match prospective constraint")
		}
		for _, attestation := range preparationValue.CheckAttestations {
			projection.ValidationProofPreparations = append(projection.ValidationProofPreparations, runtimeSemanticNativeCheckAttestationProjection{
				SourceObligation: runtimePreparationSourceObligation(preparationValue, attestation.RequiredCheckRef),
				OwnerModuleID:    constraint.OwnerModuleID, CoverageWorkItemID: preparationValue.CoverageWorkItemID, RequirementHash: preparationValue.RequirementHash,
				ValidationRecipeID: preparationValue.ValidationRecipeID, RequiredCheckRef: attestation.RequiredCheckRef,
				ObservationProtocolRef: attestation.ObservationProtocolRef, NativeCheckRef: attestation.NativeCheckRef, SupplementalNativeCheckRef: attestation.SupplementalNativeCheckRef,
				TargetRef: preparationValue.TargetRef, ContentContractRef: preparationValue.ContentContractRef, ContentRequirements: preparationValue.ContentRequirements,
			})
		}
	}
	return projection, nil
}

func runtimeSemanticNativeCheckAttestationsForRequirement(catalogs []runtimecontract.RuntimeOperationAdmissibility, requirement runtimecontract.RuntimeValidationProofRequirement) []runtimeSemanticNativeCheckAttestationProjection {
	ret := make([]runtimeSemanticNativeCheckAttestationProjection, 0)
	for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(catalogs) {
		for _, surface := range catalog.ValidationSurfaces {
			_, binding, found := runtimecontract.RuntimeValidationSurfaceDeclaringProofRequirement([]runtimecontract.RuntimeValidationSurface{surface}, requirement)
			if found {
				for _, attestation := range binding.CheckAttestations {
					ret = append(ret, runtimeSemanticNativeCheckAttestationProjection{
						OwnerModuleID: catalog.OwnerModuleID, CoverageWorkItemID: requirement.CoverageWorkItemID, RequirementHash: requirement.Hash,
						ValidationRecipeID: binding.ValidationRecipeID, RequiredCheckRef: attestation.RequiredCheckRef,
						ObservationProtocolRef: attestation.ObservationProtocolRef, NativeCheckRef: attestation.NativeCheckRef, SupplementalNativeCheckRef: attestation.SupplementalNativeCheckRef,
					})
				}
			}
			for _, preparation := range surface.ProofRequirementPreparations {
				if preparation.RequirementHash != requirement.Hash {
					continue
				}
				for _, attestation := range preparation.CheckAttestations {
					ret = append(ret, runtimeSemanticNativeCheckAttestationProjection{
						SourceObligation: runtimePreparationSourceObligation(preparation, attestation.RequiredCheckRef),
						OwnerModuleID:    catalog.OwnerModuleID, CoverageWorkItemID: preparation.CoverageWorkItemID, RequirementHash: preparation.RequirementHash,
						ValidationRecipeID: preparation.ValidationRecipeID, RequiredCheckRef: attestation.RequiredCheckRef,
						ObservationProtocolRef: attestation.ObservationProtocolRef, NativeCheckRef: attestation.NativeCheckRef, SupplementalNativeCheckRef: attestation.SupplementalNativeCheckRef,
						TargetRef: preparation.TargetRef, ContentContractRef: preparation.ContentContractRef, ContentRequirements: preparation.ContentRequirements,
					})
				}
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
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
