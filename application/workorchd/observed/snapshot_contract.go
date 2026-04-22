package observed

import "github.com/muidea/workorch/internal/pkg/events"

type SnapshotPayload interface {
	EnvelopePayload() map[string]any
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
	Title       string
	State       string
	ActiveRunID string
}

type RunPayload struct {
	State              string
	BlockingTicketID   string
	BlockingApprovalID string
}

type StepPayload struct {
	Kind   string
	State  string
	Input  map[string]any
	Output map[string]any
}

type DecisionTicketPayload struct {
	Reason            string
	State             string
	ResolutionID      string
	ResolutionType    string
	ResolutionPayload map[string]any
}

type ApprovalPayload struct {
	Reason string
	State  string
}

func FromEnvelope(envelope events.Envelope) (SnapshotItem, bool) {
	spec, ok := events.Phase1SnapshotContract(envelope.Topic)
	if !ok || !events.ValidatePhase1SnapshotVersion(envelope.Topic, envelope.Version) {
		return SnapshotItem{}, false
	}
	payload, ok := decodeSnapshotPayload(envelope.Topic, envelope.Payload)
	if !ok {
		return SnapshotItem{}, false
	}
	return SnapshotItem{
		ID:           envelope.ID,
		Version:      spec.CurrentVersion,
		Topic:        envelope.Topic,
		OccurredAt:   envelope.OccurredAt,
		SourceModule: envelope.SourceModule,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      payload,
	}, true
}

func (i SnapshotItem) Envelope() events.Envelope {
	return events.Envelope{
		ID:           i.ID,
		Version:      events.Phase1SnapshotVersion,
		Topic:        i.Topic,
		OccurredAt:   i.OccurredAt,
		SourceModule: i.SourceModule,
		SubjectType:  i.SubjectType,
		SubjectID:    i.SubjectID,
		WorkspaceID:  i.WorkspaceID,
		RunID:        i.RunID,
		StepID:       i.StepID,
		Payload:      envelopePayload(i.Payload),
	}
}

func (p WorkspacePayload) EnvelopePayload() map[string]any {
	payload := map[string]any{
		events.PayloadKeyState:       p.State,
		events.PayloadKeyActiveRunID: p.ActiveRunID,
	}
	if p.Title != "" {
		payload[events.PayloadKeyTitle] = p.Title
	}
	return payload
}

func (p RunPayload) EnvelopePayload() map[string]any {
	return map[string]any{
		events.PayloadKeyState:              p.State,
		events.PayloadKeyBlockingTicketID:   p.BlockingTicketID,
		events.PayloadKeyBlockingApprovalID: p.BlockingApprovalID,
	}
}

func (p StepPayload) EnvelopePayload() map[string]any {
	payload := map[string]any{
		events.PayloadKeyKind:  p.Kind,
		events.PayloadKeyState: p.State,
	}
	if p.Input != nil {
		payload[events.PayloadKeyInput] = cloneMap(p.Input)
	}
	if p.Output != nil {
		payload[events.PayloadKeyOutput] = cloneMap(p.Output)
	}
	return payload
}

func (p DecisionTicketPayload) EnvelopePayload() map[string]any {
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
	if p.ResolutionPayload != nil {
		payload[events.PayloadKeyPayload] = cloneMap(p.ResolutionPayload)
	}
	return payload
}

func (p ApprovalPayload) EnvelopePayload() map[string]any {
	return map[string]any{
		events.PayloadKeyReason: p.Reason,
		events.PayloadKeyState:  p.State,
	}
}

func decodeSnapshotPayload(topic string, payload map[string]any) (SnapshotPayload, bool) {
	switch topic {
	case topicWorkspaceSnapshot:
		item, ok := events.DecodeWorkspacePayload(payload)
		if !ok {
			return nil, false
		}
		return WorkspacePayload{
			Title:       item.Title,
			State:       string(item.State),
			ActiveRunID: string(item.ActiveRunID),
		}, true
	case topicRunSnapshot:
		item, ok := events.DecodeRunPayload(payload)
		if !ok {
			return nil, false
		}
		return RunPayload{
			State:              string(item.State),
			BlockingTicketID:   string(item.BlockingTicketID),
			BlockingApprovalID: string(item.BlockingApprovalID),
		}, true
	case topicStepSnapshot:
		item, ok := events.DecodeStepPayload(payload)
		if !ok {
			return nil, false
		}
		return StepPayload{
			Kind:   item.Kind,
			State:  string(item.State),
			Input:  cloneMap(item.Input),
			Output: cloneMap(item.Output),
		}, true
	case topicDecisionTicketSnapshot:
		if item, ok := events.DecodeDecisionResolutionPayload(payload); ok {
			return DecisionTicketPayload{
				Reason:            item.Reason,
				State:             string(item.State),
				ResolutionID:      string(item.ResolutionID),
				ResolutionType:    item.Type,
				ResolutionPayload: cloneMap(item.Payload),
			}, true
		}
		item, ok := events.DecodeDecisionTicketPayload(payload)
		if !ok {
			return nil, false
		}
		return DecisionTicketPayload{
			Reason: item.Reason,
			State:  string(item.State),
		}, true
	case topicApprovalSnapshot:
		item, ok := events.DecodeApprovalPayload(payload)
		if !ok {
			return nil, false
		}
		return ApprovalPayload{
			Reason: item.Reason,
			State:  string(item.State),
		}, true
	default:
		return nil, false
	}
}

func envelopePayload(payload SnapshotPayload) map[string]any {
	if payload == nil {
		return nil
	}
	return payload.EnvelopePayload()
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
