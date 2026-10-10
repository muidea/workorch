package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
)

// Stream fan-out is an HTTP projection concern. It republishes already
// observed envelopes and never issues owner commands or lifecycle transitions.

func (s *serverState) recordStreamEvent(envelope streamEvent) {
	if s == nil {
		return
	}
	if s.eventReplay != nil {
		s.eventReplay.Append(envelope)
	}
	s.streamSubscribersMu.Lock()
	defer s.streamSubscribersMu.Unlock()
	for _, ch := range s.streamSubscribers {
		enqueueEventStreamEnvelope(ch, envelope)
	}
}

func (s *serverState) subscribeStreamEvents() (<-chan streamEvent, func()) {
	if s == nil {
		return nil, func() {}
	}
	ch := make(chan streamEvent, 128)
	s.streamSubscribersMu.Lock()
	s.streamSubscriberSequence++
	id := s.streamSubscriberSequence
	s.streamSubscribers[id] = ch
	s.streamSubscribersMu.Unlock()
	return ch, func() {
		s.streamSubscribersMu.Lock()
		delete(s.streamSubscribers, id)
		s.streamSubscribersMu.Unlock()
	}
}

func isMemoryBackend(backend string) bool {
	return backend == "memory"
}

func enqueueEventStreamEnvelope(ch chan streamEvent, envelope streamEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- envelope:
		return
	default:
	}
	if !isEventStreamCriticalTopic(envelope.Topic) {
		return
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- envelope:
	default:
	}
}

func isEventStreamCriticalTopic(topic string) bool {
	switch topic {
	case workorchdevents.TopicWorkspaceProjectionChanged,
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceDeleted,
		workspaceevents.TopicWorkspaceStateChanged,
		runevents.TopicRunStateChanged:
		return true
	default:
		return false
	}
}

func writeSSEEnvelope(w http.ResponseWriter, flusher http.Flusher, envelope streamEvent) bool {
	// Projection notifications must serialize as the concrete top-level DTO so
	// remote decodeProjectionSSE can consume live and replay frames identically.
	// streamEvent.Revision is a scalar and would conflict with nested revision.
	if dto, ok := workspaceProjectionChangedSSEFromStreamEvent(envelope); ok {
		return writeProjectionSSE(w, flusher, dto)
	}
	data, err := json.Marshal(enrichSSEEnvelope(envelope))
	if err != nil {
		return true
	}
	if strings.TrimSpace(envelope.ID) != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", envelope.ID); err != nil {
			return false
		}
	}
	if _, err := fmt.Fprint(w, "event: workorch\n"); err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
