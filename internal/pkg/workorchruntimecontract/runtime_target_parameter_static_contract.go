package workorchruntimecontract

import "sync"

type runtimeStaticTargetParameterVariant struct {
	id             string
	conflicting    []string
	discriminators []string
}

type runtimeStaticTargetParameterContract struct {
	known             bool
	parameters        []RuntimeOperationParameterContract
	requireTargetRefs bool
	forbidden         []string
	requireOneOf      []string
	variants          []runtimeStaticTargetParameterVariant
}

// 仅派生本 binary 静态合同的 target-parameter 规则，不保存 owner preimage 或校验成功。
// 私有集合只供只读查询使用，不经公开合同返回或跨 owner 传输。
var runtimeStaticTargetParameterContracts = sync.OnceValue(func() map[string]runtimeStaticTargetParameterContract {
	ret := make(map[string]runtimeStaticTargetParameterContract, len(runtimeOperationDescriptorInventory))
	for _, descriptor := range runtimeOperationDescriptorInventory {
		contract, known := RuntimeOperationContractForKind(descriptor.Kind)
		value := runtimeStaticTargetParameterContract{known: known, requireTargetRefs: contract.RequireTargetRefs, forbidden: contract.ForbiddenParameters, requireOneOf: contract.RequireOneOfParameters}
		for _, parameter := range contract.Parameters {
			value.parameters = append(value.parameters, RuntimeOperationParameterContract{Name: parameter.Name, Type: parameter.Type, Required: parameter.Required})
		}
		for _, variant := range contract.Variants {
			entry := runtimeStaticTargetParameterVariant{id: variant.ID}
			entry.conflicting = append(entry.conflicting, variant.ForbiddenParameters...)
			entry.conflicting = append(entry.conflicting, variant.RequiredParameters...)
			entry.conflicting = append(entry.conflicting, variant.RequiredNonEmpty...)
			for _, discriminator := range variant.Discriminators {
				entry.discriminators = append(entry.discriminators, discriminator.Name)
			}
			value.variants = append(value.variants, entry)
		}
		ret[descriptor.Kind] = value
	}
	return ret
})

func runtimeTargetParameterContractForKind(kind string) runtimeStaticTargetParameterContract {
	return runtimeStaticTargetParameterContracts()[CanonicalRuntimeBlockKind(kind)]
}
