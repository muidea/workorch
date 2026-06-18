package server

import (
	"strings"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/events"
)

const sseStateSummaryKey = "stateSummary"

func enrichSSEEnvelope(envelope events.Envelope) events.Envelope {
	envelope = cloneSSEEnvelope(envelope)
	summary := map[string]any{}
	if envelope.Revision > 0 {
		summary["revision"] = envelope.Revision
	}
	switch envelope.Topic {
	case workspaceevents.TopicWorkspaceStateChanged:
		addSSEPayloadString(summary, "workspaceState", envelope.Payload, events.PayloadKeyState)
		addSSEPayloadString(summary, "activeRunId", envelope.Payload, events.PayloadKeyActiveRunID)
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		addSSEPayloadString(summary, "runState", envelope.Payload, events.PayloadKeyState)
		addSSEPayloadString(summary, "blockingTicketId", envelope.Payload, events.PayloadKeyBlockingTicketID)
		addSSEPayloadString(summary, "blockingApprovalId", envelope.Payload, events.PayloadKeyBlockingApprovalID)
		if progress := runProgressSummary(envelope); progress != "" {
			summary["progressSummary"] = progress
		}
	case decisionevents.TopicTicketCreated, decisionevents.TopicTicketUpdated:
		if envelope.SubjectID != "" {
			summary["blockingTicketId"] = envelope.SubjectID
		}
		addSSEPayloadString(summary, "decisionState", envelope.Payload, events.PayloadKeyState)
	case approvalevents.TopicApprovalCreated:
		if envelope.SubjectID != "" {
			summary["blockingApprovalId"] = envelope.SubjectID
		}
	case resultevents.TopicResultAccepted:
		addSSEPayloadString(summary, "resultState", envelope.Payload, events.PayloadKeyState)
		addSSEPayloadString(summary, "outcomeSummary", envelope.Payload, events.PayloadKeySummary)
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

func cloneSSEEnvelope(envelope events.Envelope) events.Envelope {
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

func runProgressSummary(envelope events.Envelope) string {
	state, _ := envelope.Payload[events.PayloadKeyState].(string)
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
