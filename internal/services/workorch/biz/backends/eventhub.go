package backend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/eventvalue"
)

// eventHubBackend extends the local backend with EventHub-driven progress
// subscriptions, replacing fixed-interval polling with reactive events.
type eventHubBackend struct {
	commandHub magicevent.Hub
	evtImpl    *eventHubEventsBackend
}

// NewEventHub creates a Backend using EventHub for progress events.
func NewEventHub(commandHub magicevent.Hub) Backend {
	b := &eventHubBackend{
		commandHub: commandHub,
	}
	b.evtImpl = &eventHubEventsBackend{parent: b}
	return b
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

func (b *eventHubBackend) sendCommand(ctx context.Context, command workorchdevents.CLICommand) (workorchdevents.CLIResult, error) {
	if b == nil || b.commandHub == nil {
		return workorchdevents.CLIResult{}, common.ErrInvalidArgument
	}
	now := time.Now().UTC()
	if command.CommandID == "" {
		command.CommandID = fmt.Sprintf("cmd-%d", now.UnixNano())
	}
	if command.CorrelationID == "" {
		command.CorrelationID = fmt.Sprintf("corr-%d", now.UnixNano())
	}
	if command.Actor == "" {
		command.Actor = "workorch-cli"
	}
	if command.IssuedAt.IsZero() {
		command.IssuedAt = now
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLICommand, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(workorchdevents.TopicCLICommand, events.EventHubLaneScope{
		SubjectType: command.SubjectType,
		SubjectID:   command.SubjectID,
		WorkspaceID: string(command.WorkspaceID),
		RunID:       string(command.RunID),
	}))
	response := b.commandHub.Send(event)
	if response == nil {
		return workorchdevents.CLIResult{}, fmt.Errorf("%w: cli command %s returned no result", common.ErrConflict, event.ID())
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return workorchdevents.CLIResult{}, err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return workorchdevents.CLIResult{}, err
	}
	result, ok := data.(workorchdevents.CLIResult)
	if !ok || result.CommandID != command.CommandID || result.CorrelationID != command.CorrelationID {
		return workorchdevents.CLIResult{}, common.ErrInvalidArgument
	}
	if !result.OK {
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}

func newCLICommand(command string, workspaceID common.WorkspaceID, runID common.RunID, subjectType string, subjectID string) workorchdevents.CLICommand {
	return workorchdevents.CLICommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Command:     command,
	}
}

func cliDataFromMap(data map[string]any) workorchdevents.CLIData {
	if len(data) == 0 {
		return workorchdevents.CLIData{}
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]workorchdevents.CLIDataField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, workorchdevents.CLIDataField{Name: key, Value: cliDataValue(data[key])})
	}
	return workorchdevents.CLIData{Fields: fields}
}

func cliDataValue(value any) workorchdevents.CLIDataValue {
	switch typed := value.(type) {
	case nil:
		return workorchdevents.CLIDataValue{Kind: "null"}
	case string:
		return workorchdevents.CLIDataValue{Kind: "string", String: typed}
	case bool:
		return workorchdevents.CLIDataValue{Kind: "bool", Bool: typed}
	case int:
		return workorchdevents.CLIDataValue{Kind: "number", Number: float64(typed)}
	case int64:
		return workorchdevents.CLIDataValue{Kind: "number", Number: float64(typed)}
	case float32:
		return workorchdevents.CLIDataValue{Kind: "number", Number: float64(typed)}
	case float64:
		return workorchdevents.CLIDataValue{Kind: "number", Number: typed}
	case map[string]any:
		return workorchdevents.CLIDataValue{Kind: "object", Object: cliDataFromMap(typed)}
	case []any:
		items := make([]workorchdevents.CLIDataValue, 0, len(typed))
		for _, item := range typed {
			items = append(items, cliDataValue(item))
		}
		return workorchdevents.CLIDataValue{Kind: "array", Array: items}
	case []string:
		items := make([]workorchdevents.CLIDataValue, 0, len(typed))
		for _, item := range typed {
			items = append(items, cliDataValue(item))
		}
		return workorchdevents.CLIDataValue{Kind: "array", Array: items}
	case []map[string]any:
		items := make([]workorchdevents.CLIDataValue, 0, len(typed))
		for _, item := range typed {
			items = append(items, cliDataValue(item))
		}
		return workorchdevents.CLIDataValue{Kind: "array", Array: items}
	default:
		return workorchdevents.CLIDataValue{Kind: "string", String: fmt.Sprint(typed)}
	}
}

// ── EventHub Events Backend ──

type eventHubEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

var eventHubObserverSequence atomic.Uint64

func (b *eventHubEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	unsubscribers := make([]func(), 0, 2)
	if b.parent.commandHub != nil {
		projectionObserverID := fmt.Sprintf("/workorchd/consumers/services/workorch/backend/%d", eventHubObserverSequence.Add(1))
		projectionObserver := magicevent.NewSimpleObserverWithMatchID(projectionObserverID, projectionObserverID, b.parent.commandHub)
		projectionObserver.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
				handler(event.Context(), convertBackendEvent(workspaceProjectionBackendEvent(data)))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
		unsubscribers = append(unsubscribers, func() {
			projectionObserver.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged)
		})

		observerID := fmt.Sprintf("/eventstream/consumers/services/workorch/backend/%d", eventHubObserverSequence.Add(1))
		observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, b.parent.commandHub)
		observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
			if ok {
				evt := convertBackendEvent(eventRecordBackendEvent(data.Record))
				if evt.Type != "" {
					handler(event.Context(), evt)
				}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
		unsubscribers = append(unsubscribers, func() {
			observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended)
		})
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

type backendEventData struct {
	ID          string         `json:"id"`
	Topic       string         `json:"topic"`
	SubjectID   string         `json:"subjectId,omitempty"`
	WorkspaceID string         `json:"workspaceId,omitempty"`
	RunID       string         `json:"runId,omitempty"`
	Revision    int64          `json:"revision,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

func eventRecordBackendEvent(data eventstreamevents.EventRecordData) backendEventData {
	return backendEventData{
		ID:          string(data.ID),
		Topic:       data.Topic,
		SubjectID:   data.SubjectID,
		WorkspaceID: string(data.WorkspaceID),
		RunID:       string(data.RunID),
		Revision:    data.Revision,
		Payload:     data.Payload.Map(),
	}
}

func workspaceProjectionBackendEvent(data workorchdevents.WorkspaceProjectionChangedData) backendEventData {
	return backendEventData{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: string(data.WorkspaceID),
		RunID:       string(data.RunID),
		Revision:    data.GlobalRevision,
		Payload: map[string]any{
			"type":                    "workspace_projection_changed",
			"workspaceId":             string(data.WorkspaceID),
			"globalRevision":          data.GlobalRevision,
			"workspaceRevision":       data.WorkspaceRevision,
			"sourceEventRevision":     data.SourceEventRevision,
			"projectedSourceRevision": data.ProjectedSourceRevision,
			"workspaceTotal":          data.WorkspaceTotal,
			"changeKind":              data.ChangeKind,
			"projectionState":         data.ProjectionState,
			"updatedAt":               data.UpdatedAt,
			"rebuiltAt":               data.RebuiltAt,
			"staleReason":             data.StaleReason,
			"error":                   data.Error,
		},
	}
}

func convertBackendEvent(envelope backendEventData) (ret Event) {
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
	case orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated:
		return Event{Type: EventWorkspaceProjectionChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
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
	command := newCLICommand(workorchdevents.CommandWorkspaceCreate, "", "", "workspace", "")
	command.CreateWorkspace = &req
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandWorkspaceGet, id, "", "workspace", string(id)))
	return decodeResultResource[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandWorkspaceList, "", "", "workspace", ""))
	return decodeResultResources[WorkspaceInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandWorkspaceSnapshot, id, "", "workspace", string(id)))
	return decodeResultResource[WorkspaceSnapshotInfo](result, err)
}
func (b *eventHubWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	command := newCLICommand(workorchdevents.CommandWorkspaceDelete, id, "", "workspace", string(id))
	command.Purge = purge
	_, err := b.parent.sendCommand(ctx, command)
	return err
}

type eventHubGoalBackend struct{ parent *eventHubBackend }

func (b *eventHubGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	command := newCLICommand(workorchdevents.CommandGoalDefine, wsID, "", "goal", "")
	command.Text = text
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandGoalList, wsID, "", "goal", ""))
	return decodeResultResources[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandGoalGet, wsID, "", "goal", string(id)))
	return decodeResultResource[GoalInfo](result, err)
}
func (b *eventHubGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error) {
	command := newCLICommand(workorchdevents.CommandGoalUpdate, wsID, "", "goal", string(id))
	command.Text = text
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[GoalInfo](result, err)
}

type eventHubDoDBackend struct{ parent *eventHubBackend }

func (b *eventHubDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	command := newCLICommand(workorchdevents.CommandDoDDefine, wsID, "", "dod", "")
	command.Items = append([]string(nil), items...)
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandDoDList, wsID, "", "dod", ""))
	return decodeResultResources[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandDoDGet, wsID, "", "dod", string(id)))
	return decodeResultResource[DoDInfo](result, err)
}
func (b *eventHubDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error) {
	command := newCLICommand(workorchdevents.CommandDoDUpdate, wsID, "", "dod", string(id))
	command.Items = append([]string(nil), items...)
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[DoDInfo](result, err)
}

type eventHubRunBackend struct{ parent *eventHubBackend }

func (b *eventHubRunBackend) Command(ctx context.Context, req WorkspaceRunCommand) (*RunInfo, error) {
	if b == nil || b.parent == nil {
		return nil, common.ErrInvalidArgument
	}
	intent, err := ValidateWorkspaceRunCommand(req)
	if err != nil {
		return nil, err
	}
	activation := mergeActivationRequest(req.Activation, req)
	source := normalizedRunCommandSource(firstNonEmptyBackendString(req.Source, activation.Source))
	if b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	executionContextPatch, err := eventvalue.FromMap(activation.ExecutionContextPatch)
	if err != nil {
		return nil, fmt.Errorf("invalid execution context patch: %w", err)
	}
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID:           req.WorkspaceID,
		RunID:                 req.ExpectedRunID,
		Intent:                string(intent),
		Reason:                req.Reason,
		RequestID:             req.RequestID,
		Source:                source,
		DefinitionRevision:    firstNonEmptyBackendString(req.DefinitionRevision, activation.DefinitionRevision),
		DecisionID:            firstNonEmptyBackendString(string(req.DecisionID), string(activation.DecisionID)),
		Priority:              req.Priority,
		GoalID:                activation.GoalID,
		DoDID:                 activation.DoDID,
		LLMProviderID:         activation.LLMProviderID,
		SoulID:                activation.SoulID,
		SoulBindings:          commonSoulBindingStrings(activation.SoulBindings),
		SkillIDs:              append([]common.SkillID(nil), activation.SkillIDs...),
		MCPServerIDs:          append([]common.MCPServerID(nil), activation.MCPServerIDs...),
		CapabilityIDs:         append([]common.MCPCapabilityID(nil), activation.CapabilityIDs...),
		MemoryIDs:             append([]common.MemoryID(nil), activation.MemoryIDs...),
		DialogLanguage:        activation.DialogLanguage,
		ExecutionContextPatch: executionContextPatch,
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "workorch/"+source, workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, events.EventHubLaneScope{
		SubjectType: "workspace-run",
		SubjectID:   string(req.WorkspaceID),
		WorkspaceID: string(req.WorkspaceID),
		RunID:       string(req.ExpectedRunID),
	}))
	response := b.parent.commandHub.Send(event)
	if response == nil {
		return nil, fmt.Errorf("%w: run command %s returned no result", common.ErrConflict, event.ID())
	}
	if err := response.Error(); err != nil {
		return nil, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || !result.Accepted {
		return nil, fmt.Errorf("%w: run command %s was not accepted", common.ErrConflict, event.ID())
	}
	var noError error
	return nil, noError
}

func normalizedRunCommandSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "client"
	}
	return source
}

func firstNonEmptyBackendString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (b *eventHubRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandRunGet, wsID, id, "run", string(id)))
	return decodeResultResource[RunInfo](result, err)
}
func (b *eventHubRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandRunList, wsID, "", "run", ""))
	return decodeResultResources[RunInfo](result, err)
}
func (b *eventHubRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandRunStatus, wsID, id, "run", string(id)))
	return decodeResultResource[RunStatusInfo](result, err)
}

type eventHubDecisionBackend struct{ parent *eventHubBackend }

func (b *eventHubDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(req.WorkspaceID, workorchdevents.CommandDecisionResolve); err != nil {
		return nil, err
	}
	command := newCLICommand(workorchdevents.CommandDecisionResolve, req.WorkspaceID, "", "decision", string(req.TicketID))
	command.ResolutionType = req.ResolutionType
	command.GovernanceData = cliDataFromMap(req.Payload)
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandDecisionBatch); err != nil {
		return nil, err
	}
	command := newCLICommand(workorchdevents.CommandDecisionBatch, wsID, "", "decision", "")
	command.DecisionIDs = append([]common.TicketID(nil), ticketIDs...)
	command.ResolutionType = resolutionType
	command.GovernanceData = cliDataFromMap(payload)
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResources[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandDecisionList, wsID, "", "decision", ""))
	return decodeResultResources[DecisionInfo](result, err)
}
func (b *eventHubDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandDecisionGet); err != nil {
		return nil, err
	}
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandDecisionGet, wsID, "", "decision", string(id)))
	return decodeResultResource[DecisionInfo](result, err)
}

type eventHubApprovalBackend struct{ parent *eventHubBackend }

func (b *eventHubApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandApprovalApprove); err != nil {
		return nil, err
	}
	command := newCLICommand(workorchdevents.CommandApprovalApprove, wsID, "", "approval", string(id))
	command.ApprovalAction = "approve"
	command.GovernanceData = cliDataFromMap(firstApprovalPayload(payload))
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandApprovalReject); err != nil {
		return nil, err
	}
	command := newCLICommand(workorchdevents.CommandApprovalReject, wsID, "", "approval", string(id))
	command.ApprovalAction = "reject"
	command.GovernanceData = cliDataFromMap(firstApprovalPayload(payload))
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...map[string]any) ([]ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandApprovalBatch); err != nil {
		return nil, err
	}
	command := newCLICommand(workorchdevents.CommandApprovalBatch, wsID, "", "approval", "")
	command.ApprovalIDs = append([]common.ApprovalID(nil), ids...)
	command.ApprovalAction = action
	command.GovernanceData = cliDataFromMap(firstApprovalPayload(payload))
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResources[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandApprovalList, wsID, "", "approval", ""))
	return decodeResultResources[ApprovalInfo](result, err)
}
func (b *eventHubApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.CommandApprovalGet); err != nil {
		return nil, err
	}
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandApprovalGet, wsID, "", "approval", string(id)))
	return decodeResultResource[ApprovalInfo](result, err)
}

func requireEventHubWorkspaceScope(wsID common.WorkspaceID, command string) error {
	if strings.TrimSpace(string(wsID)) == "" {
		return fmt.Errorf("%w: command %s requires workspace scope", common.ErrInvalidArgument, command)
	}
	return nil
}

func firstApprovalPayload(payload []map[string]any) map[string]any {
	for _, candidate := range payload {
		if len(candidate) > 0 {
			return candidate
		}
	}
	return nil
}

type eventHubSoulBackend struct{ parent *eventHubBackend }

func (b *eventHubSoulBackend) Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error) {
	command := newCLICommand(workorchdevents.CommandSoulCreate, "", "", "soul", string(s.ID))
	command.Soul = s
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandSoulGet, "", "", "soul", string(id)))
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) List(ctx context.Context) ([]SoulInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandSoulList, "", "", "soul", ""))
	return decodeResultResources[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error) {
	command := newCLICommand(workorchdevents.CommandSoulUpdate, "", "", "soul", string(id))
	command.Soul = s
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[SoulInfo](result, err)
}
func (b *eventHubSoulBackend) Delete(ctx context.Context, id common.SoulID) error {
	_, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandSoulDelete, "", "", "soul", string(id)))
	return err
}

type eventHubSkillBackend struct{ parent *eventHubBackend }

func (b *eventHubSkillBackend) Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error) {
	command := newCLICommand(workorchdevents.CommandSkillCreate, "", "", "skill", string(s.ID))
	command.Skill = s
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[SkillInfo](result, err)
}
func (b *eventHubSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandSkillGet, "", "", "skill", string(id)))
	return decodeResultResource[SkillInfo](result, err)
}
func (b *eventHubSkillBackend) List(ctx context.Context) ([]SkillInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandSkillList, "", "", "skill", ""))
	return decodeResultResources[SkillInfo](result, err)
}

type eventHubMCPBackend struct{ parent *eventHubBackend }

func (b *eventHubMCPBackend) CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error) {
	command := newCLICommand(workorchdevents.CommandMCPServerCreate, "", "", "mcp-server", string(s.ID))
	command.MCPServer = s
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMCPServerGet, "", "", "mcp-server", string(id)))
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMCPServerList, "", "", "mcp-server", ""))
	return decodeResultResources[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error) {
	command := newCLICommand(workorchdevents.CommandMCPServerUpdate, "", "", "mcp-server", string(id))
	command.MCPServer = s
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[MCPServerInfo](result, err)
}
func (b *eventHubMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMCPCapabilityList, "", "", "mcp-capability", ""))
	return decodeResultResources[MCPCapabilityInfo](result, err)
}

type eventHubMemoryBackend struct{ parent *eventHubBackend }

func (b *eventHubMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	command := newCLICommand(workorchdevents.CommandMemoryCreate, wsID, "", "memory", string(m.ID))
	command.Memory = m
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMemoryGet, wsID, "", "memory", string(id)))
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	command := newCLICommand(workorchdevents.CommandMemoryUpdate, wsID, "", "memory", string(id))
	command.Memory = m
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	_, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMemoryDelete, wsID, "", "memory", string(id)))
	return err
}
func (b *eventHubMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandMemoryList, wsID, "", "memory", ""))
	return decodeResultResources[MemoryItemInfo](result, err)
}
func (b *eventHubMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error) {
	command := newCLICommand(workorchdevents.CommandMemorySearch, wsID, "", "memory", "")
	command.MemorySearch = &req
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResources[MemoryItemInfo](result, err)
}

type eventHubTemplateBackend struct{ parent *eventHubBackend }

func (b *eventHubTemplateBackend) Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error) {
	command := newCLICommand(workorchdevents.CommandTemplateCreate, "", "", "template", t.ID)
	command.Template = t
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandTemplateGet, "", "", "template", id))
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandTemplateList, "", "", "template", ""))
	return decodeResultResources[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	command := newCLICommand(workorchdevents.CommandTemplateUpdate, "", "", "template", id)
	command.Template = t
	result, err := b.parent.sendCommand(ctx, command)
	return decodeResultResource[TemplateInfo](result, err)
}
func (b *eventHubTemplateBackend) Delete(ctx context.Context, id string) error {
	_, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandTemplateDelete, "", "", "template", id))
	return err
}

type eventHubConfigBackend struct{ parent *eventHubBackend }

func (b *eventHubConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	result, err := b.parent.sendCommand(ctx, newCLICommand(workorchdevents.CommandConfigurationGet, "", "", "configuration", ""))
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
	case SoulInfo:
		return soulInfoResourceMap(typed)
	case *SoulInfo:
		if typed == nil {
			return nil
		}
		return soulInfoResourceMap(*typed)
	case SkillInfo:
		return skillInfoResourceMap(typed)
	case *SkillInfo:
		if typed == nil {
			return nil
		}
		return skillInfoResourceMap(*typed)
	case MCPServerInfo:
		return mcpServerInfoResourceMap(typed)
	case *MCPServerInfo:
		if typed == nil {
			return nil
		}
		return mcpServerInfoResourceMap(*typed)
	case MemoryItemInfo:
		return memoryItemInfoResourceMap(typed)
	case *MemoryItemInfo:
		if typed == nil {
			return nil
		}
		return memoryItemInfoResourceMap(*typed)
	case MemorySearchRequest:
		return memorySearchRequestResourceMap(typed)
	case *MemorySearchRequest:
		if typed == nil {
			return nil
		}
		return memorySearchRequestResourceMap(*typed)
	case TemplateInfo:
		return templateInfoResourceMap(typed)
	case *TemplateInfo:
		if typed == nil {
			return nil
		}
		return templateInfoResourceMap(*typed)
	}
	return nil
}

func runInfoResourceMap(run RunInfo) map[string]any {
	return map[string]any{
		"ID":                 string(run.ID),
		"WorkspaceID":        string(run.WorkspaceID),
		"GoalID":             string(run.GoalID),
		"DoDID":              string(run.DoDID),
		"DefinitionRevision": run.DefinitionRevision,
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
		"Workspace":               snapshot.Workspace,
		"ViewModel":               snapshot.ViewModel,
		"Projection":              snapshot.Projection,
		"Outcome":                 snapshot.Outcome,
		"TaskProgress":            snapshot.TaskProgress,
		"RunHeartbeat":            snapshot.RunHeartbeat,
		"RuntimeExecution":        snapshot.RuntimeExecution,
		"ConvergenceStatus":       snapshot.ConvergenceStatus,
		"RunCommand":              snapshot.RunCommand,
		"CoverageSummary":         cloneCLIResultValue(snapshot.CoverageSummary, 0),
		"GapBaseline":             cloneCLIResultValue(snapshot.GapBaseline, 0),
		"Acceptance":              snapshot.Acceptance,
		"TaskDecompositions":      snapshot.TaskDecompositions,
		"TaskPlans":               snapshot.TaskPlans,
		"Goals":                   snapshot.Goals,
		"DoDs":                    snapshot.DoDs,
		"Runs":                    runs,
		"Results":                 snapshot.Results,
		"Artifacts":               snapshot.Artifacts,
		"ArtifactContinuations":   snapshot.ArtifactContinuations,
		"Patches":                 snapshot.Patches,
		"MemoryRecallTraces":      snapshot.MemoryRecallTraces,
		"WebSources":              snapshot.WebSources,
		"LLMContextBudgets":       snapshot.LLMContextBudgets,
		"LLMContextBudgetSummary": snapshot.LLMContextBudgetSummary,
		"LLMUsageTraces":          snapshot.LLMUsageTraces,
		"LLMUsageSummary":         snapshot.LLMUsageSummary,
		"EventRecords":            eventRecords,
		"Decisions":               snapshot.Decisions,
		"Approvals":               snapshot.Approvals,
	}
}

func soulInfoResourceMap(item SoulInfo) map[string]any {
	return map[string]any{
		"id":              string(item.ID),
		"name":            item.Name,
		"description":     item.Description,
		"businessDomains": append([]string(nil), item.BusinessDomains...),
		"soulMd":          item.SoulMD,
	}
}

func skillInfoResourceMap(item SkillInfo) map[string]any {
	return map[string]any{
		"id":          string(item.ID),
		"name":        item.Name,
		"version":     item.Version,
		"description": item.Description,
		"instruction": item.Instruction,
	}
}

func mcpServerInfoResourceMap(item MCPServerInfo) map[string]any {
	return map[string]any{
		"id":        string(item.ID),
		"name":      item.Name,
		"transport": item.Transport,
		"endpoint":  item.Endpoint,
		"enabled":   item.Enabled,
	}
}

func memoryItemInfoResourceMap(item MemoryItemInfo) map[string]any {
	return map[string]any{
		"id":                  string(item.ID),
		"memoryId":            string(item.ID),
		"workspaceId":         string(item.WorkspaceID),
		"runId":               string(item.RunID),
		"sourceType":          item.SourceType,
		"sourceRef":           item.SourceRef,
		"memoryType":          item.MemoryType,
		"content":             item.Content,
		"summary":             item.Summary,
		"tags":                append([]string(nil), item.Tags...),
		"importance":          item.Importance,
		"confidence":          item.Confidence,
		"visibility":          item.Visibility,
		"embeddingProviderId": item.EmbeddingProviderID,
		"embeddingModel":      item.EmbeddingModel,
		"embeddingText":       item.EmbeddingText,
		"vector":              append([]float64(nil), item.Vector...),
	}
}

func memorySearchRequestResourceMap(item MemorySearchRequest) map[string]any {
	return map[string]any{
		"query": item.Query,
		"tags":  append([]string(nil), item.Tags...),
		"types": append([]string(nil), item.Types...),
		"limit": item.Limit,
	}
}

func templateInfoResourceMap(item TemplateInfo) map[string]any {
	return map[string]any{
		"id":               item.ID,
		"name":             item.Name,
		"description":      item.Description,
		"goalText":         item.GoalText,
		"doDItems":         append([]string(nil), item.DoDItems...),
		"defaultSoulId":    string(item.DefaultSoulID),
		"soulBindings":     cloneStringMap(item.SoulBindings),
		"evidencePolicies": cloneCLIResultValue(item.EvidencePolicies, 0),
	}
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

func decodeResultResource[T any](result workorchdevents.CLIResult, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	for _, resource := range cliResultResources(result) {
		if typed, ok := typedResultResource[T](resource); ok {
			return typed, nil
		}
	}
	return nil, nil
}

func decodeResultResources[T any](result workorchdevents.CLIResult, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out := []T{}
	for _, resource := range cliResultCollections(result) {
		item, ok := typedResultResource[T](resource)
		if ok && item != nil {
			out = append(out, *item)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	return nil, nil
}

func cliResultResources(result workorchdevents.CLIResult) []any {
	return []any{
		result.Workspace,
		result.WorkspaceSnapshot,
		result.Goal,
		result.DoD,
		result.Run,
		result.RunStatus,
		result.Decision,
		result.Approval,
		result.Soul,
		result.Skill,
		result.MCPServer,
		result.Memory,
		result.Template,
		result.Configuration,
	}
}

func cliResultCollections(result workorchdevents.CLIResult) []any {
	ret := make([]any, 0, len(result.Workspaces)+len(result.Goals)+len(result.DoDs)+len(result.Runs)+len(result.Decisions)+len(result.Approvals)+len(result.Souls)+len(result.Skills)+len(result.MCPServers)+len(result.MCPCapabilities)+len(result.Memories)+len(result.Templates))
	for _, item := range result.Workspaces {
		ret = append(ret, item)
	}
	for _, item := range result.Goals {
		ret = append(ret, item)
	}
	for _, item := range result.DoDs {
		ret = append(ret, item)
	}
	for _, item := range result.Runs {
		ret = append(ret, item)
	}
	for _, item := range result.Decisions {
		ret = append(ret, item)
	}
	for _, item := range result.Approvals {
		ret = append(ret, item)
	}
	for _, item := range result.Souls {
		ret = append(ret, item)
	}
	for _, item := range result.Skills {
		ret = append(ret, item)
	}
	for _, item := range result.MCPServers {
		ret = append(ret, item)
	}
	for _, item := range result.MCPCapabilities {
		ret = append(ret, item)
	}
	for _, item := range result.Memories {
		ret = append(ret, item)
	}
	for _, item := range result.Templates {
		ret = append(ret, item)
	}
	return ret
}

func typedResultResource[T any](resource any) (*T, bool) {
	switch typed := resource.(type) {
	case nil:
		return nil, false
	case T:
		return &typed, true
	case *T:
		if typed == nil {
			return nil, true
		}
		return typed, true
	default:
		return nil, false
	}
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
