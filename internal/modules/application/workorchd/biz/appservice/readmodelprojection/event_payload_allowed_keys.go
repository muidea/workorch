package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadCoreAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyItemType:  {},
	recordpayload.PayloadKeyKind:      {},
	recordpayload.PayloadKeyName:      {},
	recordpayload.PayloadKeyOperation: {},
	recordpayload.PayloadKeySourceRef: {},
	recordpayload.PayloadKeyStage:     {},
	recordpayload.PayloadKeySummary:   {},
	recordpayload.PayloadKeyType:      {},
}

func isEventPayloadAllowedKey(key string) bool {
	if _, ok := eventPayloadCoreAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadIdentityAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadLifecycleAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadFailureAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadGovernanceAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadArtifactAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadLLMAllowedKeys[key]; ok {
		return true
	}
	if _, ok := eventPayloadProviderAllowedKeys[key]; ok {
		return true
	}
	_, ok := eventPayloadRuntimeAllowedKeys[key]
	return ok
}
