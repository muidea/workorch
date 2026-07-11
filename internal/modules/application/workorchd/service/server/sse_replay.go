package server

import (
	"sync"
)

const defaultSSEReplayLimit = 256

type sseReplayBuffer struct {
	mu    sync.RWMutex
	limit int
	items []streamEvent
}

func newSSEReplayBuffer(limit int) *sseReplayBuffer {
	if limit <= 0 {
		limit = defaultSSEReplayLimit
	}
	return &sseReplayBuffer{limit: limit}
}

func (b *sseReplayBuffer) Append(envelope streamEvent) {
	if b == nil || envelope.ID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) >= b.limit {
		copy(b.items, b.items[1:])
		b.items[len(b.items)-1] = envelope
		return
	}
	b.items = append(b.items, envelope)
}

func (b *sseReplayBuffer) After(lastID string, filter func(streamEvent) bool) []streamEvent {
	if b == nil || lastID == "" {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	start := -1
	for idx, item := range b.items {
		if item.ID == lastID {
			start = idx + 1
			break
		}
	}
	if start < 0 || start >= len(b.items) {
		return nil
	}
	out := make([]streamEvent, 0, len(b.items)-start)
	for _, item := range b.items[start:] {
		if filter == nil || filter(item) {
			out = append(out, item)
		}
	}
	return out
}
