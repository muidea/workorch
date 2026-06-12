package server

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (s *serverState) replayEventStreamEnvelopes(ctx context.Context, lastEventID string, workspaceID string, runID string, filter func(events.Envelope) bool) []events.Envelope {
	lastEventID = strings.TrimSpace(lastEventID)
	if s == nil || lastEventID == "" {
		return nil
	}
	seen := map[string]struct{}{}
	ret := []events.Envelope{}
	appendEnvelope := func(envelope events.Envelope) {
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
	for _, envelope := range s.persistedReplayEnvelopes(ctx, lastEventID, workspaceID, runID) {
		appendEnvelope(envelope)
	}
	if s.eventReplay != nil {
		for _, envelope := range s.eventReplay.After(lastEventID, filter) {
			appendEnvelope(envelope)
		}
	}
	return ret
}

func (s *serverState) persistedReplayEnvelopes(ctx context.Context, lastEventID string, workspaceID string, runID string) []events.Envelope {
	if s == nil || s.listEventRecords == nil || strings.TrimSpace(lastEventID) == "" {
		return nil
	}
	items, err := s.listEventRecords(ctx, s.config.Backend, s.config.DataRoot, appservice.EventRecordQuery{
		WorkspaceID:    strings.TrimSpace(workspaceID),
		RunID:          strings.TrimSpace(runID),
		CursorAfterID:  strings.TrimSpace(lastEventID),
		Limit:          maxEventRecordPageLimit,
		IncludePayload: true,
	})
	if err != nil || len(items) == 0 {
		return nil
	}
	ret := make([]events.Envelope, 0, len(items))
	for _, item := range items {
		if envelope := eventRecordSSEEnvelope(item); envelope.Topic != "" {
			ret = append(ret, envelope)
		}
	}
	return ret
}

func eventRecordSSEEnvelope(item appservice.EventRecord) events.Envelope {
	return events.Envelope{
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
		Payload:      cloneSSEPayload(item.Payload),
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
