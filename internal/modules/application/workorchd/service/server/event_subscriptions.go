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
	payload := map[string]any{
		"type":                    "workspace_projection_changed",
		"workspaceId":             string(data.WorkspaceID),
		"globalRevision":          data.GlobalRevision,
		"workspaceRevision":       data.WorkspaceRevision,
		"sourceEventRevision":     data.SourceEventRevision,
		"projectedSourceRevision": data.ProjectedSourceRevision,
		"workspaceTotal":          data.WorkspaceTotal,
		"changeKind":              data.ChangeKind,
		"projectionState":         data.ProjectionState,
		"updatedAt":               data.UpdatedAt,
		"rebuiltAt":               data.RebuiltAt,
	}
	if data.StaleReason != "" {
		payload["staleReason"] = data.StaleReason
	}
	if data.Error != "" {
		payload["error"] = data.Error
	}
	return streamEvent{
		Version:      events.WorkorchEventVersion,
		Topic:        workorchdevents.TopicWorkspaceProjectionChanged,
		OccurredAt:   data.UpdatedAt,
		SourceModule: "workorchd.workspace_projection",
		SubjectType:  "workspace_projection",
		SubjectID:    string(data.WorkspaceID),
		WorkspaceID:  string(data.WorkspaceID),
		RunID:        string(data.RunID),
		Payload:      payload,
	}
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
