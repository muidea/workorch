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

type LLMStreamProgressEvent struct {
	envelope Envelope
	Data     LLMStreamProgressPayloadData
}

type LLMUsageEvent struct {
	envelope Envelope
	Data     LLMUsagePayloadData
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

type PatchEvent struct {
	envelope Envelope
	Data     PatchPayloadData
}

type ArtifactContinuationEvent struct {
	envelope Envelope
	Data     map[string]any
}

type MemoryEvent struct {
	envelope Envelope
	Data     MemoryPayloadData
}

type MemoryRecallEvent struct {
	envelope Envelope
	Data     MemoryRecallPayloadData
}

type AutoGovernanceEvent struct {
	envelope Envelope
	Data     AutoGovernancePayloadData
}

var workorchTopics = []string{
	TopicWorkspaceCreated,
	TopicWorkspaceStateChanged,
	TopicWorkspaceDeleted,
	TopicGoalDefined,
	TopicDoDDefined,
	TopicRunCreated,
	TopicRunStateChanged,
	TopicRunLLMStreamProgress,
	TopicRunLLMUsageRecorded,
	TopicStepDispatched,
	TopicStepStateChanged,
	TopicTicketCreated,
	TopicTicketResolved,
	TopicApprovalCreated,
	TopicApprovalResolved,
	TopicResultAccepted,
	TopicResultRejected,
	TopicResultSuperseded,
	TopicArtifactCreated,
	TopicPatchPreviewed,
	TopicPatchApplied,
	TopicPatchFailed,
	TopicPatchReverted,
	TopicArtifactContinuationCreated,
	TopicArtifactContinuationChunkRecorded,
	TopicArtifactContinuationChunkRejected,
	TopicArtifactContinuationAssembled,
	TopicArtifactContinuationApplied,
	TopicArtifactContinuationFailed,
	TopicMemoryCreated,
	TopicMemoryUpdated,
	TopicMemoryDeleted,
	TopicMemoryRecallRecorded,
	TopicGovernanceAutoEvaluated,
	TopicGovernanceAutoResolved,
	TopicGovernanceAutoEscalated,
	TopicGovernanceAutoBlocked,
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
	case TopicRunLLMStreamProgress:
		event, ok := DecodeLLMStreamProgressEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicRunLLMUsageRecorded:
		event, ok := DecodeLLMUsageEvent(envelope)
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
	case TopicResultAccepted, TopicResultRejected, TopicResultSuperseded:
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
	case TopicPatchPreviewed, TopicPatchApplied, TopicPatchFailed, TopicPatchReverted:
		event, ok := DecodePatchEvent(envelope)
		if !ok {
			return nil, false
		}
		return event, true
	case TopicArtifactContinuationCreated,
		TopicArtifactContinuationChunkRecorded,
		TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled,
		TopicArtifactContinuationApplied,
		TopicArtifactContinuationFailed:
		event, ok := DecodeArtifactContinuationEvent(envelope)
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
	case TopicGovernanceAutoEvaluated, TopicGovernanceAutoResolved, TopicGovernanceAutoEscalated, TopicGovernanceAutoBlocked:
		event, ok := DecodeAutoGovernanceEvent(envelope)
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
	envelope.Payload = WorkspacePayload(data.Title, data.Description, data.DeliverableDirectory, data.InternalDirectory, data.State, data.ActiveRunID)
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

func DecodeLLMStreamProgressEvent(envelope Envelope) (LLMStreamProgressEvent, bool) {
	if envelope.Topic != TopicRunLLMStreamProgress {
		return LLMStreamProgressEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return LLMStreamProgressEvent{}, false
	}
	data, ok := DecodeLLMStreamProgressPayload(envelope.Payload)
	if !ok {
		return LLMStreamProgressEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = LLMStreamProgressPayload(
		data.WorkspaceID,
		data.RunID,
		data.StepID,
		data.ProviderID,
		data.Model,
		data.ChunkCount,
		data.ReceivedBytes,
		data.ElapsedMs,
		data.Status,
		data.Summary,
	)
	return LLMStreamProgressEvent{envelope: envelope, Data: data}, true
}

func DecodeLLMUsageEvent(envelope Envelope) (LLMUsageEvent, bool) {
	if envelope.Topic != TopicRunLLMUsageRecorded {
		return LLMUsageEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return LLMUsageEvent{}, false
	}
	data, ok := DecodeLLMUsagePayload(envelope.Payload)
	if !ok {
		return LLMUsageEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = LLMUsagePayload(data)
	return LLMUsageEvent{envelope: envelope, Data: data}, true
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
	envelope.Payload = GoalPayload(data.Text, data.DefinitionRevision, data.PreviousDefinitionHash, data.PreviousGoalID, data.LineageState)
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
	envelope.Payload = DoDPayload(data.Items, data.State, data.DefinitionRevision, data.PreviousDefinitionHash, data.PreviousDoDID, data.LineageState)
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
	if !isResultTopic(envelope.Topic) {
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

func isResultTopic(topic string) bool {
	switch topic {
	case TopicResultAccepted, TopicResultRejected, TopicResultSuperseded:
		return true
	default:
		return false
	}
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

func DecodePatchEvent(envelope Envelope) (PatchEvent, bool) {
	if envelope.Topic != TopicPatchPreviewed && envelope.Topic != TopicPatchApplied && envelope.Topic != TopicPatchFailed && envelope.Topic != TopicPatchReverted {
		return PatchEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return PatchEvent{}, false
	}
	data, ok := DecodePatchPayload(envelope.Payload)
	if !ok {
		return PatchEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = PatchPayload(data.PatchID, data.StepID, data.ToolCallID, data.PatchType, data.TargetRef, data.Summary, data.State, data.AddedLines, data.DeletedLines, data.ContentHash, data.DiffHash, data.ApprovalID, data.Risk, data.Error)
	return PatchEvent{envelope: envelope, Data: data}, true
}

func DecodeArtifactContinuationEvent(envelope Envelope) (ArtifactContinuationEvent, bool) {
	switch envelope.Topic {
	case TopicArtifactContinuationCreated,
		TopicArtifactContinuationChunkRecorded,
		TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled,
		TopicArtifactContinuationApplied,
		TopicArtifactContinuationFailed:
	default:
		return ArtifactContinuationEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return ArtifactContinuationEvent{}, false
	}
	if envelope.Payload == nil {
		return ArtifactContinuationEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	return ArtifactContinuationEvent{envelope: envelope, Data: cloneStringAnyMap(envelope.Payload)}, true
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

func DecodeAutoGovernanceEvent(envelope Envelope) (AutoGovernanceEvent, bool) {
	switch envelope.Topic {
	case TopicGovernanceAutoEvaluated, TopicGovernanceAutoResolved, TopicGovernanceAutoEscalated, TopicGovernanceAutoBlocked:
	default:
		return AutoGovernanceEvent{}, false
	}
	spec, ok := WorkorchEventContract(envelope.Topic)
	if !ok || !ValidateWorkorchEventVersion(envelope.Topic, envelope.Version) {
		return AutoGovernanceEvent{}, false
	}
	data, ok := DecodeAutoGovernancePayload(envelope.Payload)
	if !ok {
		return AutoGovernanceEvent{}, false
	}
	envelope = cloneEnvelope(envelope)
	envelope.Version = spec.CurrentVersion
	envelope.Payload = AutoGovernancePayload(
		data.TraceID,
		data.ItemType,
		data.ItemID,
		data.Action,
		data.RiskLevel,
		data.Confidence,
		data.PolicyMatched,
		data.PolicyBlocked,
		data.BudgetBefore,
		data.BudgetAfter,
		data.LLMProviderID,
		data.LLMModel,
		data.LLMDecisionPreview,
		data.PromptFingerprint,
		data.Status,
		data.Reason,
		data.EscalationReason,
	)
	return AutoGovernanceEvent{envelope: envelope, Data: data}, true
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

func (e LLMStreamProgressEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e LLMStreamProgressEvent) Topic() string {
	return e.envelope.Topic
}

func (e LLMUsageEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e LLMUsageEvent) Topic() string {
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

func (e PatchEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e PatchEvent) Topic() string {
	return e.envelope.Topic
}

func (e ArtifactContinuationEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e ArtifactContinuationEvent) Topic() string {
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

func (e AutoGovernanceEvent) Envelope() Envelope {
	return cloneEnvelope(e.envelope)
}

func (e AutoGovernanceEvent) Topic() string {
	return e.envelope.Topic
}
