package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

// eventHubBackend extends the local backend with EventHub-driven progress
// subscriptions, replacing fixed-interval polling with reactive events.
type eventHubBackend struct {
	eventHub events.Hub
	evtImpl  *eventHubEventsBackend
}

// NewEventHub creates a Backend using EventHub for progress events.
func NewEventHub(eventHub events.Hub) Backend {
	b := &eventHubBackend{
		eventHub: eventHub,
	}
	b.evtImpl = &eventHubEventsBackend{parent: b}
	return b
}

// NewEmbeddedEventHub creates an EventHub-backed backend from the active
// framework runtime initiator. CLI commands are handled by the workorchd module.
func NewEmbeddedEventHub() (Backend, func(context.Context) error, error) {
	runtime, err := workorchdeps.ResolveRuntimeHelper("application/workorch embedded backend")
	if err != nil {
		return nil, nil, fmt.Errorf("get workorch runtime initiator: %w", err)
	}
	eventHub := runtime.EventHub()
	if eventHub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return NewEventHub(eventHub), func(context.Context) error { return nil }, nil
}

func (b *eventHubBackend) Workspace() WorkspaceBackend { return &eventHubWorkspaceBackend{parent: b} }
func (b *eventHubBackend) Goal() GoalBackend           { return &eventHubGoalBackend{parent: b} }
func (b *eventHubBackend) DoD() DoDBackend             { return &eventHubDoDBackend{parent: b} }
func (b *eventHubBackend) Run() RunBackend             { return &eventHubRunBackend{parent: b} }
func (b *eventHubBackend) Decision() DecisionBackend   { return &eventHubDecisionBackend{parent: b} }
func (b *eventHubBackend) Approval() ApprovalBackend   { return &eventHubApprovalBackend{parent: b} }
func (b *eventHubBackend) Soul() SoulBackend           { return &eventHubSoulBackend{parent: b} }
func (b *eventHubBackend) Skill() SkillBackend         { return &eventHubSkillBackend{parent: b} }
func (b *eventHubBackend) MCP() MCPBackend             { return &eventHubMCPBackend{parent: b} }
func (b *eventHubBackend) Memory() MemoryBackend       { return &eventHubMemoryBackend{parent: b} }
func (b *eventHubBackend) Template() TemplateBackend   { return &eventHubTemplateBackend{parent: b} }
func (b *eventHubBackend) Config() ConfigBackend       { return &eventHubConfigBackend{parent: b} }

// Events overrides the stub to provide EventHub-driven subscriptions.
func (b *eventHubBackend) Events() EventsBackend { return b.evtImpl }

func (b *eventHubBackend) sendCommand(ctx context.Context, command string, wsID common.WorkspaceID, runID common.RunID, subjectType string, subjectID string, payload map[string]any) (events.CLIResultPayload, error) {
	if b == nil || b.eventHub == nil {
		return events.CLIResultPayload{}, common.ErrInvalidArgument
	}
	commandID := fmt.Sprintf("cmd-%d", time.Now().UTC().UnixNano())
	correlationID := fmt.Sprintf("corr-%d", time.Now().UTC().UnixNano())
	resultCh := make(chan events.CLIResultPayload, 1)

	unsubscribe := b.eventHub.Subscribe(workorchdevents.TopicCLIResult, func(_ context.Context, envelope events.Envelope) error {
		result, ok := events.DecodeCLIResultPayload(envelope.Payload)
		if !ok || result.CorrelationID != correlationID {
			return nil
		}
		select {
		case resultCh <- result:
		default:
		}
		return nil
	})
	defer unsubscribe()

	cmd := events.CLICommandPayload{
		CommandID:     commandID,
		CorrelationID: correlationID,
		Actor:         "workorch-cli",
		IssuedAt:      time.Now().UTC(),
		WorkspaceID:   wsID,
		RunID:         runID,
		SubjectType:   subjectType,
		SubjectID:     subjectID,
		Command:       command,
		Payload:       payload,
	}
	commandEnvelope := events.Envelope{
		Topic:        workorchdevents.TopicCLICommand,
		SourceModule: "workorch-cli",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(wsID),
		RunID:        string(runID),
		Payload:      events.CLICommandPayloadMap(cmd),
	}
	if err := b.eventHub.Publish(ctx, commandEnvelope); err != nil {
		if responseEnvelope, ok := events.ResponseEnvelope(err); ok {
			result, ok := events.DecodeCLIResultPayload(responseEnvelope.Payload)
			if !ok || result.CorrelationID != correlationID {
				return events.CLIResultPayload{}, common.ErrInvalidArgument
			}
			if !result.OK {
				return result, fmt.Errorf("%s", result.Error)
			}
			return result, nil
		}
		return events.CLIResultPayload{}, err
	}

	select {
	case <-ctx.Done():
		return events.CLIResultPayload{}, ctx.Err()
	case result := <-resultCh:
		if !result.OK {
			return result, fmt.Errorf("%s", result.Error)
		}
		return result, nil
	}
}

// ── EventHub Events Backend ──

type eventHubEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

func (b *eventHubEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	unsubscribers := []func(){
		b.parent.eventHub.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(ctx context.Context, envelope events.Envelope) error {
			evt := convertEnvelope(envelope)
			if evt.Type == "" {
				return nil
			}
			handler(ctx, evt)
			return nil
		}),
	}
	for _, topic := range events.WorkorchTopics() {
		topic := topic
		unsubscribers = append(unsubscribers, b.parent.eventHub.Subscribe(topic, func(ctx context.Context, envelope events.Envelope) error {
			evt := convertEnvelope(envelope)
			if evt.Type == "" {
				return nil
			}
			handler(ctx, evt)
			return nil
		}))
	}

	unsub := func() {
		for _, unsubscribe := range unsubscribers {
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}
	b.subs = append(b.subs, unsub)
	return unsub, nil
}

func convertEnvelope(envelope events.Envelope) (ret Event) {
	defer func() {
		if ret.Type != "" {
			ret.ID = envelope.ID
			ret.Revision = envelope.Revision
		}
	}()
	switch envelope.Topic {
	case workorchdevents.TopicWorkspaceProjectionChanged:
		return Event{Type: EventWorkspaceProjectionChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceCreated:
		return Event{Type: EventWorkspaceCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceStateChanged:
		return Event{Type: EventWorkspaceStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceDeleted:
		return Event{Type: EventWorkspaceDeleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicGoalDefined:
		return Event{Type: EventGoalDefined, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: enrichSubjectPayload(envelope.Payload, "goalId", envelope.SubjectID)}
	case workspaceevents.TopicDoDDefined:
		return Event{Type: EventDoDDefined, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: enrichSubjectPayload(envelope.Payload, "doDId", envelope.SubjectID)}
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		return Event{Type: EventRunStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case runevents.TopicStepDispatched:
		return Event{Type: EventStepDispatched, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case runevents.TopicStepStateChanged:
		return Event{Type: EventStepCompleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case decisionevents.TopicTicketCreated:
		return Event{Type: EventDecisionCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "ticketId", envelope.SubjectID)}
	case decisionevents.TopicTicketResolved:
		return Event{Type: EventDecisionResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "ticketId", envelope.SubjectID)}
	case approvalevents.TopicApprovalCreated:
		return Event{Type: EventApprovalCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "approvalId", envelope.SubjectID)}
	case approvalevents.TopicApprovalResolved:
		return Event{Type: EventApprovalResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "approvalId", envelope.SubjectID)}
	case resultevents.TopicResultAccepted, resultevents.TopicResultRejected, resultevents.TopicResultSuperseded:
		return Event{Type: EventResultAccepted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case memoryevents.TopicMemoryCreated:
		return Event{Type: EventMemoryCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryUpdated:
		return Event{Type: EventMemoryUpdated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryDeleted:
		return Event{Type: EventMemoryDeleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryRecallRecorded:
		return Event{Type: EventMemoryRecallRecorded, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case runevents.TopicRunLLMUsageRecorded:
		return Event{Type: EventLLMUsageRecorded, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case runevents.TopicRunLLMStreamProgress:
		return Event{Type: EventLLMStreamProgress, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case orchestratorevents.TopicPatchPreviewed:
		return Event{Type: EventPatchPreviewed, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "patchId", envelope.SubjectID)}
	case orchestratorevents.TopicPatchApplied:
		return Event{Type: EventPatchApplied, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "patchId", envelope.SubjectID)}
	case orchestratorevents.TopicPatchFailed:
		return Event{Type: EventPatchFailed, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "patchId", envelope.SubjectID)}
	case orchestratorevents.TopicPatchReverted:
		return Event{Type: EventPatchReverted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "patchId", envelope.SubjectID)}
	case orchestratorevents.TopicArtifactContinuationCreated,
		orchestratorevents.TopicArtifactContinuationChunkRecorded,
		orchestratorevents.TopicArtifactContinuationChunkRejected,
		orchestratorevents.TopicArtifactContinuationAssembled,
		orchestratorevents.TopicArtifactContinuationApplied,
		orchestratorevents.TopicArtifactContinuationFailed:
		return Event{Type: EventArtifactContinuationChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "artifactContinuationId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoEvaluated:
		return Event{Type: EventGovernanceEvaluated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoResolved:
		return Event{Type: EventGovernanceResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoEscalated:
		return Event{Type: EventGovernanceEscalated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoBlocked:
		return Event{Type: EventGovernanceBlocked, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	default:
		return Event{}
	}
}

func enrichSubjectPayload(payload map[string]any, key string, subjectID string) map[string]any {
	if subjectID == "" {
		return payload
	}
	if value, ok := payload[key].(string); ok && value != "" {
		return payload
	}
	next := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		next[k] = v
	}
	next[key] = subjectID
	return next
}

// ── EventHub command backends ──

type eventHubWorkspaceBackend struct{ parent *eventHubBackend }

func (b *eventHubWorkspaceBackend) Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandWorkspaceCreate, "", "", "workspace", "", map[string]any{
		"title":                req.Title,
		"description":          req.Description,
		"deliverableDirectory": req.DeliverableDirectory,
		"templateId":           req.TemplateID,
	})
	return decodeResultResource[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandWorkspaceGet, "", "", "workspace", string(id), nil)
	return decodeResultResource[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandWorkspaceList, "", "", "workspace", "", nil)
	return decodeResultResources[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandWorkspaceSnapshot, id, "", "workspace", string(id), nil)
	return decodeResultResource[WorkspaceSnapshotInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	_, err := b.parent.sendCommand(ctx, events.CommandWorkspaceDelete, "", "", "workspace", string(id), map[string]any{"purge": purge})
	return err
}

type eventHubGoalBackend struct{ parent *eventHubBackend }

func (b *eventHubGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandGoalDefine, wsID, "", "goal", "", map[string]any{"text": text})
	return decodeResultResource[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandGoalList, wsID, "", "goal", "", nil)
	return decodeResultResources[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandGoalGet, "", "", "goal", string(id), nil)
	return decodeResultResource[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandGoalUpdate, "", "", "goal", string(id), map[string]any{"text": text})
	return decodeResultResource[GoalInfo](result, err)
}

type eventHubDoDBackend struct{ parent *eventHubBackend }

func (b *eventHubDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDoDDefine, wsID, "", "dod", "", map[string]any{"items": items})
	return decodeResultResource[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDoDList, wsID, "", "dod", "", nil)
	return decodeResultResources[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDoDGet, "", "", "dod", string(id), nil)
	return decodeResultResource[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDoDUpdate, "", "", "dod", string(id), map[string]any{"items": items})
	return decodeResultResource[DoDInfo](result, err)
}

type eventHubRunBackend struct{ parent *eventHubBackend }

func (b *eventHubRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandRunActivate, wsID, "", "run", "", map[string]any{
		"goalId":                string(req.GoalID),
		"doDId":                 string(req.DoDID),
		"dialogLanguage":        req.DialogLanguage,
		"soulId":                string(req.SoulID),
		"skillIds":              skillIDStrings(req.SkillIDs),
		"mcpServerIds":          mcpServerIDStrings(req.MCPServerIDs),
		"priority":              req.Priority,
		"executionContextPatch": req.ExecutionContextPatch,
	})
	return decodeResultResource[RunInfo](result, err)
}

func (b *eventHubRunBackend) Restart(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	if b == nil || b.parent == nil || b.parent.eventHub == nil || wsID == "" {
		return nil, common.ErrInvalidArgument
	}
	envelope := events.Envelope{
		Topic:        workorchdevents.TopicWorkorchDCommandWorkspaceRestartRequested,
		SourceModule: "workorch-cli",
		SubjectType:  "workspace-restart",
		SubjectID:    string(wsID),
		WorkspaceID:  string(wsID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:           string(wsID),
			events.PayloadKeyGoalID:                string(req.GoalID),
			events.PayloadKeyDoDID:                 string(req.DoDID),
			events.PayloadKeyLLMProviderID:         req.LLMProviderID,
			events.PayloadKeySoulID:                string(req.SoulID),
			"dialogLanguage":                       req.DialogLanguage,
			events.PayloadKeyPriority:              req.Priority,
			events.PayloadKeyExecutionContextPatch: req.ExecutionContextPatch,
		},
	}
	if err := b.parent.eventHub.Publish(ctx, envelope); err != nil {
		if _, ok := events.ResponseEnvelope(err); ok {
			return nil, nil
		}
		return nil, err
	}
	return nil, nil
}

func (b *eventHubRunBackend) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, executionContextPatch ...map[string]any) (*RunInfo, error) {
	var payload map[string]any
	if len(executionContextPatch) > 0 && len(executionContextPatch[0]) > 0 {
		payload = map[string]any{"executionContextPatch": executionContextPatch[0]}
	}
	result, err := b.parent.sendCommand(ctx, events.CommandRunResume, wsID, id, "run", string(id), payload)
	return decodeResultResource[RunInfo](result, err)
}
func (b *eventHubRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandRunGet, wsID, id, "run", string(id), nil)
	return decodeResultResource[RunInfo](result, err)
}
func (b *eventHubRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandRunList, wsID, "", "run", "", nil)
	return decodeResultResources[RunInfo](result, err)
}
func (b *eventHubRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandRunStatus, wsID, id, "run", string(id), nil)
	return decodeResultResource[RunStatusInfo](result, err)
}

type eventHubDecisionBackend struct{ parent *eventHubBackend }

func (b *eventHubDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDecisionResolve, req.WorkspaceID, "", "decision", string(req.TicketID), map[string]any{"resolutionType": req.ResolutionType, "payload": req.Payload})
	return decodeResultResource[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDecisionBatch, wsID, "", "decision", "", map[string]any{"ticketIds": ticketIDStrings(ticketIDs), "resolutionType": resolutionType, "payload": payload})
	return decodeResultResources[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDecisionList, wsID, "", "decision", "", nil)
	return decodeResultResources[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandDecisionGet, wsID, "", "decision", string(id), nil)
	return decodeResultResource[DecisionInfo](result, err)
}

type eventHubApprovalBackend struct{ parent *eventHubBackend }

func (b *eventHubApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandApprovalApprove, wsID, "", "approval", string(id), nil)
	return decodeResultResource[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandApprovalReject, wsID, "", "approval", string(id), nil)
	return decodeResultResource[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string) ([]ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandApprovalBatch, wsID, "", "approval", "", map[string]any{"approvalIds": approvalIDStrings(ids), "action": action})
	return decodeResultResources[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandApprovalList, wsID, "", "approval", "", nil)
	return decodeResultResources[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandApprovalGet, wsID, "", "approval", string(id), nil)
	return decodeResultResource[ApprovalInfo](result, err)
}

type eventHubSoulBackend struct{ parent *eventHubBackend }

func (b *eventHubSoulBackend) Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSoulCreate, "", "", "soul", string(s.ID), resourceMap(s))
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSoulGet, "", "", "soul", string(id), nil)
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) List(ctx context.Context) ([]SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSoulList, "", "", "soul", "", nil)
	return decodeResultResources[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSoulUpdate, "", "", "soul", string(id), resourceMap(s))
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Delete(ctx context.Context, id common.SoulID) error {
	_, err := b.parent.sendCommand(ctx, events.CommandSoulDelete, "", "", "soul", string(id), nil)
	return err
}

type eventHubSkillBackend struct{ parent *eventHubBackend }

func (b *eventHubSkillBackend) Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSkillCreate, "", "", "skill", string(s.ID), resourceMap(s))
	return decodeResultResource[SkillInfo](result, err)
}
func (b *eventHubSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSkillGet, "", "", "skill", string(id), nil)
	return decodeResultResource[SkillInfo](result, err)
}
func (b *eventHubSkillBackend) List(ctx context.Context) ([]SkillInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandSkillList, "", "", "skill", "", nil)
	return decodeResultResources[SkillInfo](result, err)
}

type eventHubMCPBackend struct{ parent *eventHubBackend }

func (b *eventHubMCPBackend) CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMCPServerCreate, "", "", "mcp-server", string(s.ID), resourceMap(s))
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMCPServerGet, "", "", "mcp-server", string(id), nil)
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMCPServerList, "", "", "mcp-server", "", nil)
	return decodeResultResources[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMCPServerUpdate, "", "", "mcp-server", string(id), resourceMap(s))
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMCPCapabilityList, "", "", "mcp-capability", "", nil)
	return decodeResultResources[MCPCapabilityInfo](result, err)
}

type eventHubMemoryBackend struct{ parent *eventHubBackend }

func (b *eventHubMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMemoryCreate, wsID, "", "memory", string(m.ID), resourceMap(m))
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMemoryGet, wsID, "", "memory", string(id), nil)
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMemoryUpdate, wsID, "", "memory", string(id), resourceMap(m))
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	_, err := b.parent.sendCommand(ctx, events.CommandMemoryDelete, wsID, "", "memory", string(id), nil)
	return err
}
func (b *eventHubMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMemoryList, wsID, "", "memory", "", nil)
	return decodeResultResources[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandMemorySearch, wsID, "", "memory", "", resourceMap(req))
	return decodeResultResources[MemoryItemInfo](result, err)
}

type eventHubTemplateBackend struct{ parent *eventHubBackend }

func (b *eventHubTemplateBackend) Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandTemplateCreate, "", "", "template", t.ID, resourceMap(t))
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandTemplateGet, "", "", "template", id, nil)
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandTemplateList, "", "", "template", "", nil)
	return decodeResultResources[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandTemplateUpdate, "", "", "template", id, resourceMap(t))
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Delete(ctx context.Context, id string) error {
	_, err := b.parent.sendCommand(ctx, events.CommandTemplateDelete, "", "", "template", id, nil)
	return err
}

type eventHubConfigBackend struct{ parent *eventHubBackend }

func (b *eventHubConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	result, err := b.parent.sendCommand(ctx, events.CommandConfigurationGet, "", "", "configuration", "", nil)
	return decodeResultResource[ServiceConfig](result, err)
}

func resourceMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case RunInfo:
		return runInfoResourceMap(typed)
	case *RunInfo:
		if typed == nil {
			return nil
		}
		return runInfoResourceMap(*typed)
	case WorkspaceSnapshotInfo:
		return workspaceSnapshotResourceMap(typed)
	case *WorkspaceSnapshotInfo:
		if typed == nil {
			return nil
		}
		return workspaceSnapshotResourceMap(*typed)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func runInfoResourceMap(run RunInfo) map[string]any {
	return map[string]any{
		"ID":                 string(run.ID),
		"WorkspaceID":        string(run.WorkspaceID),
		"GoalID":             string(run.GoalID),
		"DoDID":              string(run.DoDID),
		"ExecutionContext":   compactRunExecutionContext(run.ExecutionContext),
		"State":              string(run.State),
		"Priority":           run.Priority,
		"BlockingTicketID":   string(run.BlockingTicketID),
		"BlockingApprovalID": string(run.BlockingApprovalID),
		"FailureReason":      run.FailureReason,
	}
}

func workspaceSnapshotResourceMap(snapshot WorkspaceSnapshotInfo) map[string]any {
	runs := make([]map[string]any, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs = append(runs, runInfoResourceMap(run))
	}
	eventRecords := make([]map[string]any, 0, len(snapshot.EventRecords))
	for _, record := range snapshot.EventRecords {
		eventRecords = append(eventRecords, map[string]any{
			"ID":             string(record.ID),
			"Version":        record.Version,
			"Topic":          record.Topic,
			"OccurredAt":     record.OccurredAt,
			"SourceModule":   record.SourceModule,
			"SubjectType":    record.SubjectType,
			"SubjectID":      record.SubjectID,
			"WorkspaceID":    string(record.WorkspaceID),
			"RunID":          string(record.RunID),
			"StepID":         string(record.StepID),
			"PayloadSummary": cloneCLIResultValue(record.PayloadSummary, 0),
			"PayloadRef":     record.PayloadRef,
		})
	}
	return map[string]any{
		"Workspace":             snapshot.Workspace,
		"Projection":            snapshot.Projection,
		"Outcome":               snapshot.Outcome,
		"TaskProgress":          snapshot.TaskProgress,
		"RunHeartbeat":          snapshot.RunHeartbeat,
		"RuntimeExecution":      snapshot.RuntimeExecution,
		"CoverageSummary":       cloneCLIResultValue(snapshot.CoverageSummary, 0),
		"TaskDecompositions":    snapshot.TaskDecompositions,
		"Goals":                 snapshot.Goals,
		"DoDs":                  snapshot.DoDs,
		"Runs":                  runs,
		"Results":               snapshot.Results,
		"Artifacts":             snapshot.Artifacts,
		"ArtifactContinuations": snapshot.ArtifactContinuations,
		"Patches":               snapshot.Patches,
		"MemoryRecallTraces":    snapshot.MemoryRecallTraces,
		"WebSources":            snapshot.WebSources,
		"LLMUsageTraces":        snapshot.LLMUsageTraces,
		"LLMUsageSummary":       snapshot.LLMUsageSummary,
		"EventRecords":          eventRecords,
		"Decisions":             snapshot.Decisions,
		"Approvals":             snapshot.Approvals,
	}
}

func resourceMapAlias(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func compactRunExecutionContext(context map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"goal",
		"goals",
		"dod",
		"dods",
		"operatorInputQueue",
		"latestOperatorInput",
	} {
		if value, ok := context[key]; ok {
			ret[key] = cloneCLIResultValue(value, 0)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func cloneCLIResultValue(value any, depth int) any {
	if depth >= 6 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for k, v := range typed {
			ret[k] = cloneCLIResultValue(v, depth+1)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if cloned, ok := cloneCLIResultValue(item, depth+1).(map[string]any); ok {
				ret = append(ret, cloned)
			}
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneCLIResultValue(item, depth+1))
		}
		return ret
	default:
		return value
	}
}

func resourceMaps[T any](items []T) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, resourceMap(item))
	}
	return out
}

func decodeResultResource[T any](result events.CLIResultPayload, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	if result.Resource == nil {
		return nil, nil
	}
	data, err := json.Marshal(result.Resource)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func decodeResultResources[T any](result events.CLIResultPayload, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(result.Resources))
	for _, resource := range result.Resources {
		item, err := decodeResultResource[T](events.CLIResultPayload{Resource: resource}, nil)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}

func decodeOrZero[T any](payload map[string]any) *T {
	data, _ := json.Marshal(payload)
	var out T
	_ = json.Unmarshal(data, &out)
	return &out
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func boolValue(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	if v, ok := payload[key].(bool); ok {
		return v
	}
	return false
}

func intValue(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func mapValue(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	if v, ok := payload[key].(map[string]any); ok {
		return v
	}
	return nil
}

func stringSliceValue(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func ticketIDsValue(payload map[string]any, key string) []common.TicketID {
	values := stringSliceValue(payload, key)
	out := make([]common.TicketID, 0, len(values))
	for _, value := range values {
		out = append(out, common.TicketID(value))
	}
	return out
}

func approvalIDsValue(payload map[string]any, key string) []common.ApprovalID {
	values := stringSliceValue(payload, key)
	out := make([]common.ApprovalID, 0, len(values))
	for _, value := range values {
		out = append(out, common.ApprovalID(value))
	}
	return out
}

func skillIDsValue(payload map[string]any, key string) []common.SkillID {
	values := stringSliceValue(payload, key)
	out := make([]common.SkillID, 0, len(values))
	for _, value := range values {
		out = append(out, common.SkillID(value))
	}
	return out
}

func mcpServerIDsValue(payload map[string]any, key string) []common.MCPServerID {
	values := stringSliceValue(payload, key)
	out := make([]common.MCPServerID, 0, len(values))
	for _, value := range values {
		out = append(out, common.MCPServerID(value))
	}
	return out
}

func ticketIDStrings(ids []common.TicketID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func approvalIDStrings(ids []common.ApprovalID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func skillIDStrings(ids []common.SkillID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func mcpServerIDStrings(ids []common.MCPServerID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}
