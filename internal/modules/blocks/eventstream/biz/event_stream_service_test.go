package eventstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runcommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	workspacecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type countingEventRecordRepository struct {
	base        repository.EventRecordRepository
	mu          sync.Mutex
	appendCount int
	listCount   int
}

type failingAppendEventRecordRepository struct {
	repository.EventRecordRepository
	mu       sync.Mutex
	failures int
	err      error
}

type countingEventStreamWorkspaceDeletionRepository struct {
	repository.EventStreamWorkspaceDeletionRepository
	mu       sync.Mutex
	getCount int
}

type eventStreamTestPurger struct {
	repo repository.EventRecordRepository
}

func (p eventStreamTestPurger) PurgeWorkspaceEventRecords(ctx context.Context, expected eventstreammodel.WorkspaceDeletion) (int, error) {
	if p.repo == nil || expected.Validate() != nil || expected.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge {
		return 0, common.ErrInvalidArgument
	}
	items, err := p.repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: expected.WorkspaceID, IncludePayload: true})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item == nil || item.WorkspaceID != expected.WorkspaceID {
			return 0, common.ErrConflict
		}
		if err := p.repo.Delete(common.ContextWithRuntimeWorkspaceID(ctx, expected.WorkspaceID), item.ID); err != nil {
			return 0, err
		}
		deleted++
	}
	return deleted, nil
}

func waitForEventRecords(t *testing.T, repo repository.EventRecordRepository, query workorchquerycontract.EventRecordQuery, want int) []*eventstreammodel.EventRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		records, err := repo.List(context.Background(), query)
		if err != nil {
			t.Fatalf("list event records: %v", err)
		}
		if len(records) >= want {
			return records
		}
		if time.Now().After(deadline) {
			t.Fatalf("event records = %d, want at least %d", len(records), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (r *failingAppendEventRecordRepository) Append(ctx context.Context, record *eventstreammodel.EventRecord) error {
	r.mu.Lock()
	if r.failures > 0 {
		r.failures--
		err := r.err
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	return r.EventRecordRepository.Append(ctx, record)
}

func newCountingEventRecordRepository() *countingEventRecordRepository {
	return &countingEventRecordRepository{base: memory.NewEventRecordRepository()}
}

func (r *countingEventRecordRepository) Append(ctx context.Context, item *eventstreammodel.EventRecord) error {
	r.mu.Lock()
	r.appendCount++
	r.mu.Unlock()
	return r.base.Append(ctx, item)
}

func (r *countingEventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstreammodel.EventRecord, error) {
	return r.base.Get(ctx, id)
}

func (r *countingEventRecordRepository) List(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	r.mu.Lock()
	r.listCount++
	r.mu.Unlock()
	return r.base.List(ctx, query)
}

func (r *countingEventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	return r.base.Delete(ctx, id)
}

func (r *countingEventRecordRepository) ReserveEventRecordStreamPosition(ctx context.Context) (int64, error) {
	return r.base.ReserveEventRecordStreamPosition(ctx)
}

func (r *countingEventRecordRepository) AppendCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendCount
}

func (r *countingEventRecordRepository) ListCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listCount
}

func (r *countingEventStreamWorkspaceDeletionRepository) Get(ctx context.Context, workspaceID common.WorkspaceID) (*eventstreammodel.WorkspaceDeletion, error) {
	r.mu.Lock()
	r.getCount++
	r.mu.Unlock()
	return r.EventStreamWorkspaceDeletionRepository.Get(ctx, workspaceID)
}

func (r *countingEventStreamWorkspaceDeletionRepository) GetCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getCount
}

func newEventStreamTestService(t *testing.T, repo repository.EventRecordRepository) (*Biz, magicevent.Hub) {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	deletions := memory.NewEventStreamWorkspaceDeletionRepository()
	return New(Dependencies{OwnerRepositories: eventStreamBizTestOwnerRepositories(
		repo, deletions, eventStreamTestPurger{repo: repo}), EventHub: rawHub}), rawHub
}

func TestHighWatermarksFailsClosedBeforeOwnerIOAndDeduplicatesCanonicalIDs(t *testing.T) {
	ctx := context.Background()
	records := newCountingEventRecordRepository()
	deletions := &countingEventStreamWorkspaceDeletionRepository{
		EventStreamWorkspaceDeletionRepository: memory.NewEventStreamWorkspaceDeletionRepository(),
	}
	service := &Biz{repo: records, workspaceDeletions: deletions}

	invalidInputs := [][]common.WorkspaceID{
		{"ws-valid", " "},
		{common.WorkspaceID(string([]byte{0xff}))},
		{common.WorkspaceID(strings.Repeat("x", eventstreamevents.MaxHighWatermarkWorkspaceIDBytes+1))},
	}
	for index, workspaceIDs := range invalidInputs {
		if _, err := service.highWatermarks(ctx, workspaceIDs); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("invalid input %d error = %v, want invalid argument", index, err)
		}
	}
	if records.ListCount() != 0 || deletions.GetCount() != 0 {
		t.Fatalf("invalid input reached owner I/O: event lists=%d deletion gets=%d", records.ListCount(), deletions.GetCount())
	}

	items, err := service.highWatermarks(ctx, []common.WorkspaceID{" ws-b ", "ws-a", "ws-b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].WorkspaceID != "ws-a" || items[1].WorkspaceID != "ws-b" {
		t.Fatalf("high watermarks = %+v, want canonical sorted unique IDs", items)
	}
	if records.ListCount() != 2 || deletions.GetCount() != 2 {
		t.Fatalf("canonical duplicate I/O: event lists=%d deletion gets=%d, want 2/2", records.ListCount(), deletions.GetCount())
	}
}

func TestApplicationQueriesRequireExactSourceBeforeOwnerIO(t *testing.T) {
	ctx := context.Background()
	records := newCountingEventRecordRepository()
	deletions := &countingEventStreamWorkspaceDeletionRepository{
		EventStreamWorkspaceDeletionRepository: memory.NewEventStreamWorkspaceDeletionRepository(),
	}
	service := &Biz{repo: records, workspaceDeletions: deletions}
	for _, testCase := range []struct {
		name    string
		topic   string
		source  string
		payload any
	}{
		{name: "list forged", topic: eventstreamevents.TopicEventStreamCommandList, source: "untrusted/module", payload: eventstreamevents.ListEventsCommand{Query: eventstreamevents.EventQuery{WorkspaceID: "ws-query-source"}}},
		{name: "list noncanonical", topic: eventstreamevents.TopicEventStreamCommandList, source: " application/workorchd ", payload: eventstreamevents.ListEventsCommand{Query: eventstreamevents.EventQuery{WorkspaceID: "ws-query-source"}}},
		{name: "high watermarks forged", topic: eventstreamevents.TopicEventStreamCommandHighWatermarks, source: "application/workorchd/appservice", payload: eventstreamevents.HighWatermarksCommand{WorkspaceIDs: []common.WorkspaceID{"ws-query-source"}}},
		{name: "high watermarks noncanonical", topic: eventstreamevents.TopicEventStreamCommandHighWatermarks, source: " application/workorchd ", payload: eventstreamevents.HighWatermarksCommand{WorkspaceIDs: []common.WorkspaceID{"ws-query-source"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			event := magicevent.NewEventWithContext(testCase.topic, testCase.source, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, testCase.payload)
			if _, err := service.executeCommand(event); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("execute command error = %v, want invalid argument", err)
			}
		})
	}
	if records.ListCount() != 0 || deletions.GetCount() != 0 {
		t.Fatalf("forged query reached owner I/O: event lists=%d deletion gets=%d", records.ListCount(), deletions.GetCount())
	}
}

func sendPurgeWorkspaceCommand(ctx context.Context, rawHub magicevent.Hub, workspaceID common.WorkspaceID) (eventstreamevents.PurgeWorkspaceResult, error) {
	if rawHub == nil || workspaceID == "" {
		return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
	}
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	sealEvent := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion,
		workspaceDeletionCommandSource,
		eventstreamevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.SealWorkspaceEventStreamDeletionCommand{
			WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
			DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		},
	)
	sealResult := rawHub.Send(sealEvent)
	if sealResult == nil || sealResult.Error() != nil {
		if sealResult == nil {
			return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
		}
		return eventstreamevents.PurgeWorkspaceResult{}, sealResult.Error()
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
		workspaceDeletionCommandSource,
		eventstreamevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.PurgeWorkspaceCommand{
			WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
			QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		},
	)
	result := rawHub.Send(event)
	if result == nil {
		return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
	}
	data, err := result.Get()
	if err != nil {
		return eventstreamevents.PurgeWorkspaceResult{}, err
	}
	response, ok := data.(eventstreamevents.PurgeWorkspaceResult)
	if !ok {
		return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func TestServicePersistsTypedDecisionNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	ticket := decisionevents.TicketChangedData{
		NotificationID: "decision-notification/created/ws-event-stream-typed/ticket-event-stream-typed",
		TicketID:       "ticket-event-stream-typed",
		WorkspaceID:    "ws-event-stream-typed",
		RunID:          "run-event-stream-typed",
		StepID:         "run-event-stream-typed/decision",
		Reason:         "select recovery path",
		State:          common.DecisionTicketStateOpen,
	}
	created := magicevent.NewEventWithContext(
		decisionevents.TopicTicketCreated,
		decisioncommon.ModuleID,
		decisionevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		ticket,
	)
	rawHub.Post(created)

	ticket.State = common.DecisionTicketStateResolved
	resolved := magicevent.NewEventWithContext(
		decisionevents.TopicTicketResolved,
		decisioncommon.ModuleID,
		decisionevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		decisionevents.TicketResolvedData{
			NotificationID: "decision-notification/resolved/ws-event-stream-typed/ticket-event-stream-typed/resolution-event-stream-typed",
			Ticket:         ticket,
			ResolutionID:   "resolution-event-stream-typed",
			ResolutionType: decisioncommon.ResolutionTypeReplan,
			ResolutionPayload: []workorcheventpayload.Field{{
				Name: "actor",
				Value: workorcheventpayload.Value{
					Kind:   workorcheventpayload.ValueString,
					String: "operator",
				},
			}},
			ContinuationAction: "replan",
			ResumeRunID:        "run-event-stream-typed",
			SuccessorRunID:     "run-event-stream-successor",
		},
	)
	rawHub.Post(resolved)

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: ticket.WorkspaceID, IncludePayload: true}, 2)
	if len(records) != 2 {
		t.Fatalf("records = %+v, want created and resolved", records)
	}
	if records[0].Topic != decisionevents.TopicTicketCreated || records[0].Revision != 1 {
		t.Fatalf("created record = %+v, want revision 1", records[0])
	}
	if records[1].Topic != decisionevents.TopicTicketResolved || records[1].Revision != 2 {
		t.Fatalf("resolved record = %+v, want revision 2", records[1])
	}
	resolutionType, _ := records[1].Payload.String(recordpayload.PayloadKeyType)
	resolutionHash, _ := records[1].Payload.String(recordpayload.PayloadKeyPayloadSHA256)
	if _, raw := records[1].Payload.Object(recordpayload.PayloadKeyPayload); resolutionType != string(decisioncommon.ResolutionTypeReplan) || raw || resolutionHash == "" {
		t.Fatalf("resolved payload = %+v, want typed metadata and redacted resolution material", records[1].Payload)
	}
}

func TestServicePersistsTypedApprovalNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	data := approvalevents.ApprovalChangedData{
		NotificationID: "approval-notification/resolved/ws-approval-event-stream-typed/run-approval-event-stream-typed/approval-event-stream-typed/resolution-event-stream-typed",
		ResolutionID:   "resolution-event-stream-typed",
		ApprovalID:     "approval-event-stream-typed",
		WorkspaceID:    "ws-approval-event-stream-typed",
		RunID:          "run-approval-event-stream-typed",
		StepID:         "run-approval-event-stream-typed/approval",
		Reason:         "approve workspace write",
		Action:         "apply patch",
		Context: []workorcheventpayload.Field{{
			Name: "risk",
			Value: workorcheventpayload.Value{
				Kind:   workorcheventpayload.ValueString,
				String: "medium",
			},
		}},
		State: common.ApprovalStateApproved,
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice",
		},
	}
	resolved := magicevent.NewEventWithContext(approvalevents.TopicApprovalResolved, approvalcommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	rawHub.Post(resolved)

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID, IncludePayload: true}, 1)
	if len(records) != 1 || records[0].Topic != approvalevents.TopicApprovalResolved {
		t.Fatalf("records = %+v, want approval resolved", records)
	}
	if records[0].Revision != 1 {
		t.Fatalf("approval revision = %d, want 1", records[0].Revision)
	}
	state, _ := records[0].Payload.String(recordpayload.PayloadKeyState)
	contextHash, _ := records[0].Payload.String(recordpayload.PayloadKeyContextSHA256)
	if _, raw := records[0].Payload.Object(recordpayload.PayloadKeyContext); state != string(common.ApprovalStateApproved) || raw || contextHash == "" {
		t.Fatalf("resolved approval payload = %+v", records[0].Payload)
	}
}

func TestServiceDurablyAcknowledgesExactApprovalResolutionReplay(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()
	data := approvalevents.ApprovalChangedData{
		NotificationID: "approval-notification/resolved/ws-a/run-a/approval-a/resolution-a",
		ResolutionID:   "resolution-a", ApprovalID: "approval-a", WorkspaceID: "ws-a", RunID: "run-a",
		Reason: "authorize exact target", Action: "web/search", State: common.ApprovalStateApproved,
		Context:           workorcheventpayload.New(workorcheventpayload.StringField("query", "release notes")).Fields,
		ResolutionContext: workorcheventpayload.New(workorcheventpayload.StringField("note", "reviewed")).Fields,
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice",
		},
	}
	send := func(source string, value approvalevents.ApprovalChangedData) magicevent.Result {
		return rawHub.Send(magicevent.NewEventWithContext(
			eventstreamevents.TopicEventStreamCommandRecordApprovalResolution, source, eventstreamevents.CommandDestination,
			magicevent.NewHeader(), ctx, eventstreamevents.RecordApprovalResolutionCommand{Data: eventstreamevents.ApprovalResolutionFact{
				NotificationID: value.NotificationID, ResolutionID: value.ResolutionID, ApprovalID: value.ApprovalID,
				WorkspaceID: value.WorkspaceID, RunID: value.RunID, StepID: value.StepID, Reason: value.Reason, Action: value.Action,
				Context: value.Context, ResolutionContext: value.ResolutionContext, PrincipalKind: string(value.Principal.Kind),
				PrincipalSubjectID: value.Principal.SubjectID, State: value.State,
			}},
		))
	}
	for attempt := 0; attempt < 2; attempt++ {
		result := send(approvalcommon.ModuleID, data)
		if result == nil || result.Error() != nil {
			t.Fatalf("record attempt %d result = %+v", attempt+1, result)
		}
		value, resultErr := result.Get()
		response, ok := value.(eventstreamevents.RecordApprovalResolutionResult)
		if resultErr != nil || !ok || response.NotificationID != data.NotificationID || response.ResolutionID != data.ResolutionID || response.ApprovalID != data.ApprovalID {
			t.Fatalf("record attempt %d response = %#v err=%v", attempt+1, value, resultErr)
		}
	}
	records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || string(records[0].ID) != data.NotificationID {
		t.Fatalf("records = %+v, want one stable resolution record", records)
	}
	record, err := repo.Get(ctx, common.EventRecordID(data.NotificationID))
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := recordpayload.DecodeApprovalEventPayload(record.Payload)
	if !ok || payload.ResolutionID != data.ResolutionID || payload.ResolvedByKind != string(data.Principal.Kind) || payload.ResolvedBy != data.Principal.SubjectID {
		t.Fatalf("approval audit payload = %+v/%t", payload, ok)
	}
	changed := data
	changed.Context = workorcheventpayload.New(workorcheventpayload.StringField("query", "different target")).Fields
	if result := send(approvalcommon.ModuleID, changed); result == nil || result.Error() == nil {
		t.Fatalf("changed replay result = %+v, want conflict", result)
	}
	if result := send("untrusted/module", data); result == nil || result.Error() == nil {
		t.Fatalf("untrusted record result = %+v, want error", result)
	}
}

func TestServicePersistsTypedGovernanceNotification(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	data := governanceevents.GovernanceData{
		Trace: governancemodel.AutoGovernanceTrace{
			ID:            "trace-event-stream-typed",
			WorkspaceID:   "ws-governance-event-stream-typed",
			RunID:         "run-governance-event-stream-typed",
			StepID:        "run-governance-event-stream-typed/governance",
			ItemType:      "ticket",
			ItemID:        "ticket-event-stream-typed",
			Action:        "escalate",
			RiskLevel:     "L2",
			Confidence:    0.75,
			PolicyBlocked: "manual_required",
			Status:        "escalated",
			Reason:        "human decision required",
		},
		EscalationReason: "manual_required",
	}
	event := magicevent.NewEventWithContext(governanceevents.TopicGovernanceAutoEscalated, governancecommon.ModuleID, governanceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	rawHub.Post(event)

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.Trace.WorkspaceID, IncludePayload: true}, 1)
	if len(records) != 1 || records[0].Topic != governanceevents.TopicGovernanceAutoEscalated || records[0].Revision != 1 {
		t.Fatalf("records = %+v, want governance escalated revision 1", records)
	}
	traceID, _ := records[0].Payload.String(recordpayload.PayloadKeyTraceID)
	escalationReason, _ := records[0].Payload.String(recordpayload.PayloadKeyEscalationReason)
	if traceID != string(data.Trace.ID) || escalationReason != data.EscalationReason {
		t.Fatalf("governance payload = %+v", records[0].Payload)
	}
}

func TestServicePersistsTypedRunStateNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	data := runevents.RunStateChangedData{
		RunID:                  "run-event-stream-state",
		WorkspaceID:            "ws-event-stream-state",
		RunRevision:            3,
		GoalID:                 "goal-event-stream-state",
		DoDID:                  "dod-event-stream-state",
		State:                  common.RunStateBlockedOnDecision,
		BlockingTicketID:       "ticket-event-stream-state",
		FailureReason:          "waiting for decision",
		RuntimeStateRevision:   7,
		RuntimeStateRef:        "runtime-state/ws-event-stream-state/run-event-stream-state/7",
		WorkspaceRunGeneration: 1,
		CausationID:            runevents.RunStateChangedCausationID("run-event-stream-state", 1, 3, 7),
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunStateChanged, runcommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	rawHub.Post(event)

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID, IncludePayload: true}, 1)
	if len(records) != 1 || records[0].Topic != runevents.TopicRunStateChanged || records[0].SourceModule != runcommon.ModuleID || records[0].Revision != 1 {
		t.Fatalf("records = %+v, want typed run state revision 1", records)
	}
	state, _ := records[0].Payload.String(recordpayload.PayloadKeyState)
	blockingTicketID, _ := records[0].Payload.String(recordpayload.PayloadKeyBlockingTicketID)
	if state != string(data.State) || blockingTicketID != string(data.BlockingTicketID) {
		t.Fatalf("run payload = %+v", records[0].Payload)
	}
	runtimeStateRevision, _ := records[0].Payload.Int64(recordpayload.PayloadKeyRuntimeStateRevision)
	runtimeStateRef, _ := records[0].Payload.String(recordpayload.PayloadKeyRuntimeStateRef)
	if uint64(runtimeStateRevision) != data.RuntimeStateRevision ||
		runtimeStateRef != data.RuntimeStateRef {
		t.Fatalf("run runtime payload = %+v", records[0].Payload)
	}
}

func TestServicePersistsTypedWorkspaceNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	workspaceID := common.WorkspaceID("ws-event-stream-workspace-typed")
	items := []struct {
		topic string
		data  any
	}{
		{workspaceevents.TopicWorkspaceCreated, workspaceevents.WorkspaceChangedData{
			WorkspaceID: workspaceID, Title: "typed workspace", DeliverableDirectory: "/tmp/typed", State: common.WorkspaceStateActive,
		}},
		{workspaceevents.TopicGoalDefined, workspaceevents.GoalDefinedData{
			GoalID: "goal-workspace-typed", WorkspaceID: workspaceID, Text: "typed goal", DefinitionRevision: "goal:rev-1", LineageState: "active",
		}},
		{workspaceevents.TopicDoDDefined, workspaceevents.DoDDefinedData{
			DoDID: "dod-workspace-typed", WorkspaceID: workspaceID, Items: []string{"typed acceptance"}, State: common.DoDStateReady, DefinitionRevision: "dod:rev-1", LineageState: "active",
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, "blocks/workspace", workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		rawHub.Post(event)
	}

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true}, 3)
	if len(records) != 3 {
		t.Fatalf("records = %+v, want workspace/goal/dod", records)
	}
	workspaceTitle, _ := records[0].Payload.String(recordpayload.PayloadKeyTitle)
	if records[0].Topic != workspaceevents.TopicWorkspaceCreated || workspaceTitle != "typed workspace" {
		t.Fatalf("workspace record = %+v", records[0])
	}
	goalHash, _ := records[1].Payload.String(recordpayload.PayloadKeyTextSHA256)
	if _, raw := records[1].Payload.String(recordpayload.PayloadKeyText); records[1].Topic != workspaceevents.TopicGoalDefined || raw || goalHash == "" {
		t.Fatalf("goal record = %+v", records[1])
	}
	if records[2].Topic != workspaceevents.TopicDoDDefined {
		t.Fatalf("dod record = %+v", records[2])
	}
}

func TestServicePersistsTypedResultNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	workspaceID := common.WorkspaceID("ws-event-stream-result-typed")
	runID := common.RunID("run-event-stream-result-typed")
	resultID := common.ResultID("result-event-stream-typed")
	items := []struct {
		topic string
		data  any
	}{
		{resultevents.TopicResultAccepted, resultevents.ResultChangedData{
			NotificationID: "result-notification/typed-delivery/typed-material", ResultID: resultID, WorkspaceID: workspaceID, RunID: runID, State: common.ResultStateAccepted, Title: "typed result", EvidenceRefs: []string{"step-001"}, ArtifactRefs: []common.ArtifactID{"artifact-001"},
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, "blocks/result", resultevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		rawHub.Post(event)
	}

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true}, 1)
	if len(records) != 1 {
		t.Fatalf("records = %+v, want result", records)
	}
	resultTitle, _ := records[0].Payload.String(recordpayload.PayloadKeyTitle)
	if records[0].Topic != resultevents.TopicResultAccepted || resultTitle != "typed result" {
		t.Fatalf("result record = %+v", records[0])
	}
}

func TestServiceDeduplicatesStableResultNotificationReplay(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	data := resultevents.ResultChangedData{
		NotificationID: "result-notification/delivery-1/material-1",
		ResultID:       "result-replayed", WorkspaceID: "ws-result-replayed", RunID: "run-result-replayed",
		State: common.ResultStateAccepted, Title: "replayed result",
	}
	for range 2 {
		rawHub.Post(magicevent.NewEventWithContext(
			resultevents.TopicResultAccepted,
			"blocks/result",
			resultevents.NotificationDestination,
			magicevent.NewHeader(),
			ctx,
			data,
		))
	}

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID}, 1)
	time.Sleep(20 * time.Millisecond)
	records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != common.EventRecordID(data.NotificationID) {
		t.Fatalf("replayed records = %+v, want one stable record", records)
	}
}

func TestServiceDeduplicatesStableDecisionNotificationReplay(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	data := decisionevents.TicketResolvedData{
		NotificationID: "decision-notification/resolved/ws-decision-replayed/ticket-decision-replayed/resolution-decision-replayed",
		Ticket: decisionevents.TicketChangedData{
			NotificationID: "decision-notification/created/ws-decision-replayed/ticket-decision-replayed",
			TicketID:       "ticket-decision-replayed", WorkspaceID: "ws-decision-replayed", RunID: "run-decision-replayed",
			State: common.DecisionTicketStateResolved,
		},
		ResolutionID: "resolution-decision-replayed", ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}
	for range 2 {
		rawHub.Post(magicevent.NewEventWithContext(
			decisionevents.TopicTicketResolved,
			decisioncommon.ModuleID,
			decisionevents.NotificationDestination,
			magicevent.NewHeader(),
			ctx,
			data,
		))
	}

	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.Ticket.WorkspaceID}, 1)
	time.Sleep(20 * time.Millisecond)
	records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.Ticket.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != common.EventRecordID(data.NotificationID) {
		t.Fatalf("replayed Decision records = %+v, want one stable record", records)
	}
}

func TestServiceStableOwnerNotificationReplayRejectsSourceTopicAndPayloadDrift(t *testing.T) {
	ctx := context.Background()
	type recordFactory func(source string) *eventstreammodel.EventRecord
	tests := []struct {
		name         string
		topic        string
		source       string
		destination  string
		workspaceID  common.WorkspaceID
		data         func() any
		build        recordFactory
		payloadDrift recordFactory
	}{
		{
			name: "run", topic: runevents.TopicRunStateChanged, source: runcommon.ModuleID, destination: runevents.NotificationDestination, workspaceID: "ws-stable-run",
			data: func() any { return stableRunNotificationTestData() },
			build: func(source string) *eventstreammodel.EventRecord {
				data := stableRunNotificationTestData()
				return runStateRecord(runevents.TopicRunStateChanged, source, data)
			},
			payloadDrift: func(source string) *eventstreammodel.EventRecord {
				data := stableRunNotificationTestData()
				data.FailureReason = "drifted audit payload"
				return runStateRecord(runevents.TopicRunStateChanged, source, data)
			},
		},
		{
			name: "memory recall", topic: memoryevents.TopicMemoryRecallRecorded, source: memorycommon.ModuleID, destination: memoryevents.NotificationDestination, workspaceID: "ws-stable-memory",
			data: func() any { return stableMemoryRecallNotificationTestData() },
			build: func(source string) *eventstreammodel.EventRecord {
				data := stableMemoryRecallNotificationTestData()
				return memoryRecallRecord(source, data)
			},
			payloadDrift: func(source string) *eventstreammodel.EventRecord {
				data := stableMemoryRecallNotificationTestData()
				data.Reason = "drifted audit payload"
				return memoryRecallRecord(source, data)
			},
		},
		{
			name: "LLM usage", topic: llmevents.TopicLLMUsageRecorded, source: llmcommon.ModuleID, destination: llmevents.NotificationDestination, workspaceID: "ws-stable-llm",
			data: func() any { return stableLLMUsageNotificationTestData() },
			build: func(source string) *eventstreammodel.EventRecord {
				data := stableLLMUsageNotificationTestData()
				return llmUsageRecord(source, data)
			},
			payloadDrift: func(source string) *eventstreammodel.EventRecord {
				data := stableLLMUsageNotificationTestData()
				data.Operation = "embeddings"
				return llmUsageRecord(source, data)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newCountingEventRecordRepository()
			service, rawHub := newEventStreamTestService(t, repo)
			if err := service.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(service.Stop)
			eventFor := func(topic, source string) magicevent.Event {
				return magicevent.NewEventWithContext(topic, source, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, struct{}{})
			}

			first := test.build(test.source)
			if first.ID == "" || !hasStableNotificationRecordID(first) {
				t.Fatalf("stable record ID = %q", first.ID)
			}
			for range 2 {
				rawHub.Post(magicevent.NewEventWithContext(
					test.topic, test.source, test.destination, magicevent.NewHeader(), ctx, test.data(),
				))
			}
			waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: test.workspaceID}, 1)
			deadline := time.Now().Add(2 * time.Second)
			for repo.AppendCount() < 2 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			if repo.AppendCount() != 2 {
				t.Fatalf("exact replay append attempts = %d, want 2", repo.AppendCount())
			}
			exactRecords, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: test.workspaceID, IncludePayload: true})
			if err != nil || len(exactRecords) != 1 || exactRecords[0].ID != first.ID {
				t.Fatalf("exact replay records = %+v err=%v, want one stable record", exactRecords, err)
			}
			exactReplay := test.build(test.source)
			if err := service.appendNotificationRecord(eventFor(test.topic, test.source), exactReplay, nil); err != nil {
				t.Fatalf("owner did not acknowledge exact replay: %v", err)
			}
			restartedService, _ := newEventStreamTestService(t, repo)
			if err := restartedService.appendNotificationRecord(eventFor(test.topic, test.source), test.build(test.source), nil); err != nil {
				t.Fatalf("fresh owner did not acknowledge persisted exact replay: %v", err)
			}

			payloadDrift := test.payloadDrift(test.source)
			if payloadDrift.ID != first.ID {
				t.Fatalf("payload drift escaped stable identity: %q != %q", payloadDrift.ID, first.ID)
			}
			if err := service.appendNotificationRecord(eventFor(test.topic, test.source), payloadDrift, nil); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("payload drift error = %v, want conflict", err)
			}

			sourceDrift := test.build("untrusted/module")
			if sourceDrift.ID != first.ID {
				t.Fatalf("source drift escaped stable identity: %q != %q", sourceDrift.ID, first.ID)
			}
			if err := service.appendNotificationRecord(eventFor(test.topic, "untrusted/module"), sourceDrift, nil); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("source drift error = %v, want conflict", err)
			}

			topicDrift := test.build(test.source)
			topicDrift.Topic = test.topic + "/drift"
			if err := service.appendNotificationRecord(eventFor(topicDrift.Topic, test.source), topicDrift, nil); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("topic drift error = %v, want conflict", err)
			}

			var conflictLog bytes.Buffer
			func() {
				previous := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&conflictLog, nil)))
				defer slog.SetDefault(previous)
				service.finishRecordAppend(eventFor(test.topic, test.source), test.payloadDrift(test.source), nil)
			}()
			if !strings.Contains(conflictLog.String(), "workorch eventstream append failed") || !strings.Contains(conflictLog.String(), "conflict") {
				t.Fatalf("stable replay drift was not logged as conflict: %q", conflictLog.String())
			}

			records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: test.workspaceID, IncludePayload: true})
			if err != nil || len(records) != 1 || records[0].ID != first.ID {
				t.Fatalf("persisted stable records = %+v err=%v, want one exact record", records, err)
			}
		})
	}
}

func TestRunStableNotificationIdentitySeparatesSpecificAndCanonicalTopics(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	data := stableRunNotificationTestData()
	specific := runStateRecord(runevents.TopicRunBlockedOnDecision, runcommon.ModuleID, data)
	canonical := runStateRecord(runevents.TopicRunStateChanged, runcommon.ModuleID, data)
	if specific.ID == canonical.ID {
		t.Fatalf("same Run causation collided across legitimate topics: %q", specific.ID)
	}
	if !validRunStateNotification(runevents.TopicRunBlockedOnDecision, data) || !validRunStateNotification(runevents.TopicRunStateChanged, data) {
		t.Fatal("Run owner specific/canonical topic pair was rejected")
	}
	if validRunStateNotification(runevents.TopicRunBlockedOnApproval, data) {
		t.Fatal("Run topic drift was accepted for the same state/causation")
	}
	unknownState := data
	unknownState.State = ""
	if validRunStateNotification(runevents.TopicRunStateChanged, unknownState) {
		t.Fatal("canonical Run topic accepted an unknown state")
	}
	for _, item := range []struct {
		topic  string
		record *eventstreammodel.EventRecord
	}{
		{topic: runevents.TopicRunBlockedOnDecision, record: specific},
		{topic: runevents.TopicRunStateChanged, record: canonical},
	} {
		event := magicevent.NewEventWithContext(item.topic, runcommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
		if err := service.appendNotificationRecord(event, item.record, nil); err != nil {
			t.Fatalf("append %s: %v", item.topic, err)
		}
		replay := runStateRecord(item.topic, runcommon.ModuleID, data)
		if err := service.appendNotificationRecord(event, replay, nil); err != nil {
			t.Fatalf("replay %s: %v", item.topic, err)
		}
	}
	records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID})
	if err != nil || len(records) != 2 || records[0].ID == records[1].ID {
		t.Fatalf("Run specific/canonical records = %+v err=%v", records, err)
	}
	otherWorkspaceID := stableNotificationRecordID(stableRunNotificationRecordIDPrefix, runevents.TopicRunStateChanged, "ws-other", data.RunID, data.CausationID)
	if otherWorkspaceID == canonical.ID {
		t.Fatalf("workspace-local Run identity collided globally: %q", canonical.ID)
	}
	if stableNotificationRecordID(stableRunNotificationRecordIDPrefix, runevents.TopicRunStateChanged, data.WorkspaceID, data.RunID, " ") != "" {
		t.Fatal("blank stable notification identity produced a record ID")
	}
	invalidRecall := stableMemoryRecallNotificationTestData()
	invalidRecall.TraceID = " "
	if validMemoryRecallRecordedData(invalidRecall) {
		t.Fatal("Memory recall accepted a noncanonical blank TraceID")
	}
}

func TestFinishRecordAppendIsNilEventSafe(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	(&Biz{}).finishRecordAppend(nil, nil, common.ErrInvalidArgument)
}

func stableRunNotificationTestData() runevents.RunStateChangedData {
	return runevents.RunStateChangedData{
		WorkspaceID: "ws-stable-run", RunID: "run-stable", WorkspaceRunGeneration: 1, RunRevision: 3,
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-stable", RuntimeStateRevision: 7,
		RuntimeStateRef: "runtime-state/ws-stable-run/run-stable/7",
		CausationID:     runevents.RunStateChangedCausationID("run-stable", 1, 3, 7),
	}
}

func stableMemoryRecallNotificationTestData() memoryevents.MemoryRecallRecordedData {
	return memoryevents.MemoryRecallRecordedData{
		TraceID: "trace-stable", WorkspaceID: "ws-stable-memory", RunID: "run-stable-memory", StepID: "step-stable-memory",
		Stage: "execution", Source: "semantic", Status: "completed", Reason: "stable audit payload", OccurredAt: "2026-08-30T00:00:00Z",
	}
}

func stableLLMUsageNotificationTestData() llmevents.LLMUsageRecordedData {
	return llmevents.LLMUsageRecordedData{
		RequestID: "request-stable", WorkspaceID: "ws-stable-llm", RunID: "run-stable-llm", Operation: "responses",
		Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, OccurredAt: "2026-08-30T00:00:00Z",
	}
}

func TestServicePersistsTypedMemoryNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	workspaceID := common.WorkspaceID("ws-event-stream-memory-typed")
	runID := common.RunID("run-event-stream-memory-typed")
	items := []struct {
		topic string
		data  any
	}{
		{memoryevents.TopicMemoryCreated, memoryevents.MemoryChangedData{
			MemoryID: "memory-event-stream-typed", WorkspaceID: workspaceID, RunID: runID, MemoryType: "working", Summary: "typed memory", Content: "private memory content", Tags: []string{"runtime"}, Importance: 2, Confidence: 3,
		}},
		{memoryevents.TopicMemoryRecallRecorded, memoryevents.MemoryRecallRecordedData{
			TraceID: "recall-event-stream-typed", WorkspaceID: workspaceID, RunID: runID, StepID: "step-001", Stage: "execution", Source: "semantic", Status: "completed", RetrievedMemoryIDs: []common.MemoryID{"memory-event-stream-typed"}, OccurredAt: "2026-07-11T00:00:00Z",
			SelectionEvidence: &workorchmemorycontract.RecallSelectionEvidence{
				Policy: "semantic", Status: "selected", ProviderID: "embedding-provider", ModelID: "embedding-target",
				QueryTextPreview: "goal and DoD", QueryTextSHA256: "sha256-query", MinScore: 0.1, Limit: 100,
				EmbeddingModel: "embedding-model", VectorDimension: 4, RefreshSource: "semantic_vector_search", SelectionPolicyVersion: "activation-memory-selection/v1",
				CandidateMemoryIDs: []common.MemoryID{"memory-event-stream-typed", "memory-excluded"},
				SelectedMemoryIDs:  []common.MemoryID{"memory-event-stream-typed"}, ExcludedMemoryIDs: []common.MemoryID{"memory-excluded"},
				SelectionScores:    []workorchmemorycontract.RecallSelectionScore{{ID: "memory-event-stream-typed", Score: 10, Reason: "selected_within_context_budget"}, {ID: "memory-excluded", Score: 1, Reason: "excluded_type_quota"}},
				RetrievedMemoryIDs: []common.MemoryID{"memory-event-stream-typed"}, RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: "memory-event-stream-typed", Summary: "typed memory"}},
				ContextBudget: workorchmemorycontract.RecallContextBudget{MaxItems: 8, MaxBytes: 16384, MaxEstimatedTokens: 4096, UsedItems: 1, UsedBytes: 12, EstimatedTokens: 3},
			},
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, memorycommon.ModuleID, memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		rawHub.Post(event)
	}
	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true}, 2)
	if len(records) != 2 {
		t.Fatalf("records = %+v, want memory and recall", records)
	}
	var memoryRecord, recallRecord *eventstreammodel.EventRecord
	for _, record := range records {
		switch record.Topic {
		case memoryevents.TopicMemoryCreated:
			memoryRecord = record
		case memoryevents.TopicMemoryRecallRecorded:
			recallRecord = record
		}
	}
	if memoryRecord == nil {
		t.Fatalf("memory records = %+v", records)
	}
	memorySummary, _ := memoryRecord.Payload.String(recordpayload.PayloadKeySummary)
	contentHash, _ := memoryRecord.Payload.String(recordpayload.PayloadKeyContentSHA256)
	if _, raw := memoryRecord.Payload.String(recordpayload.PayloadKeyContent); memorySummary != "typed memory" || raw || contentHash == "" {
		t.Fatalf("memory records = %+v", records)
	}
	if recallRecord == nil || recallRecord.StepID != "step-001" {
		t.Fatalf("recall records = %+v", records)
	}
	selection, ok := recallRecord.Payload.Object(recordpayload.PayloadKeySelectionEvidence)
	if !ok {
		t.Fatalf("recall selection evidence missing: %+v", recallRecord.Payload)
	}
	candidates, ok := selection.StringSlice(recordpayload.PayloadKeyCandidateMemoryIDs)
	if !ok || len(candidates) != 2 || candidates[1] != "memory-excluded" {
		t.Fatalf("recall candidate audit = %#v/%v", candidates, ok)
	}
	budget, ok := selection.Object(recordpayload.PayloadKeyContextBudget)
	if !ok {
		t.Fatalf("recall context budget missing: %+v", selection)
	}
	if usedItems, _ := budget.Int64("usedItems"); usedItems != 1 {
		t.Fatalf("recall usedItems = %d", usedItems)
	}
	queryHash, _ := selection.String(recordpayload.PayloadKeyQueryTextSHA256)
	policyVersion, _ := selection.String(recordpayload.PayloadKeySelectionPolicyVersion)
	vectorDimension, _ := selection.Int64(recordpayload.PayloadKeyVectorDimension)
	if queryHash != "sha256-query" || policyVersion != "activation-memory-selection/v1" || vectorDimension != 4 {
		t.Fatalf("recall selection provenance = hash:%q policy:%q dimension:%d", queryHash, policyVersion, vectorDimension)
	}
}

func TestServiceStartIsIdempotentAndStopReleasesSubscription(t *testing.T) {
	ctx := context.Background()
	repo := newCountingEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	if err := service.Start(); err != nil {
		t.Fatalf("second start event stream service: %v", err)
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCreated, "blocks/run", runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedData{
		WorkspaceID: "ws-event-stream-idempotent", RunID: "run-event-stream-idempotent", RunRevision: 1, State: common.RunStatePending,
		WorkspaceRunGeneration: 1,
		CausationID:            runevents.RunStateChangedCausationID("run-event-stream-idempotent", 1, 1, 0),
	})
	rawHub.Post(event)
	deadline := time.Now().Add(2 * time.Second)
	for repo.AppendCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after double start = %d, want 1", repo.AppendCount())
	}

	service.Stop()
	rawHub.Post(event)
	time.Sleep(20 * time.Millisecond)
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after stop = %d, want 1", repo.AppendCount())
	}
}

func TestPurgeWorkspaceCommandDeletesOnlyWorkspaceRecords(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start eventstream Biz: %v", err)
	}
	t.Cleanup(service.Stop)
	for _, record := range []*eventstreammodel.EventRecord{
		{ID: "event-purge-1", Version: "workorch.event", Topic: runevents.TopicRunCreated, OccurredAt: "2026-08-30T00:00:00Z", SourceModule: runcommon.ModuleID, SubjectType: "run", SubjectID: "run-purge", WorkspaceID: "ws-purge", RunID: "run-purge", StreamPosition: 1, Revision: 1},
		{ID: "event-purge-2", Version: "workorch.event", Topic: runevents.TopicRunStateChanged, OccurredAt: "2026-08-30T00:00:01Z", SourceModule: runcommon.ModuleID, SubjectType: "run", SubjectID: "run-purge", WorkspaceID: "ws-purge", RunID: "run-purge", StreamPosition: 2, Revision: 2},
		{ID: "event-keep", Version: "workorch.event", Topic: runevents.TopicRunCreated, OccurredAt: "2026-08-30T00:00:02Z", SourceModule: runcommon.ModuleID, SubjectType: "run", SubjectID: "run-keep", WorkspaceID: "ws-keep", RunID: "run-keep", StreamPosition: 3, Revision: 1},
	} {
		if err := repo.Append(ctx, record); err != nil {
			t.Fatalf("append event %s: %v", record.ID, err)
		}
	}

	response, err := sendPurgeWorkspaceCommand(ctx, rawHub, "ws-purge")
	if err != nil {
		t.Fatalf("purge workspace events: %v", err)
	}
	if response.Revision != 2 || response.PurgeID == "" || response.PurgeProofHash == "" {
		t.Fatalf("purge response = %+v, want revision 2 exact proof", response)
	}
	records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "event-keep" {
		t.Fatalf("records = %+v, want only event-keep", records)
	}
}

func TestServicePublishesTypedRecordAppendedData(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)

	received := make(chan eventstreamevents.EventRecordAppendedData, 1)
	const observerID = "/eventstream/consumers/blocks/eventstream/test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
		if ok {
			received <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) })

	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	t.Cleanup(service.Stop)

	event := magicevent.NewEventWithContext(runevents.TopicRunCreated, "blocks/run", runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedData{
		WorkspaceID: "ws-typed-appended", RunID: "run-typed-appended", RunRevision: 1, GoalID: "goal-typed-appended", DoDID: "dod-typed-appended", State: common.RunStatePending,
		WorkspaceRunGeneration: 1,
		CausationID:            runevents.RunStateChangedCausationID("run-typed-appended", 1, 1, 0),
	})
	rawHub.Post(event)

	select {
	case data := <-received:
		if data.Record.ID == "" || data.Record.RunID != "run-typed-appended" {
			t.Fatalf("appended data = %+v", data)
		}
		payload := data.Record.Payload.Map()
		if payload[recordpayload.PayloadKeyState] != string(common.RunStatePending) {
			t.Fatalf("typed payload = %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("typed record appended data was not published")
	}
}

func TestServiceLeavesWorkspaceDeletedAsTransportOnlyReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newCountingEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	received := make(chan workspaceevents.WorkspaceDeletedData, 1)
	const observerID = "/workspace/consumers/blocks/eventstream/transport-receipt-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	observer.Subscribe(workspaceevents.TopicWorkspaceDeleted, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.WorkspaceDeletedData); ok {
			received <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceDeleted) })
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)

	data := workspaceevents.WorkspaceDeletedData{
		WorkspaceID:       "ws-deleted-transport-receipt",
		RequestID:         "request-deleted-transport-receipt",
		Mode:              deletioncontract.ModeRetain,
		Disposition:       workspaceevents.DeleteWorkspaceCompleted,
		DeletionRevision:  2,
		WorkspaceRevision: 7,
		CompletedAt:       time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC),
	}
	data.DeletionID = workspaceevents.StableWorkspaceDeletionID(data.WorkspaceID, data.RequestID)
	if err := data.Validate(); err != nil {
		t.Fatalf("workspace deleted transport receipt fixture: %v", err)
	}
	rawHub.Post(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceDeleted, workspacecommon.ModuleID, workspaceevents.NotificationDestination,
		magicevent.NewHeader(), ctx, data,
	))
	select {
	case actual := <-received:
		if actual != data {
			t.Fatalf("workspace deleted receipt = %+v", actual)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace deleted transport receipt was not delivered")
	}
	time.Sleep(50 * time.Millisecond)
	if repo.AppendCount() != 0 {
		t.Fatalf("workspace deleted transport receipt attempted %d durable appends", repo.AppendCount())
	}
	if records, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID}); err != nil || len(records) != 0 {
		t.Fatalf("workspace deleted transport receipt records = %+v err=%v", records, err)
	}
}

func TestServiceDoesNotPublishAppendedWhenRepositoryAppendFails(t *testing.T) {
	ctx := context.Background()
	baseRepo := memory.NewEventRecordRepository()
	repo := &failingAppendEventRecordRepository{EventRecordRepository: baseRepo, failures: 1, err: common.ErrConflict}
	service, rawHub := newEventStreamTestService(t, repo)
	appended := make(chan struct{}, 1)
	observer := magicevent.NewSimpleObserverWithMatchID("/eventstream/test/append-failure", "/eventstream/test/append-failure", rawHub)
	observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(_ magicevent.Event, _ magicevent.Result) {
		appended <- struct{}{}
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) })
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)

	rawHub.Post(magicevent.NewEventWithContext(
		runevents.TopicRunCreated,
		"blocks/run",
		runevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.RunStateChangedData{WorkspaceID: "ws-append-failure", RunID: "run-append-failure", RunRevision: 1, State: common.RunStatePending,
			WorkspaceRunGeneration: 1, CausationID: runevents.RunStateChangedCausationID("run-append-failure", 1, 1, 0)},
	))
	select {
	case <-appended:
		t.Fatal("record appended notification was published for failed repository append")
	case <-time.After(100 * time.Millisecond):
	}
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
		Topic: runevents.TopicRunCreated, SourceModule: "blocks/run", SubjectType: "run", SubjectID: "run-append-failure",
		WorkspaceID: "ws-append-failure", RunID: "run-append-failure",
	}); err != nil {
		t.Fatalf("retry append: %v", err)
	}
	records, err := baseRepo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: "ws-append-failure"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Revision != 1 {
		t.Fatalf("records after retry = %+v, want one record at revision 1", records)
	}
}

func TestServiceRejectsForgedOwnerNotificationSource(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)
	data := workspaceevents.GoalDefinedData{
		GoalID: "goal-authenticated", WorkspaceID: "ws-authenticated", Text: "authority material",
		DefinitionRevision: "goal:00000000000000000001:hash", LineageState: "active",
	}
	post := func(source string) {
		rawHub.Post(magicevent.NewEventWithContext(
			workspaceevents.TopicGoalDefined, source, workspaceevents.NotificationDestination,
			magicevent.NewHeader(), ctx, data,
		))
	}
	post("untrusted/module")
	time.Sleep(20 * time.Millisecond)
	items, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID})
	if err != nil || len(items) != 0 {
		t.Fatalf("forged source records = %+v err=%v, want none", items, err)
	}
	post("blocks/workspace")
	items = waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{WorkspaceID: data.WorkspaceID}, 1)
	if len(items) != 1 || items[0].SourceModule != "blocks/workspace" {
		t.Fatalf("authenticated records = %+v", items)
	}
}

func TestServiceRejectsForgedSourceForEverySubscribedProducerTopic(t *testing.T) {
	ctx := context.Background()
	repository := newCountingEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repository)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)

	workspaceID := common.WorkspaceID("ws-producer-source-matrix")
	runID := common.RunID("run-producer-source-matrix")
	memoryChanged := memoryevents.MemoryChangedData{MemoryID: "memory-source-matrix", WorkspaceID: workspaceID, RunID: runID, MemoryType: "working", Summary: "source matrix"}
	memoryRecall := stableMemoryRecallNotificationTestData()
	memoryRecall.WorkspaceID, memoryRecall.RunID = workspaceID, runID
	llmUsage := stableLLMUsageNotificationTestData()
	llmUsage.WorkspaceID, llmUsage.RunID = workspaceID, runID
	resultChanged := resultevents.ResultChangedData{
		NotificationID: "result-notification/source-matrix", ResultID: "result-source-matrix",
		WorkspaceID: workspaceID, RunID: runID, State: common.ResultStateAccepted,
	}
	workspaceChanged := workspaceevents.WorkspaceChangedData{WorkspaceID: workspaceID, RunID: runID, State: common.WorkspaceStateActive}
	goalDefined := workspaceevents.GoalDefinedData{GoalID: "goal-source-matrix", WorkspaceID: workspaceID, Text: "goal", DefinitionRevision: "goal:1", LineageState: "active"}
	dodDefined := workspaceevents.DoDDefinedData{DoDID: "dod-source-matrix", WorkspaceID: workspaceID, Items: []string{"accept"}, State: common.DoDStateReady, DefinitionRevision: "dod:1", LineageState: "active"}
	governance := governanceevents.GovernanceData{Trace: governancemodel.AutoGovernanceTrace{
		ID: "trace-source-matrix", WorkspaceID: workspaceID, RunID: runID,
		ItemType: "ticket", ItemID: "ticket-source-matrix", Status: "escalated",
	}}
	approval := approvalevents.ApprovalChangedData{
		NotificationID: "approval-notification/source-matrix", ResolutionID: "resolution-source-matrix",
		ApprovalID: "approval-source-matrix", WorkspaceID: workspaceID, RunID: runID,
		State:     common.ApprovalStateApproved,
		Principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "operator-source-matrix"},
	}
	ticket := decisionevents.TicketChangedData{
		NotificationID: "decision-notification/source-matrix", TicketID: "ticket-source-matrix",
		WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen,
	}
	resolvedTicket := ticket
	resolvedTicket.State = common.DecisionTicketStateResolved
	resolution := decisionevents.TicketResolvedData{
		NotificationID: "decision-notification/resolved-source-matrix", Ticket: resolvedTicket,
		ResolutionID: "resolution-source-matrix", ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}

	type notificationCase struct {
		topic       string
		source      string
		destination string
		data        any
	}
	cases := make([]notificationCase, 0, 32)
	for _, topic := range []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted} {
		cases = append(cases, notificationCase{topic, memorycommon.ModuleID, memoryevents.NotificationDestination, memoryChanged})
	}
	cases = append(cases,
		notificationCase{memoryevents.TopicMemoryRecallRecorded, memorycommon.ModuleID, memoryevents.NotificationDestination, memoryRecall},
		notificationCase{llmevents.TopicLLMUsageRecorded, llmcommon.ModuleID, llmevents.NotificationDestination, llmUsage},
	)
	for _, topic := range []string{resultevents.TopicResultCreated, resultevents.TopicResultAccepted, resultevents.TopicResultRejected, resultevents.TopicResultSuperseded} {
		data := resultChanged
		data.NotificationID += topic
		cases = append(cases, notificationCase{topic, resultcommon.ModuleID, resultevents.NotificationDestination, data})
	}
	for _, topic := range []string{workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged} {
		cases = append(cases, notificationCase{topic, workspacecommon.ModuleID, workspaceevents.NotificationDestination, workspaceChanged})
	}
	cases = append(cases,
		notificationCase{workspaceevents.TopicGoalDefined, workspacecommon.ModuleID, workspaceevents.NotificationDestination, goalDefined},
		notificationCase{workspaceevents.TopicDoDDefined, workspacecommon.ModuleID, workspaceevents.NotificationDestination, dodDefined},
	)
	runTopics := []struct {
		topic string
		state common.RunState
	}{
		{runevents.TopicRunCreated, common.RunStatePending},
		{runevents.TopicRunStarted, common.RunStateActive},
		{runevents.TopicRunBlockedOnDecision, common.RunStateBlockedOnDecision},
		{runevents.TopicRunBlockedOnApproval, common.RunStateBlockedOnApproval},
		{runevents.TopicRunCompleted, common.RunStateCompleted},
		{runevents.TopicRunFailed, common.RunStateFailed},
		{runevents.TopicRunCancelled, common.RunStateCancelled},
		{runevents.TopicRunSuperseded, common.RunStateSuperseded},
		{runevents.TopicRunStateChanged, common.RunStateActive},
	}
	for index, item := range runTopics {
		revision := uint64(index + 1)
		data := runevents.RunStateChangedData{
			WorkspaceID: workspaceID, RunID: runID, WorkspaceRunGeneration: 1, RunRevision: revision, State: item.state,
			CausationID: runevents.RunStateChangedCausationID(runID, 1, revision, 0),
		}
		cases = append(cases, notificationCase{item.topic, runcommon.ModuleID, runevents.NotificationDestination, data})
	}
	for _, topic := range []string{
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	} {
		cases = append(cases, notificationCase{topic, governancecommon.ModuleID, governanceevents.NotificationDestination, governance})
	}
	cases = append(cases,
		notificationCase{approvalevents.TopicApprovalResolved, approvalcommon.ModuleID, approvalevents.NotificationDestination, approval},
		notificationCase{decisionevents.TopicTicketCreated, decisioncommon.ModuleID, decisionevents.NotificationDestination, ticket},
		notificationCase{decisionevents.TopicTicketResolved, decisioncommon.ModuleID, decisionevents.NotificationDestination, resolution},
	)

	for _, testCase := range cases {
		rawHub.Send(magicevent.NewEventWithContext(
			testCase.topic, "untrusted/module", testCase.destination,
			magicevent.NewHeader(), ctx, testCase.data,
		))
	}
	if repository.AppendCount() != 0 {
		t.Fatalf("forged producer matrix appended %d records", repository.AppendCount())
	}
	for _, testCase := range cases {
		rawHub.Send(magicevent.NewEventWithContext(
			testCase.topic, testCase.source, testCase.destination,
			magicevent.NewHeader(), ctx, testCase.data,
		))
	}
	if repository.AppendCount() != len(cases) {
		t.Fatalf("exact producer matrix appended %d records, want %d", repository.AppendCount(), len(cases))
	}
}

func TestServiceSanitizesAndBoundsPayloadBeforePersistence(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	secret := "must-not-cross-eventstream"
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
		ID: "event-sanitize", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "test-1",
		WorkspaceID: "ws-sanitize",
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("apiToken", secret),
			workorcheventpayload.StringField("summary", strings.Repeat("x", recordpayload.MaxAuditPayloadBytes)),
		),
	}); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-sanitize"), "event-sanitize")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(item.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > recordpayload.MaxAuditPayloadBytes || strings.Contains(string(encoded), secret) {
		t.Fatalf("sanitized payload bytes=%d payload=%s", len(encoded), encoded)
	}
	redacted, _ := item.Payload.String("apiToken")
	if redacted != "[redacted]" {
		t.Fatalf("secret field = %q, want redacted", redacted)
	}
}

func TestServiceCanonicalizesOccurredAtBeforePersistence(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	record := &eventstreammodel.EventRecord{
		ID: "event-canonical-time", Topic: "/test", OccurredAt: "2026-08-19T19:00:00.5-05:00",
		SourceModule: "blocks/test", SubjectType: "test", SubjectID: "canonical-time",
	}
	if err := service.appendRecord(ctx, record); err != nil {
		t.Fatalf("append offset timestamp: %v", err)
	}
	stored, err := repo.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("get canonical timestamp record: %v", err)
	}
	if stored.OccurredAt != "2026-08-20T00:00:00.5Z" {
		t.Fatalf("stored OccurredAt = %q, want canonical UTC", stored.OccurredAt)
	}
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
		ID: "event-invalid-time", Topic: "/test", OccurredAt: "not-rfc3339",
		SourceModule: "blocks/test", SubjectType: "test", SubjectID: "invalid-time",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid OccurredAt error = %v, want invalid argument", err)
	}
}

func TestServiceRetriesTransientNotificationAppendFailure(t *testing.T) {
	ctx := context.Background()
	baseRepo := memory.NewEventRecordRepository()
	repo := &failingAppendEventRecordRepository{EventRecordRepository: baseRepo, failures: 2, err: errors.New("temporary append failure")}
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)
	rawHub.Post(magicevent.NewEventWithContext(
		runevents.TopicRunCreated, runcommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx,
		runevents.RunStateChangedData{WorkspaceID: "ws-retry", RunID: "run-retry", RunRevision: 1, State: common.RunStatePending,
			WorkspaceRunGeneration: 1, CausationID: runevents.RunStateChangedCausationID("run-retry", 1, 1, 0)},
	))
	items := waitForEventRecords(t, baseRepo, workorchquerycontract.EventRecordQuery{WorkspaceID: "ws-retry"}, 1)
	if len(items) != 1 || items[0].Revision != 1 || items[0].StreamPosition != 1 {
		t.Fatalf("retried records = %+v", items)
	}
}

func TestServiceIssuesGlobalAppendOrderAcrossWorkspaceClockSkew(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	for _, item := range []*eventstreammodel.EventRecord{
		{
			ID: "event-global-1", Topic: "/test", OccurredAt: "2026-08-20T23:59:00Z",
			SourceModule: "blocks/test", SubjectType: "global-order", SubjectID: "global-1", WorkspaceID: "ws-global-a",
		},
		{
			ID: "event-global-2", Topic: "/test", OccurredAt: "2026-08-20T00:00:00Z",
			SourceModule: "blocks/test", SubjectType: "global-order", SubjectID: "global-2", WorkspaceID: "ws-global-b",
		},
	} {
		if err := service.appendRecord(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	items, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{SubjectType: "global-order"})
	if err != nil || len(items) != 2 || items[0].ID != "event-global-1" || items[1].ID != "event-global-2" ||
		items[0].StreamPosition != 1 || items[1].StreamPosition != 2 {
		t.Fatalf("global order = %+v err=%v", items, err)
	}
	after, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{SubjectType: "global-order", CursorAfterID: "event-global-1", Limit: 1})
	if err != nil || len(after) != 1 || after[0].ID != "event-global-2" {
		t.Fatalf("global cursor = %+v err=%v", after, err)
	}
}

func TestServicePurgePermanentlyFencesWorkspaceAndPreservesGlobalPosition(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start eventstream Biz: %v", err)
	}
	defer service.Stop()
	appendWorkspaceRecord := func(id string) {
		if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
			ID: common.EventRecordID(id), Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: id, WorkspaceID: "ws-revision-reset",
		}); err != nil {
			t.Fatal(err)
		}
	}
	appendWorkspaceRecord("event-reset-1")
	appendWorkspaceRecord("event-reset-2")
	response, err := sendPurgeWorkspaceCommand(common.ContextWithRuntimeWorkspaceID(ctx, "ws-revision-reset"), rawHub, "ws-revision-reset")
	if err != nil || response.Revision != 2 {
		t.Fatalf("purge response=%+v err=%v", response, err)
	}
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
		ID: "event-reset-3", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "event-reset-3", WorkspaceID: "ws-revision-reset",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-purge append err = %v, want not found fence", err)
	}
	if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
		ID: "event-global-after-purge", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "event-global-after-purge",
	}); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Get(ctx, "event-global-after-purge")
	if err != nil || item.StreamPosition != 3 {
		t.Fatalf("global record after purge = %+v err=%v, want stream position 3", item, err)
	}
}

func TestLLMUsageScopeAllowsProcessAndWorkspaceBeforeRun(t *testing.T) {
	for _, data := range []llmevents.LLMUsageRecordedData{
		{RequestID: "setup-call", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming},
		{RequestID: "embedding-call", WorkspaceID: "workspace-1", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming},
		{RequestID: "runtime-call", LogicalRequestID: "runtime-logical", PhysicalCallID: "runtime-call", WorkspaceID: "workspace-1", RunID: "run-1", Kind: "runtime_candidate", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming, ProviderSubmitted: true, RequestBytes: 1},
	} {
		if !validLLMUsageRecordedData(data) {
			t.Fatalf("valid LLM usage scope rejected: %+v", data)
		}
	}
	if validLLMUsageRecordedData(llmevents.LLMUsageRecordedData{RequestID: "orphan-run", RunID: "run-1", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming}) {
		t.Fatal("LLM usage accepted Run scope without Workspace")
	}
}

func TestServicePersistsProcessAndWorkspaceScopedLLMUsage(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()

	for _, data := range []llmevents.LLMUsageRecordedData{
		{RequestID: "setup-call", Operation: "responses", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming, ProviderSubmitted: true, RequestBytes: 1, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{RequestID: "embedding-call", WorkspaceID: "workspace-1", Operation: "embeddings", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true, RequestBytes: 1, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{RequestID: "runtime-call", LogicalRequestID: "runtime-logical", PhysicalCallID: "runtime-call", WorkspaceID: "workspace-1", RunID: "run-1", Kind: "runtime_candidate", Operation: "responses", Status: "completed", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true, RequestBytes: 1, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)},
	} {
		rawHub.Post(magicevent.NewEventWithContext(
			llmevents.TopicLLMUsageRecorded, llmcommon.ModuleID, llmevents.NotificationDestination,
			magicevent.NewHeader(), ctx, data,
		))
	}
	records := waitForEventRecords(t, repo, workorchquerycontract.EventRecordQuery{SubjectType: "llm_usage", IncludePayload: true}, 3)
	scopes := map[string]common.WorkspaceID{}
	modes := map[string]string{}
	for _, record := range records {
		scopes[record.SubjectID] = record.WorkspaceID
		modes[record.SubjectID], _ = record.Payload.String(recordpayload.PayloadKeyPhysicalResponseMode)
		logicalRequestID, logicalPresent := record.Payload.String(recordpayload.PayloadKeyLogicalRequestID)
		physicalCallID, physicalPresent := record.Payload.String(recordpayload.PayloadKeyPhysicalCallID)
		if record.SubjectID == "runtime-call" {
			if !logicalPresent || logicalRequestID != "runtime-logical" || !physicalPresent || physicalCallID != "runtime-call" {
				t.Fatalf("runtime request identity = %q/%v %q/%v", logicalRequestID, logicalPresent, physicalCallID, physicalPresent)
			}
		} else if logicalPresent || physicalPresent {
			t.Fatalf("generic usage unexpectedly gained runtime request identity: %+v", record.Payload)
		}
		for _, forbidden := range []string{"rawBody", "content", "prompt", "apiKey", "authorization", "endpoint"} {
			if _, found := record.Payload.Field(forbidden); found {
				t.Fatalf("durable LLM usage record leaked forbidden field %q: %+v", forbidden, record.Payload)
			}
		}
	}
	if scopes["setup-call"] != "" || scopes["embedding-call"] != "workspace-1" || scopes["runtime-call"] != "workspace-1" {
		t.Fatalf("persisted LLM usage scopes = %+v", scopes)
	}
	if modes["setup-call"] != string(llmaudit.PhysicalResponseModeStreaming) || modes["embedding-call"] != string(llmaudit.PhysicalResponseModeNonStreaming) || modes["runtime-call"] != string(llmaudit.PhysicalResponseModeNonStreaming) {
		t.Fatalf("persisted LLM response modes = %+v", modes)
	}
}
