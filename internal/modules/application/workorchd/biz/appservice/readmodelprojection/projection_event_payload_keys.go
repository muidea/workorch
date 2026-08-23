package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: event_payload_allowed_keys.go

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

// source: event_payload_artifact_allowed_keys.go

var eventPayloadArtifactAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyArtifactRefs: {},
	recordpayload.PayloadKeyArtifactType: {},
	recordpayload.PayloadKeyContentHash:  {},
	recordpayload.PayloadKeyContentType:  {},
	recordpayload.PayloadKeyPatchID:      {},
	recordpayload.PayloadKeyPatchType:    {},
	recordpayload.PayloadKeyResultID:     {},
	recordpayload.PayloadKeyTargetRef:    {},
	recordpayload.PayloadKeyURI:          {},
}

// source: event_payload_failure_allowed_keys.go

var eventPayloadFailureAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyError:         {},
	recordpayload.PayloadKeyFailureClass:  {},
	recordpayload.PayloadKeyFailureReason: {},
}

// source: event_payload_governance_allowed_keys.go

var eventPayloadGovernanceAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyAction:             {},
	recordpayload.PayloadKeyApprovalID:         {},
	recordpayload.PayloadKeyBlockingApprovalID: {},
	recordpayload.PayloadKeyBlockingTicketID:   {},
	recordpayload.PayloadKeyConfidenceScore:    {},
	recordpayload.PayloadKeyLLMDecisionPreview: {},
	recordpayload.PayloadKeyPolicy:             {},
	recordpayload.PayloadKeyReason:             {},
	recordpayload.PayloadKeyRisk:               {},
}

// source: event_payload_identity_allowed_keys.go

var eventPayloadIdentityAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyActiveRunID:      {},
	recordpayload.PayloadKeyDoDID:            {},
	recordpayload.PayloadKeyEvidenceRefs:     {},
	recordpayload.PayloadKeyGoalID:           {},
	recordpayload.PayloadKeyItemID:           {},
	recordpayload.PayloadKeyLogicalRequestID: {},
	recordpayload.PayloadKeyPhysicalCallID:   {},
	recordpayload.PayloadKeyRequestID:        {},
	recordpayload.PayloadKeyRunID:            {},
	recordpayload.PayloadKeyStepID:           {},
	recordpayload.PayloadKeyToolCallID:       {},
	recordpayload.PayloadKeyTraceID:          {},
	recordpayload.PayloadKeyWorkspaceID:      {},
}

// source: event_payload_lifecycle_allowed_keys.go

var eventPayloadLifecycleAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyCompletedAt: {},
	recordpayload.PayloadKeyStartedAt:   {},
	recordpayload.PayloadKeyState:       {},
	recordpayload.PayloadKeyStatus:      {},
}

// source: event_payload_llm_allowed_keys.go

var eventPayloadLLMAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyLLMModel:      {},
	recordpayload.PayloadKeyLLMProviderID: {},
	recordpayload.PayloadKeyUsageSource:   {},
}

// source: event_payload_provider_allowed_keys.go

var eventPayloadProviderAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyModel:      {},
	recordpayload.PayloadKeyProvider:   {},
	recordpayload.PayloadKeyProviderID: {},
}

// source: event_payload_runtime_allowed_keys.go

var eventPayloadRuntimeAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyAddedLines:           {},
	recordpayload.PayloadKeyAPISurface:           {},
	recordpayload.PayloadKeyCacheMarkersSent:     {},
	recordpayload.PayloadKeyCacheUsage:           {},
	recordpayload.PayloadKeyChunkCount:           {},
	recordpayload.PayloadKeyDeletedLines:         {},
	recordpayload.PayloadKeyDiffHash:             {},
	recordpayload.PayloadKeyDurationMs:           {},
	recordpayload.PayloadKeyElapsedMs:            {},
	recordpayload.PayloadKeyEstimated:            {},
	recordpayload.PayloadKeyOutputTokens:         {},
	recordpayload.PayloadKeyPolicyBlocked:        {},
	recordpayload.PayloadKeyPolicyMatched:        {},
	recordpayload.PayloadKeyPromptFingerprint:    {},
	recordpayload.PayloadKeyReceivedBytes:        {},
	recordpayload.PayloadKeyRequestBytes:         {},
	recordpayload.PayloadKeyResponseBytes:        {},
	recordpayload.PayloadKeyPhysicalResponseMode: {},
	recordpayload.PayloadKeyProviderSubmitted:    {},
	recordpayload.PayloadKeyStreaming:            {},
	recordpayload.PayloadKeyTotalTokens:          {},
}
