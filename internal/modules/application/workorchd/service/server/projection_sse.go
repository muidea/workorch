package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

// workspaceProjectionChangedSSE is the concrete HTTP/SSE wire DTO for
// projection-view notifications. It is the sole live/replay serialization
// contract for view=projection and must match the remote backend decoder.
//
// Importantly this DTO does NOT reuse streamEvent: streamEvent.Revision is a
// scalar and would conflict with the nested revision object required here.
type workspaceProjectionChangedSSE struct {
	ID               string                         `json:"id"`
	StreamGeneration string                         `json:"streamGeneration,omitempty"`
	StreamRevision   int64                          `json:"streamRevision"`
	WorkspaceID      string                         `json:"workspaceId"`
	RunID            string                         `json:"runId,omitempty"`
	Revision         workspaceProjectionRevisionSSE `json:"revision"`
	ChangedSections  []string                       `json:"changedSections,omitempty"`
	ProjectionState  string                         `json:"projectionState,omitempty"`
	UpdatedAt        string                         `json:"updatedAt,omitempty"`
}

type workspaceProjectionRevisionSSE struct {
	StreamGeneration        string `json:"streamGeneration,omitempty"`
	StreamRevision          int64  `json:"streamRevision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
	ActivityRevision        int64  `json:"activityRevision,omitempty"`
}

func workspaceProjectionChangedSSEFromData(data workorchdevents.WorkspaceProjectionChangedData) workspaceProjectionChangedSSE {
	changedSections := make([]string, 0, len(data.ChangedSections))
	for _, section := range data.ChangedSections {
		if value := strings.TrimSpace(string(section)); value != "" {
			changedSections = append(changedSections, value)
		}
	}
	id := projectionNotificationID(data)
	// FR05: streamRevision shares the same restart-safe floor as IDs.
	streamRevision := data.GlobalRevision
	if data.WorkspaceRevision > streamRevision {
		streamRevision = data.WorkspaceRevision
	}
	if data.ActivityRevision > streamRevision {
		streamRevision = data.ActivityRevision
	}
	return workspaceProjectionChangedSSE{
		ID:               id,
		StreamGeneration: data.StreamGeneration,
		StreamRevision:   streamRevision,
		WorkspaceID:      string(data.WorkspaceID),
		RunID:            string(data.RunID),
		Revision: workspaceProjectionRevisionSSE{
			StreamGeneration:        data.StreamGeneration,
			StreamRevision:          streamRevision,
			WorkspaceRevision:       data.WorkspaceRevision,
			ProjectedSourceRevision: data.ProjectedSourceRevision,
			RuntimeStateRevision:    data.RuntimeStateRevision,
			EvidenceSequence:        data.EvidenceSequence,
			ActivityRevision:        data.ActivityRevision,
		},
		ChangedSections: changedSections,
		ProjectionState: data.ProjectionState,
		UpdatedAt:       firstNonEmptyString(data.UpdatedAt, data.RebuiltAt),
	}
}

// workspaceProjectionChangedSSEFromStreamEvent reconstructs the concrete wire
// DTO from an in-process streamEvent that was produced by
// workspaceProjectionStreamEvent. Used by live write and in-memory replay so
// both paths share one serializer.
func workspaceProjectionChangedSSEFromStreamEvent(envelope streamEvent) (workspaceProjectionChangedSSE, bool) {
	if strings.TrimSpace(envelope.Topic) != workorchdevents.TopicWorkspaceProjectionChanged {
		return workspaceProjectionChangedSSE{}, false
	}
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(payloadString(envelope.Payload, "workspaceId"))
	}
	if workspaceID == "" {
		return workspaceProjectionChangedSSE{}, false
	}
	id := strings.TrimSpace(envelope.ID)
	if id == "" {
		id = strings.TrimSpace(payloadString(envelope.Payload, "id"))
	}
	streamRevision := envelope.Revision
	if streamRevision == 0 {
		streamRevision = payloadInt64(envelope.Payload, "streamRevision", "globalRevision")
	}
	if streamRevision == 0 {
		streamRevision = payloadNestedInt64(envelope.Payload, "revision", "streamRevision")
	}
	runID := strings.TrimSpace(envelope.RunID)
	if runID == "" {
		runID = strings.TrimSpace(payloadString(envelope.Payload, "runId"))
	}
	changedSections := payloadStringSlice(envelope.Payload, "changedSections")
	streamGeneration := payloadString(envelope.Payload, "streamGeneration")
	return workspaceProjectionChangedSSE{
		ID:               id,
		StreamGeneration: streamGeneration,
		StreamRevision:   streamRevision,
		WorkspaceID:      workspaceID,
		RunID:            runID,
		Revision: workspaceProjectionRevisionSSE{
			StreamGeneration:        streamGeneration,
			StreamRevision:          streamRevision,
			WorkspaceRevision:       payloadInt64OrNested(envelope.Payload, "workspaceRevision", "revision", "workspaceRevision"),
			ProjectedSourceRevision: payloadInt64OrNested(envelope.Payload, "projectedSourceRevision", "revision", "projectedSourceRevision"),
			RuntimeStateRevision:    payloadUint64OrNested(envelope.Payload, "runtimeStateRevision", "revision", "runtimeStateRevision"),
			EvidenceSequence:        payloadUint64OrNested(envelope.Payload, "evidenceSequence", "revision", "evidenceSequence"),
			ActivityRevision:        payloadInt64OrNested(envelope.Payload, "activityRevision", "revision", "activityRevision"),
		},
		ChangedSections: changedSections,
		ProjectionState: firstNonEmptyString(payloadString(envelope.Payload, "projectionState"), payloadString(envelope.Payload, "state")),
		UpdatedAt:       firstNonEmptyString(payloadString(envelope.Payload, "updatedAt"), envelope.OccurredAt, payloadString(envelope.Payload, "rebuiltAt")),
	}, true
}

func writeProjectionSSE(w http.ResponseWriter, flusher http.Flusher, dto workspaceProjectionChangedSSE) bool {
	data, err := json.Marshal(dto)
	if err != nil {
		return true
	}
	if strings.TrimSpace(dto.ID) != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", dto.ID); err != nil {
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func payloadStringSlice(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" && value != "<nil>" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func payloadInt64(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := asInt64(payload[key]); ok {
			return value
		}
	}
	return 0
}

func payloadUint64(payload map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		if value, ok := asUint64(payload[key]); ok {
			return value
		}
	}
	return 0
}

func payloadNestedInt64(payload map[string]any, objectKey, fieldKey string) int64 {
	if payload == nil {
		return 0
	}
	raw, ok := payload[objectKey]
	if !ok {
		return 0
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	value, _ := asInt64(nested[fieldKey])
	return value
}

func payloadNestedUint64(payload map[string]any, objectKey, fieldKey string) uint64 {
	if payload == nil {
		return 0
	}
	raw, ok := payload[objectKey]
	if !ok {
		return 0
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	value, _ := asUint64(nested[fieldKey])
	return value
}

func payloadInt64OrNested(payload map[string]any, topKey, objectKey, fieldKey string) int64 {
	if value := payloadInt64(payload, topKey); value != 0 {
		return value
	}
	return payloadNestedInt64(payload, objectKey, fieldKey)
}

func payloadUint64OrNested(payload map[string]any, topKey, objectKey, fieldKey string) uint64 {
	if value := payloadUint64(payload, topKey); value != 0 {
		return value
	}
	return payloadNestedUint64(payload, objectKey, fieldKey)
}

func asInt64(raw any) (int64, bool) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	default:
		return 0, false
	}
}

func asUint64(raw any) (uint64, bool) {
	switch typed := raw.(type) {
	case int:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int32:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case uint:
		return uint64(typed), true
	case uint32:
		return uint64(typed), true
	case uint64:
		return typed, true
	case float64:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case float32:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	default:
		return 0, false
	}
}
