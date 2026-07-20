package runtimecore

import (
	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
)

type ManagedRuntime struct {
	collector *eventCollector
}

func NewManagedRuntime(commandHub magicevent.Hub) *ManagedRuntime {
	if commandHub == nil {
		return nil
	}
	return &ManagedRuntime{
		collector: newEventCollector(commandHub),
	}
}

func (r *ManagedRuntime) PublishedEventRecords() []eventstreamevents.EventRecordData {
	if r == nil {
		return nil
	}
	return publishedEventRecordsFromCollector(r.collector)
}

func (r *ManagedRuntime) Close() {
	if r == nil {
		return
	}
	if r.collector != nil {
		r.collector.Close()
		r.collector = nil
	}
}

func publishedEventRecordsFromCollector(collector *eventCollector) []eventstreamevents.EventRecordData {
	if collector == nil {
		return nil
	}
	return collector.PublishedEventRecords()
}
