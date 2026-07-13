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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeV2ActivationContext(
	dialogLanguage string,
	defaultSoul *soulmodel.Soul,
	stageSouls map[string]*soulmodel.Soul,
	soulRouting appmodelcore.SoulRouting,
	skills []contextregistrycore.Skill,
	mcpServerIDs []string,
	capabilities []*mcpmodel.Capability,
	memories []*memorymodel.MemoryItem,
	evidencePolicies []map[string]any,
	executionContract map[string]any,
	continuation runtimecontract.RuntimeContinuationContext,
) runtimecontract.RuntimeActivationContext {
	ret := runtimecontract.RuntimeActivationContext{
		DialogLanguage:    strings.TrimSpace(dialogLanguage),
		DefaultSoul:       runtimeV2SoulContext(defaultSoul),
		Skills:            runtimeV2SkillContexts(skills),
		MCPServerIDs:      runtimeV2MCPServerIDs(mcpServerIDs),
		MCPCapabilities:   runtimeV2MCPCapabilityContexts(capabilities),
		Memories:          runtimeV2MemoryContexts(memories),
		EvidencePolicies:  runtimeV2EvidencePolicies(evidencePolicies),
		ExecutionContract: runtimeV2ExecutionContract(executionContract),
		Continuation:      continuation,
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
		StateChangePolicy: runtimecontract.RuntimeStateChangePolicy(runtimeV2String(input["stateChangePolicy"])),
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
			contract.EvidenceRequirements = append(contract.EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: runtimeV2String(item["id"]), Scope: runtimeV2String(item["scope"]), EvidenceRole: runtimeV2String(item["evidenceRole"]),
				RequireDirectEvidence: runtimeV2Bool(item["requireDirectEvidence"]), RequireClaimEvidence: runtimeV2Bool(item["requireClaimEvidence"]),
				RequireExecutableBehaviorEvidence: runtimeV2Bool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: runtimeV2Bool(item["requireCoverageTargets"]),
				AcceptedStepKinds: runtimeV2Strings(item["acceptedStepKinds"]), AcceptedOperations: runtimeV2Strings(item["acceptedOperations"]),
				RejectedStepKinds: runtimeV2Strings(item["rejectedStepKinds"]), Description: runtimeV2String(item["description"]),
			})
		}
		ret.ProfileContracts = append(ret.ProfileContracts, contract)
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

func runtimeV2EvidencePolicies(items []map[string]any) []runtimecontract.RuntimeEvidencePolicy {
	ret := make([]runtimecontract.RuntimeEvidencePolicy, 0, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, runtimecontract.RuntimeEvidencePolicy{
			ID: runtimeV2String(item["id"]), Profile: runtimeV2String(item["profile"]), Rule: runtimeV2String(item["rule"]),
			AppliesTo: runtimeV2String(item["appliesTo"]), CommandName: runtimeV2String(item["commandName"]),
			ArgsContain: runtimeV2Strings(item["argsContain"]), ZeroWorkMarkers: runtimeV2Strings(item["zeroWorkMarkers"]),
			WorkEvidenceMarkers: runtimeV2Strings(item["workEvidenceMarkers"]), QualityOnZeroWork: runtimeV2String(item["qualityOnZeroWork"]),
			MarksZeroWork: runtimeV2Bool(item["zeroWork"]), EvidenceTags: runtimeV2Strings(item["evidenceTags"]),
			EvidenceReason: runtimeV2String(item["evidenceReason"]), CoverageRequirements: runtimeV2CoverageEvidenceRequirements(item["coverageRequirements"]),
			Description: runtimeV2String(item["description"]),
		})
	}
	return runtimecontract.NormalizeRuntimeEvidencePolicies(ret)
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
		ret = append(ret, runtimecontract.RuntimeMCPCapabilityContext{
			ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
			Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
			InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
			RiskLevel: item.RiskLevel, URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status,
		})
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
