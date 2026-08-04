package server

import (
	"context"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (s *serverState) replayEventStreamEnvelopes(ctx context.Context, lastEventID string, workspaceID string, runID string, view string, filter func(streamEvent) bool) []streamEvent {
	lastEventID = strings.TrimSpace(lastEventID)
	if s == nil || lastEventID == "" {
		return nil
	}
	seen := map[string]struct{}{}
	ret := []streamEvent{}
	appendEnvelope := func(envelope streamEvent) {
		if filter != nil && !filter(envelope) {
			return
		}
		id := strings.TrimSpace(envelope.ID)
		if id != "" {
			if _, ok := seen[id]; ok {
				return
			}
			seen[id] = struct{}{}
		}
		ret = append(ret, envelope)
	}
	// Projection notifications are not owner EventRecords. Live and reconnect
	// replay must share the concrete projection DTO path via the in-memory
	// buffer (writeSSEEnvelope → writeProjectionSSE). Do not reconstruct
	// projection frames from generic event-record envelopes.
	if !isProjectionEventStreamView(view) {
		for _, envelope := range s.persistedReplayEnvelopes(ctx, lastEventID, workspaceID, runID, eventStreamReplayTopic(view)) {
			appendEnvelope(envelope)
		}
	}
	if s.eventReplay != nil {
		for _, envelope := range s.eventReplay.After(lastEventID, filter) {
			appendEnvelope(envelope)
		}
	}
	return ret
}

func eventStreamReplayTopic(view string) string {
	if isProjectionEventStreamView(view) {
		// Intentionally unused for projection view replay (see above).
		return workorchdevents.TopicWorkspaceProjectionChanged
	}
	return ""
}

func (s *serverState) persistedReplayEnvelopes(ctx context.Context, lastEventID string, workspaceID string, runID string, topic string) []streamEvent {
	if s == nil || s.listEventRecords == nil || strings.TrimSpace(lastEventID) == "" {
		return nil
	}
	items, err := s.listEventRecords(ctx, s.config.Backend, s.config.DataRoot, workorchquerycontract.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(strings.TrimSpace(workspaceID)),
		RunID:          common.RunID(strings.TrimSpace(runID)),
		Topic:          strings.TrimSpace(topic),
		CursorAfterID:  common.EventRecordID(strings.TrimSpace(lastEventID)),
		Limit:          maxEventRecordPageLimit,
		IncludePayload: eventStreamReplayIncludePayload(topic),
	})
	if err != nil || len(items) == 0 {
		return nil
	}
	ret := make([]streamEvent, 0, len(items))
	for _, item := range items {
		if envelope := eventRecordSSEEnvelope(item); envelope.Topic != "" {
			ret = append(ret, envelope)
		}
	}
	return ret
}

func eventStreamReplayIncludePayload(topic string) bool {
	return strings.TrimSpace(topic) == ""
}

func eventRecordSSEEnvelope(item projectioncontract.EventRecord) streamEvent {
	return streamEvent{
		ID:           item.ID,
		Version:      item.Version,
		Topic:        item.Topic,
		OccurredAt:   item.OccurredAt,
		SourceModule: item.SourceModule,
		SubjectType:  item.SubjectType,
		SubjectID:    item.SubjectID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		StepID:       item.StepID,
		Revision:     item.Revision,
		Payload:      cloneSSEPayload(item.Payload.Map()),
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
