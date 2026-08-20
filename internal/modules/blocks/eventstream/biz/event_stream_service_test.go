package eventstream

import (
	"context"
	"encoding/json"
	"errors"
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
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runcommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type countingEventRecordRepository struct {
	base        repository.EventRecordRepository
	mu          sync.Mutex
	appendCount int
}

type failingAppendEventRecordRepository struct {
	repository.EventRecordRepository
	mu       sync.Mutex
	failures int
	err      error
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
	return r.base.List(ctx, query)
}

func (r *countingEventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	return r.base.Delete(ctx, id)
}

func (r *countingEventRecordRepository) PurgeWorkspace(ctx context.Context, workspaceID common.WorkspaceID) (int, error) {
	return r.base.PurgeWorkspace(ctx, workspaceID)
}

func (r *countingEventRecordRepository) ReserveEventRecordStreamPosition(ctx context.Context) (int64, error) {
	return r.base.ReserveEventRecordStreamPosition(ctx)
}

func (r *countingEventRecordRepository) AppendCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendCount
}

func newEventStreamTestService(t *testing.T, repo repository.EventRecordRepository) (*Service, magicevent.Hub) {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	return NewService(repo, rawHub, nil), rawHub
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
			approvalevents.TopicApprovalCommandRecordResolution, source, approvalevents.ResolutionRecordCommandDestination,
			magicevent.NewHeader(), ctx, approvalevents.RecordApprovalResolutionCommand{Data: value},
		))
	}
	for attempt := 0; attempt < 2; attempt++ {
		result := send(approvalcommon.ModuleID, data)
		if result == nil || result.Error() != nil {
			t.Fatalf("record attempt %d result = %+v", attempt+1, result)
		}
		value, resultErr := result.Get()
		response, ok := value.(approvalevents.RecordApprovalResolutionResult)
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
		RunID:                "run-event-stream-state",
		WorkspaceID:          "ws-event-stream-state",
		GoalID:               "goal-event-stream-state",
		DoDID:                "dod-event-stream-state",
		State:                common.RunStateBlockedOnDecision,
		BlockingTicketID:     "ticket-event-stream-state",
		FailureReason:        "waiting for decision",
		RuntimeStateRevision: 7,
		RuntimeStateRef:      "runtime-state/ws-event-stream-state/run-event-stream-state/7",
		CausationID:          "run-state/run-event-stream-state/7",
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
		WorkspaceID: "ws-event-stream-idempotent", RunID: "run-event-stream-idempotent", State: common.RunStatePending, CausationID: "run-created/run-event-stream-idempotent",
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

func TestServicePurgeWorkspaceEventsDeletesOnlyWorkspaceRecords(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	for _, record := range []*eventstreammodel.EventRecord{
		{ID: "event-purge-1", Topic: runevents.TopicRunCreated, WorkspaceID: "ws-purge", RunID: "run-purge"},
		{ID: "event-purge-2", Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-purge", RunID: "run-purge"},
		{ID: "event-keep", Topic: runevents.TopicRunCreated, WorkspaceID: "ws-keep", RunID: "run-keep"},
	} {
		if err := repo.Append(ctx, record); err != nil {
			t.Fatalf("append event %s: %v", record.ID, err)
		}
	}

	deleted, err := service.PurgeWorkspaceEvents(ctx, "ws-purge")
	if err != nil {
		t.Fatalf("purge workspace events: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
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
		WorkspaceID: "ws-typed-appended", RunID: "run-typed-appended", GoalID: "goal-typed-appended", DoDID: "dod-typed-appended", State: common.RunStatePending, CausationID: "run-created/run-typed-appended",
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
		runevents.RunStateChangedData{WorkspaceID: "ws-append-failure", RunID: "run-append-failure", State: common.RunStatePending, CausationID: "run-created/run-append-failure"},
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
		runevents.RunStateChangedData{WorkspaceID: "ws-retry", RunID: "run-retry", State: common.RunStatePending, CausationID: "run-created/run-retry"},
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

func TestServicePurgeResetsRevisionFloorConsistentlyWithRestart(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, _ := newEventStreamTestService(t, repo)
	appendWorkspaceRecord := func(id string) {
		if err := service.appendRecord(ctx, &eventstreammodel.EventRecord{
			ID: common.EventRecordID(id), Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: id, WorkspaceID: "ws-revision-reset",
		}); err != nil {
			t.Fatal(err)
		}
	}
	appendWorkspaceRecord("event-reset-1")
	appendWorkspaceRecord("event-reset-2")
	if deleted, err := service.PurgeWorkspaceEvents(common.ContextWithRuntimeWorkspaceID(ctx, "ws-revision-reset"), "ws-revision-reset"); err != nil || deleted != 2 {
		t.Fatalf("purge deleted=%d err=%v", deleted, err)
	}
	appendWorkspaceRecord("event-reset-3")
	item, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-revision-reset"), "event-reset-3")
	if err != nil || item.Revision != 1 || item.StreamPosition != 3 {
		t.Fatalf("post-purge record = %+v err=%v, want revision 1/stream position 3", item, err)
	}
	restarted, _ := newEventStreamTestService(t, repo)
	if err := restarted.appendRecord(ctx, &eventstreammodel.EventRecord{
		ID: "event-reset-4", Topic: "/test", SourceModule: "blocks/test", SubjectType: "test", SubjectID: "event-reset-4", WorkspaceID: "ws-revision-reset",
	}); err != nil {
		t.Fatal(err)
	}
	item, err = repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-revision-reset"), "event-reset-4")
	if err != nil || item.Revision != 2 || item.StreamPosition != 4 {
		t.Fatalf("post-restart record = %+v err=%v, want revision 2/stream position 4", item, err)
	}
}
