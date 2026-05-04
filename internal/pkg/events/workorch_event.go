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

type GoalEvent struct {
	envelope Envelope
	Data     GoalPayloadData
}

type DoDEvent struct {
	envelope Envelope
	Data     DoDPayloadData
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

type ResultEvent struct {
	envelope Envelope
	Data     ResultPayloadData
}

type ArtifactEvent struct {
	envelope Envelope
	Data     ArtifactPayloadData
}

type MemoryEvent struct {
	envelope Envelope
	Data     MemoryPayloadData
}

type MemoryRecallEvent struct {
	envelope Envelope
	Data     MemoryRecallPayloadData
}

var workorchTopics = []string{
	TopicWorkspaceCreated,
	TopicWorkspaceStateChanged,
	TopicWorkspaceDeleted,
	TopicGoalDefined,
	TopicDoDDefined,
	TopicRunCreated,
	TopicRunStateChanged,
	TopicStepDispatched,
	TopicStepStateChanged,
	TopicTicketCreated,
	TopicTicketResolved,
	TopicApprovalCreated,
	TopicApprovalResolved,
	TopicResultAccepted,
	TopicArtifactCreated,
	TopicMemoryCreated,
	TopicMemoryUpdated,
	TopicMemoryDeleted,
	TopicMemoryRecallRecorded,
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
	case TopicWorkspaceCreated, TopicWorkspaceStateChanged, TopicWorkspaceDeleted:
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
	case TopicGoalDefined:
		event, ok := DecodeGoalEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicDoDDefined:
		event, ok := DecodeDoDEvent(envelope)
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
	case TopicResultAccepted:
		event, ok := DecodeResultEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicArtifactCreated:
		event, ok := DecodeArtifactEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicMemoryCreated, TopicMemoryUpdated, TopicMemoryDeleted:
		event, ok := DecodeMemoryEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicMemoryRecallRecorded:
		event, ok := DecodeMemoryRecallEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	default:
		return nil, false
	}
}

func DecodeWorkspaceEvent(envelope Envelope) (WorkspaceEvent, bool) {
	if envelope.Topic != TopicWorkspaceCreated && envelope.Topic != TopicWorkspaceStateChanged && envelope.Topic != TopicWorkspaceDeleted {
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

func DecodeGoalEvent(envelope Envelope) (GoalEvent, bool) {
	if envelope.Topic != TopicGoalDefined {
		return GoalEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return GoalEvent{}, false
	}
	data, ok := DecodeGoalPayload(envelope.Payload)
	if !ok {
		return GoalEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = GoalPayload(data.Text)
	return GoalEvent{envelope: envelope, Data: data}, true
}

func DecodeDoDEvent(envelope Envelope) (DoDEvent, bool) {
	if envelope.Topic != TopicDoDDefined {
		return DoDEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return DoDEvent{}, false
	}
	data, ok := DecodeDoDPayload(envelope.Payload)
	if !ok {
		return DoDEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = DoDPayload(data.Items, data.State)
	return DoDEvent{envelope: envelope, Data: data}, true
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
	envelope.Payload = ApprovalPayload(data.Reason, data.Action, data.Context, data.State)
	return ApprovalEvent{envelope: envelope, Data: data}, true
}

func DecodeResultEvent(envelope Envelope) (ResultEvent, bool) {
	if envelope.Topic != TopicResultAccepted {
		return ResultEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return ResultEvent{}, false
	}
	data, ok := DecodeResultPayload(envelope.Payload)
	if !ok {
		return ResultEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = ResultPayload(data.GoalID, data.DoDID, data.State, data.Title, data.Summary, data.SourceType, data.SourceRef, data.EvidenceRefs, data.ArtifactRefs)
	return ResultEvent{envelope: envelope, Data: data}, true
}

func DecodeArtifactEvent(envelope Envelope) (ArtifactEvent, bool) {
	if envelope.Topic != TopicArtifactCreated {
		return ArtifactEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return ArtifactEvent{}, false
	}
	data, ok := DecodeArtifactPayload(envelope.Payload)
	if !ok {
		return ArtifactEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = ArtifactPayload(data.ResultID, data.StepID, data.ToolCallID, data.PatchID, data.ValidationResultID, data.ArtifactType, data.Name, data.Summary, data.URI, data.ContentType, data.State, data.EvidenceRefs)
	return ArtifactEvent{envelope: envelope, Data: data}, true
}

func DecodeMemoryEvent(envelope Envelope) (MemoryEvent, bool) {
	if envelope.Topic != TopicMemoryCreated && envelope.Topic != TopicMemoryUpdated && envelope.Topic != TopicMemoryDeleted {
		return MemoryEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return MemoryEvent{}, false
	}
	data, ok := DecodeMemoryPayload(envelope.Payload)
	if !ok {
		return MemoryEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = MemoryPayload(data.MemoryType, data.Summary, data.Content, data.SourceType, data.SourceRef, data.Tags, data.Importance, data.Confidence, data.Visibility)
	return MemoryEvent{envelope: envelope, Data: data}, true
}

func DecodeMemoryRecallEvent(envelope Envelope) (MemoryRecallEvent, bool) {
	if envelope.Topic != TopicMemoryRecallRecorded {
		return MemoryRecallEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return MemoryRecallEvent{}, false
	}
	data, ok := DecodeMemoryRecallPayload(envelope.Payload)
	if !ok {
		return MemoryRecallEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = MemoryRecallPayload(data.TraceID, data.Stage, data.Source, data.Policy, data.Status, data.ProviderID, data.Operation, data.RetrievedMemoryIDs, data.Reason)
	return MemoryRecallEvent{envelope: envelope, Data: data}, true
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

func (e GoalEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e GoalEvent) Topic() string {
	return e.envelope.Topic
}

func (e DoDEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e DoDEvent) Topic() string {
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

func (e ResultEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e ResultEvent) Topic() string {
	return e.envelope.Topic
}

func (e ArtifactEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e ArtifactEvent) Topic() string {
	return e.envelope.Topic
}

func (e MemoryEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e MemoryEvent) Topic() string {
	return e.envelope.Topic
}

func (e MemoryRecallEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e MemoryRecallEvent) Topic() string {
	return e.envelope.Topic
}
