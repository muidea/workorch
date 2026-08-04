package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

const (
	eventStreamViewFull       = "full"
	eventStreamViewProjection = "projection"
)

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

// RecordWorkspaceProjectionChanged and RecordEventRecordAppended are invoked
// by the application Biz's typed EventHub handlers. HTTP only stores and
// fans out already-typed notifications; it never owns an observer.
func (s *serverState) RecordWorkspaceProjectionChanged(_ context.Context, data workorchdevents.WorkspaceProjectionChangedData) {
	if s != nil {
		s.recordStreamEvent(workspaceProjectionStreamEvent(data))
	}
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
		Version:      recordpayload.WorkorchEventVersion,
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

func (s *serverState) RecordEventRecordAppended(_ context.Context, data eventstreamevents.EventRecordData) {
	if s == nil {
		return
	}
	if envelope, valid := eventRecordStreamEvent(data); valid {
		s.recordStreamEvent(envelope)
	}
}

// RecordRunStateChanged accepts only the concrete Run owner notification for
// process-local metrics, terminal-snapshot cache invalidation, and status-wait
// wakeups. It deliberately does not derive those facts from an Eventstream
// payload: Eventstream is audit/display infrastructure and may replay
// independently.
func (s *serverState) RecordRunStateChanged(_ context.Context, data runevents.RunStateChangedData) {
	if s == nil {
		return
	}
	s.metrics.observeRunState(
		string(data.WorkspaceID),
		string(data.RunID),
		string(data.State),
		data.RuntimeStateRevision,
		0,
	)
	s.notifyRunStatusChangedForRun(appservice.Run{
		ID:          string(data.RunID),
		WorkspaceID: string(data.WorkspaceID),
		State:       string(data.State),
	})
}

// RecordRuntimeDriveObserved is a process-local metric projection of the
// Orchestrator owner's immutable telemetry. It intentionally has no access to
// a Runtime repository, reducer, scheduler, or EventHub.
func (s *serverState) RecordRuntimeDriveObserved(data orchestratorevents.RuntimeDriveObservedData) {
	if s == nil || data.Validate() != nil {
		return
	}
	s.metrics.observeRuntimeDrive(data)
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
