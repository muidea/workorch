package runtimecore

import (
	"fmt"
	"sync"
	"sync/atomic"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
)

const maxCollectedEventRecords = 256

var collectorObserverSequence atomic.Uint64

// eventCollector is a WorkOrchd Biz-owned derived read model. It is not a
// formal runtime authority and is released before the command hub terminates.
type eventCollector struct {
	basebiz.Base
	mu         sync.RWMutex
	published  []eventstreamevents.EventRecordData
	observerID string
}

func newEventCollector(hub magicevent.Hub) *eventCollector {
	collector := &eventCollector{Base: basebiz.New(modulecommon.ModuleID, hub, nil)}
	if hub == nil {
		return collector
	}
	observerID := fmt.Sprintf("/eventstream/consumers/application/workorchd/observed/%d", collectorObserverSequence.Add(1))
	collector.observerID = observerID
	collector.SubscribeMatchedFunc(observerID, observerID, eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
		if !ok || !observed.ValidWorkorchEventRecord(data.Record) {
			return
		}
		collector.mu.Lock()
		collector.published = append(collector.published, observed.CloneEventRecordData(data.Record))
		if len(collector.published) > maxCollectedEventRecords {
			copy(collector.published, collector.published[len(collector.published)-maxCollectedEventRecords:])
			collector.published = collector.published[:maxCollectedEventRecords]
		}
		collector.mu.Unlock()
	})
	return collector
}

func (c *eventCollector) PublishedEventRecords() []eventstreamevents.EventRecordData {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]eventstreamevents.EventRecordData, 0, len(c.published))
	for _, item := range c.published {
		out = append(out, observed.CloneEventRecordData(item))
	}
	return out
}

func (c *eventCollector) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	observerID := c.observerID
	c.observerID = ""
	c.mu.Unlock()
	if observerID != "" {
		c.UnsubscribeMatchedFunc(observerID, eventstreamevents.TopicEventStreamRecordAppended)
	}
}
