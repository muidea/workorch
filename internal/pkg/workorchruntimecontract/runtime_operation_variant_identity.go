package workorchruntimecontract

import (
	"strings"
	"sync"
)

type runtimeStaticOperationVariant struct {
	id                          string
	effectScope                 RuntimeEffectScope
	terminalSnapshotProtocolRef string
}

type runtimeStaticOperationVariantIdentity struct {
	known    bool
	ids      []string
	variants []runtimeStaticOperationVariant
}

// 仅派生本 binary 的编译期合同身份；不保存 owner facts 或任何校验成功。
var runtimeStaticOperationVariantIdentities = sync.OnceValue(func() map[string]runtimeStaticOperationVariantIdentity {
	ret := make(map[string]runtimeStaticOperationVariantIdentity, len(runtimeOperationDescriptorInventory))
	for _, descriptor := range runtimeOperationDescriptorInventory {
		contract, known := RuntimeOperationContractForKind(descriptor.Kind)
		identity := runtimeStaticOperationVariantIdentity{known: known}
		if known && len(contract.Variants) > 0 {
			identity.ids = make([]string, 0, len(contract.Variants))
			identity.variants = make([]runtimeStaticOperationVariant, 0, len(contract.Variants))
			for _, variant := range contract.Variants {
				identity.ids = append(identity.ids, variant.ID)
				identity.variants = append(identity.variants, runtimeStaticOperationVariant{
					id:                          variant.ID,
					effectScope:                 variant.EffectScope,
					terminalSnapshotProtocolRef: variant.TerminalSnapshotProtocolRef,
				})
			}
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

func runtimeStaticOperationSupportsTerminalSnapshotProtocol(kind, variantID string) (known, supported bool) {
	identity := runtimeStaticOperationVariantIdentities()[CanonicalRuntimeBlockKind(kind)]
	if !identity.known {
		return false, false
	}
	for _, variant := range identity.variants {
		if variant.id == variantID && variant.terminalSnapshotProtocolRef == RuntimeTerminalOwnerSnapshotProtocol {
			return true, true
		}
	}
	return true, false
}

func runtimeStaticLineageReceiptInventoryMatches(rule RuntimeLineageAssertionRule, descriptor RuntimeOperationDescriptor) bool {
	identity := runtimeStaticOperationVariantIdentities()[rule.OperationKind]
	if !identity.known || descriptor.Kind != rule.OperationKind {
		return false
	}
	if len(identity.variants) == 0 {
		return rule.OperationVariantID == "" && descriptor.DefaultEffectScope != RuntimeEffectUnknown && rule.RequiredEffectScope == descriptor.DefaultEffectScope
	}
	matches := 0
	for _, variant := range identity.variants {
		if variant.id != rule.OperationVariantID {
			continue
		}
		effect := variant.effectScope
		if effect == "" {
			effect = descriptor.DefaultEffectScope
		}
		if validSealedRuntimeEffectScope(effect) && effect == rule.RequiredEffectScope {
			matches++
		}
	}
	return rule.OperationVariantID != "" && matches == 1
}
