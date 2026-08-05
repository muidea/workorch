package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// eventHubBackend extends the local backend with EventHub-driven progress
// subscriptions, replacing fixed-interval polling with reactive events.
type eventHubBackend struct {
	commandHub magicevent.Hub
	evtImpl    *eventHubEventsBackend
}

// NewEventHub creates a port.Backend using EventHub for progress events.
func NewEventHub(commandHub magicevent.Hub) port.Backend {
	b := &eventHubBackend{
		commandHub: commandHub,
	}
	b.evtImpl = &eventHubEventsBackend{parent: b}
	return b
}

func (b *eventHubBackend) Workspace() port.WorkspaceBackend {
	return &eventHubWorkspaceBackend{parent: b}
}
func (b *eventHubBackend) Goal() port.GoalBackend         { return &eventHubGoalBackend{parent: b} }
func (b *eventHubBackend) DoD() port.DoDBackend           { return &eventHubDoDBackend{parent: b} }
func (b *eventHubBackend) Run() port.RunBackend           { return &eventHubRunBackend{parent: b} }
func (b *eventHubBackend) Decision() port.DecisionBackend { return &eventHubDecisionBackend{parent: b} }
func (b *eventHubBackend) Approval() port.ApprovalBackend { return &eventHubApprovalBackend{parent: b} }
func (b *eventHubBackend) Soul() port.SoulBackend         { return &eventHubSoulBackend{parent: b} }
func (b *eventHubBackend) Skill() port.SkillBackend       { return &eventHubSkillBackend{parent: b} }
func (b *eventHubBackend) MCP() port.MCPBackend           { return &eventHubMCPBackend{parent: b} }
func (b *eventHubBackend) Memory() port.MemoryBackend     { return &eventHubMemoryBackend{parent: b} }
func (b *eventHubBackend) Template() port.TemplateBackend { return &eventHubTemplateBackend{parent: b} }
func (b *eventHubBackend) Config() port.ConfigBackend     { return &eventHubConfigBackend{parent: b} }

// Events overrides the stub to provide EventHub-driven subscriptions.
func (b *eventHubBackend) Events() port.EventsBackend { return b.evtImpl }

// ProjectionEvents is the typed projection-invalidation surface for TUI.
func (b *eventHubBackend) ProjectionEvents() port.WorkspaceProjectionEventsBackend {
	return &eventHubProjectionEventsBackend{parent: b}
}

// ── EventHub Events port.Backend ──

type eventHubEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

var eventHubObserverSequence atomic.Uint64

func (b *eventHubEventsBackend) Subscribe(ctx context.Context, handler port.EventHandler) (func(), error) {
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

// ── EventHub Projection Events port.Backend (typed, TUI-facing) ──

type eventHubProjectionEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

func (b *eventHubProjectionEventsBackend) SubscribeWorkspaceProjection(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (func(), error) {
	if b == nil || b.parent == nil {
		return func() {}, fmt.Errorf("%w: eventhub projection backend unavailable", common.ErrInvalidArgument)
	}
	if onChanged == nil {
		return func() {}, fmt.Errorf("%w: projection changed handler required", common.ErrInvalidArgument)
	}
	publishStatus := func(status port.SubscriptionStatus) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	publishStatus(port.SubscriptionStatus{State: port.SubscriptionConnecting})
	if b.parent.commandHub == nil {
		publishStatus(port.SubscriptionStatus{State: port.SubscriptionDegraded, LastError: "command hub unavailable"})
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
	publishStatus(port.SubscriptionStatus{State: port.SubscriptionLive})
	return unsub, nil
}

func workspaceProjectionChangedInfoFromData(data workorchdevents.WorkspaceProjectionChangedData, eventID string) projectioncontract.WorkspaceProjectionChanged {
	return projectioncontract.WorkspaceProjectionChanged{
		ID:          strings.TrimSpace(eventID),
		WorkspaceID: string(data.WorkspaceID),
		RunID:       string(data.RunID),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			StreamGeneration:        data.StreamGeneration,
			StreamRevision:          data.GlobalRevision,
			WorkspaceRevision:       data.WorkspaceRevision,
			ProjectedSourceRevision: data.ProjectedSourceRevision,
			RuntimeStateRevision:    data.RuntimeStateRevision,
			EvidenceSequence:        data.EvidenceSequence,
			ActivityRevision:        data.ActivityRevision,
		},
		ChangedSections: append([]projectioncontract.WorkspaceProjectionSection(nil), data.ChangedSections...),
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

func convertBackendEvent(envelope backendEventData) (ret port.Event) {
	defer func() {
		if ret.Type != "" {
			ret.ID = envelope.ID
			ret.Revision = envelope.Revision
		}
	}()
	switch envelope.Topic {
	case workorchdevents.TopicWorkspaceProjectionChanged:
		return port.Event{Type: port.EventWorkspaceProjectionChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceCreated:
		return port.Event{Type: port.EventWorkspaceCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceStateChanged:
		return port.Event{Type: port.EventWorkspaceStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicWorkspaceDeleted:
		return port.Event{Type: port.EventWorkspaceDeleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case workspaceevents.TopicGoalDefined:
		return port.Event{Type: port.EventGoalDefined, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: enrichSubjectPayload(envelope.Payload, "goalId", envelope.SubjectID)}
	case workspaceevents.TopicDoDDefined:
		return port.Event{Type: port.EventDoDDefined, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: enrichSubjectPayload(envelope.Payload, "doDId", envelope.SubjectID)}
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		return port.Event{Type: port.EventRunStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case decisionevents.TopicTicketCreated:
		return port.Event{Type: port.EventDecisionCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "ticketId", envelope.SubjectID)}
	case decisionevents.TopicTicketResolved:
		return port.Event{Type: port.EventDecisionResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "ticketId", envelope.SubjectID)}
	case approvalevents.TopicApprovalResolved:
		return port.Event{Type: port.EventApprovalResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "approvalId", envelope.SubjectID)}
	case resultevents.TopicResultAccepted, resultevents.TopicResultRejected, resultevents.TopicResultSuperseded:
		return port.Event{Type: port.EventResultAccepted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case memoryevents.TopicMemoryCreated:
		return port.Event{Type: port.EventMemoryCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryUpdated:
		return port.Event{Type: port.EventMemoryUpdated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryDeleted:
		return port.Event{Type: port.EventMemoryDeleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "memoryId", envelope.SubjectID)}
	case memoryevents.TopicMemoryRecallRecorded:
		return port.Event{Type: port.EventMemoryRecallRecorded, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoEvaluated:
		return port.Event{Type: port.EventGovernanceEvaluated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoResolved:
		return port.Event{Type: port.EventGovernanceResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoEscalated:
		return port.Event{Type: port.EventGovernanceEscalated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	case governanceevents.TopicGovernanceAutoBlocked:
		return port.Event{Type: port.EventGovernanceBlocked, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: enrichSubjectPayload(envelope.Payload, "traceId", envelope.SubjectID)}
	default:
		return port.Event{}
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

func (b *eventHubWorkspaceBackend) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceCreateCommand{Request: req})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceCreate, "workspace", "", "", "")
	result, err := cliWorkspaceCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.WorkspaceProjection(result.Workspace)
	return &item, mapErr
}
func (b *eventHubWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceGetCommand{WorkspaceID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceGet, "workspace", string(id), id, "")
	result, err := cliWorkspaceGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.WorkspaceProjection(result.Workspace)
	return &item, mapErr
}
func (b *eventHubWorkspaceBackend) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceList, "workspace", "all", "", "")
	result, err := cliWorkspaceListResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	items := make([]projectioncontract.Workspace, 0, len(result.Workspaces))
	for _, value := range result.Workspaces {
		item, mapErr := workorchdeventmapping.WorkspaceProjection(value)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return items, nil
}
func (b *eventHubWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceSnapshot, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceSnapshotCommand{WorkspaceID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceSnapshot, "workspace", string(id), id, "")
	result, err := cliWorkspaceSnapshotResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.SnapshotPort(result.Snapshot)
	return &item, mapErr
}

func (b *eventHubWorkspaceBackend) RuntimeView(ctx context.Context, req workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
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
		workorchdevents.ReadWorkspaceRuntimeViewCommand{Query: req},
	)
	bindCLIEvent(event, workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView, "workspace", string(req.WorkspaceID), req.WorkspaceID, "")
	result, err := cliWorkspaceRuntimeViewResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	view := workorchdeventmapping.RuntimeViewPort(result.View)
	return &view, nil
}

func (b *eventHubWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*projectioncontract.Workspace, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceUpdateLLMTarget, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIWorkspaceUpdateLLMTargetCommand{WorkspaceID: id, Target: target})
	bindCLIEvent(event, workorchdevents.TopicCLIWorkspaceUpdateLLMTarget, "workspace", string(id), id, "")
	result, err := cliWorkspaceUpdateLLMTargetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.WorkspaceProjection(result.Workspace)
	return &item, mapErr
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

func (b *eventHubGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error) {
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
func (b *eventHubGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIGoalList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIGoalListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIGoalList, "workspace", string(wsID), wsID, "")
	result, err := cliGoalListResponse(b.parent.commandHub.Send(event))
	return result.Goals, err
}
func (b *eventHubGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error) {
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
func (b *eventHubGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error) {
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

func (b *eventHubDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error) {
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
func (b *eventHubDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDoDList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDoDListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIDoDList, "workspace", string(wsID), wsID, "")
	result, err := cliDoDListResponse(b.parent.commandHub.Send(event))
	return result.DoDs, err
}
func (b *eventHubDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error) {
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
func (b *eventHubDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error) {
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

func (b *eventHubRunBackend) Command(ctx context.Context, req port.WorkspaceRunCommand) (*port.RunCommandResult, error) {
	if b == nil || b.parent == nil {
		return nil, common.ErrInvalidArgument
	}
	intent, err := port.ValidateWorkspaceRunCommand(req)
	if err != nil {
		return nil, err
	}
	source := normalizedRunCommandSource(req.Source)
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = newCLIRequestID()
	}
	if b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: req.WorkspaceID,
		Intent:      string(intent),
		Reason:      req.Reason,
		RequestID:   requestID,
		Source:      source,
		Priority:    req.Priority,
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "workorch/"+source, workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(workorchdevents.Contracts(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, eventcontract.LaneScope{
		SubjectType: "workspace-run",
		SubjectID:   string(req.WorkspaceID),
		WorkspaceID: string(req.WorkspaceID),
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
	if !ok || result.WorkspaceID != req.WorkspaceID || strings.TrimSpace(result.RequestID) != requestID || string(result.Kind) != string(intent) {
		return nil, fmt.Errorf("%w: run command %s returned an invalid owner outcome", common.ErrConflict, event.ID())
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("%w: run command %s returned an invalid owner admission: %v", common.ErrConflict, event.ID(), err)
	}
	returnValue := port.RunCommandResult(runtimecontract.RuntimeExecutionIntentAdmission(result))
	return &returnValue, nil
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

func (b *eventHubRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunGetCommand{WorkspaceID: wsID, RunID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIRunGet, "run", string(id), wsID, id)
	result, err := cliRunGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	ret := projectioncontract.CloneRun(result.Run)
	return &ret, nil
}
func (b *eventHubRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIRunList, "workspace", string(wsID), wsID, "")
	result, err := cliRunListResponse(b.parent.commandHub.Send(event))
	return projectioncontract.CloneRuns(result.Runs), err
}
func (b *eventHubRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIRunStatus, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIRunStatusCommand{WorkspaceID: wsID, RunID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIRunStatus, "run", string(id), wsID, id)
	result, err := cliRunStatusResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	status := projectioncontract.CloneRunStatus(result.Status)
	return &status, nil
}

type eventHubDecisionBackend struct{ parent *eventHubBackend }

func (b *eventHubDecisionBackend) Resolve(ctx context.Context, req port.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
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
func (b *eventHubDecisionBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
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
func (b *eventHubDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIDecisionListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIDecisionList, "workspace", string(wsID), wsID, "")
	result, err := cliDecisionListResponse(b.parent.commandHub.Send(event))
	return result.Decisions, err
}
func (b *eventHubDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error) {
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

func (b *eventHubApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
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
func (b *eventHubApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
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
func (b *eventHubApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
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
func (b *eventHubApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIApprovalList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIApprovalListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIApprovalList, "workspace", string(wsID), wsID, "")
	result, err := cliApprovalListResponse(b.parent.commandHub.Send(event))
	return result.Approvals, err
}
func (b *eventHubApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error) {
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

func (b *eventHubSoulBackend) Create(ctx context.Context, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
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
func (b *eventHubSoulBackend) Get(ctx context.Context, id common.SoulID) (*workorchmanagementcontract.Soul, error) {
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
func (b *eventHubSoulBackend) List(ctx context.Context) ([]workorchmanagementcontract.Soul, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISoulList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISoulListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLISoulList, "soul", "all", "", "")
	result, err := cliSoulListResponse(b.parent.commandHub.Send(event))
	return result.Souls, err
}
func (b *eventHubSoulBackend) Update(ctx context.Context, id common.SoulID, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
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

func (b *eventHubSkillBackend) Create(ctx context.Context, s *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error) {
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
func (b *eventHubSkillBackend) Get(ctx context.Context, id common.SkillID) (*workorchmanagementcontract.Skill, error) {
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
func (b *eventHubSkillBackend) List(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLISkillList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLISkillListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLISkillList, "skill", "all", "", "")
	result, err := cliSkillListResponse(b.parent.commandHub.Send(event))
	return result.Skills, err
}

type eventHubMCPBackend struct{ parent *eventHubBackend }

func (b *eventHubMCPBackend) CreateServer(ctx context.Context, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerCreateCommand{Server: workorchdeventmapping.MCPServerMutationDTO(*s)})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerCreate, "mcp-server", s.ID, "", "")
	result, err := cliMCPServerCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	server, mapErr := workorchdeventmapping.MCPServerManagement(result.Server)
	return &server, mapErr
}
func (b *eventHubMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*workorchmanagementcontract.MCPServer, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerGetCommand{ServerID: id})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerGet, "mcp-server", string(id), "", "")
	result, err := cliMCPServerGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	server, mapErr := workorchdeventmapping.MCPServerManagement(result.Server)
	return &server, mapErr
}
func (b *eventHubMCPBackend) ListServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerList, "mcp-server", "all", "", "")
	result, err := cliMCPServerListResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return workorchdeventmapping.MCPServerManagements(result.Servers)
}
func (b *eventHubMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || s == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPServerUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPServerUpdateCommand{ServerID: id, Server: workorchdeventmapping.MCPServerMutationDTO(*s)})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPServerUpdate, "mcp-server", string(id), "", "")
	result, err := cliMCPServerUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	server, mapErr := workorchdeventmapping.MCPServerManagement(result.Server)
	return &server, mapErr
}
func (b *eventHubMCPBackend) ListCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMCPCapabilityList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMCPCapabilityListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLIMCPCapabilityList, "mcp-capability", "all", "", "")
	result, err := cliMCPCapabilityListResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return workorchdeventmapping.MCPCapabilityManagements(result.Capabilities), nil
}

type eventHubMemoryBackend struct{ parent *eventHubBackend }

func (b *eventHubMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || m == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryCreateCommand{WorkspaceID: wsID, Memory: *m})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryCreate, "memory", m.MemoryID, wsID, common.RunID(m.RunID))
	result, err := cliMemoryCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return &result.Memory, nil
}
func (b *eventHubMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchmanagementcontract.MemoryItem, error) {
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
func (b *eventHubMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || m == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryUpdateCommand{WorkspaceID: wsID, MemoryID: id, Memory: *m})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryUpdate, "memory", string(id), wsID, common.RunID(m.RunID))
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
func (b *eventHubMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemoryList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemoryListCommand{WorkspaceID: wsID})
	bindCLIEvent(event, workorchdevents.TopicCLIMemoryList, "workspace", string(wsID), wsID, "")
	result, err := cliMemoryListResponse(b.parent.commandHub.Send(event))
	return result.Memories, err
}
func (b *eventHubMemoryBackend) Search(ctx context.Context, req workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	req = req.Normalized()
	if req.WorkspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIMemorySearch, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIMemorySearchCommand{Query: req})
	bindCLIEvent(event, workorchdevents.TopicCLIMemorySearch, "workspace", string(req.WorkspaceID), req.WorkspaceID, "")
	result, err := cliMemorySearchResponse(b.parent.commandHub.Send(event))
	return result.Memories, err
}

type eventHubTemplateBackend struct{ parent *eventHubBackend }

func (b *eventHubTemplateBackend) Create(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || t == nil {
		return nil, common.ErrInvalidArgument
	}
	dto, mapErr := workorchdeventmapping.TemplateDTO(*t)
	if mapErr != nil {
		return nil, mapErr
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateCreate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateCreateCommand{Template: dto})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateCreate, "template", t.ID, "", "")
	result, err := cliTemplateCreateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.TemplateManagement(result.Template)
	return &item, mapErr
}
func (b *eventHubTemplateBackend) Get(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateGet, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateGetCommand{TemplateID: id})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateGet, "template", id, "", "")
	result, err := cliTemplateGetResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.TemplateManagement(result.Template)
	return &item, mapErr
}
func (b *eventHubTemplateBackend) List(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateList, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateListCommand{})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateList, "template", "all", "", "")
	result, err := cliTemplateListResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return workorchdeventmapping.TemplateManagements(result.Templates)
}
func (b *eventHubTemplateBackend) Update(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil || t == nil {
		return nil, common.ErrInvalidArgument
	}
	dto, mapErr := workorchdeventmapping.TemplateDTO(*t)
	if mapErr != nil {
		return nil, mapErr
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLITemplateUpdate, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLITemplateUpdateCommand{TemplateID: id, Template: dto})
	bindCLIEvent(event, workorchdevents.TopicCLITemplateUpdate, "template", id, "", "")
	result, err := cliTemplateUpdateResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	item, mapErr := workorchdeventmapping.TemplateManagement(result.Template)
	return &item, mapErr
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

func (b *eventHubConfigBackend) Get(ctx context.Context) (*port.ServiceConfig, error) {
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

func (b *eventHubConfigBackend) ListChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	cfg, err := b.Get(ctx)
	if err != nil {
		return nil, err
	}
	return port.NormalizeChatModelTargetOptions(cfg.ChatModelTargets), nil
}

func (b *eventHubConfigBackend) RefreshChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	if b == nil || b.parent == nil || b.parent.commandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIConfigurationRefreshChatModels, "workorch-cli", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.CLIConfigurationRefreshChatModelsCommand{ProviderID: runtimecontract.BuiltinAIProxyProviderID})
	bindCLIEvent(event, workorchdevents.TopicCLIConfigurationRefreshChatModels, "configuration", "chat-models", "", "")
	result, err := cliConfigurationRefreshChatModelsResponse(b.parent.commandHub.Send(event))
	if err != nil {
		return nil, err
	}
	return port.NormalizeChatModelTargetOptions(result.Configuration.ChatModelTargets), nil
}
