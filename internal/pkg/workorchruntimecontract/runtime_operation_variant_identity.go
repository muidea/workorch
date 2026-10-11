package workorchruntimecontract

import (
	"strings"
	"sync"
)

type runtimeStaticOperationVariantIdentity struct {
	known bool
	ids   []string
}

// 仅派生本 binary 的编译期合同身份；不保存 owner facts 或任何校验成功。
var runtimeStaticOperationVariantIdentities = sync.OnceValue(func() map[string]runtimeStaticOperationVariantIdentity {
	ret := make(map[string]runtimeStaticOperationVariantIdentity, len(runtimeOperationDescriptorInventory))
	for _, descriptor := range runtimeOperationDescriptorInventory {
		contract, known := RuntimeOperationContractForKind(descriptor.Kind)
		identity := runtimeStaticOperationVariantIdentity{known: known}
		for _, variant := range contract.Variants {
			identity.ids = append(identity.ids, variant.ID)
		}
		ret[descriptor.Kind] = identity
	}
	return ret
})

func runtimeOperationVariantIdentity(kind, variantID string) (known, hasVariants, contains bool) {
	identity := runtimeStaticOperationVariantIdentities()[CanonicalRuntimeBlockKind(kind)]
	variantID = strings.TrimSpace(variantID)
	for _, id := range identity.ids {
		if id == variantID {
			contains = true
			break
		}
	}
	return identity.known, len(identity.ids) != 0, contains
}
