package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// 保留 20329964 的完整静态库存编码，不调用新的 binary 固定值入口。
func referenceRuntimeOperationContractRevision() string {
	kinds := []string{StepKindFileList, StepKindFileRead, StepKindFileWrite, StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview, StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindGitRead, StepKindGitWrite, StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet, StepKindSSHCommand, StepKindWebSearch, StepKindWebFetch}
	h := sha256.New()
	for _, kind := range kinds {
		contract, ok := RuntimeOperationContractForKind(kind)
		if !ok {
			continue
		}
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok {
			continue
		}
		writeHashString(h, contract.Kind)
		// Descriptor fields are consumed by reducer feasibility and sealed
		// dispatch admission.  They must advance the executable contract revision
		// together with parameter rules so an already-persisted Run never changes
		// its create/effect semantics after a binary replacement.
		writeHashString(h, descriptor.OwnerModuleID)
		writeHashString(h, string(descriptor.DefaultEffectScope))
		writeHashString(h, string(descriptor.CapabilityClass))
		writeHashString(h, string(descriptor.IdempotencyClass))
		writeHashString(h, fmt.Sprintf("%t", descriptor.RequiresOwnerReceipt))
		writeHashString(h, string(descriptor.TargetCardinality))
		writeHashString(h, string(descriptor.FailureTargetProtocol))
		writeHashString(h, string(descriptor.RepairSurfaceConfirmationProtocol))
		for _, state := range descriptor.AllowedStates {
			writeHashString(h, string(state))
		}
		for _, p := range contract.Parameters {
			writeHashString(h, p.Name)
			writeHashString(h, string(p.Type))
			writeHashString(h, fmt.Sprintf("%t", p.Required))
			writeHashString(h, string(p.CorrectionRole))
		}
		writeHashStrings(h, contract.RequireOneOfParameters)
		writeHashString(h, fmt.Sprintf("%t", contract.RequireTargetRefs))
		writeHashStrings(h, contract.ForbiddenParameters)
		for _, constraint := range NormalizeRuntimeOperationCandidateConstraints(contract.CandidateConstraints) {
			writeHashString(h, constraint.Ref)
			writeHashString(h, string(constraint.Kind))
			writeHashString(h, constraint.ParameterName)
			writeHashString(h, string(constraint.RequiredShape))
		}
		for _, rule := range NormalizeRuntimeOperationValueRules(contract.ValueRules) {
			writeHashStrings(h, runtimeOperationValueRuleHashStrings(rule))
		}
		writeHashString(h, runtimeOperationDynamicParameterPolicy(contract.Kind))
		writeHashString(h, runtimeOperationValidationPolicy(contract.Kind))
		for _, v := range contract.Variants {
			writeHashString(h, v.ID)
			for _, d := range v.Discriminators {
				writeHashString(h, d.Name)
				writeHashString(h, d.EqualsString)
			}
			writeHashStrings(h, v.RequiredParameters)
			writeHashStrings(h, v.RequiredNonEmpty)
			writeHashStrings(h, v.ForbiddenParameters)
			for _, pair := range v.DistinctParameterPairs {
				writeHashString(h, pair.Left)
				writeHashString(h, pair.Right)
			}
			for _, constraint := range NormalizeRuntimeOperationCandidateConstraints(v.CandidateConstraints) {
				writeHashString(h, constraint.Ref)
				writeHashString(h, string(constraint.Kind))
				writeHashString(h, constraint.ParameterName)
				writeHashString(h, string(constraint.RequiredShape))
			}
			for _, rule := range NormalizeRuntimeOperationValueRules(v.ValueRules) {
				writeHashStrings(h, runtimeOperationValueRuleHashStrings(rule))
			}
			writeHashString(h, string(v.EffectScope))
			writeHashString(h, string(v.IdempotencyClass))
		}
	}
	return "operation-contract-" + hex.EncodeToString(h.Sum(nil))[:20]
}
