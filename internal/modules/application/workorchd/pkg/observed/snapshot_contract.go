package observed

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const SnapshotVersion = "workorch.v1"

type SnapshotPayload interface {
	PayloadMap() map[string]any
}

type SnapshotItem struct {
	ID           string
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  string
	RunID        string
	StepID       string
	Payload      SnapshotPayload
}

type WorkspacePayload struct {
	Title                string
	Description          string
	DeliverableDirectory string
	State                string
	ActiveRunID          string
}

type RunPayload struct {
	GoalID             string
	DoDID              string
	State              string
	BlockingTicketID   string
	BlockingApprovalID string
	FailureReason      string
}

type DecisionTicketPayload struct {
	Reason            string
	State             string
	ResolutionID      string
	ResolutionType    string
	ResolutionPayload workorcheventpayload.Payload
}

type ApprovalPayload struct {
	Reason  string
	Action  string
	Context workorcheventpayload.Payload
	State   string
}

func (i SnapshotItem) PayloadMap() map[string]any {
	return snapshotPayloadMap(i.Payload)
}

func (p WorkspacePayload) PayloadMap() map[string]any {
	payload := map[string]any{
		events.PayloadKeyState:       p.State,
		events.PayloadKeyActiveRunID: p.ActiveRunID,
	}
	if p.Title != "" {
		payload[events.PayloadKeyTitle] = p.Title
	}
	if p.Description != "" {
		payload[events.PayloadKeyDescription] = p.Description
	}
	if p.DeliverableDirectory != "" {
		payload[events.PayloadKeyDeliverableDirectory] = p.DeliverableDirectory
	}
	return payload
}

func (p RunPayload) PayloadMap() map[string]any {
	payload := map[string]any{
		events.PayloadKeyState:              p.State,
		events.PayloadKeyBlockingTicketID:   p.BlockingTicketID,
		events.PayloadKeyBlockingApprovalID: p.BlockingApprovalID,
	}
	if p.GoalID != "" {
		payload[events.PayloadKeyGoalID] = p.GoalID
	}
	if p.DoDID != "" {
		payload[events.PayloadKeyDoDID] = p.DoDID
	}
	if p.FailureReason != "" {
		payload[events.PayloadKeyFailureReason] = p.FailureReason
	}
	return payload
}

func (p DecisionTicketPayload) PayloadMap() map[string]any {
	payload := map[string]any{
		events.PayloadKeyReason: p.Reason,
		events.PayloadKeyState:  p.State,
	}
	if p.ResolutionID != "" {
		payload[events.PayloadKeyResolutionID] = p.ResolutionID
	}
	if p.ResolutionType != "" {
		payload[events.PayloadKeyType] = p.ResolutionType
	}
	if len(p.ResolutionPayload.Fields) > 0 {
		payload[events.PayloadKeyPayload] = p.ResolutionPayload.Map()
	}
	return payload
}

func (p ApprovalPayload) PayloadMap() map[string]any {
	payload := map[string]any{
		events.PayloadKeyReason: p.Reason,
		events.PayloadKeyState:  p.State,
	}
	if p.Action != "" {
		payload["action"] = p.Action
	}
	if len(p.Context.Fields) > 0 {
		payload["context"] = p.Context.Map()
	}
	return payload
}

func snapshotPayloadMap(payload SnapshotPayload) map[string]any {
	if payload == nil {
		return nil
	}
	return payload.PayloadMap()
}

func cloneSnapshotMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = cloneSnapshotValue(v)
	}
	return out
}

func cloneSnapshotSlice(input []any) []any {
	if input == nil {
		return nil
	}
	out := make([]any, len(input))
	for i, v := range input {
		out[i] = cloneSnapshotValue(v)
	}
	return out
}

func cloneSnapshotValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSnapshotMap(typed)
	case []any:
		return cloneSnapshotSlice(typed)
	default:
		return typed
	}
}

func cloneMap(input map[string]any) map[string]any {
	return cloneSnapshotMap(input)
}
