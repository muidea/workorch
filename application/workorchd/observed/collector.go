package observed

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/pkg/events"
)

type Collector struct {
	mu            sync.RWMutex
	published     []events.Phase1Event
	unsubscribers []func()
}

func NewCollector(hub events.Hub) *Collector {
	collector := &Collector{}
	if hub == nil {
		return collector
	}

	collector.unsubscribers = events.SubscribePhase1(hub, func(_ context.Context, event events.Phase1Event) error {
		collector.mu.Lock()
		defer collector.mu.Unlock()
		collector.published = append(collector.published, clonePhase1Event(event))
		return nil
	})
	return collector
}

func (c *Collector) PublishedPhase1Events() []events.Phase1Event {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]events.Phase1Event, 0, len(c.published))
	for _, item := range c.published {
		out = append(out, clonePhase1Event(item))
	}
	return out
}

func (c *Collector) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	unsubscribers := c.unsubscribers
	c.unsubscribers = nil
	c.mu.Unlock()

	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

func cloneEnvelope(item events.Envelope) events.Envelope {
	cloned := item
	if item.Payload != nil {
		cloned.Payload = make(map[string]any, len(item.Payload))
		for k, v := range item.Payload {
			cloned.Payload[k] = v
		}
	}
	return cloned
}

func clonePhase1Event(item events.Phase1Event) events.Phase1Event {
	if item == nil {
		return nil
	}
	cloned, ok := events.DecodePhase1Event(cloneEnvelope(item.Envelope()))
	if !ok {
		return nil
	}
	return cloned
}
