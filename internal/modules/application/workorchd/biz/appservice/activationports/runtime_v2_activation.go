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

func RuntimeV2ActivationContext(
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
	targetDeclaration *runtimecontract.RuntimeTargetDeclaration,
) runtimecontract.RuntimeActivationContext {
	ret := runtimecontract.RuntimeActivationContext{
		ProviderTarget:    providerTarget.Normalized(),
		DialogLanguage:    strings.TrimSpace(dialogLanguage),
		DefaultSoul:       runtimeV2SoulContext(defaultSoul),
		Skills:            runtimeV2SkillContexts(skills),
		MCPServerIDs:      runtimeV2MCPServerIDs(mcpServerIDs),
		MCPCapabilities:   runtimeV2MCPCapabilityContexts(capabilities),
		Memories:          runtimeV2MemoryContexts(memories),
		EvidencePolicies:  runtimeV2EvidencePolicies(evidencePolicies),
		ExecutionContract: runtimeV2ExecutionContract(executionContract),
		Continuation:      continuation,
		TargetDeclaration: cloneRuntimeTargetDeclaration(targetDeclaration),
	}
	stages := make([]string, 0, len(soulRouting.Bindings))
	for stage := range soulRouting.Bindings {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	for _, stage := range stages {
		binding := soulRouting.Bindings[stage]
		soul := runtimeV2SoulContext(stageSouls[stage])
		if soul == nil {
			continue
		}
		ret.StageSoulBindings = append(ret.StageSoulBindings, runtimecontract.RuntimeStageSoulBinding{
			Stage: stage, Source: binding.Source, Soul: *soul,
		})
	}
	return ret.Normalized()
}

func cloneRuntimeTargetDeclaration(input *runtimecontract.RuntimeTargetDeclaration) *runtimecontract.RuntimeTargetDeclaration {
	if input == nil {
		return nil
	}
	value := input.Clone()
	return &value
}

func RuntimeV2DefinitionChangeImpact(input map[string]any) *runtimecontract.RuntimeDefinitionChangeImpact {
	if len(input) == 0 {
		return nil
	}
	goalChange := runtimeV2Map(input["goalChange"])
	dodChange := runtimeV2Map(input["doDChange"])
	ret := runtimecontract.RuntimeDefinitionChangeImpact{
		Version: runtimeV2String(input["version"]), Changed: runtimeV2Bool(input["changed"]), Policy: runtimeV2String(input["policy"]),
		RequiredPreExecutionAssessment: runtimeV2Bool(input["requiredPreExecutionAssessment"]),
		GoalChange:                     runtimeV2DefinitionChangeSet(goalChange, "effectiveGoalId", "latestGoalId", "previousGoalIds", "allGoalIds", "allGoalsAreConstraints"),
		DoDChange:                      runtimeV2DefinitionChangeSet(dodChange, "effectiveDoDId", "latestDoDId", "previousDoDIds", "allDoDIds", "allDoDItemsAreAcceptanceGates"),
		StrategyRequirements:           runtimeV2Strings(input["strategyRequirements"]),
	}
	ret = ret.Normalized()
	return &ret
}

func runtimeV2DefinitionChangeSet(input map[string]any, effectiveKey, latestKey, previousKey, allKey, constrainKey string) runtimecontract.RuntimeDefinitionChangeSet {
	ret := runtimecontract.RuntimeDefinitionChangeSet{
		EffectiveDefinitionID: runtimeV2String(input[effectiveKey]), LatestDefinitionID: runtimeV2String(input[latestKey]),
		PreviousDefinitionIDs: runtimeV2Strings(input[previousKey]), AllDefinitionIDs: runtimeV2Strings(input[allKey]),
		ChangedInPlace: runtimeV2Bool(input["changedInPlace"]), AllEntriesConstrain: runtimeV2Bool(input[constrainKey]),
	}
	for _, revision := range runtimeV2MapSlice(input["definitionRevisions"]) {
		ret.DefinitionRevisions = append(ret.DefinitionRevisions, runtimecontract.RuntimeDefinitionRevision{
			ID: runtimeV2String(revision["id"]), RevisionCount: runtimeV2Int(revision["revisionCount"]), Source: runtimeV2String(revision["source"]),
			DefinitionRevision: runtimeV2String(revision["definitionRevision"]), PreviousDefinitionHash: runtimeV2String(revision["previousDefinitionHash"]),
			PreviousDefinitionID: runtimeV2String(revision["previousDefinitionId"]), LastEventID: runtimeV2String(revision["lastEventId"]),
			LastOccurredAt: runtimeV2String(revision["lastOccurredAt"]),
		})
	}
	return ret
}

func runtimeV2ExecutionContract(input map[string]any) runtimecontract.RuntimeExecutionContract {
	ret := runtimecontract.RuntimeExecutionContract{
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		StateChangePolicy:         runtimecontract.RuntimeStateChangePolicy(runtimeV2String(input["stateChangePolicy"])),
		MaxRepairAttemptsPerFocus: runtimeV2Int(input["maxRepairAttemptsPerFocus"]),
	}
	if continuation := runtimeV2Map(input["candidateOutputContinuation"]); len(continuation) > 0 {
		ret.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{
			MaxChunks: runtimeV2Int(continuation["maxChunks"]), TailBytes: runtimeV2Int(continuation["tailBytes"]),
		}
	}
	for _, item := range runtimeV2MapSlice(input["capabilityUsagePolicies"]) {
		ret.CapabilityUsagePolicies = append(ret.CapabilityUsagePolicies, runtimecontract.RuntimeCapabilityUsagePolicy{
			ID: runtimeV2String(item["id"]), Scope: runtimeV2String(item["scope"]),
			AppliesToCategory: runtimeV2String(item["appliesToCategory"]), EvidenceStrength: runtimeV2String(item["evidenceStrength"]),
			UsageMode: runtimeV2String(item["usageMode"]), FallbackPolicy: runtimeV2String(item["fallbackPolicy"]),
			TriggerStages: runtimeV2Strings(item["triggerStages"]), TriggerSignals: runtimeV2Strings(item["triggerSignals"]),
			PreferredTools: runtimeV2Strings(item["preferredTools"]), RequiredAfterKinds: runtimeV2Strings(item["requiredAfterKinds"]),
			Notes: runtimeV2Strings(item["notes"]),
		})
	}
	for _, item := range runtimeV2MapSlice(input["validationPolicies"]) {
		ret.ValidationPolicies = append(ret.ValidationPolicies, runtimecontract.RuntimeValidationPolicy{
			ID: runtimeV2String(item["id"]), Scope: runtimeV2String(item["scope"]),
			AppliesToEvidenceKinds: runtimeV2Strings(item["appliesToEvidenceKinds"]),
			ValidationSafetyClass:  runtimeV2String(item["validationSafetyClass"]),
			DiscoveryRequired:      runtimeV2Bool(item["discoveryRequired"]), DiscoverySources: runtimeV2Strings(item["discoverySources"]),
			AllowedExecutionModes:                      runtimeV2Strings(item["allowedExecutionModes"]),
			ValidationFailureImpliesStateChange:        runtimeV2Bool(item["validationFailureImpliesStateChange"]),
			ImplementationGapDiagnosticClasses:         runtimeV2Strings(item["implementationGapDiagnosticClasses"]),
			MaxValidationOnlyRetriesPerFocus:           runtimeV2Int(item["maxValidationOnlyRetriesPerFocus"]),
			PreferMinimalPatchBeforeRepeatedValidation: runtimeV2Bool(item["preferMinimalPatchBeforeRepeatedValidation"]),
			IfNoTargetRoute:                            runtimeV2String(item["ifNoTargetRoute"]), ValidationScratchRequired: runtimeV2Bool(item["validationScratchRequired"]),
			AllowedValidationModes: runtimeV2Strings(item["allowedValidationModes"]), Notes: runtimeV2Strings(item["notes"]),
		})
	}
	profileContracts := runtimeV2Map(input["profileContracts"])
	profileIDs := make([]string, 0, len(profileContracts))
	for profileID := range profileContracts {
		profileIDs = append(profileIDs, profileID)
	}
	sort.Strings(profileIDs)
	for _, profileID := range profileIDs {
		profile := runtimeV2Map(profileContracts[profileID])
		contract := runtimecontract.RuntimeProfileContract{ID: profileID}
		for _, item := range runtimeV2MapSlice(profile["acceptanceDimensions"]) {
			contract.AcceptanceDimensions = append(contract.AcceptanceDimensions, runtimecontract.RuntimeAcceptanceDimension{
				ID: runtimeV2String(item["id"]), Required: runtimeV2Bool(item["required"]), Description: runtimeV2String(item["description"]),
			})
		}
		for _, item := range runtimeV2MapSlice(profile["deliverableExpectations"]) {
			contract.DeliverableExpectations = append(contract.DeliverableExpectations, runtimecontract.RuntimeDeliverableExpectation{
				ID: runtimeV2String(item["id"]), Required: runtimeV2Bool(item["required"]),
				EvidenceRole: runtimeV2String(item["evidenceRole"]), Description: runtimeV2String(item["description"]),
			})
		}
		for _, item := range runtimeV2MapSlice(profile["evidenceRequirements"]) {
			acceptedVariants := make([]runtimecontract.RuntimeOperationVariantRef, 0)
			for _, variant := range runtimeV2MapSlice(item["acceptedOperationVariants"]) {
				acceptedVariants = append(acceptedVariants, runtimecontract.RuntimeOperationVariantRef{Kind: runtimeV2String(variant["kind"]), VariantID: runtimeV2String(variant["variantId"])})
			}
			stateRequirements := make([]runtimecontract.RuntimeProfileCapabilityStateRequirement, 0)
			for _, state := range runtimeV2MapSlice(item["capabilityStateRequirements"]) {
				stateRequirements = append(stateRequirements, runtimecontract.RuntimeProfileCapabilityStateRequirement{OwnerModuleID: runtimeV2String(state["ownerModuleId"]), CapabilityStateKind: runtimeV2String(state["capabilityStateKind"]), SubjectRef: runtimeV2String(state["subjectRef"])})
			}
			contract.EvidenceRequirements = append(contract.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: runtimeV2String(item["id"]), Scope: runtimeV2String(item["scope"]), EvidenceRole: runtimeV2String(item["evidenceRole"]),
				TargetRole: runtimeV2String(item["targetRole"]), TargetRefs: runtimeV2Strings(item["targetRefs"]),
				TargetDiscoverySources: runtimeV2Strings(item["targetDiscoverySources"]), TargetSelectorRef: runtimeV2String(item["targetSelectorRef"]), RequireTargetGrounding: runtimeV2Bool(item["requireTargetGrounding"]),
				RequiredEvidenceKinds: runtimeV2Strings(item["requiredEvidenceKinds"]),
				RequireDirectEvidence: runtimeV2Bool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeV2Bool(item["requireClaimEvidence"]),
				RequireExecutableBehaviorEvidence: runtimeV2Bool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: runtimeV2Bool(item["requireCoverageTargets"]),
				AcceptedStepKinds:           runtimeV2Strings(item["acceptedStepKinds"]),
				AcceptedOperationVariants:   acceptedVariants,
				CapabilityStateRequirements: stateRequirements,
				RejectedStepKinds:           runtimeV2Strings(item["rejectedStepKinds"]), Description: runtimeV2String(item["description"]),
			})
		}
		ret.ProfileContracts = append(ret.ProfileContracts, contract)
	}
	if policy := runtimeV2Map(input["acceptanceSlicePolicy"]); len(policy) > 0 {
		slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{
			SchemaVersion:            runtimeV2String(policy["schemaVersion"]),
			MaxWorkItems:             runtimeV2Int(policy["maxWorkItems"]),
			MaxFocuses:               runtimeV2Int(policy["maxFocuses"]),
			MaxCandidateContextItems: runtimeV2Int(policy["maxCandidateContextItems"]),
			CompletionPolicy:         runtimeV2String(policy["completionPolicy"]),
			ResultPolicy:             runtimeV2String(policy["resultPolicy"]),
		}
		for _, item := range runtimeV2MapSlice(policy["groups"]) {
			match := runtimeV2Map(item["match"])
			slicePolicy.Groups = append(slicePolicy.Groups, runtimecontract.RuntimeAcceptanceSliceGroupPolicy{
				ID: runtimeV2String(item["id"]), Title: runtimeV2String(item["title"]), Scope: runtimeV2String(item["scope"]),
				Priority: runtimeV2Int(item["priority"]), MaxWorkItems: runtimeV2Int(item["maxWorkItems"]),
				Match: runtimecontract.RuntimeAcceptanceSliceMatch{
					EvidenceRoles:        runtimeV2Strings(match["evidenceRoles"]),
					MissingEvidenceKinds: runtimeV2Strings(match["missingEvidenceKinds"]),
					SourceRequirementIDs: runtimeV2Strings(match["sourceRequirementIds"]),
					SourceScopes:         runtimeV2Strings(match["sourceScopes"]),
					All:                  runtimeV2Bool(match["all"]),
				},
			})
		}
		ret.AcceptanceSlicePolicy = &slicePolicy
	}
	if policy := runtimeV2Map(input["templateStagePolicies"]); len(policy) > 0 {
		stagePolicy := runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: runtimeV2String(policy["schemaVersion"])}
		for _, item := range runtimeV2MapSlice(policy["stages"]) {
			entry := runtimeV2Map(item["entry"])
			completion := runtimeV2Map(item["completion"])
			governance := runtimeV2Map(item["governance"])
			policyRefs := runtimeV2Map(item["policyRefs"])
			stage := runtimecontract.RuntimeTemplateStage{
				ID: runtimeV2String(item["id"]), SoulBinding: runtimeV2String(item["soulBinding"]),
				Entry: runtimecontract.RuntimeTemplateStageEntry{
					NextAction: runtimeV2String(entry["nextAction"]), RequiresDefinitionRevision: runtimeV2Bool(entry["requiresDefinitionRevision"]),
					RequiresGapBaseline: runtimeV2Bool(entry["requiresGapBaseline"]),
				},
				Completion: runtimecontract.RuntimeTemplateStageCompletion{Policy: runtimeV2String(completion["policy"])},
				Governance: runtimecontract.RuntimeTemplateStageGovernance{
					DecisionMode: runtimeV2String(governance["decisionMode"]), ApprovalMode: runtimeV2String(governance["approvalMode"]),
				},
				PolicyRefs: runtimecontract.RuntimeTemplateStagePolicyRefs{
					EvidencePolicies: runtimeV2Strings(policyRefs["evidencePolicies"]), CapabilityUsagePolicies: runtimeV2Strings(policyRefs["capabilityUsagePolicies"]),
				},
			}
			for _, output := range runtimeV2MapSlice(item["requiredOutputs"]) {
				stage.RequiredOutputs = append(stage.RequiredOutputs, runtimecontract.RuntimeTemplateStageRequiredOutput{
					Kind: runtimeV2String(output["kind"]), EvidenceKinds: runtimeV2Strings(output["evidenceKinds"]),
				})
			}
			stagePolicy.Stages = append(stagePolicy.Stages, stage)
		}
		ret.TemplateStagePolicy = &stagePolicy
	}
	return ret.Normalized()
}

func runtimeV2EvidencePolicies(items []workorcheventpayload.Payload) []runtimecontract.RuntimeEvidencePolicy {
	ret := make([]runtimecontract.RuntimeEvidencePolicy, 0, len(items))
	for _, item := range items {
		if len(item.Fields) == 0 {
			continue
		}
		ret = append(ret, runtimecontract.RuntimeEvidencePolicy{
			ID: payloadString(item, "id"), Profile: payloadString(item, "profile"), Rule: payloadString(item, "rule"),
			AppliesTo: payloadString(item, "appliesTo"), CommandName: payloadString(item, "commandName"),
			ArgsContain: payloadStrings(item, "argsContain"), ZeroWorkMarkers: payloadStrings(item, "zeroWorkMarkers"),
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
			ID: payloadString(payload, "id"), Scope: payloadString(payload, "scope"),
			RequiredEvidenceKinds: payloadStrings(payload, "requiredEvidenceKinds"),
			RequireDirectEvidence: payloadBool(payload, "requireDirectEvidence"), RequireClaimEvidence: payloadBool(payload, "requireClaimEvidence"),
			RequireExecutableBehaviorEvidence: payloadBool(payload, "requireExecutableBehaviorEvidence"),
			AcceptedStepKinds:                 payloadStrings(payload, "acceptedStepKinds"), RejectedStepKinds: payloadStrings(payload, "rejectedStepKinds"),
			Description: payloadString(payload, "description"),
		})
	}
	return ret
}

func runtimeV2CoverageEvidenceRequirements(value any) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	items := runtimeV2MapSlice(value)
	ret := make([]runtimecontract.RuntimeCoverageEvidenceRequirement, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimecontract.RuntimeCoverageEvidenceRequirement{
			ID: runtimeV2String(item["id"]), Scope: runtimeV2String(item["scope"]),
			RequiredEvidenceKinds: runtimeV2Strings(item["requiredEvidenceKinds"]),
			RequireDirectEvidence: runtimeV2Bool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeV2Bool(item["requireClaimEvidence"]),
			RequireExecutableBehaviorEvidence: runtimeV2Bool(item["requireExecutableBehaviorEvidence"]),
			AcceptedStepKinds:                 runtimeV2Strings(item["acceptedStepKinds"]), RejectedStepKinds: runtimeV2Strings(item["rejectedStepKinds"]),
			Description: runtimeV2String(item["description"]),
		})
	}
	return ret
}

func runtimeV2MapSlice(value any) []map[string]any {
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

func runtimeV2Map(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func runtimeV2Strings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := runtimeV2String(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func runtimeV2String(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func runtimeV2Bool(value any) bool {
	result, _ := value.(bool)
	return result
}

func runtimeV2Int(value any) int {
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

func runtimeV2SoulContext(item *soulmodel.Soul) *runtimecontract.RuntimeSoulContext {
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

func runtimeV2SkillContexts(items []contextregistrycore.Skill) []runtimecontract.RuntimeSkillContext {
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

func runtimeV2MCPServerIDs(values []string) []common.MCPServerID {
	ret := make([]common.MCPServerID, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, common.MCPServerID(value))
		}
	}
	return ret
}

func runtimeV2MCPCapabilityContexts(items []*mcpmodel.Capability) []runtimecontract.RuntimeMCPCapabilityContext {
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

func runtimeV2MemoryContexts(items []*memorymodel.MemoryItem) []runtimecontract.RuntimeMemoryContext {
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
