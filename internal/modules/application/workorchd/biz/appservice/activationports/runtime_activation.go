package activationports

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func RuntimeActivationContext(
	providerSelection runtimecontract.ProviderInferenceSelection,
	dialogLanguage string,
	defaultSoul *soulmodel.Soul,
	stageSouls map[string]*soulmodel.Soul,
	soulRouting appmodelcore.SoulRouting,
	skills []workorchmanagementcontract.Skill,
	mcpServerIDs []string,
	capabilities []*mcpmodel.Capability,
	memories []*memorymodel.MemoryItem,
	memorySelection *MemorySelectionEvidence,
	evidencePolicies []workorcheventpayload.Payload,
	executionContract map[string]any,
	continuation runtimecontract.RuntimeContinuationContext,
) (runtimecontract.RuntimeActivationContext, error) {
	if err := validateRuntimePresealTargetDeclarations(executionContract, nil); err != nil {
		return runtimecontract.RuntimeActivationContext{}, err
	}
	runtimePolicies, err := compileRuntimeEvidencePolicies("Workspace", evidencePolicies)
	if err != nil {
		return runtimecontract.RuntimeActivationContext{}, err
	}
	mcpCapabilities := runtimeMCPCapabilityContexts(capabilities)
	runtimeSkills, skillBudget, err := runtimeSkillContexts(skills)
	if err != nil {
		return runtimecontract.RuntimeActivationContext{}, err
	}
	ret := runtimecontract.RuntimeActivationContext{
		ProviderSelection:  providerSelection.Normalized(),
		DialogLanguage:     strings.TrimSpace(dialogLanguage),
		DefaultSoul:        runtimeSoulContext(defaultSoul),
		Skills:             runtimeSkills,
		SkillContextBudget: skillBudget,
		MCPServerIDs:       runtimeMCPServerIDs(mcpServerIDs),
		MCPCapabilities:    mcpCapabilities,
		OperationPolicies:  runtimeMCPOperationPolicies(mcpCapabilities),
		Memories:           runtimeMemoryContexts(memories),
		MemorySelection:    runtimeMemorySelectionEvidence(memorySelection),
		EvidencePolicies:   runtimePolicies,
		ExecutionContract:  runtimeExecutionContract(executionContract),
		Continuation:       continuation,
	}
	stages := make([]string, 0, len(soulRouting.Bindings))
	for stage := range soulRouting.Bindings {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	for _, stage := range stages {
		binding := soulRouting.Bindings[stage]
		soul := runtimeSoulContext(stageSouls[stage])
		if soul == nil {
			continue
		}
		ret.StageSoulBindings = append(ret.StageSoulBindings, runtimecontract.RuntimeStageSoulBinding{
			Stage: stage, Source: binding.Source, Soul: *soul,
		})
	}
	return ret.Normalized(), nil
}

// validateRuntimePresealTargetDeclarations rejects the removed exact-target
// startup surface instead of silently treating it as a compatibility alias.
// Startup may seal selector candidate bounds; the concrete prospective target
// first appears in an SM-3 proposal and becomes authority only after SM-2
// binding.
func validateRuntimePresealTargetDeclarations(executionContract map[string]any, evidencePolicies []workorcheventpayload.Payload) error {
	if runtimeValueContainsField(executionContract, "prospectiveTargetRef") {
		return fmt.Errorf("%w: pre-seal execution contract rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
	}
	if runtimeExecutionContractContainsBootstrapArtifactField(executionContract, "targetRef") {
		return fmt.Errorf("%w: pre-seal bootstrap artifact rejects targetRef; the semantic proposal must choose its target after activation", common.ErrInvalidArgument)
	}
	for _, policy := range evidencePolicies {
		if runtimePayloadFieldsContainField(policy.Fields, "prospectiveTargetRef") {
			return fmt.Errorf("%w: pre-seal evidence policy rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
		}
	}
	return nil
}

func runtimeExecutionContractContainsBootstrapArtifactField(executionContract map[string]any, fieldName string) bool {
	profiles := runtimeMap(executionContract["profileContracts"])
	for _, rawProfile := range profiles {
		profile := runtimeMap(rawProfile)
		for _, recipe := range runtimeMapSlice(profile["bootstrapRecipes"]) {
			for _, artifact := range runtimeMapSlice(recipe["artifacts"]) {
				for key := range artifact {
					if strings.EqualFold(strings.TrimSpace(key), fieldName) {
						return true
					}
				}
			}
		}
	}
	return false
}

func runtimeValueContainsField(value any, fieldName string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(strings.TrimSpace(key), fieldName) || runtimeValueContainsField(child, fieldName) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if runtimeValueContainsField(child, fieldName) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if runtimeValueContainsField(child, fieldName) {
				return true
			}
		}
	}
	return false
}

func runtimePayloadFieldsContainField(fields []workorcheventpayload.Field, fieldName string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), fieldName) {
			return true
		}
		if runtimePayloadValueContainsField(field.Value, fieldName) {
			return true
		}
	}
	return false
}

func runtimePayloadValueContainsField(value workorcheventpayload.Value, fieldName string) bool {
	switch value.Kind {
	case workorcheventpayload.ValueObject:
		return runtimePayloadFieldsContainField(value.Object, fieldName)
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if runtimePayloadValueContainsField(item, fieldName) {
				return true
			}
		}
	}
	return false
}

// validateRuntimeEvidencePolicyInputs keeps the raw owner snapshot lossless at
// the admission boundary. Runtime normalization is intentionally ID-based; an
// empty or duplicate ID would otherwise be silently dropped or overwritten
// before the typed policy validator could observe it.
func validateRuntimeEvidencePolicyInputs(owner string, items []workorcheventpayload.Payload) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "Runtime"
	}
	if err := validateRuntimePresealTargetDeclarations(nil, items); err != nil {
		return fmt.Errorf("%s evidence policies: %w", owner, err)
	}
	seen := make(map[string]struct{}, len(items))
	for idx, item := range items {
		id, ok := item.String("id")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return fmt.Errorf("%w: %s evidence policy %d requires a non-empty string id", common.ErrInvalidArgument, owner, idx+1)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: %s evidence policies duplicate id %q", common.ErrInvalidArgument, owner, id)
		}
		seen[id] = struct{}{}
		policy := runtimeEvidencePolicy(item)
		if err := policy.Validate(); err != nil {
			return fmt.Errorf("%w: %s evidence policy %q is invalid: %v", common.ErrInvalidArgument, owner, id, err)
		}
	}
	return nil
}

func RuntimeDefinitionChangeImpact(input map[string]any) *runtimecontract.RuntimeDefinitionChangeImpact {
	if len(input) == 0 {
		return nil
	}
	goalChange := runtimeMap(input["goalChange"])
	dodChange := runtimeMap(input["doDChange"])
	ret := runtimecontract.RuntimeDefinitionChangeImpact{
		Version: runtimeString(input["version"]), Changed: runtimeBool(input["changed"]), Policy: runtimeString(input["policy"]),
		RequiredPreExecutionAssessment: runtimeBool(input["requiredPreExecutionAssessment"]),
		GoalChange:                     runtimeDefinitionChangeSet(goalChange, "effectiveGoalId", "latestGoalId", "previousGoalIds", "allGoalIds", "allGoalsAreConstraints"),
		DoDChange:                      runtimeDefinitionChangeSet(dodChange, "effectiveDoDId", "latestDoDId", "previousDoDIds", "allDoDIds", "allDoDItemsAreAcceptanceGates"),
		StrategyRequirements:           runtimeStrings(input["strategyRequirements"]),
	}
	ret = ret.Normalized()
	return &ret
}

func runtimeDefinitionChangeSet(input map[string]any, effectiveKey, latestKey, previousKey, allKey, constrainKey string) runtimecontract.RuntimeDefinitionChangeSet {
	ret := runtimecontract.RuntimeDefinitionChangeSet{
		EffectiveDefinitionID: runtimeString(input[effectiveKey]), LatestDefinitionID: runtimeString(input[latestKey]),
		PreviousDefinitionIDs: runtimeStrings(input[previousKey]), AllDefinitionIDs: runtimeStrings(input[allKey]),
		ChangedInPlace: runtimeBool(input["changedInPlace"]), AllEntriesConstrain: runtimeBool(input[constrainKey]),
	}
	for _, revision := range runtimeMapSlice(input["definitionRevisions"]) {
		ret.DefinitionRevisions = append(ret.DefinitionRevisions, runtimecontract.RuntimeDefinitionRevision{
			ID: runtimeString(revision["id"]), RevisionCount: runtimeInt(revision["revisionCount"]), Source: runtimeString(revision["source"]),
			DefinitionRevision: runtimeString(revision["definitionRevision"]), PreviousDefinitionHash: runtimeString(revision["previousDefinitionHash"]),
			PreviousDefinitionID: runtimeString(revision["previousDefinitionId"]), LastEventID: runtimeString(revision["lastEventId"]),
			LastOccurredAt: runtimeString(revision["lastOccurredAt"]),
		})
	}
	return ret
}

func runtimeExecutionContract(input map[string]any) runtimecontract.RuntimeExecutionContract {
	ret := runtimecontract.RuntimeExecutionContract{
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		StateChangePolicy:         runtimecontract.RuntimeStateChangePolicy(runtimeString(input["stateChangePolicy"])),
		MaxRepairAttemptsPerFocus: runtimeInt(input["maxRepairAttemptsPerFocus"]),
	}
	if recovery := runtimeMap(input["candidateDocumentRecovery"]); len(recovery) > 0 {
		ret.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{
			MaxAttempts: runtimeInt(recovery["maxAttempts"]),
		}
	}
	for _, item := range runtimeMapSlice(input["capabilityUsagePolicies"]) {
		ret.CapabilityUsagePolicies = append(ret.CapabilityUsagePolicies, runtimecontract.RuntimeCapabilityUsagePolicy{
			ID: runtimeString(item["id"]), Scope: runtimeString(item["scope"]),
			AppliesToCategory: runtimeString(item["appliesToCategory"]), EvidenceStrength: runtimeString(item["evidenceStrength"]),
			UsageMode: runtimeString(item["usageMode"]), FallbackPolicy: runtimeString(item["fallbackPolicy"]),
			TriggerStages: runtimeStrings(item["triggerStages"]), TriggerSignals: runtimeStrings(item["triggerSignals"]),
			PreferredTools: runtimeStrings(item["preferredTools"]), RequiredAfterKinds: runtimeStrings(item["requiredAfterKinds"]),
			Notes: runtimeStrings(item["notes"]),
		})
	}
	for _, item := range runtimeMapSlice(input["validationPolicies"]) {
		ret.ValidationPolicies = append(ret.ValidationPolicies, runtimecontract.RuntimeValidationPolicy{
			ID: runtimeString(item["id"]), Scope: runtimeString(item["scope"]),
			AppliesToEvidenceKinds: runtimeStrings(item["appliesToEvidenceKinds"]),
			ValidationSafetyClass:  runtimeString(item["validationSafetyClass"]),
			DiscoveryRequired:      runtimeBool(item["discoveryRequired"]), DiscoverySources: runtimeStrings(item["discoverySources"]),
			AllowedExecutionModes:            runtimeStrings(item["allowedExecutionModes"]),
			MaxValidationOnlyRetriesPerFocus: runtimeInt(item["maxValidationOnlyRetriesPerFocus"]),
			ValidationScratchRequired:        runtimeBool(item["validationScratchRequired"]),
			AllowedValidationModes:           runtimeStrings(item["allowedValidationModes"]), Notes: runtimeStrings(item["notes"]),
		})
	}
	profileContracts := runtimeMap(input["profileContracts"])
	profileIDs := make([]string, 0, len(profileContracts))
	for profileID := range profileContracts {
		profileIDs = append(profileIDs, profileID)
	}
	sort.Strings(profileIDs)
	for _, profileID := range profileIDs {
		profile := runtimeMap(profileContracts[profileID])
		contract := runtimecontract.RuntimeProfileContract{ID: profileID}
		for _, item := range runtimeMapSlice(profile["acceptanceDimensions"]) {
			contract.AcceptanceDimensions = append(contract.AcceptanceDimensions, runtimecontract.RuntimeAcceptanceDimension{
				ID: runtimeString(item["id"]), Required: runtimeBool(item["required"]), Description: runtimeString(item["description"]),
			})
		}
		for _, item := range runtimeMapSlice(profile["deliverableExpectations"]) {
			contract.DeliverableExpectations = append(contract.DeliverableExpectations, runtimecontract.RuntimeDeliverableExpectation{
				ID: runtimeString(item["id"]), Required: runtimeBool(item["required"]),
				EvidenceRole: runtimeString(item["evidenceRole"]), Description: runtimeString(item["description"]),
			})
		}
		for _, item := range runtimeMapSlice(profile["coverageDependencies"]) {
			contract.CoverageDependencies = append(contract.CoverageDependencies, runtimecontract.RuntimeCoverageDependency{
				ID:                     runtimeString(item["id"]),
				SourceScope:            runtimeString(item["sourceScope"]),
				SourceRequirementID:    runtimeString(item["sourceRequirementId"]),
				DependentScope:         runtimeString(item["dependentScope"]),
				DependentRequirementID: runtimeString(item["dependentRequirementId"]),
				RequireSourceSettled:   runtimeBool(item["requireSourceSettled"]),
				TriggerEvidenceKinds:   runtimeStrings(item["triggerEvidenceKinds"]),
			})
		}
		for _, item := range runtimeMapSlice(profile["bootstrapRecipes"]) {
			recipe := runtimecontract.RuntimeBootstrapRecipe{
				ID:                runtimeString(item["id"]),
				RequiredGoalTerms: runtimeStrings(item["requiredGoalTerms"]),
				RequiredDoDTerms:  runtimeStrings(item["requiredDoDTerms"]),
			}
			for _, artifact := range runtimeMapSlice(item["artifacts"]) {
				recipe.Artifacts = append(recipe.Artifacts, runtimecontract.RuntimeBootstrapArtifact{
					ID: runtimeString(artifact["id"]), TargetRole: runtimeString(artifact["targetRole"]),
					AllowedOperationKinds: runtimeStrings(artifact["allowedOperationKinds"]), DependsOnArtifactIDs: runtimeStrings(artifact["dependsOnArtifactIds"]),
				})
			}
			contract.BootstrapRecipes = append(contract.BootstrapRecipes, recipe)
		}
		for _, item := range runtimeMapSlice(profile["evidenceRequirements"]) {
			requirementID := runtimeString(item["id"])
			requirementScope := runtimeString(item["scope"])
			acceptedVariants := make([]runtimecontract.RuntimeOperationVariantRef, 0)
			for _, variant := range runtimeMapSlice(item["acceptedOperationVariants"]) {
				acceptedVariants = append(acceptedVariants, runtimecontract.RuntimeOperationVariantRef{Kind: runtimeString(variant["kind"]), VariantID: runtimeString(variant["variantId"])})
			}
			stateRequirements := make([]runtimecontract.RuntimeProfileCapabilityStateRequirement, 0)
			for _, state := range runtimeMapSlice(item["capabilityStateRequirements"]) {
				stateRequirements = append(stateRequirements, runtimecontract.RuntimeProfileCapabilityStateRequirement{OwnerModuleID: runtimeString(state["ownerModuleId"]), CapabilityStateKind: runtimeString(state["capabilityStateKind"]), SubjectRef: runtimeString(state["subjectRef"])})
			}
			ownerFacts := make([]runtimecontract.RuntimeOwnerFactRequirement, 0)
			for _, ownerFact := range runtimeMapSlice(item["requiredOwnerFacts"]) {
				ownerFacts = append(ownerFacts, runtimecontract.RuntimeOwnerFactRequirement{
					OwnerModuleID: runtimeString(ownerFact["ownerModuleId"]),
					FactKind:      runtimecontract.RuntimeOwnerFactKind(runtimeString(ownerFact["factKind"])),
					OperationKind: runtimeString(ownerFact["operationKind"]),
				})
			}
			ownerFactConvergences := make([]runtimecontract.RuntimeOwnerFactConvergence, 0)
			for _, convergence := range runtimeMapSlice(item["ownerFactConvergences"]) {
				ownerFactConvergences = append(ownerFactConvergences, runtimecontract.RuntimeOwnerFactConvergence{
					Requirement: runtimecontract.RuntimeOwnerFactRequirement{
						OwnerModuleID: runtimeString(convergence["ownerModuleId"]),
						FactKind:      runtimecontract.RuntimeOwnerFactKind(runtimeString(convergence["factKind"])),
						OperationKind: runtimeString(convergence["operationKind"]),
					},
					RequirePreexisting:                 runtimeBool(convergence["requirePreexisting"]),
					MissingTrigger:                     runtimeString(convergence["missingTrigger"]),
					AcquisitionOwnerModuleID:           runtimeString(convergence["acquisitionOwnerModuleId"]),
					AcquisitionOperationKind:           runtimeString(convergence["acquisitionOperationKind"]),
					AcquisitionOperationVariantID:      runtimeString(convergence["acquisitionOperationVariantId"]),
					AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicy(runtimeString(convergence["acquisitionTargetPolicy"])),
					AcquisitionTargetSelectorRef:       runtimeString(convergence["acquisitionTargetSelectorRef"]),
					ProspectiveSatisfactionContractRef: runtimeString(convergence["prospectiveSatisfactionContractRef"]),
					TargetSelectorRef:                  runtimeString(convergence["targetSelectorRef"]),
					TargetSelectorOwnerRef:             runtimeString(convergence["targetSelectorOwnerRef"]),
					TargetSelectorSubjectRef:           runtimeString(convergence["targetSelectorSubjectRef"]),
					ReobserveOwnerModuleID:             runtimeString(convergence["reobserveOwnerModuleId"]),
					ReobserveFactKind:                  runtimecontract.RuntimeOwnerFactKind(runtimeString(convergence["reobserveFactKind"])),
					MaxAcquisitionAttempts:             runtimeInt(convergence["maxAcquisitionAttempts"]),
					DependsOnRequirementRefs:           runtimeStrings(convergence["dependsOnRequirementRefs"]),
				})
			}
			validationFailureRepairConvergence := runtimeValidationFailureRepairConvergence(item)
			contract.EvidenceRequirements = append(contract.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: requirementID, Scope: requirementScope, EvidenceRole: runtimeString(item["evidenceRole"]),
				TargetRole: runtimeString(item["targetRole"]), TargetRefs: runtimeStrings(item["targetRefs"]),
				TargetDiscoverySources: runtimeStrings(item["targetDiscoverySources"]), TargetSelectorRef: runtimeString(item["targetSelectorRef"]), TargetSelectorOwnerRef: runtimeString(item["targetSelectorOwnerRef"]), TargetSelectorSubjectRef: runtimeString(item["targetSelectorSubjectRef"]), TargetSelectorCandidateRefs: runtimeStrings(item["targetSelectorCandidateRefs"]), ProspectiveTargetAllOfConstraintRefs: runtimeStrings(item["prospectiveTargetAllOfConstraintRefs"]), RequireTargetGrounding: runtimeBool(item["requireTargetGrounding"]),
				RequiredEvidenceKinds: runtimeStrings(item["requiredEvidenceKinds"]),
				RequireDirectEvidence: runtimeBool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeBool(item["requireClaimEvidence"]),
				RequireExecutableBehaviorEvidence: runtimeBool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: runtimeBool(item["requireCoverageTargets"]),
				AcceptedStepKinds:                  runtimeStrings(item["acceptedStepKinds"]),
				AcceptedOperationVariants:          acceptedVariants,
				ValidationRecipeID:                 runtimeString(item["validationRecipeId"]),
				RequiredCheckRefs:                  runtimeStrings(item["requiredCheckRefs"]),
				MetricConstraints:                  runtimeStrings(item["metricConstraints"]),
				ArtifactRefs:                       runtimeStrings(item["artifactRefs"]),
				RequiredOwnerFacts:                 ownerFacts,
				OwnerFactConvergences:              ownerFactConvergences,
				ValidationFailureRepairConvergence: validationFailureRepairConvergence,
				CapabilityStateRequirements:        stateRequirements,
				RejectedStepKinds:                  runtimeStrings(item["rejectedStepKinds"]), Description: runtimeString(item["description"]),
			})
		}
		ret.ProfileContracts = append(ret.ProfileContracts, contract)
	}
	if policy := runtimeMap(input["acceptanceSlicePolicy"]); len(policy) > 0 {
		slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{
			SchemaVersion:            runtimeString(policy["schemaVersion"]),
			MaxWorkItems:             runtimeInt(policy["maxWorkItems"]),
			MaxFocuses:               runtimeInt(policy["maxFocuses"]),
			MaxCandidateContextItems: runtimeInt(policy["maxCandidateContextItems"]),
			CompletionPolicy:         runtimeString(policy["completionPolicy"]),
			ResultPolicy:             runtimeString(policy["resultPolicy"]),
		}
		for _, item := range runtimeMapSlice(policy["groups"]) {
			match := runtimeMap(item["match"])
			slicePolicy.Groups = append(slicePolicy.Groups, runtimecontract.RuntimeAcceptanceSliceGroupPolicy{
				ID: runtimeString(item["id"]), Title: runtimeString(item["title"]), Scope: runtimeString(item["scope"]),
				Priority: runtimeInt(item["priority"]), MaxWorkItems: runtimeInt(item["maxWorkItems"]),
				Match: runtimecontract.RuntimeAcceptanceSliceMatch{
					EvidenceRoles:        runtimeStrings(match["evidenceRoles"]),
					MissingEvidenceKinds: runtimeStrings(match["missingEvidenceKinds"]),
					SourceRequirementIDs: runtimeStrings(match["sourceRequirementIds"]),
					SourceScopes:         runtimeStrings(match["sourceScopes"]),
					All:                  runtimeBool(match["all"]),
				},
			})
		}
		ret.AcceptanceSlicePolicy = &slicePolicy
	}
	if policy := runtimeMap(input["templateStagePolicies"]); len(policy) > 0 {
		stagePolicy := runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: runtimeString(policy["schemaVersion"])}
		for _, item := range runtimeMapSlice(policy["stages"]) {
			completion := runtimeMap(item["completion"])
			governance := runtimeMap(item["governance"])
			policyRefs := runtimeMap(item["policyRefs"])
			stage := runtimecontract.RuntimeTemplateStage{
				ID: runtimeString(item["id"]), SoulBinding: runtimeString(item["soulBinding"]),
				Completion: runtimecontract.RuntimeTemplateStageCompletion{Policy: runtimeString(completion["policy"])},
				Governance: runtimecontract.RuntimeTemplateStageGovernance{
					DecisionMode: runtimeString(governance["decisionMode"]), ApprovalMode: runtimeString(governance["approvalMode"]),
				},
				PolicyRefs: runtimecontract.RuntimeTemplateStagePolicyRefs{
					EvidencePolicies: runtimeStrings(policyRefs["evidencePolicies"]), CapabilityUsagePolicies: runtimeStrings(policyRefs["capabilityUsagePolicies"]),
				},
			}
			for _, output := range runtimeMapSlice(item["requiredOutputs"]) {
				stage.RequiredOutputs = append(stage.RequiredOutputs, runtimecontract.RuntimeTemplateStageRequiredOutput{
					Kind: runtimeString(output["kind"]), EvidenceKinds: runtimeStrings(output["evidenceKinds"]),
				})
			}
			stagePolicy.Stages = append(stagePolicy.Stages, stage)
		}
		ret.TemplateStagePolicy = &stagePolicy
	}
	return ret.Normalized()
}

func runtimeValidationFailureRepairConvergence(input map[string]any) *runtimecontract.RuntimeValidationFailureRepairConvergence {
	rawValue, present := input["validationFailureRepairConvergence"]
	if !present {
		return nil
	}
	raw, object := rawValue.(map[string]any)
	if !object || !runtimeValidationFailureRepairConvergenceHasExactShape(raw) {
		// Keep malformed current input visible to the typed admission compiler.
		// Treating null, a scalar, an array, an empty object, an unknown field or
		// an alternative relation as absence would otherwise admit a lossy
		// profile after normalization.
		return &runtimecontract.RuntimeValidationFailureRepairConvergence{}
	}
	return &runtimecontract.RuntimeValidationFailureRepairConvergence{
		ID:                                           runtimeString(raw["id"]),
		TriggerOwnerModuleID:                         runtimeString(raw["triggerOwnerModuleId"]),
		TriggerOperationKind:                         runtimeString(raw["triggerOperationKind"]),
		TriggerDiagnosticClasses:                     runtimeStrings(raw["triggerDiagnosticClasses"]),
		RepairEvidenceKind:                           runtimeString(raw["repairEvidenceKind"]),
		RepairOwnerModuleID:                          runtimeString(raw["repairOwnerModuleId"]),
		RepairOperationKinds:                         runtimeStrings(raw["repairOperationKinds"]),
		RepairTargetPolicy:                           runtimecontract.RuntimeOperationTargetPolicy(runtimeString(raw["repairTargetPolicy"])),
		RepairTargetRole:                             runtimeString(raw["repairTargetRole"]),
		RepairDeclarationTargetSelectorRef:           runtimeString(raw["repairDeclarationTargetSelectorRef"]),
		RepairDeclarationTargetSelectorOwnerRef:      runtimeString(raw["repairDeclarationTargetSelectorOwnerRef"]),
		RepairDeclarationTargetSelectorSubjectRef:    runtimeString(raw["repairDeclarationTargetSelectorSubjectRef"]),
		RepairDeclarationTargetSelectorCandidateRefs: runtimeStrings(raw["repairDeclarationTargetSelectorCandidateRefs"]),
		RepairOperationTargetSelectorRef:             runtimeString(raw["repairOperationTargetSelectorRef"]),
		RequireFreshValidation:                       runtimeBool(raw["requireFreshValidation"]),
		MaxAttempts:                                  runtimeInt(raw["maxAttempts"]),
	}
}

func runtimeValidationFailureRepairConvergenceHasExactShape(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for key, value := range raw {
		switch key {
		case "id",
			"triggerOwnerModuleId",
			"triggerOperationKind",
			"repairEvidenceKind",
			"repairOwnerModuleId",
			"repairTargetPolicy",
			"repairTargetRole",
			"repairDeclarationTargetSelectorRef",
			"repairDeclarationTargetSelectorOwnerRef",
			"repairDeclarationTargetSelectorSubjectRef",
			"repairOperationTargetSelectorRef":
			if _, ok := value.(string); !ok {
				return false
			}
		case "triggerDiagnosticClasses", "repairOperationKinds", "repairDeclarationTargetSelectorCandidateRefs":
			if !runtimeStringListHasExactShape(value) {
				return false
			}
		case "requireFreshValidation":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "maxAttempts":
			if !runtimeIntegerHasExactShape(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func runtimeStringListHasExactShape(value any) bool {
	switch typed := value.(type) {
	case []string:
		return true
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func runtimeIntegerHasExactShape(value any) bool {
	switch typed := value.(type) {
	case int, int64:
		return true
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0) && typed == math.Trunc(typed)
	default:
		return false
	}
}

func compileRuntimeEvidencePolicies(owner string, items []workorcheventpayload.Payload) ([]runtimecontract.RuntimeEvidencePolicy, error) {
	if err := validateRuntimeEvidencePolicyInputs(owner, items); err != nil {
		return nil, err
	}
	ret := make([]runtimecontract.RuntimeEvidencePolicy, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimeEvidencePolicy(item))
	}
	normalized := runtimecontract.NormalizeRuntimeEvidencePolicies(ret)
	if len(normalized) != len(ret) {
		return nil, fmt.Errorf("%w: %s evidence policy normalization changed policy cardinality", common.ErrInvalidArgument, strings.TrimSpace(owner))
	}
	return normalized, nil
}

func runtimeEvidencePolicy(item workorcheventpayload.Payload) runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: payloadString(item, "id"), Profile: payloadString(item, "profile"), Rule: payloadString(item, "rule"),
		AppliesTo: payloadString(item, "appliesTo"), CommandName: payloadString(item, "commandName"),
		ArgsContain: payloadStrings(item, "argsContain"), RecipeDeclared: payloadBool(item, "recipeDeclared"), RecipeArgs: payloadStrings(item, "recipeArgs"), RecipeTimeoutMillis: int64(payloadInt(item, "recipeTimeoutMillis")),
		RequiredArgs: payloadStrings(item, "requiredArgs"), ZeroWorkMarkers: payloadStrings(item, "zeroWorkMarkers"),
		WorkEvidenceMarkers: payloadStrings(item, "workEvidenceMarkers"), QualityOnZeroWork: payloadString(item, "qualityOnZeroWork"),
		MarksZeroWork: payloadBool(item, "zeroWork"), EvidenceTags: payloadStrings(item, "evidenceTags"),
		EvidenceReason: payloadString(item, "evidenceReason"), CoverageRequirements: payloadCoverageEvidenceRequirements(item, "coverageRequirements"),
		Description: payloadString(item, "description"),
	}
}

func payloadString(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return value
}

func payloadBool(input workorcheventpayload.Payload, key string) bool {
	value, _ := input.Bool(key)
	return value
}

func payloadInt(input workorcheventpayload.Payload, key string) int64 {
	value, _ := input.Int64(key)
	return value
}

func payloadStrings(input workorcheventpayload.Payload, key string) []string {
	value, _ := input.StringSlice(key)
	return value
}

func payloadCoverageEvidenceRequirements(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	value, ok := input.Field(key)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]runtimecontract.RuntimeCoverageEvidenceRequirement, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		payload := workorcheventpayload.Payload{Fields: item.Object}
		ret = append(ret, runtimecontract.RuntimeCoverageEvidenceRequirement{
			ID: payloadString(payload, "id"), Scope: payloadString(payload, "scope"), EvidenceRole: payloadString(payload, "evidenceRole"),
			TargetRole: payloadString(payload, "targetRole"), TargetRefs: payloadStrings(payload, "targetRefs"),
			TargetDiscoverySources: payloadStrings(payload, "targetDiscoverySources"), TargetSelectorRef: payloadString(payload, "targetSelectorRef"),
			TargetSelectorOwnerRef: payloadString(payload, "targetSelectorOwnerRef"), TargetSelectorSubjectRef: payloadString(payload, "targetSelectorSubjectRef"),
			TargetSelectorCandidateRefs: payloadStrings(payload, "targetSelectorCandidateRefs"), ProspectiveTargetAllOfConstraintRefs: payloadStrings(payload, "prospectiveTargetAllOfConstraintRefs"), RequireTargetGrounding: payloadBool(payload, "requireTargetGrounding"),
			RequiredEvidenceKinds: payloadStrings(payload, "requiredEvidenceKinds"),
			RequireDirectEvidence: payloadBool(payload, "requireDirectEvidence"), RequireClaimEvidence: payloadBool(payload, "requireClaimEvidence"),
			RequireExecutableBehaviorEvidence: payloadBool(payload, "requireExecutableBehaviorEvidence"), RequireCoverageTargets: payloadBool(payload, "requireCoverageTargets"),
			AcceptedStepKinds: payloadStrings(payload, "acceptedStepKinds"), RejectedStepKinds: payloadStrings(payload, "rejectedStepKinds"),
			AcceptedOperationVariants: runtimeOperationVariantsFromPayload(payload, "acceptedOperationVariants"),
			ValidationRecipeID:        payloadString(payload, "validationRecipeId"), RequiredCheckRefs: payloadStrings(payload, "requiredCheckRefs"),
			MetricConstraints: payloadStrings(payload, "metricConstraints"), ArtifactRefs: payloadStrings(payload, "artifactRefs"),
			Description: payloadString(payload, "description"),
		})
	}
	return ret
}

func runtimeOperationVariantsFromPayload(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeOperationVariantRef {
	value, ok := input.Field(key)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		payload := workorcheventpayload.Payload{Fields: item.Object}
		ret = append(ret, runtimecontract.RuntimeOperationVariantRef{Kind: payloadString(payload, "kind"), VariantID: payloadString(payload, "variantId")})
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}

func runtimeCoverageEvidenceRequirements(value any) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	items := runtimeMapSlice(value)
	ret := make([]runtimecontract.RuntimeCoverageEvidenceRequirement, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimecontract.RuntimeCoverageEvidenceRequirement{
			ID: runtimeString(item["id"]), Scope: runtimeString(item["scope"]), EvidenceRole: runtimeString(item["evidenceRole"]),
			TargetRole: runtimeString(item["targetRole"]), TargetRefs: runtimeStrings(item["targetRefs"]),
			TargetDiscoverySources: runtimeStrings(item["targetDiscoverySources"]), TargetSelectorRef: runtimeString(item["targetSelectorRef"]),
			TargetSelectorOwnerRef: runtimeString(item["targetSelectorOwnerRef"]), TargetSelectorSubjectRef: runtimeString(item["targetSelectorSubjectRef"]),
			TargetSelectorCandidateRefs: runtimeStrings(item["targetSelectorCandidateRefs"]), ProspectiveTargetAllOfConstraintRefs: runtimeStrings(item["prospectiveTargetAllOfConstraintRefs"]), RequireTargetGrounding: runtimeBool(item["requireTargetGrounding"]),
			RequiredEvidenceKinds: runtimeStrings(item["requiredEvidenceKinds"]),
			RequireDirectEvidence: runtimeBool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeBool(item["requireClaimEvidence"]),
			RequireExecutableBehaviorEvidence: runtimeBool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: runtimeBool(item["requireCoverageTargets"]),
			AcceptedStepKinds: runtimeStrings(item["acceptedStepKinds"]), RejectedStepKinds: runtimeStrings(item["rejectedStepKinds"]),
			AcceptedOperationVariants: runtimeOperationVariants(item["acceptedOperationVariants"]),
			ValidationRecipeID:        runtimeString(item["validationRecipeId"]), RequiredCheckRefs: runtimeStrings(item["requiredCheckRefs"]),
			MetricConstraints: runtimeStrings(item["metricConstraints"]), ArtifactRefs: runtimeStrings(item["artifactRefs"]),
			Description: runtimeString(item["description"]),
		})
	}
	return ret
}

func runtimeOperationVariants(value any) []runtimecontract.RuntimeOperationVariantRef {
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0)
	for _, item := range runtimeMapSlice(value) {
		ret = append(ret, runtimecontract.RuntimeOperationVariantRef{Kind: runtimeString(item["kind"]), VariantID: runtimeString(item["variantId"])})
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}

func runtimeMapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func runtimeMap(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func runtimeStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := runtimeString(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func runtimeString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func runtimeBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func runtimeInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func runtimeSoulContext(item *soulmodel.Soul) *runtimecontract.RuntimeSoulContext {
	if item == nil {
		return nil
	}
	return &runtimecontract.RuntimeSoulContext{
		ID: item.ID, Revision: item.Revision, ContentHash: item.ContentHash, SourceProvenance: item.SourceProvenance, Name: item.Name, Description: item.Description,
		BusinessDomains: append([]string(nil), item.BusinessDomains...), Instructions: item.SoulMD,
		AllowedSkillRefs:            append([]string(nil), item.AllowedSkillRefs...),
		AllowedMCPServerRefs:        append([]string(nil), item.AllowedMCPServerRefs...),
		AllowedCapabilityRefs:       append([]string(nil), item.AllowedCapabilityRefs...),
		AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...),
		AllowedToolKinds:            append([]string(nil), item.AllowedToolKinds...),
	}
}

func runtimeSkillContexts(items []workorchmanagementcontract.Skill) ([]runtimecontract.RuntimeSkillContext, *runtimecontract.RuntimeSkillContextBudget, error) {
	ret := make([]runtimecontract.RuntimeSkillContext, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimecontract.RuntimeSkillContext{
			ID: item.ID, Name: item.Name, Version: item.Version, Description: item.Description,
			TriggerRules: append([]string(nil), item.TriggerRules...), Instruction: item.Instruction,
			ReferenceRefs: append([]string(nil), item.ReferenceRefs...), RequiredToolKinds: append([]string(nil), item.RequiredToolKinds...),
			RequiredMCPServerRefs: append([]string(nil), item.RequiredMCPServerRefs...), RequiredCapabilityRefs: append([]string(nil), item.RequiredCapabilityRefs...),
			Constraints: append([]string(nil), item.Constraints...),
			Provenance: workorchskillcontract.Provenance{
				Source: item.Source, Scope: item.Scope, WorkspaceID: common.WorkspaceID(item.WorkspaceID), ContentHash: item.ContentHash,
			},
		})
	}
	return runtimecontract.SealRuntimeSkillContexts(ret)
}

func runtimeMCPServerIDs(values []string) []common.MCPServerID {
	ret := make([]common.MCPServerID, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, common.MCPServerID(value))
		}
	}
	return ret
}

func runtimeMCPCapabilityContexts(items []*mcpmodel.Capability) []runtimecontract.RuntimeMCPCapabilityContext {
	ret := make([]runtimecontract.RuntimeMCPCapabilityContext, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cap := runtimecontract.RuntimeMCPCapabilityContext{
			ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
			Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
			InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
			RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass,
			URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: capabilityAvailability(item.Enabled),
		}
		// RVC-R8: registry Kind=tool is not a Runtime operation kind; seal OperationKind + EffectScope.
		ret = append(ret, runtimecontract.SealMCPCapabilityEffect(cap))
	}
	return ret
}

func capabilityAvailability(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// runtimeMCPOperationPolicies is the application-side adapter from the
// MCP-registry owner's typed capability metadata to Runtime's owner-neutral
// effect/replay fact. Runtime core never needs to understand server/tool
// parameter names after this boundary.
func runtimeMCPOperationPolicies(capabilities []runtimecontract.RuntimeMCPCapabilityContext) []runtimecontract.RuntimeOperationPolicyFact {
	ret := make([]runtimecontract.RuntimeOperationPolicyFact, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = runtimecontract.SealMCPCapabilityEffect(capability)
		if !capability.Enabled || capability.OperationKind != runtimecontract.StepKindMCPToolCall || capability.ID == "" || capability.ServerID == "" || strings.TrimSpace(capability.Name) == "" {
			continue
		}
		serverID := string(capability.ServerID)
		tool := capability.Name
		ret = append(ret, runtimecontract.RuntimeOperationPolicyFact{
			ID:            "operation-policy/" + string(capability.ID),
			OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			OperationKind: capability.OperationKind,
			SubjectRef:    string(capability.ID),
			ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{
				{Name: "serverId", String: &serverID},
				{Name: "tool", String: &tool},
			},
			EffectScope:      runtimecontract.RuntimeEffectScope(capability.EffectScope),
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass),
		})
	}
	return runtimecontract.NormalizeRuntimeOperationPolicyFacts(ret)
}

func runtimeMemoryContexts(items []*memorymodel.MemoryItem) []runtimecontract.RuntimeMemoryContext {
	ret := make([]runtimecontract.RuntimeMemoryContext, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		provenance := "workspace"
		if item.RunID != "" {
			provenance = "historical_run"
		}
		ret = append(ret, runtimecontract.RuntimeMemoryContext{
			ID: item.ID, MemoryType: item.MemoryType, Summary: item.Summary, Content: item.Content,
			Tags: append([]string(nil), item.Tags...), Importance: item.Importance, Confidence: item.Confidence,
			Visibility: item.Visibility, Provenance: provenance, VectorScore: item.VectorScore,
		})
	}
	return ret
}

func runtimeMemorySelectionEvidence(input *MemorySelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	return runtimeMemorySelectionEvidenceDepth(input, 0)
}

func runtimeMemorySelectionEvidenceDepth(input *MemorySelectionEvidence, depth int) *workorchmemorycontract.RecallSelectionEvidence {
	if input == nil {
		return nil
	}
	if depth > 1 {
		return nil
	}
	ret := workorchmemorycontract.RecallSelectionEvidence{
		Policy: input.Policy, Status: input.Status, Reason: input.Reason,
		ProviderID: input.ProviderID, ModelID: input.ModelID, Operation: input.Operation,
		QueryTextPreview: input.QueryTextPreview, QueryTextSHA256: input.QueryTextSHA256,
		MinScore: input.MinScore, Limit: input.Limit, EmbeddingModel: input.EmbeddingModel,
		VectorDimension: input.VectorDimension, RefreshSource: input.RefreshSource,
		SelectionPolicyVersion: input.SelectionPolicyVersion,
		CandidateMemoryIDs:     runtimeMemoryIDs(input.CandidateMemoryIDs),
		SelectedMemoryIDs:      runtimeMemoryIDs(input.SelectedMemoryIDs),
		ExcludedMemoryIDs:      runtimeMemoryIDs(input.ExcludedMemoryIDs),
		TypeQuota:              append([]workorchmemorycontract.TypeQuotaEntry(nil), input.TypeQuota...),
		SelectionScores:        append([]workorchmemorycontract.RecallSelectionScore(nil), input.SelectionScores...),
		RetrievedMemoryIDs:     runtimeMemoryIDs(input.RetrievedMemoryIDs),
		ContextBudget:          input.ContextBudget,
		FallbackFrom:           input.FallbackFrom,
		SemanticRecall:         runtimeMemorySelectionEvidenceDepth(input.SemanticRecall, depth+1),
	}
	ret.RetrievedMemoryHits = make([]workorchmemorycontract.RecallHit, 0, len(input.RetrievedMemoryHits))
	for _, hit := range input.RetrievedMemoryHits {
		ret.RetrievedMemoryHits = append(ret.RetrievedMemoryHits, workorchmemorycontract.RecallHit{
			ID: common.MemoryID(hit.ID), Summary: hit.Summary, VectorScore: hit.Score, VectorDimension: hit.VectorDimension,
		})
	}
	value := ret.Normalized()
	return &value
}

func runtimeMemoryIDs(values []string) []common.MemoryID {
	if values == nil {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(values))
	for _, value := range values {
		ret = append(ret, common.MemoryID(strings.TrimSpace(value)))
	}
	return ret
}
