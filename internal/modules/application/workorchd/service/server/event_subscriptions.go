package server

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	eventStreamViewFull       = "full"
	eventStreamViewProjection = "projection"
)

var serverStreamObserverSequence atomic.Uint64

type eventRecordHandler func(context.Context, eventstreamevents.EventRecordData) error
type streamEventHandler func(context.Context, streamEvent) error

func normalizeEventStreamView(view string) string {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "", eventStreamViewProjection:
		return eventStreamViewProjection
	case eventStreamViewFull:
		return eventStreamViewFull
	default:
		return eventStreamViewProjection
	}
}

func isProjectionEventStreamView(view string) bool {
	return normalizeEventStreamView(view) == eventStreamViewProjection
}

func subscribeServerStreamEvents(commandHub magicevent.Hub, view string, handler streamEventHandler) []func() {
	if handler == nil {
		return nil
	}
	unsubscribers := make([]func(), 0, 2)
	if commandHub != nil {
		unsubscribers = append(unsubscribers, subscribeWorkspaceProjectionChanged(commandHub, handler))
	}
	if isProjectionEventStreamView(view) {
		return unsubscribers
	}
	if commandHub == nil {
		return unsubscribers
	}
	unsubscribers = append(unsubscribers, subscribeEventRecordAppended(commandHub, "event-stream", func(ctx context.Context, record eventstreamevents.EventRecordData) error {
		envelope, valid := eventRecordStreamEvent(record)
		if !valid {
			return nil
		}
		return handler(ctx, envelope)
	}))
	return unsubscribers
}

func subscribeWorkspaceProjectionChanged(commandHub magicevent.Hub, handler streamEventHandler) func() {
	if commandHub == nil || handler == nil {
		return func() {}
	}
	observerID := fmt.Sprintf("/workorchd/consumers/application/workorchd/server/projection/%d", serverStreamObserverSequence.Add(1))
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
			_ = handler(event.Context(), workspaceProjectionStreamEvent(data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	return func() { observer.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged) }
}

func workspaceProjectionStreamEvent(data workorchdevents.WorkspaceProjectionChangedData) streamEvent {
	// In-process envelope carries typed projection facts for filters/replay buffer.
	// The HTTP/SSE wire frame is always the concrete workspaceProjectionChangedSSE
	// DTO (see writeSSEEnvelope / writeProjectionSSE); Payload is never the wire body.
	dto := workspaceProjectionChangedSSEFromData(data)
	changedSections := append([]string(nil), dto.ChangedSections...)
	payload := map[string]any{
		"id":               dto.ID,
		"streamGeneration": dto.StreamGeneration,
		"streamRevision":   dto.StreamRevision,
		"workspaceId":      dto.WorkspaceID,
		"runId":            dto.RunID,
		"revision": map[string]any{
			"streamGeneration":        dto.Revision.StreamGeneration,
			"streamRevision":          dto.Revision.StreamRevision,
			"workspaceRevision":       dto.Revision.WorkspaceRevision,
			"projectedSourceRevision": dto.Revision.ProjectedSourceRevision,
			"runtimeStateRevision":    dto.Revision.RuntimeStateRevision,
			"evidenceSequence":        dto.Revision.EvidenceSequence,
			"activityRevision":        dto.Revision.ActivityRevision,
		},
		"changedSections": changedSections,
		"projectionState": dto.ProjectionState,
		"updatedAt":       dto.UpdatedAt,
	}
	return streamEvent{
		ID:           dto.ID,
		Version:      events.WorkorchEventVersion,
		Topic:        workorchdevents.TopicWorkspaceProjectionChanged,
		OccurredAt:   dto.UpdatedAt,
		SourceModule: "workorchd.workspace_projection",
		SubjectType:  "workspace_projection",
		SubjectID:    dto.WorkspaceID,
		WorkspaceID:  dto.WorkspaceID,
		RunID:        dto.RunID,
		Revision:     dto.StreamRevision,
		Payload:      payload,
	}
}

func projectionNotificationID(data workorchdevents.WorkspaceProjectionChangedData) string {
	workspaceID := strings.TrimSpace(string(data.WorkspaceID))
	if workspaceID == "" {
		workspaceID = "workspace"
	}
	// FR05: ID uses the max durable-comparable stream key so restart-seeded
	// GlobalRevision/WorkspaceRevision/ActivityRevision floors cannot silently
	// collide with pre-restart Last-Event-ID values.
	seq := data.GlobalRevision
	if data.WorkspaceRevision > seq {
		seq = data.WorkspaceRevision
	}
	if data.ActivityRevision > seq {
		seq = data.ActivityRevision
	}
	generation := strings.TrimSpace(data.StreamGeneration)
	if generation == "" {
		// Keep the historical shape for explicit legacy fixtures/clients. Production
		// projection stores always publish a non-empty generation.
		return fmt.Sprintf("projection:%s:%d", workspaceID, seq)
	}
	return fmt.Sprintf("projection:%s:%s:%d", generation, workspaceID, seq)
}

func subscribeEventRecordAppended(commandHub magicevent.Hub, scope string, handler eventRecordHandler) func() {
	if commandHub == nil || handler == nil {
		return func() {}
	}
	observerID := fmt.Sprintf("/eventstream/consumers/application/workorchd/server/%s/%d", strings.Trim(strings.TrimSpace(scope), "/"), serverStreamObserverSequence.Add(1))
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, commandHub)
	observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
		if ok {
			_ = handler(event.Context(), data.Record)
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	return func() {
		observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended)
	}
}

func eventRecordStreamEvent(data eventstreamevents.EventRecordData) (streamEvent, bool) {
	if data.Topic == "" {
		return streamEvent{}, false
	}
	return streamEvent{
		ID:           string(data.ID),
		Version:      data.Version,
		Topic:        data.Topic,
		OccurredAt:   data.OccurredAt,
		SourceModule: data.SourceModule,
		SubjectType:  data.SubjectType,
		SubjectID:    data.SubjectID,
		WorkspaceID:  string(data.WorkspaceID),
		RunID:        string(data.RunID),
		StepID:       string(data.StepID),
		Revision:     data.Revision,
		Payload:      data.Payload.Map(),
	}, true
}

func unsubscribeAll(unsubscribers []func()) {
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}
