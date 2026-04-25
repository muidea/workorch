package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

type Handler func(context.Context, Envelope) error

type Hub interface {
	Publish(context.Context, Envelope) error
	Subscribe(string, Handler) func()
}

type MemoryHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[int]Handler
	nextID      int
	published   []Envelope
}

func NewMemoryHub() *MemoryHub {
	return &MemoryHub{subscribers: map[string]map[int]Handler{}}
}

func (h *MemoryHub) Publish(ctx context.Context, envelope Envelope) error {
	envelope = prepareEnvelope(envelope)
	if envelope.Topic == "" {
		return common.ErrInvalidArgument
	}

	h.mu.Lock()
	h.published = append(h.published, envelope)
	handlers := append([]Handler{}, h.handlersLocked(envelope.Topic)...)
	h.mu.Unlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, cloneEnvelope(envelope)); err != nil {
			return fmt.Errorf("handle event %s: %w", envelope.Topic, err)
		}
	}
	return nil
}

func (h *MemoryHub) Subscribe(topic string, handler Handler) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[topic] == nil {
		h.subscribers[topic] = map[int]Handler{}
	}
	id := h.nextID
	h.nextID++
	h.subscribers[topic][id] = handler
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.subscribers[topic] == nil {
			return
		}
		delete(h.subscribers[topic], id)
		if len(h.subscribers[topic]) == 0 {
			delete(h.subscribers, topic)
		}
	}
}

func (h *MemoryHub) Published() []Envelope {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Envelope, 0, len(h.published))
	for _, item := range h.published {
		out = append(out, cloneEnvelope(item))
	}
	return out
}

func (h *MemoryHub) handlersLocked(topic string) []Handler {
	var handlers []Handler
	for _, pattern := range []string{topic, "*"} {
		for _, handler := range h.subscribers[pattern] {
			handlers = append(handlers, handler)
		}
	}
	return handlers
}

func cloneEnvelope(item Envelope) Envelope {
	cloned := item
	if item.Payload != nil {
		cloned.Payload = make(map[string]any, len(item.Payload))
		for k, v := range item.Payload {
			cloned.Payload[k] = v
		}
	}
	return cloned
}
