package runtimeprofiledecode

import (
	"sort"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Decode converts profileContracts from the configuration value shape into
// typed Runtime Profile contracts. It intentionally preserves legacy coercion
// behavior; callers validate exact shape before merging or admission.
func Decode(value any) []runtimecontract.RuntimeProfileContract {
	profiles := Map(value)
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]runtimecontract.RuntimeProfileContract, 0, len(ids))
	for _, profileID := range ids {
		profile := Map(profiles[profileID])
		contract := runtimecontract.RuntimeProfileContract{ID: profileID}
		for _, item := range MapSlice(profile["acceptanceDimensions"]) {
			contract.AcceptanceDimensions = append(contract.AcceptanceDimensions, runtimecontract.RuntimeAcceptanceDimension{
				ID: String(item["id"]), Required: Bool(item["required"]), Description: String(item["description"]),
			})
		}
		for _, item := range MapSlice(profile["deliverableExpectations"]) {
			contract.DeliverableExpectations = append(contract.DeliverableExpectations, runtimecontract.RuntimeDeliverableExpectation{
				ID: String(item["id"]), Required: Bool(item["required"]),
				EvidenceRole: String(item["evidenceRole"]), Description: String(item["description"]),
				RequiredEvidencePurposeContractRef: String(item["requiredEvidencePurposeContractRef"]),
			})
		}
		for _, item := range MapSlice(profile["coverageDependencies"]) {
			contract.CoverageDependencies = append(contract.CoverageDependencies, runtimecontract.RuntimeCoverageDependency{
				ID: String(item["id"]), SourceScope: String(item["sourceScope"]), SourceRequirementID: String(item["sourceRequirementId"]),
				DependentScope: String(item["dependentScope"]), DependentRequirementID: String(item["dependentRequirementId"]),
				RequireSourceSettled: Bool(item["requireSourceSettled"]), TriggerEvidenceKinds: Strings(item["triggerEvidenceKinds"]),
			})
		}
		for _, item := range MapSlice(profile["bootstrapRecipes"]) {
			recipe := runtimecontract.RuntimeBootstrapRecipe{
				ID: String(item["id"]), RequiredGoalTerms: Strings(item["requiredGoalTerms"]), RequiredDoDTerms: Strings(item["requiredDoDTerms"]),
			}
			for _, artifact := range MapSlice(item["artifacts"]) {
				recipe.Artifacts = append(recipe.Artifacts, runtimecontract.RuntimeBootstrapArtifact{
					ID: String(artifact["id"]), TargetRole: String(artifact["targetRole"]),
					AllowedOperationKinds: Strings(artifact["allowedOperationKinds"]), DependsOnArtifactIDs: Strings(artifact["dependsOnArtifactIds"]),
				})
			}
			contract.BootstrapRecipes = append(contract.BootstrapRecipes, recipe)
		}
		for _, item := range MapSlice(profile["evidenceRequirements"]) {
			acceptedVariants := make([]runtimecontract.RuntimeOperationVariantRef, 0)
			for _, variant := range MapSlice(item["acceptedOperationVariants"]) {
				acceptedVariants = append(acceptedVariants, runtimecontract.RuntimeOperationVariantRef{Kind: String(variant["kind"]), VariantID: String(variant["variantId"])})
			}
			stateRequirements := make([]runtimecontract.RuntimeProfileCapabilityStateRequirement, 0)
			for _, state := range MapSlice(item["capabilityStateRequirements"]) {
				stateRequirements = append(stateRequirements, runtimecontract.RuntimeProfileCapabilityStateRequirement{OwnerModuleID: String(state["ownerModuleId"]), CapabilityStateKind: String(state["capabilityStateKind"]), SubjectRef: String(state["subjectRef"])})
			}
			ownerFacts := make([]runtimecontract.RuntimeOwnerFactRequirement, 0)
			for _, ownerFact := range MapSlice(item["requiredOwnerFacts"]) {
				ownerFacts = append(ownerFacts, runtimecontract.RuntimeOwnerFactRequirement{
					OwnerModuleID: String(ownerFact["ownerModuleId"]), FactKind: runtimecontract.RuntimeOwnerFactKind(String(ownerFact["factKind"])),
					OperationKind: String(ownerFact["operationKind"]),
				})
			}
			ownerFactConvergences := make([]runtimecontract.RuntimeOwnerFactConvergence, 0)
			for _, convergence := range MapSlice(item["ownerFactConvergences"]) {
				ownerFactConvergences = append(ownerFactConvergences, runtimecontract.RuntimeOwnerFactConvergence{
					Requirement: runtimecontract.RuntimeOwnerFactRequirement{
						OwnerModuleID: String(convergence["ownerModuleId"]), FactKind: runtimecontract.RuntimeOwnerFactKind(String(convergence["factKind"])),
						OperationKind: String(convergence["operationKind"]),
					},
					RequirePreexisting: Bool(convergence["requirePreexisting"]), MissingTrigger: String(convergence["missingTrigger"]),
					AcquisitionOwnerModuleID: String(convergence["acquisitionOwnerModuleId"]), AcquisitionOperationKind: String(convergence["acquisitionOperationKind"]),
					AcquisitionOperationVariantID:      String(convergence["acquisitionOperationVariantId"]),
					AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicy(String(convergence["acquisitionTargetPolicy"])),
					AcquisitionTargetSelectorRef:       String(convergence["acquisitionTargetSelectorRef"]),
					ProspectiveSatisfactionContractRef: String(convergence["prospectiveSatisfactionContractRef"]),
					TargetSelectorRef:                  String(convergence["targetSelectorRef"]), TargetSelectorOwnerRef: String(convergence["targetSelectorOwnerRef"]),
					TargetSelectorSubjectRef: String(convergence["targetSelectorSubjectRef"]), ReobserveOwnerModuleID: String(convergence["reobserveOwnerModuleId"]),
					ReobserveFactKind:      runtimecontract.RuntimeOwnerFactKind(String(convergence["reobserveFactKind"])),
					MaxAcquisitionAttempts: Int(convergence["maxAcquisitionAttempts"]), DependsOnRequirementRefs: Strings(convergence["dependsOnRequirementRefs"]),
				})
			}
			var lineageAssertions []runtimecontract.RuntimeLineageAssertionRule
			for _, rule := range MapSlice(item["lineageAssertions"]) {
				lineageAssertions = append(lineageAssertions, runtimecontract.RuntimeLineageAssertionRule{Ref: String(rule["ref"]), Kind: runtimecontract.RuntimeLineageAssertionKind(String(rule["kind"])), ConsumptionPoint: runtimecontract.RuntimeProofPhase(String(rule["consumptionPoint"])), MinCompletedRounds: Int(rule["minCompletedRounds"]), OwnerModuleID: String(rule["ownerModuleId"]), OperationKind: String(rule["operationKind"]), OperationVariantID: String(rule["operationVariantId"]), RequiredEffectScope: runtimecontract.RuntimeEffectScope(String(rule["requiredEffectScope"])), TerminalContractRef: String(rule["terminalContractRef"]), ReceiptSourceScope: String(rule["receiptSourceScope"]), ReceiptSourceID: String(rule["receiptSourceId"]), ValidationSourceScope: String(rule["validationSourceScope"]), ValidationSourceID: String(rule["validationSourceId"])})
				lineageAssertions[len(lineageAssertions)-1].ExecutionSourceContractRef = String(rule["executionSourceContractRef"])
				lineageAssertions[len(lineageAssertions)-1].RepairHistoryContractRef = String(rule["repairHistoryContractRef"])
				lineageAssertions[len(lineageAssertions)-1].ReceiptChoiceContractRef = String(rule["receiptChoiceContractRef"])
				for i, choice := range MapSlice(rule["receiptOperationChoices"]) {
					if i < 4 {
						lineageAssertions[len(lineageAssertions)-1].ReceiptOperationChoices[i] = runtimecontract.RuntimeOperationVariantRef{Kind: String(choice["kind"]), VariantID: String(choice["variantId"])}
					}
				}
			}
			repair := ValidationFailureRepairConvergence(item)
			implementation := ImplementationWorkPolicy(item)
			contract.EvidenceRequirements = append(contract.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: String(item["id"]), Scope: String(item["scope"]), EvidenceRole: String(item["evidenceRole"]),
				TargetRole: String(item["targetRole"]), TargetRefs: Strings(item["targetRefs"]), TargetDiscoverySources: Strings(item["targetDiscoverySources"]),
				TargetSelectorRef: String(item["targetSelectorRef"]), TargetSelectorOwnerRef: String(item["targetSelectorOwnerRef"]),
				TargetSelectorSubjectRef: String(item["targetSelectorSubjectRef"]), TargetSelectorCandidateRefs: Strings(item["targetSelectorCandidateRefs"]),
				ProspectiveTargetAllOfConstraintRefs: Strings(item["prospectiveTargetAllOfConstraintRefs"]), RequireTargetGrounding: Bool(item["requireTargetGrounding"]),
				RequiredEvidenceKinds: Strings(item["requiredEvidenceKinds"]), RequireDirectEvidence: Bool(item["requireDirectEvidence"]),
				RequireClaimEvidence: Bool(item["requireClaimEvidence"]), RequireExecutableBehaviorEvidence: Bool(item["requireExecutableBehaviorEvidence"]),
				RequireCoverageTargets: Bool(item["requireCoverageTargets"]), AcceptedStepKinds: Strings(item["acceptedStepKinds"]),
				AcceptedOperationVariants: acceptedVariants, ValidationRecipeID: String(item["validationRecipeId"]),
				RequiredCheckRefs: Strings(item["requiredCheckRefs"]), MetricConstraints: Strings(item["metricConstraints"]), ArtifactRefs: Strings(item["artifactRefs"]),
				RequiredOwnerFacts: ownerFacts, OwnerFactConvergences: ownerFactConvergences,
				LineageAssertions:                  lineageAssertions,
				ValidationFailureRepairConvergence: repair, ImplementationWorkPolicy: implementation,
				CapabilityStateRequirements: stateRequirements, RejectedStepKinds: Strings(item["rejectedStepKinds"]), Description: String(item["description"]),
			})
		}
		ret = append(ret, contract)
	}
	return runtimecontract.NormalizeRuntimeProfileContracts(ret)
}

func ImplementationWorkPolicy(input map[string]any) *runtimecontract.RuntimeImplementationWorkPolicy {
	raw, present := input["implementationWorkPolicy"]
	if !present {
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok || object == nil {
		return &runtimecontract.RuntimeImplementationWorkPolicy{}
	}
	if err := ValidateObject(object, "implementationWork", "implementationWorkPolicy"); err != nil {
		return &runtimecontract.RuntimeImplementationWorkPolicy{}
	}
	policy := &runtimecontract.RuntimeImplementationWorkPolicy{
		InputRevisionProtocolRef: String(object["inputRevisionProtocolRef"]),
		ProtocolRef:              String(object["protocolRef"]), OwnerModuleID: String(object["ownerModuleId"]),
		TargetSelectorRef: String(object["targetSelectorRef"]), TargetSelectorOwnerRef: String(object["targetSelectorOwnerRef"]),
		TargetSelectorSubjectRef: String(object["targetSelectorSubjectRef"]), TargetSelectorCandidateRefs: Strings(object["targetSelectorCandidateRefs"]),
		ProspectiveTargetAllOfConstraintRefs: Strings(object["prospectiveTargetAllOfConstraintRefs"]),
		MaxRoundsPerCWI:                      Int(object["maxRoundsPerCWI"]), MaxTargetsPerRound: Int(object["maxTargetsPerRound"]),
		RequireFreshObservation: Bool(object["requireFreshObservation"]), RequireFreshValidation: Bool(object["requireFreshValidation"]),
		ProtectValidationInputs: Bool(object["protectValidationInputs"]),
	}
	policy.AllowedOperations = make([]runtimecontract.RuntimeOperationVariantRef, 0)
	for _, item := range MapSlice(object["allowedOperations"]) {
		policy.AllowedOperations = append(policy.AllowedOperations, runtimecontract.RuntimeOperationVariantRef{Kind: String(item["kind"]), VariantID: String(item["variantId"])})
	}
	for _, value := range Strings(object["allowedTargetPolicies"]) {
		policy.AllowedTargetPolicies = append(policy.AllowedTargetPolicies, runtimecontract.RuntimeOperationTargetPolicy(value))
	}
	return policy
}

func ValidationFailureRepairConvergence(input map[string]any) *runtimecontract.RuntimeValidationFailureRepairConvergence {
	rawValue, present := input["validationFailureRepairConvergence"]
	if !present {
		return nil
	}
	raw, object := rawValue.(map[string]any)
	if !object || !validationFailureRepairConvergenceHasExactShape(raw) {
		return &runtimecontract.RuntimeValidationFailureRepairConvergence{}
	}
	return &runtimecontract.RuntimeValidationFailureRepairConvergence{
		ID: String(raw["id"]), Routes: validationFailureRepairRoutes(raw["routes"]),
		RequireFreshValidation: Bool(raw["requireFreshValidation"]), MaxAttempts: Int(raw["maxAttempts"]),
	}
}

func validationFailureRepairRoutes(value any) []runtimecontract.RuntimeValidationFailureRepairRoute {
	items, exact := ObjectSliceExact(value)
	if !exact {
		return []runtimecontract.RuntimeValidationFailureRepairRoute{{}}
	}
	ret := make([]runtimecontract.RuntimeValidationFailureRepairRoute, 0, len(items))
	for _, raw := range items {
		if !validationFailureRepairRouteHasExactShape(raw) {
			return []runtimecontract.RuntimeValidationFailureRepairRoute{{}}
		}
		ret = append(ret, runtimecontract.RuntimeValidationFailureRepairRoute{
			ID: String(raw["id"]), RequireInputAddition: Bool(raw["requireInputAddition"]), RequireInputRepair: Bool(raw["requireInputRepair"]),
			TriggerOwnerModuleID: String(raw["triggerOwnerModuleId"]), TriggerOperationKind: String(raw["triggerOperationKind"]),
			TriggerDiagnosticClass: String(raw["triggerDiagnosticClass"]), TriggerEvidenceTargetType: String(raw["triggerEvidenceTargetType"]),
			TriggerOwnerDiagnosticCode: String(raw["triggerOwnerDiagnosticCode"]), RepairEvidenceKind: String(raw["repairEvidenceKind"]),
			RepairOwnerModuleID: String(raw["repairOwnerModuleId"]), RepairOperationKinds: Strings(raw["repairOperationKinds"]),
			RepairTargetPolicy: runtimecontract.RuntimeOperationTargetPolicy(String(raw["repairTargetPolicy"])), RepairTargetRole: String(raw["repairTargetRole"]),
			RepairDeclarationTargetSelectorRef:           String(raw["repairDeclarationTargetSelectorRef"]),
			RepairDeclarationTargetSelectorOwnerRef:      String(raw["repairDeclarationTargetSelectorOwnerRef"]),
			RepairDeclarationTargetSelectorSubjectRef:    String(raw["repairDeclarationTargetSelectorSubjectRef"]),
			RepairDeclarationTargetSelectorCandidateRefs: Strings(raw["repairDeclarationTargetSelectorCandidateRefs"]),
			RepairOperationTargetSelectorRef:             String(raw["repairOperationTargetSelectorRef"]),
		})
	}
	return ret
}

func ValidationFailureRepairConvergenceHasExactShape(raw map[string]any) bool {
	return validationFailureRepairConvergenceHasExactShape(raw)
}

func ValidationFailureRepairRouteHasExactShape(raw map[string]any) bool {
	return validationFailureRepairRouteHasExactShape(raw)
}

func validationFailureRepairConvergenceHasExactShape(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for key, value := range raw {
		switch key {
		case "id":
			if _, ok := value.(string); !ok {
				return false
			}
		case "routes":
			items, exact := ObjectSliceExact(value)
			if !exact || len(items) == 0 {
				return false
			}
			for _, item := range items {
				if !validationFailureRepairRouteHasExactShape(item) {
					return false
				}
			}
		case "requireFreshValidation":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "maxAttempts":
			if !IntegerHasExactShape(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validationFailureRepairRouteHasExactShape(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for key, value := range raw {
		switch key {
		case "id", "triggerOwnerModuleId", "triggerOperationKind", "triggerDiagnosticClass", "triggerEvidenceTargetType", "triggerOwnerDiagnosticCode",
			"repairEvidenceKind", "repairOwnerModuleId", "repairTargetPolicy", "repairTargetRole",
			"repairDeclarationTargetSelectorRef", "repairDeclarationTargetSelectorOwnerRef", "repairDeclarationTargetSelectorSubjectRef", "repairOperationTargetSelectorRef":
			if _, ok := value.(string); !ok {
				return false
			}
		case "requireInputAddition", "requireInputRepair":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "repairOperationKinds", "repairDeclarationTargetSelectorCandidateRefs":
			if !StringListHasExactShape(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
