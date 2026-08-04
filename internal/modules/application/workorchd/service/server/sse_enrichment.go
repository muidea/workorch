package server

import (
	"strings"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
)

const sseStateSummaryKey = "stateSummary"

func enrichSSEEnvelope(envelope streamEvent) streamEvent {
	envelope = cloneSSEEnvelope(envelope)
	summary := map[string]any{}
	if envelope.Revision > 0 {
		summary["revision"] = envelope.Revision
	}
	switch envelope.Topic {
	case workspaceevents.TopicWorkspaceStateChanged:
		addSSEPayloadString(summary, "workspaceState", envelope.Payload, recordpayload.PayloadKeyState)
		addSSEPayloadString(summary, "activeRunId", envelope.Payload, recordpayload.PayloadKeyActiveRunID)
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		addSSEPayloadString(summary, "runState", envelope.Payload, recordpayload.PayloadKeyState)
		addSSEPayloadString(summary, "blockingTicketId", envelope.Payload, recordpayload.PayloadKeyBlockingTicketID)
		addSSEPayloadString(summary, "blockingApprovalId", envelope.Payload, recordpayload.PayloadKeyBlockingApprovalID)
		if progress := runProgressSummary(envelope); progress != "" {
			summary["progressSummary"] = progress
		}
	case decisionevents.TopicTicketCreated:
		if envelope.SubjectID != "" {
			summary["blockingTicketId"] = envelope.SubjectID
		}
		addSSEPayloadString(summary, "decisionState", envelope.Payload, recordpayload.PayloadKeyState)
	case resultevents.TopicResultAccepted:
		addSSEPayloadString(summary, "resultState", envelope.Payload, recordpayload.PayloadKeyState)
		addSSEPayloadString(summary, "outcomeSummary", envelope.Payload, recordpayload.PayloadKeySummary)
	}
	if len(summary) == 0 {
		return envelope
	}
	if envelope.Payload == nil {
		envelope.Payload = map[string]any{}
	}
	envelope.Payload[sseStateSummaryKey] = summary
	return envelope
}

func cloneSSEEnvelope(envelope streamEvent) streamEvent {
	cloned := envelope
	if envelope.Payload != nil {
		cloned.Payload = make(map[string]any, len(envelope.Payload)+1)
		for key, value := range envelope.Payload {
			cloned.Payload[key] = value
		}
	}
	return cloned
}

func addSSEPayloadString(out map[string]any, key string, payload map[string]any, payloadKey string) {
	if out == nil || len(payload) == 0 {
		return
	}
	if value, ok := payload[payloadKey].(string); ok && strings.TrimSpace(value) != "" {
		out[key] = value
	}
}

func runProgressSummary(envelope streamEvent) string {
	state, _ := envelope.Payload[recordpayload.PayloadKeyState].(string)
	switch state {
	case "":
		return ""
	case "blocked_on_decision":
		return "blocked on decision"
	case "blocked_on_approval":
		return "blocked on approval"
	case "failed", "cancelled", "superseded", "completed":
		return state
	default:
		return "run " + state
	}
}
