package activationports

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeActivationContext(
	providerTarget runtimecontract.ProviderModelTarget,
	dialogLanguage string,
	defaultSoul *soulmodel.Soul,
	stageSouls map[string]*soulmodel.Soul,
	soulRouting appmodelcore.SoulRouting,
	skills []contextregistrycore.Skill,
	mcpServerIDs []string,
	capabilities []*mcpmodel.Capability,
	memories []*memorymodel.MemoryItem,
	evidencePolicies []workorcheventpayload.Payload,
	executionContract map[string]any,
	continuation runtimecontract.RuntimeContinuationContext,
) runtimecontract.RuntimeActivationContext {
	ret := runtimecontract.RuntimeActivationContext{
		ProviderTarget:    providerTarget.Normalized(),
		DialogLanguage:    strings.TrimSpace(dialogLanguage),
		DefaultSoul:       runtimeSoulContext(defaultSoul),
		Skills:            runtimeSkillContexts(skills),
		MCPServerIDs:      runtimeMCPServerIDs(mcpServerIDs),
		MCPCapabilities:   runtimeMCPCapabilityContexts(capabilities),
		Memories:          runtimeMemoryContexts(memories),
		EvidencePolicies:  runtimeEvidencePolicies(evidencePolicies),
		ExecutionContract: runtimeExecutionContract(executionContract),
		Continuation:      continuation,
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
	return ret.Normalized()
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
	if continuation := runtimeMap(input["candidateOutputContinuation"]); len(continuation) > 0 {
		ret.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{
			MaxChunks: runtimeInt(continuation["maxChunks"]), TailBytes: runtimeInt(continuation["tailBytes"]),
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
					ID: runtimeString(artifact["id"]), TargetRole: runtimeString(artifact["targetRole"]), TargetRef: runtimeString(artifact["targetRef"]),
					AllowedOperationKinds: runtimeStrings(artifact["allowedOperationKinds"]), DependsOnArtifactIDs: runtimeStrings(artifact["dependsOnArtifactIds"]),
				})
			}
			contract.BootstrapRecipes = append(contract.BootstrapRecipes, recipe)
		}
		for _, item := range runtimeMapSlice(profile["evidenceRequirements"]) {
			acceptedVariants := make([]runtimecontract.RuntimeOperationVariantRef, 0)
			for _, variant := range runtimeMapSlice(item["acceptedOperationVariants"]) {
				acceptedVariants = append(acceptedVariants, runtimecontract.RuntimeOperationVariantRef{Kind: runtimeString(variant["kind"]), VariantID: runtimeString(variant["variantId"])})
			}
			stateRequirements := make([]runtimecontract.RuntimeProfileCapabilityStateRequirement, 0)
			for _, state := range runtimeMapSlice(item["capabilityStateRequirements"]) {
				stateRequirements = append(stateRequirements, runtimecontract.RuntimeProfileCapabilityStateRequirement{OwnerModuleID: runtimeString(state["ownerModuleId"]), CapabilityStateKind: runtimeString(state["capabilityStateKind"]), SubjectRef: runtimeString(state["subjectRef"])})
			}
			contract.EvidenceRequirements = append(contract.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: runtimeString(item["id"]), Scope: runtimeString(item["scope"]), EvidenceRole: runtimeString(item["evidenceRole"]),
				TargetRole: runtimeString(item["targetRole"]), TargetRefs: runtimeStrings(item["targetRefs"]),
				TargetDiscoverySources: runtimeStrings(item["targetDiscoverySources"]), TargetSelectorRef: runtimeString(item["targetSelectorRef"]), TargetSelectorOwnerRef: runtimeString(item["targetSelectorOwnerRef"]), TargetSelectorSubjectRef: runtimeString(item["targetSelectorSubjectRef"]), TargetSelectorCandidateRefs: runtimeStrings(item["targetSelectorCandidateRefs"]), ProspectiveTargetRef: runtimeString(item["prospectiveTargetRef"]), RequireTargetGrounding: runtimeBool(item["requireTargetGrounding"]),
				RequiredEvidenceKinds: runtimeStrings(item["requiredEvidenceKinds"]),
				RequireDirectEvidence: runtimeBool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeBool(item["requireClaimEvidence"]),
				RequireExecutableBehaviorEvidence: runtimeBool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: runtimeBool(item["requireCoverageTargets"]),
				AcceptedStepKinds:           runtimeStrings(item["acceptedStepKinds"]),
				AcceptedOperationVariants:   acceptedVariants,
				ValidationRecipeID:          runtimeString(item["validationRecipeId"]),
				RequiredCheckRefs:           runtimeStrings(item["requiredCheckRefs"]),
				MetricConstraints:           runtimeStrings(item["metricConstraints"]),
				ArtifactRefs:                runtimeStrings(item["artifactRefs"]),
				CapabilityStateRequirements: stateRequirements,
				RejectedStepKinds:           runtimeStrings(item["rejectedStepKinds"]), Description: runtimeString(item["description"]),
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
			entry := runtimeMap(item["entry"])
			completion := runtimeMap(item["completion"])
			governance := runtimeMap(item["governance"])
			policyRefs := runtimeMap(item["policyRefs"])
			stage := runtimecontract.RuntimeTemplateStage{
				ID: runtimeString(item["id"]), SoulBinding: runtimeString(item["soulBinding"]),
				Entry: runtimecontract.RuntimeTemplateStageEntry{
					NextAction: runtimeString(entry["nextAction"]), RequiresDefinitionRevision: runtimeBool(entry["requiresDefinitionRevision"]),
					RequiresGapBaseline: runtimeBool(entry["requiresGapBaseline"]),
				},
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

func runtimeEvidencePolicies(items []workorcheventpayload.Payload) []runtimecontract.RuntimeEvidencePolicy {
	ret := make([]runtimecontract.RuntimeEvidencePolicy, 0, len(items))
	for _, item := range items {
		if len(item.Fields) == 0 {
			continue
		}
		ret = append(ret, runtimecontract.RuntimeEvidencePolicy{
			ID: payloadString(item, "id"), Profile: payloadString(item, "profile"), Rule: payloadString(item, "rule"),
			AppliesTo: payloadString(item, "appliesTo"), CommandName: payloadString(item, "commandName"),
			ArgsContain: payloadStrings(item, "argsContain"), RecipeDeclared: payloadBool(item, "recipeDeclared"), RecipeArgs: payloadStrings(item, "recipeArgs"), RecipeTimeoutMillis: int64(payloadInt(item, "recipeTimeoutMillis")),
			RequiredArgs: payloadStrings(item, "requiredArgs"), ZeroWorkMarkers: payloadStrings(item, "zeroWorkMarkers"),
			WorkEvidenceMarkers: payloadStrings(item, "workEvidenceMarkers"), QualityOnZeroWork: payloadString(item, "qualityOnZeroWork"),
			MarksZeroWork: payloadBool(item, "zeroWork"), EvidenceTags: payloadStrings(item, "evidenceTags"),
			EvidenceReason: payloadString(item, "evidenceReason"), CoverageRequirements: payloadCoverageEvidenceRequirements(item, "coverageRequirements"),
			Description: payloadString(item, "description"),
		})
	}
	return runtimecontract.NormalizeRuntimeEvidencePolicies(ret)
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
			TargetSelectorCandidateRefs: payloadStrings(payload, "targetSelectorCandidateRefs"), ProspectiveTargetRef: payloadString(payload, "prospectiveTargetRef"), RequireTargetGrounding: payloadBool(payload, "requireTargetGrounding"),
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
			TargetSelectorCandidateRefs: runtimeStrings(item["targetSelectorCandidateRefs"]), ProspectiveTargetRef: runtimeString(item["prospectiveTargetRef"]), RequireTargetGrounding: runtimeBool(item["requireTargetGrounding"]),
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
		ID: item.ID, Name: item.Name, Description: item.Description,
		BusinessDomains: append([]string(nil), item.BusinessDomains...), Instructions: item.SoulMD,
		AllowedSkillRefs:            append([]string(nil), item.AllowedSkillRefs...),
		AllowedMCPServerRefs:        append([]string(nil), item.AllowedMCPServerRefs...),
		AllowedCapabilityRefs:       append([]string(nil), item.AllowedCapabilityRefs...),
		AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...),
		AllowedToolKinds:            append([]string(nil), item.AllowedToolKinds...),
	}
}

func runtimeSkillContexts(items []contextregistrycore.Skill) []runtimecontract.RuntimeSkillContext {
	ret := make([]runtimecontract.RuntimeSkillContext, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimecontract.RuntimeSkillContext{
			ID: item.ID, Name: item.Name, Version: item.Version, Description: item.Description,
			TriggerRules: append([]string(nil), item.TriggerRules...), Instruction: item.Instruction,
			ReferenceRefs: append([]string(nil), item.ReferenceRefs...), RequiredToolKinds: append([]string(nil), item.RequiredToolKinds...),
			RequiredMCPServerRefs: append([]string(nil), item.RequiredMCPServerRefs...), RequiredCapabilityRefs: append([]string(nil), item.RequiredCapabilityRefs...),
			Constraints: append([]string(nil), item.Constraints...), Source: item.Source, Scope: item.Scope,
			WorkspaceID: item.WorkspaceID, ContentHash: item.ContentHash,
		})
	}
	return ret
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
			URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status,
		}
		// RVC-R8: registry Kind=tool is not a Runtime operation kind; seal OperationKind + EffectScope.
		ret = append(ret, runtimecontract.SealMCPCapabilityEffect(cap))
	}
	return ret
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
