package readmodelprojection

import (
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func canonicalizeEventRecordPayload(topic string, subjectType string, payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	if strings.TrimSpace(topic) != "/step/dispatched" &&
		strings.TrimSpace(topic) != "/step/state/changed" &&
		strings.TrimSpace(subjectType) != "step" {
		return payload
	}
	if kind, ok := payload[events.PayloadKeyKind].(string); ok {
		payload[events.PayloadKeyKind] = llmcommon.CanonicalRuntimeBlockKind(kind)
	}
	for _, key := range []string{events.PayloadKeyInput, events.PayloadKeyOutput} {
		if nested := readModelMapValue(payload[key]); len(nested) > 0 {
			payload[key] = canonicalizeEventRecordStepMap(nested)
		}
	}
	return payload
}
