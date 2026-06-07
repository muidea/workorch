package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ManagedRuntime struct {
	Runtime
	collector *observed.Collector
	eventHub  events.Hub
}

func NewManagedRuntime(runtime Runtime, eventHub events.Hub) *ManagedRuntime {
	if runtime == nil {
		return nil
	}
	return &ManagedRuntime{
		Runtime:   runtime,
		collector: observed.NewCollector(eventHub),
		eventHub:  eventHub,
	}
}

func (r *ManagedRuntime) PublishedWorkorchEvents() []events.WorkorchEvent {
	if r == nil {
		return nil
	}
	return mergePublishedWorkorchEvents(publishedWorkorchEventsFromHub(r.eventHub), publishedWorkorchEventsFromCollector(r.collector))
}

func (r *ManagedRuntime) Close() {
	if r == nil {
		return
	}
	if r.collector != nil {
		r.collector.Close()
		r.collector = nil
	}
	r.eventHub = nil
}

type publishedEnvelopeStore interface {
	Published() []events.Envelope
}

func publishedWorkorchEventsFromHub(eventHub events.Hub) []events.WorkorchEvent {
	store, ok := eventHub.(publishedEnvelopeStore)
	if !ok || store == nil {
		return nil
	}
	envelopes := store.Published()
	published := make([]events.WorkorchEvent, 0, len(envelopes))
	for _, envelope := range envelopes {
		item, ok := events.DecodeWorkorchEvent(envelope)
		if !ok {
			continue
		}
		published = append(published, item)
	}
	return published
}

func publishedWorkorchEventsFromCollector(collector *observed.Collector) []events.WorkorchEvent {
	if collector == nil {
		return nil
	}
	return collector.PublishedWorkorchEvents()
}

func workorchEventKey(item events.WorkorchEvent) string {
	if item == nil {
		return ""
	}
	envelope := item.Envelope()
	return envelope.Topic + "\x00" +
		envelope.OccurredAt + "\x00" +
		envelope.SubjectType + "\x00" +
		envelope.SubjectID + "\x00" +
		envelope.WorkspaceID + "\x00" +
		envelope.RunID + "\x00" +
		envelope.StepID
}
