package events

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

type Handler func(context.Context, Envelope) error

type responseError struct {
	envelope Envelope
}

func (e responseError) Error() string {
	return "event response"
}

func Response(envelope Envelope) error {
	return responseError{envelope: prepareEnvelope(envelope)}
}

func ResponseEnvelope(err error) (Envelope, bool) {
	var response responseError
	if !errors.As(err, &response) {
		return Envelope{}, false
	}
	return cloneEnvelope(response.envelope), true
}

type Hub interface {
	Publish(context.Context, Envelope) error
	Subscribe(string, Handler) func()
}

type AsyncHub interface {
	PublishAsync(context.Context, Envelope) error
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
			if _, ok := ResponseEnvelope(err); ok {
				return err
			}
			return fmt.Errorf("handle event %s: %w", envelope.Topic, err)
		}
	}
	return nil
}

func (h *MemoryHub) PublishAsync(ctx context.Context, envelope Envelope) error {
	return h.Publish(ctx, envelope)
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
		cloned.Payload = cloneStringAnyMap(item.Payload)
	}
	return cloned
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
