package server

import (
	"context"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (s *serverState) replayEventStream(ctx context.Context, lastEventID string, workspaceID string, runID string, view string, filter func(streamEvent) bool, emit func(streamEvent) bool) bool {
	lastEventID = strings.TrimSpace(lastEventID)
	if s == nil || lastEventID == "" || emit == nil {
		return true
	}
	emitEnvelope := func(envelope streamEvent) bool {
		if filter != nil && !filter(envelope) {
			return true
		}
		return emit(envelope)
	}
	// Projection notifications are not owner EventRecords. Live and reconnect
	// replay must share the concrete projection DTO path via the in-memory
	// buffer (writeSSEEnvelope → writeProjectionSSE). Do not reconstruct
	// projection frames from generic event-record envelopes.
	if !isProjectionEventStreamView(view) {
		// Full audit replay is always read page-by-page from Eventstream
		// persistence. The live subscription is installed before this call, so
		// events appended during catch-up are queued without depending on the
		// bounded process-local replay ring.
		return s.replayPersistedEventStream(ctx, lastEventID, workspaceID, runID, eventStreamReplayTopic(view), emitEnvelope)
	}
	if s.eventReplay != nil {
		for _, envelope := range s.eventReplay.After(lastEventID, filter) {
			if !emit(envelope) {
				return false
			}
		}
	}
	return true
}

func eventStreamReplayTopic(view string) string {
	if isProjectionEventStreamView(view) {
		// Intentionally unused for projection view replay (see above).
		return workorchdevents.TopicWorkspaceProjectionChanged
	}
	return ""
}

func (s *serverState) replayPersistedEventStream(ctx context.Context, lastEventID string, workspaceID string, runID string, topic string, emit func(streamEvent) bool) bool {
	if s == nil || s.listEventRecords == nil || strings.TrimSpace(lastEventID) == "" {
		return true
	}
	cursor := common.EventRecordID(strings.TrimSpace(lastEventID))
	for {
		if err := ctx.Err(); err != nil {
			return false
		}
		items, err := s.listEventRecords(ctx, s.config.Backend, s.config.DataRoot, workorchquerycontract.EventRecordQuery{
			WorkspaceID:    common.WorkspaceID(strings.TrimSpace(workspaceID)),
			RunID:          common.RunID(strings.TrimSpace(runID)),
			Topic:          strings.TrimSpace(topic),
			CursorAfterID:  cursor,
			Limit:          maxEventRecordPageLimit,
			IncludePayload: eventStreamReplayIncludePayload(topic),
		})
		if err != nil || len(items) == 0 {
			return err == nil
		}
		nextCursor := cursor
		for _, item := range items {
			if envelope := eventRecordSSEEnvelope(item); envelope.Topic != "" && !emit(envelope) {
				return false
			}
			if id := strings.TrimSpace(item.ID); id != "" {
				nextCursor = common.EventRecordID(id)
			}
		}
		if len(items) < maxEventRecordPageLimit || nextCursor == cursor {
			return true
		}
		cursor = nextCursor
	}
}

func eventStreamReplayIncludePayload(topic string) bool {
	return strings.TrimSpace(topic) == ""
}

func eventRecordSSEEnvelope(item projectioncontract.EventRecord) streamEvent {
	return streamEvent{
		ID:             item.ID,
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		StreamPosition: item.StreamPosition,
		Revision:       item.Revision,
		Payload:        cloneSSEPayload(item.Payload.Map()),
	}
}

func cloneSSEPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	ret := make(map[string]any, len(payload))
	for key, value := range payload {
		ret[key] = value
	}
	return ret
}
