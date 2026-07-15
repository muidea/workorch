package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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

// ProjectionEvents is the typed projection-invalidation surface for TUI.
func (b *eventHubBackend) ProjectionEvents() WorkspaceProjectionEventsBackend {
	return &eventHubProjectionEventsBackend{parent: b}
}

// ── EventHub Events Backend ──

type eventHubEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

var eventHubObserverSequence atomic.Uint64

func (b *eventHubEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	// Generic/full event stream for agent/CLI tooling only.
	// TUI projection refresh must use ProjectionEvents, not this path.
	b.mu.Lock()
	defer b.mu.Unlock()

	unsubscribers := make([]func(), 0, 1)
	if b.parent.commandHub != nil {
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

// ── EventHub Projection Events Backend (typed, TUI-facing) ──

type eventHubProjectionEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

func (b *eventHubProjectionEventsBackend) SubscribeWorkspaceProjection(
	ctx context.Context,
	onChanged WorkspaceProjectionChangedHandler,
	onStatus SubscriptionStatusHandler,
) (func(), error) {
	if b == nil || b.parent == nil {
		return func() {}, fmt.Errorf("%w: eventhub projection backend unavailable", common.ErrInvalidArgument)
	}
	if onChanged == nil {
		return func() {}, fmt.Errorf("%w: projection changed handler required", common.ErrInvalidArgument)
	}
	publishStatus := func(status SubscriptionStatusInfo) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	publishStatus(SubscriptionStatusInfo{State: SubscriptionConnecting})
	if b.parent.commandHub == nil {
		publishStatus(SubscriptionStatusInfo{State: SubscriptionDegraded, LastError: "command hub unavailable"})
		return func() {}, fmt.Errorf("%w: command hub unavailable", common.ErrInvalidArgument)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	observerID := fmt.Sprintf("/workorchd/consumers/services/workorch/projection/%d", eventHubObserverSequence.Add(1))
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, b.parent.commandHub)
	observer.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
			info := workspaceProjectionChangedInfoFromData(data, event.ID())
			onChanged(event.Context(), info)
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	unsub := func() {
		observer.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged)
	}
	b.subs = append(b.subs, unsub)
	// Subscription is established once the typed observer is attached.
	publishStatus(SubscriptionStatusInfo{State: SubscriptionLive})
	return unsub, nil
}

func workspaceProjectionChangedInfoFromData(data workorchdevents.WorkspaceProjectionChangedData, eventID string) WorkspaceProjectionChangedInfo {
	return WorkspaceProjectionChangedInfo{
		ID:          strings.TrimSpace(eventID),
		WorkspaceID: data.WorkspaceID,
		RunID:       data.RunID,
		Revision: WorkspaceRefreshRevision{
			StreamGeneration:        data.StreamGeneration,
			StreamRevision:          data.GlobalRevision,
			WorkspaceRevision:       data.WorkspaceRevision,
			ProjectedSourceRevision: data.ProjectedSourceRevision,
			RuntimeStateRevision:    data.RuntimeStateRevision,
			EvidenceSequence:        data.EvidenceSequence,
			ActivityRevision:        data.ActivityRevision,
		},
		ChangedSections: append([]WorkspaceProjectionSection(nil), data.ChangedSections...),
		ProjectionState: data.ProjectionState,
		UpdatedAt:       data.UpdatedAt,
	}
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
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceCreateCommand{Request: req})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceCreate, "workspace", "", "", "")
	result, err := cliWorkspaceCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Workspace, nil
}
func (b *eventHubWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceGetCommand{WorkspaceID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceGet, "workspace", string(id), id, "")
	result, err := cliWorkspaceGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Workspace, nil
}
func (b *eventHubWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceList, "workspace", "all", "", "")
	result, err := cliWorkspaceListResponse(b.parent.commandHub.Send(event))
	return result.Workspaces, err
}
func (b *eventHubWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceSnapshot, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceSnapshotCommand{WorkspaceID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceSnapshot, "workspace", string(id), id, "")
	result, err := cliWorkspaceSnapshotResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Snapshot, nil
}

func (b *eventHubWorkspaceBackend) RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*WorkspaceRuntimeViewInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(string(req.WorkspaceID)) == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(
		workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView,
		"workorch-cli",
		workorchdevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		workorchdevents.ReadWorkspaceRuntimeViewCommand{
			WorkspaceID:   req.WorkspaceID,
			ActivityLimit: req.ActivityLimit,
			BeforeCursor:  req.BeforeCursor,
		},
	)
	bindCLIEvent(event, workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView, "workspace", string(req.WorkspaceID), req.WorkspaceID, "")
	result, err := cliWorkspaceRuntimeViewResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	view := result.View
	return &view, nil
}

func (b *eventHubWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceDelete, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceDeleteCommand{WorkspaceID: id, Purge: purge})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceDelete, "workspace", string(id), id, "")
	_, err := cliWorkspaceDeleteResponse(b.parent.commandHub.Send(event))
	return err
}

type eventHubGoalBackend struct{ parent *eventHubBackend }

func (b *eventHubGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIGoalDefine, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIGoalDefineCommand{WorkspaceID: wsID, Text: text})
	bindCLIEvent(event, workorchdevents.TopicCLIGoalDefine, "goal", "", wsID, "")
	result, err := cliGoalDefineResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Goal, nil
}
func (b *eventHubGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIGoalList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIGoalListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIGoalList, "workspace", string(wsID), wsID, "")
	result, err := cliGoalListResponse(b.parent.commandHub.Send(event))
	return result.Goals, err
}
func (b *eventHubGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIGoalGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIGoalGetCommand{WorkspaceID: wsID, GoalID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIGoalGet, "goal", string(id), wsID, "")
	result, err := cliGoalGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Goal, nil
}
func (b *eventHubGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIGoalUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIGoalUpdateCommand{WorkspaceID: wsID, GoalID: id, Text: text})
	bindCLIEvent(event, workorchdevents.TopicCLIGoalUpdate, "goal", string(id), wsID, "")
	result, err := cliGoalUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Goal, nil
}

type eventHubDoDBackend struct{ parent *eventHubBackend }

func (b *eventHubDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDoDDefine, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDoDDefineCommand{WorkspaceID: wsID, Items: append([]string(nil), items...)})
	bindCLIEvent(event, workorchdevents.TopicCLIDoDDefine, "dod", "", wsID, "")
	result, err := cliDoDDefineResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.DoD, nil
}
func (b *eventHubDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDoDList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDoDListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIDoDList, "workspace", string(wsID), wsID, "")
	result, err := cliDoDListResponse(b.parent.commandHub.Send(event))
	return result.DoDs, err
}
func (b *eventHubDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDoDGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDoDGetCommand{WorkspaceID: wsID, DoDID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIDoDGet, "dod", string(id), wsID, "")
	result, err := cliDoDGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.DoD, nil
}
func (b *eventHubDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDoDUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDoDUpdateCommand{WorkspaceID: wsID, DoDID: id, Items: append([]string(nil), items...)})
	bindCLIEvent(event, workorchdevents.TopicCLIDoDUpdate, "dod", string(id), wsID, "")
	result, err := cliDoDUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.DoD, nil
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
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID:        req.WorkspaceID,
		RunID:              req.ExpectedRunID,
		Intent:             string(intent),
		Reason:             req.Reason,
		RequestID:          req.RequestID,
		Source:             source,
		DefinitionRevision: firstNonEmptyBackendString(req.DefinitionRevision, activation.DefinitionRevision),
		DecisionID:         firstNonEmptyBackendString(string(req.DecisionID), string(activation.DecisionID)),
		Priority:           req.Priority,
		GoalID:             activation.GoalID,
		DoDID:              activation.DoDID,
		LLMTarget:          activation.LLMTarget,
		SoulID:             activation.SoulID,
		SoulBindings:       commonSoulBindingStrings(activation.SoulBindings),
		SkillIDs:           append([]common.SkillID(nil), activation.SkillIDs...),
		MCPServerIDs:       append([]common.MCPServerID(nil), activation.MCPServerIDs...),
		CapabilityIDs:      append([]common.MCPCapabilityID(nil), activation.CapabilityIDs...),
		MemoryIDs:          append([]common.MemoryID(nil), activation.MemoryIDs...),
		DialogLanguage:     activation.DialogLanguage,
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
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunGetCommand{WorkspaceID: wsID, RunID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIRunGet, "run", string(id), wsID, id)
	result, err := cliRunGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Run, nil
}
func (b *eventHubRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIRunList, "workspace", string(wsID), wsID, "")
	result, err := cliRunListResponse(b.parent.commandHub.Send(event))
	return result.Runs, err
}
func (b *eventHubRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunStatus, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunStatusCommand{WorkspaceID: wsID, RunID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIRunStatus, "run", string(id), wsID, id)
	result, err := cliRunStatusResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Status, nil
}

type eventHubDecisionBackend struct{ parent *eventHubBackend }

func (b *eventHubDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(req.WorkspaceID, workorchdevents.TopicCLIDecisionResolve); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionResolve, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDecisionResolveCommand{RequestID: newCLIRequestID(), Request: req})
	bindCLIEvent(event, workorchdevents.TopicCLIDecisionResolve, "decision", string(req.TicketID), req.WorkspaceID, "")
	result, err := cliDecisionResolveResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Decision, nil
}
func (b *eventHubDecisionBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIDecisionBatchResolve); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workorchdevents.CLIDecisionBatchResolveCommand{RequestID: newCLIRequestID(), WorkspaceID: wsID, TicketIDs: append([]common.TicketID(nil), ticketIDs...), ResolutionType: resolutionType, Payload: payload.Clone()}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionBatchResolve, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindCLIEvent(event, workorchdevents.TopicCLIDecisionBatchResolve, "workspace", string(wsID), wsID, "")
	result, err := cliDecisionBatchResolveResponse(b.parent.commandHub.Send(event))
	return result.Decisions, err
}
func (b *eventHubDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDecisionListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIDecisionList, "workspace", string(wsID), wsID, "")
	result, err := cliDecisionListResponse(b.parent.commandHub.Send(event))
	return result.Decisions, err
}
func (b *eventHubDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIDecisionGet); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDecisionGetCommand{WorkspaceID: wsID, TicketID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIDecisionGet, "decision", string(id), wsID, "")
	result, err := cliDecisionGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Decision, nil
}

type eventHubApprovalBackend struct{ parent *eventHubBackend }

func (b *eventHubApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIApprovalApprove); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workorchdevents.CLIApprovalApproveCommand{RequestID: newCLIRequestID(), WorkspaceID: wsID, ApprovalID: id, Payload: firstTypedApprovalPayload(payload)}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalApprove, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalApprove, "approval", string(id), wsID, "")
	result, err := cliApprovalApproveResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Approval, nil
}
func (b *eventHubApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIApprovalReject); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workorchdevents.CLIApprovalRejectCommand{RequestID: newCLIRequestID(), WorkspaceID: wsID, ApprovalID: id, Payload: firstTypedApprovalPayload(payload)}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalReject, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalReject, "approval", string(id), wsID, "")
	result, err := cliApprovalRejectResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Approval, nil
}
func (b *eventHubApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIApprovalBatchResolve); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workorchdevents.CLIApprovalBatchResolveCommand{RequestID: newCLIRequestID(), WorkspaceID: wsID, ApprovalIDs: append([]common.ApprovalID(nil), ids...), Action: action, Payload: firstTypedApprovalPayload(payload)}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalBatchResolve, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalBatchResolve, "workspace", string(wsID), wsID, "")
	result, err := cliApprovalBatchResolveResponse(b.parent.commandHub.Send(event))
	return result.Approvals, err
}

func firstTypedApprovalPayload(payload []workorcheventpayload.Payload) workorcheventpayload.Payload {
	for _, candidate := range payload {
		if len(candidate.Fields) > 0 {
			return candidate.Clone()
		}
	}
	return workorcheventpayload.Payload{}
}
func (b *eventHubApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIApprovalListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalList, "workspace", string(wsID), wsID, "")
	result, err := cliApprovalListResponse(b.parent.commandHub.Send(event))
	return result.Approvals, err
}
func (b *eventHubApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	if err := requireEventHubWorkspaceScope(wsID, workorchdevents.TopicCLIApprovalGet); err != nil {
		return nil, err
	}
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIApprovalGetCommand{WorkspaceID: wsID, ApprovalID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalGet, "approval", string(id), wsID, "")
	result, err := cliApprovalGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Approval, nil
}

func newCLIRequestID() string {
	return fmt.Sprintf("cli-%d", time.Now().UTC().UnixNano())
}

func requireEventHubWorkspaceScope(wsID common.WorkspaceID, command string) error {
	if strings.TrimSpace(string(wsID)) == "" {
		return fmt.Errorf("%w: command %s requires workspace scope", common.ErrInvalidArgument, command)
	}
	return nil
}

type eventHubSoulBackend struct{ parent *eventHubBackend }

func (b *eventHubSoulBackend) Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulCreateCommand{Soul: *s})
	bindCLIEvent(event, workorchdevents.TopicCLISoulCreate, "soul", string(s.ID), "", "")
	result, err := cliSoulCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Soul, nil
}
func (b *eventHubSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulGetCommand{SoulID: id})
	bindCLIEvent(event, workorchdevents.TopicCLISoulGet, "soul", string(id), "", "")
	result, err := cliSoulGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Soul, nil
}
func (b *eventHubSoulBackend) List(ctx context.Context) ([]SoulInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLISoulList, "soul", "all", "", "")
	result, err := cliSoulListResponse(b.parent.commandHub.Send(event))
	return result.Souls, err
}
func (b *eventHubSoulBackend) Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulUpdateCommand{SoulID: id, Soul: *s})
	bindCLIEvent(event, workorchdevents.TopicCLISoulUpdate, "soul", string(id), "", "")
	result, err := cliSoulUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Soul, nil
}
func (b *eventHubSoulBackend) Delete(ctx context.Context, id common.SoulID) error {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulDelete, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulDeleteCommand{SoulID: id})
	bindCLIEvent(event, workorchdevents.TopicCLISoulDelete, "soul", string(id), "", "")
	_, err := cliSoulDeleteResponse(b.parent.commandHub.Send(event))
	return err
}

type eventHubSkillBackend struct{ parent *eventHubBackend }

func (b *eventHubSkillBackend) Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISkillCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISkillCreateCommand{Skill: *s})
	bindCLIEvent(event, workorchdevents.TopicCLISkillCreate, "skill", string(s.ID), "", "")
	result, err := cliSkillCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Skill, nil
}
func (b *eventHubSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISkillGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISkillGetCommand{SkillID: id})
	bindCLIEvent(event, workorchdevents.TopicCLISkillGet, "skill", string(id), "", "")
	result, err := cliSkillGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Skill, nil
}
func (b *eventHubSkillBackend) List(ctx context.Context) ([]SkillInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISkillList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISkillListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLISkillList, "skill", "all", "", "")
	result, err := cliSkillListResponse(b.parent.commandHub.Send(event))
	return result.Skills, err
}

type eventHubMCPBackend struct{ parent *eventHubBackend }

func (b *eventHubMCPBackend) CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerCreateCommand{Server: *s})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerCreate, "mcp-server", string(s.ID), "", "")
	result, err := cliMCPServerCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Server, nil
}
func (b *eventHubMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerGetCommand{ServerID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerGet, "mcp-server", string(id), "", "")
	result, err := cliMCPServerGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Server, nil
}
func (b *eventHubMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerList, "mcp-server", "all", "", "")
	result, err := cliMCPServerListResponse(b.parent.commandHub.Send(event))
	return result.Servers, err
}
func (b *eventHubMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerUpdateCommand{ServerID: id, Server: *s})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerUpdate, "mcp-server", string(id), "", "")
	result, err := cliMCPServerUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Server, nil
}
func (b *eventHubMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPCapabilityList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPCapabilityListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPCapabilityList, "mcp-capability", "all", "", "")
	result, err := cliMCPCapabilityListResponse(b.parent.commandHub.Send(event))
	return result.Capabilities, err
}

type eventHubMemoryBackend struct{ parent *eventHubBackend }

func (b *eventHubMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || m == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryCreateCommand{WorkspaceID: wsID, Memory: *m})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryCreate, "memory", string(m.ID), wsID, m.RunID)
	result, err := cliMemoryCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Memory, nil
}
func (b *eventHubMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryGetCommand{WorkspaceID: wsID, MemoryID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryGet, "memory", string(id), wsID, "")
	result, err := cliMemoryGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Memory, nil
}
func (b *eventHubMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || m == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryUpdateCommand{WorkspaceID: wsID, MemoryID: id, Memory: *m})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryUpdate, "memory", string(id), wsID, m.RunID)
	result, err := cliMemoryUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Memory, nil
}
func (b *eventHubMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryDelete, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryDeleteCommand{WorkspaceID: wsID, MemoryID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryDelete, "memory", string(id), wsID, "")
	_, err := cliMemoryDeleteResponse(b.parent.commandHub.Send(event))
	return err
}
func (b *eventHubMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryList, "workspace", string(wsID), wsID, "")
	result, err := cliMemoryListResponse(b.parent.commandHub.Send(event))
	return result.Memories, err
}
func (b *eventHubMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemorySearch, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemorySearchCommand{WorkspaceID: wsID, Request: req})
	bindCLIEvent(event, workorchdevents.TopicCLIMemorySearch, "workspace", string(wsID), wsID, "")
	result, err := cliMemorySearchResponse(b.parent.commandHub.Send(event))
	return result.Memories, err
}

type eventHubTemplateBackend struct{ parent *eventHubBackend }

func (b *eventHubTemplateBackend) Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || t == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateCreateCommand{Template: *t})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateCreate, "template", t.ID, "", "")
	result, err := cliTemplateCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Template, nil
}
func (b *eventHubTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateGetCommand{TemplateID: id})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateGet, "template", id, "", "")
	result, err := cliTemplateGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Template, nil
}
func (b *eventHubTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateList, "template", "all", "", "")
	result, err := cliTemplateListResponse(b.parent.commandHub.Send(event))
	return result.Templates, err
}
func (b *eventHubTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || t == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateUpdateCommand{TemplateID: id, Template: *t})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateUpdate, "template", id, "", "")
	result, err := cliTemplateUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Template, nil
}
func (b *eventHubTemplateBackend) Delete(ctx context.Context, id string) error {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateDelete, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateDeleteCommand{TemplateID: id})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateDelete, "template", id, "", "")
	_, err := cliTemplateDeleteResponse(b.parent.commandHub.Send(event))
	return err
}

type eventHubConfigBackend struct{ parent *eventHubBackend }

func (b *eventHubConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIConfigurationGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIConfigurationGetCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIConfigurationGet, "configuration", "service", "", "")
	result, err := cliConfigurationGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Configuration, nil
}
