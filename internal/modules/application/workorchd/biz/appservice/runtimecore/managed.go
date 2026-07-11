package runtimecore

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
)

type ManagedRuntime struct {
	Runtime
	collector *observed.Collector
}

func NewManagedRuntime(runtime Runtime, commandHub magicevent.Hub) *ManagedRuntime {
	if runtime == nil {
		return nil
	}
	return &ManagedRuntime{
		Runtime:   runtime,
		collector: observed.NewCollector(commandHub),
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

func publishedEventRecordsFromCollector(collector *observed.Collector) []eventstreamevents.EventRecordData {
	if collector == nil {
		return nil
	}
	return collector.PublishedEventRecords()
}
