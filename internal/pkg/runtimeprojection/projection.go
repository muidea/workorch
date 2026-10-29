package runtimeprojection

import (
	"fmt"
	"strings"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func ValidateRuntimePresealTargetDeclarations(executionContract map[string]any, evidencePolicies []workorcheventpayload.Payload) error {
	if err := ValidateRuntimeExecutionLimitsInput(executionContract); err != nil {
		return err
	}
	if err := validateRuntimeProfileInputShape(executionContract); err != nil {
		return err
	}
	if RuntimeValueContainsField(executionContract, "prospectiveTargetRef") {
		return fmt.Errorf("%w: pre-seal execution contract rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
	}
	if RuntimeExecutionContractContainsBootstrapArtifactField(executionContract, "targetRef") {
		return fmt.Errorf("%w: pre-seal bootstrap artifact rejects targetRef; the semantic proposal must choose its target after activation", common.ErrInvalidArgument)
	}
	for _, policy := range evidencePolicies {
		if RuntimePayloadFieldsContainField(policy.Fields, "prospectiveTargetRef") {
			return fmt.Errorf("%w: pre-seal evidence policy rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
		}
	}
	return nil
}

func RuntimeExecutionContractContainsBootstrapArtifactField(executionContract map[string]any, fieldName string) bool {
	profiles := RuntimeMap(executionContract["profileContracts"])
	for _, rawProfile := range profiles {
		profile := RuntimeMap(rawProfile)
		for _, recipe := range RuntimeMapSlice(profile["bootstrapRecipes"]) {
			for _, artifact := range RuntimeMapSlice(recipe["artifacts"]) {
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

func RuntimeValueContainsField(value any, fieldName string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(strings.TrimSpace(key), fieldName) || RuntimeValueContainsField(child, fieldName) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if RuntimeValueContainsField(child, fieldName) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if RuntimeValueContainsField(child, fieldName) {
				return true
			}
		}
	}
	return false
}

func RuntimePayloadFieldsContainField(fields []workorcheventpayload.Field, fieldName string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), fieldName) {
			return true
		}
		if RuntimePayloadValueContainsField(field.Value, fieldName) {
			return true
		}
	}
	return false
}

func RuntimePayloadValueContainsField(value workorcheventpayload.Value, fieldName string) bool {
	switch value.Kind {
	case workorcheventpayload.ValueObject:
		return RuntimePayloadFieldsContainField(value.Object, fieldName)
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if RuntimePayloadValueContainsField(item, fieldName) {
				return true
			}
		}
	}
	return false
}

// ValidateRuntimeEvidencePolicyInputs keeps the raw owner snapshot lossless at
// the admission boundary. Runtime normalization is intentionally ID-based; an
// empty or duplicate ID would otherwise be silently dropped or overwritten
// before the typed policy validator could observe it.
func ValidateRuntimeEvidencePolicyInputs(owner string, items []workorcheventpayload.Payload) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "Runtime"
	}
	if err := ValidateRuntimePresealTargetDeclarations(nil, items); err != nil {
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
		policy := RuntimeEvidencePolicy(item)
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
	goalChange := RuntimeMap(input["goalChange"])
	dodChange := RuntimeMap(input["doDChange"])
	ret := runtimecontract.RuntimeDefinitionChangeImpact{
		Version: RuntimeString(input["version"]), Changed: RuntimeBool(input["changed"]), Policy: RuntimeString(input["policy"]),
		RequiredPreExecutionAssessment: RuntimeBool(input["requiredPreExecutionAssessment"]),
		GoalChange:                     RuntimeDefinitionChangeSet(goalChange, "effectiveGoalId", "latestGoalId", "previousGoalIds", "allGoalIds", "allGoalsAreConstraints"),
		DoDChange:                      RuntimeDefinitionChangeSet(dodChange, "effectiveDoDId", "latestDoDId", "previousDoDIds", "allDoDIds", "allDoDItemsAreAcceptanceGates"),
		StrategyRequirements:           RuntimeStrings(input["strategyRequirements"]),
	}
	ret = ret.Normalized()
	return &ret
}

func RuntimeDefinitionChangeSet(input map[string]any, effectiveKey, latestKey, previousKey, allKey, constrainKey string) runtimecontract.RuntimeDefinitionChangeSet {
	ret := runtimecontract.RuntimeDefinitionChangeSet{
		EffectiveDefinitionID: RuntimeString(input[effectiveKey]), LatestDefinitionID: RuntimeString(input[latestKey]),
		PreviousDefinitionIDs: RuntimeStrings(input[previousKey]), AllDefinitionIDs: RuntimeStrings(input[allKey]),
		ChangedInPlace: RuntimeBool(input["changedInPlace"]), AllEntriesConstrain: RuntimeBool(input[constrainKey]),
	}
	for _, revision := range RuntimeMapSlice(input["definitionRevisions"]) {
		ret.DefinitionRevisions = append(ret.DefinitionRevisions, runtimecontract.RuntimeDefinitionRevision{
			ID: RuntimeString(revision["id"]), RevisionCount: RuntimeInt(revision["revisionCount"]), Source: RuntimeString(revision["source"]),
			DefinitionRevision: RuntimeString(revision["definitionRevision"]), PreviousDefinitionHash: RuntimeString(revision["previousDefinitionHash"]),
			PreviousDefinitionID: RuntimeString(revision["previousDefinitionId"]), LastEventID: RuntimeString(revision["lastEventId"]),
			LastOccurredAt: RuntimeString(revision["lastOccurredAt"]),
		})
	}
	return ret
}

func RuntimeExecutionContract(input map[string]any) runtimecontract.RuntimeExecutionContract {
	ret := runtimecontract.RuntimeExecutionContract{
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		StateChangePolicy:         runtimecontract.RuntimeStateChangePolicy(RuntimeString(input["stateChangePolicy"])),
		MaxRepairAttemptsPerFocus: RuntimeInt(input["maxRepairAttemptsPerFocus"]),
		ExecutionLimits:           RuntimeExecutionLimits(input),
	}
	if recovery := RuntimeMap(input["candidateDocumentRecovery"]); len(recovery) > 0 {
		ret.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{
			MaxAttempts: RuntimeInt(recovery["maxAttempts"]),
		}
	}
	for _, item := range RuntimeMapSlice(input["capabilityUsagePolicies"]) {
		ret.CapabilityUsagePolicies = append(ret.CapabilityUsagePolicies, runtimecontract.RuntimeCapabilityUsagePolicy{
			ID: RuntimeString(item["id"]), Scope: RuntimeString(item["scope"]),
			AppliesToCategory: RuntimeString(item["appliesToCategory"]), EvidenceStrength: RuntimeString(item["evidenceStrength"]),
			UsageMode: RuntimeString(item["usageMode"]), FallbackPolicy: RuntimeString(item["fallbackPolicy"]),
			TriggerStages: RuntimeStrings(item["triggerStages"]), TriggerSignals: RuntimeStrings(item["triggerSignals"]),
			PreferredTools: RuntimeStrings(item["preferredTools"]), RequiredAfterKinds: RuntimeStrings(item["requiredAfterKinds"]),
			Notes: RuntimeStrings(item["notes"]),
		})
	}
	for _, item := range RuntimeMapSlice(input["validationPolicies"]) {
		ret.ValidationPolicies = append(ret.ValidationPolicies, runtimecontract.RuntimeValidationPolicy{
			ID: RuntimeString(item["id"]), Scope: RuntimeString(item["scope"]),
			AppliesToEvidenceKinds: RuntimeStrings(item["appliesToEvidenceKinds"]),
			ValidationSafetyClass:  RuntimeString(item["validationSafetyClass"]),
			DiscoveryRequired:      RuntimeBool(item["discoveryRequired"]), DiscoverySources: RuntimeStrings(item["discoverySources"]),
			AllowedExecutionModes:            RuntimeStrings(item["allowedExecutionModes"]),
			MaxValidationOnlyRetriesPerFocus: RuntimeInt(item["maxValidationOnlyRetriesPerFocus"]),
			ValidationScratchRequired:        RuntimeBool(item["validationScratchRequired"]),
			AllowedValidationModes:           RuntimeStrings(item["allowedValidationModes"]), Notes: RuntimeStrings(item["notes"]),
		})
	}
	ret.ProfileContracts = runtimeprofiledecode.Decode(input["profileContracts"])
	if policy := RuntimeMap(input["acceptanceSlicePolicy"]); len(policy) > 0 {
		slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{
			SchemaVersion:            RuntimeString(policy["schemaVersion"]),
			MaxWorkItems:             RuntimeInt(policy["maxWorkItems"]),
			MaxFocuses:               RuntimeInt(policy["maxFocuses"]),
			MaxCandidateContextItems: RuntimeInt(policy["maxCandidateContextItems"]),
			CompletionPolicy:         RuntimeString(policy["completionPolicy"]),
			ResultPolicy:             RuntimeString(policy["resultPolicy"]),
		}
		for _, item := range RuntimeMapSlice(policy["groups"]) {
			match := RuntimeMap(item["match"])
			slicePolicy.Groups = append(slicePolicy.Groups, runtimecontract.RuntimeAcceptanceSliceGroupPolicy{
				ID: RuntimeString(item["id"]), Title: RuntimeString(item["title"]), Scope: RuntimeString(item["scope"]),
				Priority: RuntimeInt(item["priority"]), MaxWorkItems: RuntimeInt(item["maxWorkItems"]),
				Match: runtimecontract.RuntimeAcceptanceSliceMatch{
					EvidenceRoles:        RuntimeStrings(match["evidenceRoles"]),
					MissingEvidenceKinds: RuntimeStrings(match["missingEvidenceKinds"]),
					SourceRequirementIDs: RuntimeStrings(match["sourceRequirementIds"]),
					SourceScopes:         RuntimeStrings(match["sourceScopes"]),
					All:                  RuntimeBool(match["all"]),
				},
			})
		}
		ret.AcceptanceSlicePolicy = &slicePolicy
	}
	if policy := RuntimeMap(input["templateStagePolicies"]); len(policy) > 0 {
		stagePolicy := runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: RuntimeString(policy["schemaVersion"])}
		for _, item := range RuntimeMapSlice(policy["stages"]) {
			completion := RuntimeMap(item["completion"])
			governance := RuntimeMap(item["governance"])
			policyRefs := RuntimeMap(item["policyRefs"])
			stage := runtimecontract.RuntimeTemplateStage{
				ID: RuntimeString(item["id"]), SoulBinding: RuntimeString(item["soulBinding"]),
				Completion: runtimecontract.RuntimeTemplateStageCompletion{Policy: RuntimeString(completion["policy"])},
				Governance: runtimecontract.RuntimeTemplateStageGovernance{
					DecisionMode: RuntimeString(governance["decisionMode"]), ApprovalMode: RuntimeString(governance["approvalMode"]),
				},
				PolicyRefs: runtimecontract.RuntimeTemplateStagePolicyRefs{
					EvidencePolicies: RuntimeStrings(policyRefs["evidencePolicies"]), CapabilityUsagePolicies: RuntimeStrings(policyRefs["capabilityUsagePolicies"]),
				},
			}
			for _, output := range RuntimeMapSlice(item["requiredOutputs"]) {
				stage.RequiredOutputs = append(stage.RequiredOutputs, runtimecontract.RuntimeTemplateStageRequiredOutput{
					Kind: RuntimeString(output["kind"]), EvidenceKinds: RuntimeStrings(output["evidenceKinds"]),
				})
			}
			stagePolicy.Stages = append(stagePolicy.Stages, stage)
		}
		ret.TemplateStagePolicy = &stagePolicy
	}
	return ret.Normalized()
}

func RuntimeValidationFailureRepairConvergence(input map[string]any) *runtimecontract.RuntimeValidationFailureRepairConvergence {
	return runtimeprofiledecode.ValidationFailureRepairConvergence(input)
}

func RuntimeValidationFailureRepairConvergenceHasExactShape(raw map[string]any) bool {
	return runtimeprofiledecode.ValidationFailureRepairConvergenceHasExactShape(raw)
}

func RuntimeValidationFailureRepairRouteHasExactShape(raw map[string]any) bool {
	return runtimeprofiledecode.ValidationFailureRepairRouteHasExactShape(raw)
}

func RuntimeObjectSliceExact(value any) ([]map[string]any, bool) {
	return runtimeprofiledecode.ObjectSliceExact(value)
}

func RuntimeStringListHasExactShape(value any) bool {
	return runtimeprofiledecode.StringListHasExactShape(value)
}

func RuntimeIntegerHasExactShape(value any) bool {
	return runtimeprofiledecode.IntegerHasExactShape(value)
}

func CompileRuntimeEvidencePolicies(owner string, items []workorcheventpayload.Payload) ([]runtimecontract.RuntimeEvidencePolicy, error) {
	if err := ValidateRuntimeEvidencePolicyInputs(owner, items); err != nil {
		return nil, err
	}
	ret := make([]runtimecontract.RuntimeEvidencePolicy, 0, len(items))
	for _, item := range items {
		ret = append(ret, RuntimeEvidencePolicy(item))
	}
	normalized := runtimecontract.NormalizeRuntimeEvidencePolicies(ret)
	if len(normalized) != len(ret) {
		return nil, fmt.Errorf("%w: %s evidence policy normalization changed policy cardinality", common.ErrInvalidArgument, strings.TrimSpace(owner))
	}
	return normalized, nil
}

func RuntimeEvidencePolicy(item workorcheventpayload.Payload) runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: PayloadString(item, "id"), Profile: PayloadString(item, "profile"), Rule: PayloadString(item, "rule"),
		AppliesTo: PayloadString(item, "appliesTo"), CommandName: PayloadString(item, "commandName"),
		RecipeAdapterRef:             PayloadString(item, "recipeAdapterRef"),
		RecipeInputReviewProtocolRef: PayloadString(item, "recipeInputReviewProtocolRef"),
		ArgsContain:                  PayloadStrings(item, "argsContain"), RecipeDeclared: PayloadBool(item, "recipeDeclared"), RecipeArgs: PayloadStrings(item, "recipeArgs"), RecipeTimeoutMillis: int64(PayloadInt(item, "recipeTimeoutMillis")),
		RequiredArgs: PayloadStrings(item, "requiredArgs"), ZeroWorkMarkers: PayloadStrings(item, "zeroWorkMarkers"),
		WorkEvidenceMarkers: PayloadStrings(item, "workEvidenceMarkers"), QualityOnZeroWork: PayloadString(item, "qualityOnZeroWork"),
		MarksZeroWork: PayloadBool(item, "zeroWork"), EvidenceTags: PayloadStrings(item, "evidenceTags"),
		EvidenceReason: PayloadString(item, "evidenceReason"), CoverageRequirements: PayloadCoverageEvidenceRequirements(item, "coverageRequirements"),
		Description: PayloadString(item, "description"),
	}
}

func PayloadString(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return value
}

func PayloadBool(input workorcheventpayload.Payload, key string) bool {
	value, _ := input.Bool(key)
	return value
}

func PayloadInt(input workorcheventpayload.Payload, key string) int64 {
	value, _ := input.Int64(key)
	return value
}

func PayloadStrings(input workorcheventpayload.Payload, key string) []string {
	value, _ := input.StringSlice(key)
	return value
}

func PayloadCoverageEvidenceRequirements(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeCoverageEvidenceRequirement {
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
			ID: PayloadString(payload, "id"), Scope: PayloadString(payload, "scope"), EvidenceRole: PayloadString(payload, "evidenceRole"),
			TargetRole: PayloadString(payload, "targetRole"), TargetRefs: PayloadStrings(payload, "targetRefs"),
			TargetDiscoverySources: PayloadStrings(payload, "targetDiscoverySources"), TargetSelectorRef: PayloadString(payload, "targetSelectorRef"),
			TargetSelectorOwnerRef: PayloadString(payload, "targetSelectorOwnerRef"), TargetSelectorSubjectRef: PayloadString(payload, "targetSelectorSubjectRef"),
			TargetSelectorCandidateRefs: PayloadStrings(payload, "targetSelectorCandidateRefs"), ProspectiveTargetAllOfConstraintRefs: PayloadStrings(payload, "prospectiveTargetAllOfConstraintRefs"), RequireTargetGrounding: PayloadBool(payload, "requireTargetGrounding"),
			RequiredEvidenceKinds: PayloadStrings(payload, "requiredEvidenceKinds"),
			RequireDirectEvidence: PayloadBool(payload, "requireDirectEvidence"), RequireClaimEvidence: PayloadBool(payload, "requireClaimEvidence"),
			RequireExecutableBehaviorEvidence: PayloadBool(payload, "requireExecutableBehaviorEvidence"), RequireCoverageTargets: PayloadBool(payload, "requireCoverageTargets"),
			AcceptedStepKinds: PayloadStrings(payload, "acceptedStepKinds"), RejectedStepKinds: PayloadStrings(payload, "rejectedStepKinds"),
			AcceptedOperationVariants: RuntimeOperationVariantsFromPayload(payload, "acceptedOperationVariants"),
			ValidationRecipeID:        PayloadString(payload, "validationRecipeId"), RequiredCheckRefs: PayloadStrings(payload, "requiredCheckRefs"),
			MetricConstraints: PayloadStrings(payload, "metricConstraints"), ArtifactRefs: PayloadStrings(payload, "artifactRefs"),
			Description: PayloadString(payload, "description"),
		})
	}
	return ret
}

func RuntimeOperationVariantsFromPayload(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeOperationVariantRef {
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
		ret = append(ret, runtimecontract.RuntimeOperationVariantRef{Kind: PayloadString(payload, "kind"), VariantID: PayloadString(payload, "variantId")})
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}

func RuntimeCoverageEvidenceRequirements(value any) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	items := RuntimeMapSlice(value)
	ret := make([]runtimecontract.RuntimeCoverageEvidenceRequirement, 0, len(items))
	for _, item := range items {
		ret = append(ret, runtimecontract.RuntimeCoverageEvidenceRequirement{
			ID: RuntimeString(item["id"]), Scope: RuntimeString(item["scope"]), EvidenceRole: RuntimeString(item["evidenceRole"]),
			TargetRole: RuntimeString(item["targetRole"]), TargetRefs: RuntimeStrings(item["targetRefs"]),
			TargetDiscoverySources: RuntimeStrings(item["targetDiscoverySources"]), TargetSelectorRef: RuntimeString(item["targetSelectorRef"]),
			TargetSelectorOwnerRef: RuntimeString(item["targetSelectorOwnerRef"]), TargetSelectorSubjectRef: RuntimeString(item["targetSelectorSubjectRef"]),
			TargetSelectorCandidateRefs: RuntimeStrings(item["targetSelectorCandidateRefs"]), ProspectiveTargetAllOfConstraintRefs: RuntimeStrings(item["prospectiveTargetAllOfConstraintRefs"]), RequireTargetGrounding: RuntimeBool(item["requireTargetGrounding"]),
			RequiredEvidenceKinds: RuntimeStrings(item["requiredEvidenceKinds"]),
			RequireDirectEvidence: RuntimeBool(item["requireDirectEvidence"]), RequireClaimEvidence: RuntimeBool(item["requireClaimEvidence"]),
			RequireExecutableBehaviorEvidence: RuntimeBool(item["requireExecutableBehaviorEvidence"]), RequireCoverageTargets: RuntimeBool(item["requireCoverageTargets"]),
			AcceptedStepKinds: RuntimeStrings(item["acceptedStepKinds"]), RejectedStepKinds: RuntimeStrings(item["rejectedStepKinds"]),
			AcceptedOperationVariants: RuntimeOperationVariants(item["acceptedOperationVariants"]),
			ValidationRecipeID:        RuntimeString(item["validationRecipeId"]), RequiredCheckRefs: RuntimeStrings(item["requiredCheckRefs"]),
			MetricConstraints: RuntimeStrings(item["metricConstraints"]), ArtifactRefs: RuntimeStrings(item["artifactRefs"]),
			Description: RuntimeString(item["description"]),
		})
	}
	return ret
}

func RuntimeOperationVariants(value any) []runtimecontract.RuntimeOperationVariantRef {
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0)
	for _, item := range RuntimeMapSlice(value) {
		ret = append(ret, runtimecontract.RuntimeOperationVariantRef{Kind: RuntimeString(item["kind"]), VariantID: RuntimeString(item["variantId"])})
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}

func RuntimeMapSlice(value any) []map[string]any {
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

func RuntimeMap(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func RuntimeStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := RuntimeString(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func RuntimeString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func RuntimeBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func RuntimeInt(value any) int {
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

func RuntimeSoulContext(item *soulmodel.Soul) *runtimecontract.RuntimeSoulContext {
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

func RuntimeSkillContexts(items []workorchmanagementcontract.Skill) ([]runtimecontract.RuntimeSkillContext, *runtimecontract.RuntimeSkillContextBudget, error) {
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

func RuntimeMCPServerIDs(values []string) []common.MCPServerID {
	ret := make([]common.MCPServerID, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, common.MCPServerID(value))
		}
	}
	return ret
}

func RuntimeMCPCapabilityContexts(items []*mcpmodel.Capability) []runtimecontract.RuntimeMCPCapabilityContext {
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
			URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: CapabilityAvailability(item.Enabled),
		}
		// RVC-R8: registry Kind=tool is not a Runtime operation kind; seal OperationKind + EffectScope.
		ret = append(ret, runtimecontract.SealMCPCapabilityEffect(cap))
	}
	return ret
}

func CapabilityAvailability(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// RuntimeMCPOperationPolicies is the application-side adapter from the
// MCP-registry owner's typed capability metadata to Runtime's owner-neutral
// effect/replay fact. Runtime core never needs to understand server/tool
// parameter names after this boundary.
func RuntimeMCPOperationPolicies(capabilities []runtimecontract.RuntimeMCPCapabilityContext) []runtimecontract.RuntimeOperationPolicyFact {
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

func RuntimeMemoryContexts(items []*memorymodel.MemoryItem) []runtimecontract.RuntimeMemoryContext {
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

func RuntimeMemorySelectionEvidence(input *MemorySelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	return RuntimeMemorySelectionEvidenceDepth(input, 0)
}

func RuntimeMemorySelectionEvidenceDepth(input *MemorySelectionEvidence, depth int) *workorchmemorycontract.RecallSelectionEvidence {
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
		CandidateMemoryIDs:     RuntimeMemoryIDs(input.CandidateMemoryIDs),
		SelectedMemoryIDs:      RuntimeMemoryIDs(input.SelectedMemoryIDs),
		ExcludedMemoryIDs:      RuntimeMemoryIDs(input.ExcludedMemoryIDs),
		TypeQuota:              append([]workorchmemorycontract.TypeQuotaEntry(nil), input.TypeQuota...),
		SelectionScores:        append([]workorchmemorycontract.RecallSelectionScore(nil), input.SelectionScores...),
		RetrievedMemoryIDs:     RuntimeMemoryIDs(input.RetrievedMemoryIDs),
		ContextBudget:          input.ContextBudget,
		FallbackFrom:           input.FallbackFrom,
		SemanticRecall:         RuntimeMemorySelectionEvidenceDepth(input.SemanticRecall, depth+1),
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

func RuntimeMemoryIDs(values []string) []common.MemoryID {
	if values == nil {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(values))
	for _, value := range values {
		ret = append(ret, common.MemoryID(strings.TrimSpace(value)))
	}
	return ret
}

func RuntimeExecutionLimits(input map[string]any) *runtimecontract.RuntimeExecutionLimits {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value := RuntimeMap(raw)
	return &runtimecontract.RuntimeExecutionLimits{
		ProtocolRef:                RuntimeString(value["protocolRef"]),
		MaxProviderCalls:           RuntimeInt(value["maxProviderCalls"]),
		MaxCapabilityOperations:    RuntimeInt(value["maxCapabilityOperations"]),
		MaxProofBatchesPerCWI:      RuntimeInt(value["maxProofBatchesPerCWI"]),
		MaxDurableDispatchesPerCWI: RuntimeInt(value["maxDurableDispatchesPerCWI"]),
	}
}

// Shape and range checks run on both raw sources before template precedence
// can hide an invalid declaration, and independently at final activation.
func ValidateRuntimeExecutionLimitsInput(input map[string]any) error {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: executionLimits requires an object", common.ErrInvalidArgument)
	}
	for key := range value {
		switch key {
		case "protocolRef", "maxProviderCalls", "maxCapabilityOperations", "maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI":
		default:
			return fmt.Errorf("%w: unknown executionLimits field %q", common.ErrInvalidArgument, key)
		}
	}
	for _, key := range []string{"maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI"} {
		if bound, present := value[key]; present {
			if !RuntimeIntegerHasExactShape(bound) || RuntimeInt(bound) < 1 {
				return fmt.Errorf("%w: executionLimits.%s must be a positive exact integer", common.ErrInvalidArgument, key)
			}
		}
	}
	protocol, ok := value["protocolRef"].(string)
	if !ok || protocol != runtimecontract.RuntimeExecutionLimitsV1 {
		return fmt.Errorf("%w: executionLimits.protocolRef must select the current protocol", common.ErrInvalidArgument)
	}
	for _, key := range []string{"maxProviderCalls", "maxCapabilityOperations"} {
		if !RuntimeIntegerHasExactShape(value[key]) {
			return fmt.Errorf("%w: executionLimits.%s must be an exact integer", common.ErrInvalidArgument, key)
		}
	}
	if err := RuntimeExecutionLimits(input).Validate(); err != nil {
		return fmt.Errorf("%w: executionLimits: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func validateRuntimeProfileInputShape(input map[string]any) error {
	return runtimeprofiledecode.ValidateInputShape(input)
}
