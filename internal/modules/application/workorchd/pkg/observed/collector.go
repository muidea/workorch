package observed

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/pkg/events"
)

type Collector struct {
	mu            sync.RWMutex
	published     []events.WorkorchEvent
	unsubscribers []func()
}

const maxCollectedWorkorchEvents = 256

func NewCollector(hub events.Hub) *Collector {
	collector := &Collector{}
	if hub == nil {
		return collector
	}

	collector.unsubscribers = events.SubscribeWorkorch(hub, func(_ context.Context, event events.WorkorchEvent) error {
		collector.mu.Lock()
		defer collector.mu.Unlock()
		collector.published = append(collector.published, cloneWorkorchEvent(event))
		if len(collector.published) > maxCollectedWorkorchEvents {
			copy(collector.published, collector.published[len(collector.published)-maxCollectedWorkorchEvents:])
			collector.published = collector.published[:maxCollectedWorkorchEvents]
		}
		return nil
	})
	return collector
}

func (c *Collector) PublishedWorkorchEvents() []events.WorkorchEvent {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]events.WorkorchEvent, 0, len(c.published))
	for _, item := range c.published {
		out = append(out, cloneWorkorchEvent(item))
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
		cloned.Payload = cloneEventMap(item.Payload)
	}
	return cloned
}

func cloneEventMap(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	cloned := make(map[string]any, len(item))
	for k, v := range item {
		cloned[k] = cloneEventValue(v)
	}
	return cloned
}

func cloneEventSlice(items []any) []any {
	if items == nil {
		return nil
	}
	cloned := make([]any, len(items))
	for i, item := range items {
		cloned[i] = cloneEventValue(item)
	}
	return cloned
}

func cloneEventValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneEventMap(typed)
	case []any:
		return cloneEventSlice(typed)
	default:
		return typed
	}
}

func cloneWorkorchEvent(item events.WorkorchEvent) events.WorkorchEvent {
	if item == nil {
		return nil
	}
	cloned, ok := events.DecodeWorkorchEvent(cloneEnvelope(item.Envelope()))
	if !ok {
		return nil
	}
	return cloned
}
