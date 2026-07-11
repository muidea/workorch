package observed

import (
	"fmt"
	"sync"
	"sync/atomic"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Collector struct {
	mu        sync.RWMutex
	published []eventstreamevents.EventRecordData
	observer  magicevent.SimpleObserver
}

const maxCollectedEventRecords = 256

var collectorObserverSequence atomic.Uint64

func NewCollector(hub magicevent.Hub) *Collector {
	collector := &Collector{}
	if hub == nil {
		return collector
	}

	observerID := fmt.Sprintf("/eventstream/consumers/application/workorchd/observed/%d", collectorObserverSequence.Add(1))
	collector.observer = magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	collector.observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
		if !ok {
			if result != nil {
				result.Set(nil, nil)
			}
			return
		}
		if !validObservedRecord(data.Record) {
			if result != nil {
				result.Set(nil, nil)
			}
			return
		}
		collector.mu.Lock()
		defer collector.mu.Unlock()
		collector.published = append(collector.published, cloneEventRecordData(data.Record))
		if len(collector.published) > maxCollectedEventRecords {
			copy(collector.published, collector.published[len(collector.published)-maxCollectedEventRecords:])
			collector.published = collector.published[:maxCollectedEventRecords]
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	return collector
}

func validObservedRecord(item eventstreamevents.EventRecordData) bool {
	if !events.ValidateWorkorchEventVersion(item.Topic, item.Version) {
		return false
	}
	payload := item.Payload.Map()
	switch item.Topic {
	case workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged:
		_, ok := events.DecodeWorkspacePayload(payload)
		return ok
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		_, ok := events.DecodeRunPayload(payload)
		return ok
	case runevents.TopicStepDispatched, runevents.TopicStepStateChanged:
		_, ok := events.DecodeStepPayload(payload)
		return ok
	case decisionevents.TopicTicketResolved:
		_, ok := events.DecodeDecisionResolutionPayload(payload)
		return ok
	case decisionevents.TopicTicketCreated, decisionevents.TopicTicketUpdated:
		_, ok := events.DecodeDecisionTicketPayload(payload)
		return ok
	case approvalevents.TopicApprovalCreated, approvalevents.TopicApprovalResolved:
		_, ok := events.DecodeApprovalPayload(payload)
		return ok
	default:
		return false
	}
}

func (c *Collector) PublishedEventRecords() []eventstreamevents.EventRecordData {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]eventstreamevents.EventRecordData, 0, len(c.published))
	for _, item := range c.published {
		out = append(out, cloneEventRecordData(item))
	}
	return out
}

func (c *Collector) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	observer := c.observer
	c.observer = nil
	c.mu.Unlock()
	if observer != nil {
		observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended)
	}
}

func cloneEventRecordData(item eventstreamevents.EventRecordData) eventstreamevents.EventRecordData {
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
