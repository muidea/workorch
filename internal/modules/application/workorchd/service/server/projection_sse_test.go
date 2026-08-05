package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	backend "github.com/muidea/workorch/internal/services/workorch/biz/backends"
)

// remoteProjectionChangedDTO mirrors the remote backend decoder shape.
// Keep field tags identical to backends.workspaceProjectionChangedSSE.
type remoteProjectionChangedDTO struct {
	ID               string `json:"id"`
	StreamGeneration string `json:"streamGeneration,omitempty"`
	StreamRevision   int64  `json:"streamRevision"`
	WorkspaceID      string `json:"workspaceId"`
	RunID            string `json:"runId,omitempty"`
	Revision         struct {
		StreamGeneration        string `json:"streamGeneration,omitempty"`
		StreamRevision          int64  `json:"streamRevision,omitempty"`
		WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
		ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
		RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
		EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
		ActivityRevision        int64  `json:"activityRevision,omitempty"`
	} `json:"revision"`
	ChangedSections []string `json:"changedSections"`
	ProjectionState string   `json:"projectionState"`
	UpdatedAt       string   `json:"updatedAt"`
}

func TestProjectionSSEWireContractMatchesRemoteDecoder(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?view=projection&workspaceId=ws-wire", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	lines := make(chan string, 32)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		readErr <- scanner.Err()
	}()
	waitSSEConnected(t, lines, readErr)

	testStateCommandHub(state).Post(magicevent.NewEventWithContext(
		workorchdevents.TopicWorkspaceProjectionChanged,
		"workorchd/projection",
		workorchdevents.NotificationDestination,
		magicevent.NewHeader(),
		context.Background(),
		workorchdevents.WorkspaceProjectionChangedData{
			StreamGeneration:        "server-wire-generation",
			WorkspaceID:             "ws-wire",
			RunID:                   "run-wire",
			GlobalRevision:          99,
			WorkspaceRevision:       12,
			ProjectedSourceRevision: 40,
			RuntimeStateRevision:    77,
			EvidenceSequence:        6,
			ActivityRevision:        15,
			ChangedSections: []projectioncontract.WorkspaceProjectionSection{
				projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
				projectioncontract.WorkspaceProjectionSectionEvidence,
				projectioncontract.WorkspaceProjectionSectionActivity,
			},
			ProjectionState: "ready",
			UpdatedAt:       "2026-07-15T16:00:00Z",
		},
	))

	deadline := time.After(2 * time.Second)
	var eventID string
	var dataLine string
	for dataLine == "" {
		select {
		case line := <-lines:
			switch {
			case strings.HasPrefix(line, "id:"):
				eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "data:"):
				dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		case err := <-readErr:
			t.Fatalf("stream closed: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for projection wire frame")
		}
	}

	// Must not look like streamEvent envelope (no topic/payload/version keys as authority).
	var raw map[string]any
	if err := json.Unmarshal([]byte(dataLine), &raw); err != nil {
		t.Fatalf("raw json: %v body=%s", err, dataLine)
	}
	if _, ok := raw["topic"]; ok {
		t.Fatalf("projection wire must not use streamEvent topic envelope: %s", dataLine)
	}
	if _, ok := raw["payload"]; ok {
		t.Fatalf("projection wire must not nest concrete DTO under payload: %s", dataLine)
	}
	if rev, ok := raw["revision"]; ok {
		if _, isNum := rev.(float64); isNum {
			t.Fatalf("revision must be object, not scalar: %s", dataLine)
		}
	} else {
		t.Fatalf("missing revision object: %s", dataLine)
	}

	var dto remoteProjectionChangedDTO
	if err := json.Unmarshal([]byte(dataLine), &dto); err != nil {
		t.Fatalf("remote-shaped decode failed: %v body=%s", err, dataLine)
	}
	if eventID != "projection:server-wire-generation:ws-wire:99" || dto.ID != eventID {
		t.Fatalf("id event=%q dto=%q, want generation-scoped projection id", eventID, dto.ID)
	}
	if dto.StreamGeneration != "server-wire-generation" || dto.Revision.StreamGeneration != "server-wire-generation" {
		t.Fatalf("stream generation mismatch: %+v", dto)
	}
	if dto.StreamRevision != 99 || dto.Revision.StreamRevision != 99 {
		t.Fatalf("stream revision mismatch: %+v", dto)
	}
	if dto.WorkspaceID != "ws-wire" || dto.RunID != "run-wire" {
		t.Fatalf("identity mismatch: %+v", dto)
	}
	if dto.Revision.WorkspaceRevision != 12 ||
		dto.Revision.ProjectedSourceRevision != 40 ||
		dto.Revision.RuntimeStateRevision != 77 ||
		dto.Revision.EvidenceSequence != 6 ||
		dto.Revision.ActivityRevision != 15 {
		t.Fatalf("multi-domain revisions mismatch: %+v", dto.Revision)
	}
	if len(dto.ChangedSections) != 3 {
		t.Fatalf("changedSections = %v", dto.ChangedSections)
	}
	if dto.ProjectionState != "ready" || dto.UpdatedAt != "2026-07-15T16:00:00Z" {
		t.Fatalf("state/updatedAt = %+v", dto)
	}
}

func TestWorkspaceProjectionChangedSSEFromDataIsStable(t *testing.T) {
	data := workorchdevents.WorkspaceProjectionChangedData{
		StreamGeneration:     "server-cross-generation",
		WorkspaceID:          "ws-stable",
		GlobalRevision:       3,
		WorkspaceRevision:    1,
		RuntimeStateRevision: 2,
		ActivityRevision:     4,
		ChangedSections:      []projectioncontract.WorkspaceProjectionSection{projectioncontract.WorkspaceProjectionSectionOverview},
		ProjectionState:      "ready",
		UpdatedAt:            "2026-07-15T16:02:00Z",
	}
	first := workspaceProjectionChangedSSEFromData(data)
	second := workspaceProjectionChangedSSEFromData(data)
	if first.ID != second.ID || first.StreamRevision != second.StreamRevision || first.Revision.ActivityRevision != second.Revision.ActivityRevision {
		t.Fatalf("unstable dto: %+v vs %+v", first, second)
	}
	// round-trip through streamEvent reconstruction used by writeSSEEnvelope
	envelope := workspaceProjectionStreamEvent(data)
	fromEnvelope, ok := workspaceProjectionChangedSSEFromStreamEvent(envelope)
	if !ok {
		t.Fatal("streamEvent reconstruction failed")
	}
	if fromEnvelope.ID != first.ID || fromEnvelope.StreamRevision != first.StreamRevision ||
		fromEnvelope.Revision.ActivityRevision != first.Revision.ActivityRevision ||
		fromEnvelope.Revision.RuntimeStateRevision != first.Revision.RuntimeStateRevision {
		t.Fatalf("streamEvent reconstruct mismatch: %+v vs %+v", fromEnvelope, first)
	}
}

// CR06: real server serializer frame must be consumed by production remote decoder.
// This is the cross-boundary contract evidence — not a server-local DTO copy decode.
func TestProjectionSSEFrameConsumedByProductionRemoteDecoder(t *testing.T) {
	// Build the exact wire DTO the server serializer would emit.
	data := workorchdevents.WorkspaceProjectionChangedData{
		StreamGeneration:        "server-cross-generation",
		WorkspaceID:             "ws-cross",
		RunID:                   "run-cross",
		GlobalRevision:          42,
		WorkspaceRevision:       11,
		ProjectedSourceRevision: 30,
		RuntimeStateRevision:    55,
		EvidenceSequence:        4,
		ActivityRevision:        9,
		ChangedSections: []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
			projectioncontract.WorkspaceProjectionSectionActivity,
		},
		ProjectionState: "ready",
		UpdatedAt:       "2026-07-15T18:00:00Z",
	}
	dto := workspaceProjectionChangedSSEFromData(data)

	// Capture the SSE frame bytes exactly as writeProjectionSSE would write them.
	rec := httptest.NewRecorder()
	if !writeProjectionSSE(rec, rec, dto) {
		t.Fatal("writeProjectionSSE failed")
	}
	frame := rec.Body.String()
	if !strings.Contains(frame, "event: workorch") || !strings.Contains(frame, "data:") {
		t.Fatalf("unexpected SSE frame shape: %q", frame)
	}

	// Parse SSE frame lines the same way remote backend does.
	var eventID string
	var dataLines []string
	for _, line := range strings.Split(frame, "\n") {
		switch {
		case strings.HasPrefix(line, "id:"):
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if eventID == "" || len(dataLines) == 0 {
		t.Fatalf("failed to parse SSE frame: id=%q data=%v frame=%q", eventID, dataLines, frame)
	}

	// Production decoder — not a copied DTO + json.Unmarshal.
	info, ok := backend.DecodeProjectionSSE(dataLines, eventID)
	if !ok {
		t.Fatalf("production DecodeProjectionSSE rejected server frame: id=%q data=%v", eventID, dataLines)
	}
	if string(info.WorkspaceID) != "ws-cross" || string(info.RunID) != "run-cross" {
		t.Fatalf("identity mismatch: %+v", info)
	}
	if info.ID != eventID || info.ID != "projection:server-cross-generation:ws-cross:42" {
		t.Fatalf("id = %q eventID=%q, want generation-scoped projection id", info.ID, eventID)
	}
	if info.Revision.StreamGeneration != "server-cross-generation" {
		t.Fatalf("decoder lost stream generation: %+v", info.Revision)
	}
	if info.Revision.StreamRevision != 42 ||
		info.Revision.WorkspaceRevision != 11 ||
		info.Revision.ProjectedSourceRevision != 30 ||
		info.Revision.RuntimeStateRevision != 55 ||
		info.Revision.EvidenceSequence != 4 ||
		info.Revision.ActivityRevision != 9 {
		t.Fatalf("revision mismatch: %+v", info.Revision)
	}
	if len(info.ChangedSections) != 2 {
		t.Fatalf("changedSections = %v", info.ChangedSections)
	}
	if info.ProjectionState != "ready" || info.UpdatedAt != "2026-07-15T18:00:00Z" {
		t.Fatalf("state/updatedAt = %+v", info)
	}

	// Also exercise the live HTTP path: server mux → production decoder.
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?view=projection&workspaceId=ws-live-cross", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	lines := make(chan string, 32)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		readErr <- scanner.Err()
	}()
	waitSSEConnected(t, lines, readErr)

	testStateCommandHub(state).Post(magicevent.NewEventWithContext(
		workorchdevents.TopicWorkspaceProjectionChanged,
		"workorchd/projection",
		workorchdevents.NotificationDestination,
		magicevent.NewHeader(),
		context.Background(),
		workorchdevents.WorkspaceProjectionChangedData{
			WorkspaceID:             "ws-live-cross",
			RunID:                   "run-live-cross",
			GlobalRevision:          7,
			WorkspaceRevision:       3,
			ProjectedSourceRevision: 5,
			RuntimeStateRevision:    10,
			EvidenceSequence:        2,
			ActivityRevision:        4,
			ChangedSections: []projectioncontract.WorkspaceProjectionSection{
				projectioncontract.WorkspaceProjectionSectionActivity,
			},
			ProjectionState: "ready",
			UpdatedAt:       "2026-07-15T18:05:00Z",
		},
	))

	deadline := time.After(2 * time.Second)
	var liveID string
	var liveData []string
	for len(liveData) == 0 {
		select {
		case line := <-lines:
			switch {
			case strings.HasPrefix(line, "id:"):
				liveID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "data:"):
				liveData = append(liveData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		case err := <-readErr:
			t.Fatalf("stream closed: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for live projection frame")
		}
	}
	liveInfo, ok := backend.DecodeProjectionSSE(liveData, liveID)
	if !ok {
		t.Fatalf("production decoder rejected live server frame: id=%q data=%v", liveID, liveData)
	}
	if string(liveInfo.WorkspaceID) != "ws-live-cross" || liveInfo.Revision.ActivityRevision != 4 {
		t.Fatalf("live decoded info mismatch: %+v", liveInfo)
	}
}

// FR05: notification ID and streamRevision prefer the max durable floor so
// post-restart sequences cannot silently collide with pre-restart Last-Event-IDs.
func TestProjectionNotificationIDUsesRestartSafeFloor(t *testing.T) {
	data := workorchdevents.WorkspaceProjectionChangedData{
		WorkspaceID:       "ws-id-floor",
		GlobalRevision:    3,
		WorkspaceRevision: 9_000_000,
		ActivityRevision:  8_000_000,
	}
	id := projectionNotificationID(data)
	if id != "projection:ws-id-floor:9000000" {
		t.Fatalf("id = %q, want projection:ws-id-floor:9000000", id)
	}
	dto := workspaceProjectionChangedSSEFromData(data)
	if dto.StreamRevision != 9_000_000 || dto.Revision.StreamRevision != 9_000_000 {
		t.Fatalf("stream revision = %d/%d, want 9000000", dto.StreamRevision, dto.Revision.StreamRevision)
	}
	if dto.ID != id {
		t.Fatalf("dto id = %q, want %q", dto.ID, id)
	}
}

func TestProjectionNotificationIDScopesSequenceByStreamGeneration(t *testing.T) {
	old := workorchdevents.WorkspaceProjectionChangedData{
		StreamGeneration: "server-old", WorkspaceID: "ws-generation", GlobalRevision: 2,
	}
	neu := old
	neu.StreamGeneration = "server-new"
	if projectionNotificationID(old) == projectionNotificationID(neu) {
		t.Fatalf("generation change must prevent Last-Event-ID collision: old=%q new=%q", projectionNotificationID(old), projectionNotificationID(neu))
	}
}
