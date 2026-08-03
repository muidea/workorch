package observed

import (
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

// ValidWorkorchEventRecord validates an event-stream DTO without subscribing,
// caching, or otherwise taking ownership of runtime state.
func ValidWorkorchEventRecord(item eventstreamevents.EventRecordData) bool {
	if item.Version != events.WorkorchEventVersion {
		return false
	}
	switch item.Topic {
	case workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged:
		_, ok := events.DecodeWorkspaceEventPayload(item.Payload)
		return ok
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		_, ok := events.DecodeRunEventPayload(item.Payload)
		return ok
	case decisionevents.TopicTicketResolved:
		_, ok := events.DecodeDecisionResolutionEventPayload(item.Payload)
		return ok
	case decisionevents.TopicTicketCreated, decisionevents.TopicTicketUpdated:
		_, ok := events.DecodeDecisionTicketEventPayload(item.Payload)
		return ok
	case approvalevents.TopicApprovalCreated, approvalevents.TopicApprovalResolved:
		_, ok := events.DecodeApprovalEventPayload(item.Payload)
		return ok
	default:
		return false
	}
}

// CloneEventRecordData returns a deep DTO copy for read-model consumers.
func CloneEventRecordData(item eventstreamevents.EventRecordData) eventstreamevents.EventRecordData {
	item.Payload = cloneEventPayload(item.Payload)
	item.PayloadSummary = cloneEventPayload(item.PayloadSummary)
	return item
}

func cloneEventPayload(payload eventstreamevents.EventPayload) eventstreamevents.EventPayload {
	if payload.Fields == nil {
		return eventstreamevents.EventPayload{}
	}
	ret := eventstreamevents.EventPayload{Fields: make([]eventstreamevents.EventField, len(payload.Fields))}
	for idx, field := range payload.Fields {
		ret.Fields[idx] = eventstreamevents.EventField{Name: field.Name, Value: cloneEventValue(field.Value)}
	}
	return ret
}

func cloneEventValue(value eventstreamevents.EventValue) eventstreamevents.EventValue {
	ret := value
	if value.Object != nil {
		ret.Object = make([]eventstreamevents.EventField, len(value.Object))
		for idx, field := range value.Object {
			ret.Object[idx] = eventstreamevents.EventField{Name: field.Name, Value: cloneEventValue(field.Value)}
		}
	}
	if value.Array != nil {
		ret.Array = make([]eventstreamevents.EventValue, len(value.Array))
		for idx, item := range value.Array {
			ret.Array[idx] = cloneEventValue(item)
		}
	}
	return ret
}
