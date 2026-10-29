package activationports

import (
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"
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
	memorySelection *runtimeprojection.MemorySelectionEvidence,
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
	return runtimeprojection.ValidateRuntimePresealTargetDeclarations(executionContract, evidencePolicies)
}
func runtimeExecutionContractContainsBootstrapArtifactField(executionContract map[string]any, fieldName string) bool {
	return runtimeprojection.RuntimeExecutionContractContainsBootstrapArtifactField(executionContract, fieldName)
}
func runtimeValueContainsField(value any, fieldName string) bool {
	return runtimeprojection.RuntimeValueContainsField(value, fieldName)
}
func runtimePayloadFieldsContainField(fields []workorcheventpayload.Field, fieldName string) bool {
	return runtimeprojection.RuntimePayloadFieldsContainField(fields, fieldName)
}
func runtimePayloadValueContainsField(value workorcheventpayload.Value, fieldName string) bool {
	return runtimeprojection.RuntimePayloadValueContainsField(value, fieldName)
}
func validateRuntimeEvidencePolicyInputs(owner string, items []workorcheventpayload.Payload) error {
	return runtimeprojection.ValidateRuntimeEvidencePolicyInputs(owner, items)
}
func RuntimeDefinitionChangeImpact(input map[string]any) *runtimecontract.RuntimeDefinitionChangeImpact {
	return runtimeprojection.RuntimeDefinitionChangeImpact(input)
}
func runtimeDefinitionChangeSet(input map[string]any, effectiveKey, latestKey, previousKey, allKey, constrainKey string) runtimecontract.RuntimeDefinitionChangeSet {
	return runtimeprojection.RuntimeDefinitionChangeSet(input, effectiveKey, latestKey, previousKey, allKey, constrainKey)
}
func runtimeExecutionContract(input map[string]any) runtimecontract.RuntimeExecutionContract {
	return runtimeprojection.RuntimeExecutionContract(input)
}
func runtimeValidationFailureRepairConvergence(input map[string]any) *runtimecontract.RuntimeValidationFailureRepairConvergence {
	return runtimeprojection.RuntimeValidationFailureRepairConvergence(input)
}
func runtimeValidationFailureRepairConvergenceHasExactShape(raw map[string]any) bool {
	return runtimeprojection.RuntimeValidationFailureRepairConvergenceHasExactShape(raw)
}
func runtimeValidationFailureRepairRouteHasExactShape(raw map[string]any) bool {
	return runtimeprojection.RuntimeValidationFailureRepairRouteHasExactShape(raw)
}
func runtimeObjectSliceExact(value any) ([]map[string]any, bool) {
	return runtimeprojection.RuntimeObjectSliceExact(value)
}
func runtimeStringListHasExactShape(value any) bool {
	return runtimeprojection.RuntimeStringListHasExactShape(value)
}
func runtimeIntegerHasExactShape(value any) bool {
	return runtimeprojection.RuntimeIntegerHasExactShape(value)
}
func compileRuntimeEvidencePolicies(owner string, items []workorcheventpayload.Payload) ([]runtimecontract.RuntimeEvidencePolicy, error) {
	return runtimeprojection.CompileRuntimeEvidencePolicies(owner, items)
}
func runtimeEvidencePolicy(item workorcheventpayload.Payload) runtimecontract.RuntimeEvidencePolicy {
	return runtimeprojection.RuntimeEvidencePolicy(item)
}
func payloadString(input workorcheventpayload.Payload, key string) string {
	return runtimeprojection.PayloadString(input, key)
}
func payloadBool(input workorcheventpayload.Payload, key string) bool {
	return runtimeprojection.PayloadBool(input, key)
}
func payloadInt(input workorcheventpayload.Payload, key string) int64 {
	return runtimeprojection.PayloadInt(input, key)
}
func payloadStrings(input workorcheventpayload.Payload, key string) []string {
	return runtimeprojection.PayloadStrings(input, key)
}
func payloadCoverageEvidenceRequirements(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	return runtimeprojection.PayloadCoverageEvidenceRequirements(input, key)
}
func runtimeOperationVariantsFromPayload(input workorcheventpayload.Payload, key string) []runtimecontract.RuntimeOperationVariantRef {
	return runtimeprojection.RuntimeOperationVariantsFromPayload(input, key)
}
func runtimeCoverageEvidenceRequirements(value any) []runtimecontract.RuntimeCoverageEvidenceRequirement {
	return runtimeprojection.RuntimeCoverageEvidenceRequirements(value)
}
func runtimeOperationVariants(value any) []runtimecontract.RuntimeOperationVariantRef {
	return runtimeprojection.RuntimeOperationVariants(value)
}
func runtimeMapSlice(value any) []map[string]any { return runtimeprojection.RuntimeMapSlice(value) }
func runtimeMap(value any) map[string]any        { return runtimeprojection.RuntimeMap(value) }
func runtimeStrings(value any) []string          { return runtimeprojection.RuntimeStrings(value) }
func runtimeString(value any) string             { return runtimeprojection.RuntimeString(value) }
func runtimeBool(value any) bool                 { return runtimeprojection.RuntimeBool(value) }
func runtimeInt(value any) int                   { return runtimeprojection.RuntimeInt(value) }
func runtimeSoulContext(item *soulmodel.Soul) *runtimecontract.RuntimeSoulContext {
	return runtimeprojection.RuntimeSoulContext(item)
}
func runtimeSkillContexts(items []workorchmanagementcontract.Skill) ([]runtimecontract.RuntimeSkillContext, *runtimecontract.RuntimeSkillContextBudget, error) {
	return runtimeprojection.RuntimeSkillContexts(items)
}
func runtimeMCPServerIDs(values []string) []common.MCPServerID {
	return runtimeprojection.RuntimeMCPServerIDs(values)
}
func runtimeMCPCapabilityContexts(items []*mcpmodel.Capability) []runtimecontract.RuntimeMCPCapabilityContext {
	return runtimeprojection.RuntimeMCPCapabilityContexts(items)
}
func capabilityAvailability(enabled bool) string {
	return runtimeprojection.CapabilityAvailability(enabled)
}
func runtimeMCPOperationPolicies(capabilities []runtimecontract.RuntimeMCPCapabilityContext) []runtimecontract.RuntimeOperationPolicyFact {
	return runtimeprojection.RuntimeMCPOperationPolicies(capabilities)
}
func runtimeMemoryContexts(items []*memorymodel.MemoryItem) []runtimecontract.RuntimeMemoryContext {
	return runtimeprojection.RuntimeMemoryContexts(items)
}
func runtimeMemorySelectionEvidence(input *runtimeprojection.MemorySelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	return runtimeprojection.RuntimeMemorySelectionEvidence(input)
}
func runtimeMemorySelectionEvidenceDepth(input *runtimeprojection.MemorySelectionEvidence, depth int) *workorchmemorycontract.RecallSelectionEvidence {
	return runtimeprojection.RuntimeMemorySelectionEvidenceDepth(input, depth)
}
func runtimeMemoryIDs(values []string) []common.MemoryID {
	return runtimeprojection.RuntimeMemoryIDs(values)
}
func runtimeExecutionLimits(input map[string]any) *runtimecontract.RuntimeExecutionLimits {
	return runtimeprojection.RuntimeExecutionLimits(input)
}
func validateRuntimeExecutionLimitsInput(input map[string]any) error {
	return runtimeprojection.ValidateRuntimeExecutionLimitsInput(input)
}
