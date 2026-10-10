package observed

import (
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// ValidWorkorchEventRecord validates an event-stream DTO without subscribing,
// caching, or otherwise taking ownership of runtime state.
func ValidWorkorchEventRecord(item eventstreamevents.EventRecordData) bool {
	if item.Version != recordpayload.WorkorchEventVersion {
		return false
	}
	switch item.Topic {
	case workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged:
		_, ok := recordpayload.DecodeWorkspaceEventPayload(item.Payload)
		return ok
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		_, ok := recordpayload.DecodeRunEventPayload(item.Payload)
		return ok
	case decisionevents.TopicTicketResolved:
		_, ok := recordpayload.DecodeDecisionResolutionEventPayload(item.Payload)
		return ok
	case decisionevents.TopicTicketCreated:
		_, ok := recordpayload.DecodeDecisionTicketEventPayload(item.Payload)
		return ok
	case approvalevents.TopicApprovalResolved:
		_, ok := recordpayload.DecodeApprovalEventPayload(item.Payload)
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

func cloneEventPayload(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	if payload.Fields == nil {
		return workorcheventpayload.Payload{}
	}
	ret := workorcheventpayload.Payload{Fields: make([]workorcheventpayload.Field, len(payload.Fields))}
	for idx, field := range payload.Fields {
		ret.Fields[idx] = workorcheventpayload.Field{Name: field.Name, Value: cloneEventValue(field.Value)}
	}
	return ret
}

func cloneEventValue(value workorcheventpayload.Value) workorcheventpayload.Value {
	ret := value
	if value.Object != nil {
		ret.Object = make([]workorcheventpayload.Field, len(value.Object))
		for idx, field := range value.Object {
			ret.Object[idx] = workorcheventpayload.Field{Name: field.Name, Value: cloneEventValue(field.Value)}
		}
	}
	if value.Array != nil {
		ret.Array = make([]workorcheventpayload.Value, len(value.Array))
		for idx, item := range value.Array {
			ret.Array[idx] = cloneEventValue(item)
		}
	}
	return ret
}
