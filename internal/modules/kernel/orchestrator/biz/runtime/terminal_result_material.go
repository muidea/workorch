package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// DeriveRuntimeTerminalResultMaterial 是 SM-4 owner-local 薄适配。
// 生成与 fresh-load 使用同一纯 compiler，不调用 aggregate.Validate，避免 seal 反证递归。
func DeriveRuntimeTerminalResultMaterial(aggregate RuntimeAggregate) (runtimecontract.RuntimeTerminalResultMaterial, error) {
	return runtimecontract.DeriveRuntimeTerminalResultMaterialFromState(terminalMaterialSource(aggregate))
}

// DeriveRuntimeTerminalRequirementOutcomes 只适配 typed 历史材料，不重裁当前 credit。
func DeriveRuntimeTerminalRequirementOutcomes(aggregate RuntimeAggregate, ownerSet runtimecontract.RuntimeOwnerRevisionSet) []runtimecontract.RuntimeRequirementOutcome {
	return runtimecontract.DeriveRuntimeTerminalRequirementOutcomesFromState(terminalMaterialSource(aggregate), ownerSet)
}
