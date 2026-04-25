package events

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkorchEvent interface {
	Envelope() Envelope
	Topic() string
}

type WorkspaceEvent struct {
	envelope Envelope
	Data     WorkspacePayloadData
}

type RunEvent struct {
	envelope Envelope
	Data     RunPayloadData
}

type StepEvent struct {
	envelope Envelope
	Data     StepPayloadData
}

type DecisionTicketEvent struct {
	envelope          Envelope
	Data              DecisionTicketPayloadData
	ResolutionID      common.ResolutionID
	ResolutionType    string
	ResolutionPayload map[string]any
}

type ApprovalEvent struct {
	envelope Envelope
	Data     ApprovalPayloadData
}

var workorchTopics = []string{
	TopicWorkspaceCreated,
	TopicWorkspaceStateChanged,
	TopicRunCreated,
	TopicRunStateChanged,
	TopicStepDispatched,
	TopicStepStateChanged,
	TopicTicketCreated,
	TopicTicketResolved,
	TopicApprovalCreated,
	TopicApprovalResolved,
}

func WorkorchTopics() []string {
	return append([]string(nil), workorchTopics...)
}

func SubscribeWorkorch(hub Hub, handler func(context.Context, WorkorchEvent) error) []func() {
	if hub == nil || handler == nil {
		return nil
	}

	unsubscribers := make([]func(), 0, len(workorchTopics))
	for _, topic := range workorchTopics {
		unsubscribe := hub.Subscribe(topic, func(ctx context.Context, envelope Envelope) error {
			event, ok := DecodeWorkorchEvent(envelope)
			if !ok {
				return nil
			}
			return handler(ctx, event)
		})
		unsubscribers = append(unsubscribers, unsubscribe)
	}
	return unsubscribers
}

func NormalizeWorkorchEnvelope(envelope Envelope) (Envelope, bool) {
	event, ok := DecodeWorkorchEvent(envelope)
	if !ok {
		return Envelope{}, false
	}
	return event.Envelope(), true
}

func DecodeWorkorchEvent(envelope Envelope) (WorkorchEvent, bool) {
	switch envelope.Topic {
	case TopicWorkspaceCreated, TopicWorkspaceStateChanged:
		event, ok := DecodeWorkspaceEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicRunCreated, TopicRunStateChanged:
		event, ok := DecodeRunEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicStepDispatched, TopicStepStateChanged:
		event, ok := DecodeStepEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicTicketCreated, TopicTicketResolved:
		event, ok := DecodeDecisionTicketEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicApprovalCreated, TopicApprovalResolved:
		event, ok := DecodeApprovalEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	default:
		return nil, false
	}
}

func DecodeWorkspaceEvent(envelope Envelope) (WorkspaceEvent, bool) {
	if envelope.Topic != TopicWorkspaceCreated && envelope.Topic != TopicWorkspaceStateChanged {
		return WorkspaceEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return WorkspaceEvent{}, false
	}
	data, ok := DecodeWorkspacePayload(envelope.Payload)
	if !ok {
		return WorkspaceEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = WorkspacePayload(data.Title, data.Description, data.WorkingDirectory, data.State, data.ActiveRunID)
	return WorkspaceEvent{envelope: envelope, Data: data}, true
}

func DecodeRunEvent(envelope Envelope) (RunEvent, bool) {
	if envelope.Topic != TopicRunCreated && envelope.Topic != TopicRunStateChanged {
		return RunEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return RunEvent{}, false
	}
	data, ok := DecodeRunPayload(envelope.Payload)
	if !ok {
		return RunEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = RunPayload(
		data.GoalID,
		data.DoDID,
		data.State,
		data.BlockingTicketID,
		data.BlockingApprovalID,
		data.FailureReason,
	)
	return RunEvent{envelope: envelope, Data: data}, true
}

func DecodeStepEvent(envelope Envelope) (StepEvent, bool) {
	if envelope.Topic != TopicStepDispatched && envelope.Topic != TopicStepStateChanged {
		return StepEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return StepEvent{}, false
	}
	data, ok := DecodeStepPayload(envelope.Payload)
	if !ok {
		return StepEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = StepPayload(data.Kind, data.State, data.Input, data.Output)
	return StepEvent{envelope: envelope, Data: data}, true
}

func DecodeDecisionTicketEvent(envelope Envelope) (DecisionTicketEvent, bool) {
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return DecisionTicketEvent{}, false
	}
	if envelope.Topic == TopicTicketResolved {
		data, ok := DecodeDecisionResolutionPayload(envelope.Payload)
		if !ok {
			return DecisionTicketEvent{}, false
		}
		envelope = cloneEnvelope(envelope)
		envelope.Version = spec.CurrentVersion
		envelope.Payload = DecisionResolutionPayload(data.Reason, data.State, data.ResolutionID, data.Type, data.Payload)
		return DecisionTicketEvent{
			envelope:          envelope,
			Data:              data.DecisionTicketPayloadData,
			ResolutionID:      data.ResolutionID,
			ResolutionType:    data.Type,
			ResolutionPayload: clonePayloadMap(data.Payload),
		}, true
	}
	if envelope.Topic != TopicTicketCreated {
		return DecisionTicketEvent{}, false
	}
	data, ok := DecodeDecisionTicketPayload(envelope.Payload)
	if !ok {
		return DecisionTicketEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = DecisionTicketPayload(data.Reason, data.State)
	return DecisionTicketEvent{envelope: envelope, Data: data}, true
}

func DecodeApprovalEvent(envelope Envelope) (ApprovalEvent, bool) {
	if envelope.Topic != TopicApprovalCreated && envelope.Topic != TopicApprovalResolved {
		return ApprovalEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return ApprovalEvent{}, false
	}
	data, ok := DecodeApprovalPayload(envelope.Payload)
	if !ok {
		return ApprovalEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = ApprovalPayload(data.Reason, data.State)
	return ApprovalEvent{envelope: envelope, Data: data}, true
}

func (e WorkspaceEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e WorkspaceEvent) Topic() string {
	return e.envelope.Topic
}

func (e RunEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e RunEvent) Topic() string {
	return e.envelope.Topic
}

func (e StepEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e StepEvent) Topic() string {
	return e.envelope.Topic
}

func (e DecisionTicketEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e DecisionTicketEvent) Topic() string {
	return e.envelope.Topic
}

func (e ApprovalEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e ApprovalEvent) Topic() string {
	return e.envelope.Topic
}
