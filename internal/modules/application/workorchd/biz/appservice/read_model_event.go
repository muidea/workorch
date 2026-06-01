package appservice

import (
	"strings"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func mapEventRecords(items []*eventstreammodel.EventRecord) []EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, EventRecord{
			ID:             string(item.ID),
			Version:        item.Version,
			Topic:          item.Topic,
			OccurredAt:     item.OccurredAt,
			SourceModule:   item.SourceModule,
			SubjectType:    item.SubjectType,
			SubjectID:      item.SubjectID,
			WorkspaceID:    string(item.WorkspaceID),
			RunID:          string(item.RunID),
			StepID:         string(item.StepID),
			Revision:       item.Revision,
			Payload:        canonicalizeEventRecordPayload(item.Topic, item.SubjectType, cloneReadModelMap(item.Payload)),
			PayloadSummary: cloneReadModelMap(item.PayloadSummary),
			PayloadRef:     item.PayloadRef,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func canonicalizeEventRecordPayload(topic string, subjectType string, payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	if strings.TrimSpace(topic) != events.TopicStepDispatched &&
		strings.TrimSpace(topic) != events.TopicStepStateChanged &&
		strings.TrimSpace(subjectType) != "step" {
		return payload
	}
	if kind, ok := payload[events.PayloadKeyKind].(string); ok {
		payload[events.PayloadKeyKind] = canonicalReadModelStepKind(kind)
	}
	for _, key := range []string{events.PayloadKeyInput, events.PayloadKeyOutput} {
		if nested := readModelMapValue(payload[key]); len(nested) > 0 {
			payload[key] = canonicalizeEventRecordStepMap(nested)
		}
	}
	return payload
}

func canonicalizeEventRecordStepMap(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	const eventRecordExecutionGraphKey = "executionGraph"
	if graph := readModelMapValue(payload[eventRecordExecutionGraphKey]); len(graph) > 0 {
		if nodes := readModelMapSliceValue(graph["nodes"]); len(nodes) > 0 {
			for idx := range nodes {
				if kind, ok := nodes[idx]["kind"].(string); ok {
					nodes[idx]["kind"] = canonicalReadModelStepKind(kind)
				}
			}
			graph["nodes"] = nodes
			payload[eventRecordExecutionGraphKey] = graph
		}
	}
	return payload
}

func mapProjectedEventRecords(items []*eventstreammodel.EventRecord) []EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, EventRecord{
			ID:             string(item.ID),
			Version:        item.Version,
			Topic:          item.Topic,
			OccurredAt:     item.OccurredAt,
			SourceModule:   item.SourceModule,
			SubjectType:    item.SubjectType,
			SubjectID:      item.SubjectID,
			WorkspaceID:    string(item.WorkspaceID),
			RunID:          string(item.RunID),
			StepID:         string(item.StepID),
			Revision:       item.Revision,
			Payload:        projectEventPayload(item.Payload),
			PayloadSummary: cloneReadModelMap(item.PayloadSummary),
			PayloadRef:     item.PayloadRef,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectEventPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		events.PayloadKeyAction:             {},
		events.PayloadKeyActiveRunID:        {},
		events.PayloadKeyAddedLines:         {},
		events.PayloadKeyApprovalID:         {},
		events.PayloadKeyArtifactRefs:       {},
		events.PayloadKeyArtifactType:       {},
		events.PayloadKeyBlockingApprovalID: {},
		events.PayloadKeyBlockingTicketID:   {},
		events.PayloadKeyCacheUsage:         {},
		events.PayloadKeyChunkCount:         {},
		events.PayloadKeyCompletedAt:        {},
		events.PayloadKeyConfidenceScore:    {},
		events.PayloadKeyContentHash:        {},
		events.PayloadKeyContentType:        {},
		events.PayloadKeyDeletedLines:       {},
		events.PayloadKeyDiffHash:           {},
		events.PayloadKeyDoDID:              {},
		events.PayloadKeyDurationMs:         {},
		events.PayloadKeyElapsedMs:          {},
		events.PayloadKeyError:              {},
		events.PayloadKeyEstimated:          {},
		events.PayloadKeyEvidenceRefs:       {},
		events.PayloadKeyFailureClass:       {},
		events.PayloadKeyFailureReason:      {},
		events.PayloadKeyGoalID:             {},
		events.PayloadKeyItemID:             {},
		events.PayloadKeyItemType:           {},
		events.PayloadKeyKind:               {},
		events.PayloadKeyLLMDecisionPreview: {},
		events.PayloadKeyLLMModel:           {},
		events.PayloadKeyLLMProviderID:      {},
		events.PayloadKeyModel:              {},
		events.PayloadKeyName:               {},
		events.PayloadKeyOperation:          {},
		events.PayloadKeyOutputTokens:       {},
		events.PayloadKeyPatchID:            {},
		events.PayloadKeyPatchType:          {},
		events.PayloadKeyPolicy:             {},
		events.PayloadKeyPolicyBlocked:      {},
		events.PayloadKeyPolicyMatched:      {},
		events.PayloadKeyPromptFingerprint:  {},
		events.PayloadKeyProvider:           {},
		events.PayloadKeyProviderID:         {},
		events.PayloadKeyReason:             {},
		events.PayloadKeyReceivedBytes:      {},
		events.PayloadKeyRequestBytes:       {},
		events.PayloadKeyResponseBytes:      {},
		events.PayloadKeyResultID:           {},
		events.PayloadKeyRisk:               {},
		events.PayloadKeyRunID:              {},
		events.PayloadKeySourceRef:          {},
		events.PayloadKeyStage:              {},
		events.PayloadKeyStartedAt:          {},
		events.PayloadKeyState:              {},
		events.PayloadKeyStatus:             {},
		events.PayloadKeyStepID:             {},
		events.PayloadKeyStreaming:          {},
		events.PayloadKeySummary:            {},
		events.PayloadKeyTargetRef:          {},
		events.PayloadKeyToolCallID:         {},
		events.PayloadKeyTotalTokens:        {},
		events.PayloadKeyTraceID:            {},
		events.PayloadKeyType:               {},
		events.PayloadKeyURI:                {},
		events.PayloadKeyUsageSource:        {},
		events.PayloadKeyWorkspaceID:        {},
	}
	ret := map[string]any{}
	for key, value := range payload {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if projected, ok := projectEventPayloadValue(value); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectEventPayloadValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= eventPayloadStringLimit {
			return typed, true
		}
		return typed[:eventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []string:
		limit := min(len(typed), eventPayloadListLimit)
		ret := make([]string, 0, limit)
		for _, item := range typed[:limit] {
			if len(item) > eventPayloadStringLimit {
				item = item[:eventPayloadStringLimit] + "..."
			}
			ret = append(ret, item)
		}
		return ret, true
	case []any:
		limit := min(len(typed), eventPayloadListLimit)
		ret := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			if projected, ok := projectEventPayloadValue(item); ok {
				ret = append(ret, projected)
			}
		}
		if len(ret) == 0 {
			return nil, false
		}
		return ret, true
	default:
		return nil, false
	}
}
