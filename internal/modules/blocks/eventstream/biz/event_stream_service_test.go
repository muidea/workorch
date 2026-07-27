package eventstream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

func (r *countingEventRecordRepository) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return r.base.List(ctx, query)
}

func (r *countingEventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	return r.base.Delete(ctx, id)
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
		TicketID:    "ticket-event-stream-typed",
		WorkspaceID: "ws-event-stream-typed",
		RunID:       "run-event-stream-typed",
		StepID:      "run-event-stream-typed/decision",
		Reason:      "select recovery path",
		State:       common.DecisionTicketStateOpen,
	}
	created := magicevent.NewEventWithContext(
		decisionevents.TopicTicketCreated,
		decisioncommon.ModuleID,
		decisionevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		ticket,
	)
	if result := rawHub.Send(created); result == nil || result.Error() != nil {
		t.Fatalf("send typed ticket created: %v", result)
	}

	ticket.State = common.DecisionTicketStateResolved
	resolved := magicevent.NewEventWithContext(
		decisionevents.TopicTicketResolved,
		decisioncommon.ModuleID,
		decisionevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		decisionevents.TicketResolvedData{
			Ticket:         ticket,
			ResolutionID:   "resolution-event-stream-typed",
			ResolutionType: decisioncommon.ResolutionTypeReplan,
			ResolutionPayload: []decisionevents.DecisionEvidenceField{{
				Name: "actor",
				Value: decisionevents.DecisionEvidenceValue{
					Kind:   decisionevents.DecisionEvidenceValueString,
					String: "operator",
				},
			}},
			ContinuationAction: "replan",
			ResumeRunID:        "run-event-stream-typed",
			SuccessorRunID:     "run-event-stream-successor",
		},
	)
	if result := rawHub.Send(resolved); result == nil || result.Error() != nil {
		t.Fatalf("send typed ticket resolved: %v", result)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: ticket.WorkspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed decision records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want created and resolved", records)
	}
	if records[0].Topic != decisionevents.TopicTicketCreated || records[0].Revision != 1 {
		t.Fatalf("created record = %+v, want revision 1", records[0])
	}
	if records[1].Topic != decisionevents.TopicTicketResolved || records[1].Revision != 2 {
		t.Fatalf("resolved record = %+v, want revision 2", records[1])
	}
	resolutionPayload, _ := records[1].Payload.Object(events.PayloadKeyPayload)
	resolutionActor, _ := resolutionPayload.String("actor")
	resolutionType, _ := records[1].Payload.String(events.PayloadKeyType)
	if resolutionType != string(decisioncommon.ResolutionTypeReplan) || resolutionActor != "operator" {
		t.Fatalf("resolved payload = %+v, want typed resolution payload", records[1].Payload)
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
		ApprovalID:  "approval-event-stream-typed",
		WorkspaceID: "ws-approval-event-stream-typed",
		RunID:       "run-approval-event-stream-typed",
		StepID:      "run-approval-event-stream-typed/approval",
		Reason:      "approve workspace write",
		Action:      "apply patch",
		Context: []approvalevents.ApprovalEvidenceField{{
			Name: "risk",
			Value: approvalevents.ApprovalEvidenceValue{
				Kind:   approvalevents.ApprovalEvidenceValueString,
				String: "medium",
			},
		}},
		State: common.ApprovalStatePending,
	}
	created := magicevent.NewEventWithContext(approvalevents.TopicApprovalCreated, approvalcommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	if result := rawHub.Send(created); result == nil || result.Error() != nil {
		t.Fatalf("send typed approval created: %v", result)
	}

	data.State = common.ApprovalStateApproved
	resolved := magicevent.NewEventWithContext(approvalevents.TopicApprovalResolved, approvalcommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	if result := rawHub.Send(resolved); result == nil || result.Error() != nil {
		t.Fatalf("send typed approval resolved: %v", result)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: data.WorkspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed approval records: %v", err)
	}
	if len(records) != 2 || records[0].Topic != approvalevents.TopicApprovalCreated || records[1].Topic != approvalevents.TopicApprovalResolved {
		t.Fatalf("records = %+v, want approval created and resolved", records)
	}
	if records[0].Revision != 1 || records[1].Revision != 2 {
		t.Fatalf("approval revisions = %d/%d, want 1/2", records[0].Revision, records[1].Revision)
	}
	contextPayload, _ := records[1].Payload.Object(events.PayloadKeyContext)
	risk, _ := contextPayload.String("risk")
	state, _ := records[1].Payload.String(events.PayloadKeyState)
	if state != string(common.ApprovalStateApproved) || risk != "medium" {
		t.Fatalf("resolved approval payload = %+v", records[1].Payload)
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
	if result := rawHub.Send(event); result == nil || result.Error() != nil {
		t.Fatalf("send typed governance event: %v", result)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: data.Trace.WorkspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed governance records: %v", err)
	}
	if len(records) != 1 || records[0].Topic != governanceevents.TopicGovernanceAutoEscalated || records[0].Revision != 1 {
		t.Fatalf("records = %+v, want governance escalated revision 1", records)
	}
	traceID, _ := records[0].Payload.String(events.PayloadKeyTraceID)
	escalationReason, _ := records[0].Payload.String(events.PayloadKeyEscalationReason)
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
		RunID:                   "run-event-stream-state",
		WorkspaceID:             "ws-event-stream-state",
		GoalID:                  "goal-event-stream-state",
		DoDID:                   "dod-event-stream-state",
		State:                   common.RunStateBlockedOnDecision,
		BlockingTicketID:        "ticket-event-stream-state",
		FailureReason:           "waiting for decision",
		RuntimeStateRevision:    7,
		RuntimeConvergenceState: runtimecontract.ConvergenceStateNeedGovernance,
		RuntimeAction:           runtimecontract.RuntimeActionRequestGovernance,
		RuntimeFocusID:          "focus-003",
		AuthoritySnapshotID:     "authority-003",
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunStateChanged, "kernel/orchestrator", runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	if result := rawHub.Send(event); result == nil || result.Error() != nil {
		t.Fatalf("send typed run state event: %v", result)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: data.WorkspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed run records: %v", err)
	}
	if len(records) != 1 || records[0].Topic != runevents.TopicRunStateChanged || records[0].SourceModule != "kernel/orchestrator" || records[0].Revision != 1 {
		t.Fatalf("records = %+v, want typed run state revision 1", records)
	}
	state, _ := records[0].Payload.String(events.PayloadKeyState)
	blockingTicketID, _ := records[0].Payload.String(events.PayloadKeyBlockingTicketID)
	if state != string(data.State) || blockingTicketID != string(data.BlockingTicketID) {
		t.Fatalf("run payload = %+v", records[0].Payload)
	}
	runtimeStateRevision, _ := records[0].Payload.Int64(events.PayloadKeyRuntimeStateRevision)
	runtimeState, _ := records[0].Payload.String(events.PayloadKeyRuntimeState)
	runtimeAction, _ := records[0].Payload.String(events.PayloadKeyRuntimeAction)
	runtimeFocusID, _ := records[0].Payload.String(events.PayloadKeyRuntimeFocusID)
	authoritySnapshotID, _ := records[0].Payload.String(events.PayloadKeyAuthoritySnapshotID)
	if uint64(runtimeStateRevision) != data.RuntimeStateRevision ||
		runtimeState != string(data.RuntimeConvergenceState) ||
		runtimeAction != string(data.RuntimeAction) ||
		runtimeFocusID != data.RuntimeFocusID ||
		authoritySnapshotID != data.AuthoritySnapshotID {
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
		if result := rawHub.Send(event); result == nil || result.Error() != nil {
			t.Fatalf("send typed %s: %v", item.topic, result)
		}
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed workspace records: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %+v, want workspace/goal/dod", records)
	}
	workspaceTitle, _ := records[0].Payload.String(events.PayloadKeyTitle)
	if records[0].Topic != workspaceevents.TopicWorkspaceCreated || workspaceTitle != "typed workspace" {
		t.Fatalf("workspace record = %+v", records[0])
	}
	goalText, _ := records[1].Payload.String(events.PayloadKeyText)
	if records[1].Topic != workspaceevents.TopicGoalDefined || goalText != "typed goal" {
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
			ResultID: resultID, WorkspaceID: workspaceID, RunID: runID, State: common.ResultStateAccepted, Title: "typed result", EvidenceRefs: []string{"step-001"}, ArtifactRefs: []common.ArtifactID{"artifact-001"},
		}},
		{resultevents.TopicArtifactCreated, resultevents.ArtifactCreatedData{
			ArtifactID: "artifact-001", WorkspaceID: workspaceID, RunID: runID, ResultID: resultID, StepID: "step-001", ArtifactType: "document", Name: "result.md", State: common.ArtifactStateAvailable,
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, "kernel/orchestrator", resultevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		if result := rawHub.Send(event); result == nil || result.Error() != nil {
			t.Fatalf("send typed %s: %v", item.topic, result)
		}
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed result records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want result and artifact", records)
	}
	resultTitle, _ := records[0].Payload.String(events.PayloadKeyTitle)
	if records[0].Topic != resultevents.TopicResultAccepted || resultTitle != "typed result" {
		t.Fatalf("result record = %+v", records[0])
	}
	artifactName, _ := records[1].Payload.String(events.PayloadKeyName)
	if records[1].Topic != resultevents.TopicArtifactCreated || artifactName != "result.md" {
		t.Fatalf("artifact record = %+v", records[1])
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
			MemoryID: "memory-event-stream-typed", WorkspaceID: workspaceID, RunID: runID, MemoryType: "working", Summary: "typed memory", Tags: []string{"runtime"}, Importance: 2, Confidence: 3,
		}},
		{memoryevents.TopicMemoryRecallRecorded, memoryevents.MemoryRecallRecordedData{
			TraceID: "recall-event-stream-typed", WorkspaceID: workspaceID, RunID: runID, StepID: "step-001", Stage: "execution", Source: "semantic", Status: "completed", RetrievedMemoryIDs: []common.MemoryID{"memory-event-stream-typed"}, OccurredAt: "2026-07-11T00:00:00Z",
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, "workorchd/appservice", memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		if result := rawHub.Send(event); result == nil || result.Error() != nil {
			t.Fatalf("send typed %s: %v", item.topic, result)
		}
	}
	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed memory records: %v", err)
	}
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
	memorySummary, _ := memoryRecord.Payload.String(events.PayloadKeySummary)
	if memorySummary != "typed memory" {
		t.Fatalf("memory records = %+v", records)
	}
	if recallRecord == nil || recallRecord.StepID != "step-001" {
		t.Fatalf("recall records = %+v", records)
	}
}

func TestServicePersistsTypedOrchestratorDetailNotifications(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()
	workspaceID := common.WorkspaceID("ws-event-stream-orchestrator-detail")
	runID := common.RunID("run-event-stream-orchestrator-detail")
	items := []struct {
		topic string
		data  any
	}{
		{orchestratorevents.TopicPatchApplied, orchestratorevents.PatchChangedData{
			PatchID: "patch-detail", WorkspaceID: workspaceID, RunID: runID, StepID: "step-detail", TargetRef: "result.txt", State: common.PatchStateApplied,
		}},
		{orchestratorevents.TopicArtifactContinuationAssembled, orchestratorevents.ArtifactContinuationChangedData{
			ContinuationID: "continuation-detail", WorkspaceID: workspaceID, RunID: runID, StepID: "step-detail", ArtifactID: "artifact-001", TargetRef: "result.txt", State: common.ArtifactContinuationStateAssembling, ReceivedSequences: []int{0, 1}, BytesWritten: 42,
		}},
	}
	for _, item := range items {
		event := magicevent.NewEventWithContext(item.topic, "kernel/orchestrator", orchestratorevents.NotificationDestination, magicevent.NewHeader(), ctx, item.data)
		if result := rawHub.Send(event); result == nil || result.Error() != nil {
			t.Fatalf("send typed %s: %v", item.topic, result)
		}
	}
	records, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list typed orchestrator detail records: %v", err)
	}
	if len(records) != 2 || records[0].Topic != orchestratorevents.TopicPatchApplied || records[1].Topic != orchestratorevents.TopicArtifactContinuationAssembled {
		t.Fatalf("records = %+v", records)
	}
	targetRef, _ := records[0].Payload.String(events.PayloadKeyTargetRef)
	bytesWritten, _ := records[1].Payload.Int64("bytesWritten")
	if targetRef != "result.txt" || bytesWritten != 42 {
		t.Fatalf("detail payloads = %+v / %+v", records[0].Payload, records[1].Payload)
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
		WorkspaceID: "ws-event-stream-idempotent", RunID: "run-event-stream-idempotent", State: common.RunStatePending,
	})
	if result := rawHub.Send(event); result == nil || result.Error() != nil {
		t.Fatalf("send typed run event: %v", result)
	}
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after double start = %d, want 1", repo.AppendCount())
	}

	service.Stop()
	rawHub.Post(event)
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after stop = %d, want 1", repo.AppendCount())
	}
}

func TestServicePersistsPatchEventsFromHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service, rawHub := newEventStreamTestService(t, repo)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	patchEvent := magicevent.NewEventWithContext(orchestratorevents.TopicPatchApplied, "kernel/orchestrator", orchestratorevents.NotificationDestination, magicevent.NewHeader(), ctx, orchestratorevents.PatchChangedData{
		PatchID: "patch-event-stream", WorkspaceID: "ws-event-stream", RunID: "run-event-stream", StepID: "run-event-stream/step-1",
		ToolCallID: "run-event-stream/step-1/tool-call", PatchType: "file_write", TargetRef: "result.txt", Summary: "write result", State: common.PatchStateApplied, AddedLines: 1, ContentHash: "content-sha",
	})
	if result := rawHub.Send(patchEvent); result == nil || result.Error() != nil {
		t.Fatalf("send typed patch event: %v", result)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{RunID: "run-event-stream", IncludePayload: true})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v, want one persisted patch event", records)
	}
	targetRef, _ := records[0].Payload.String(events.PayloadKeyTargetRef)
	if records[0].Topic != orchestratorevents.TopicPatchApplied || targetRef != "result.txt" {
		t.Fatalf("records = %+v, want persisted patch event", records)
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
	records, err := repo.List(ctx, repository.EventRecordQuery{})
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
	observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
		if ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) })

	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	t.Cleanup(service.Stop)

	event := magicevent.NewEventWithContext(runevents.TopicRunCreated, "blocks/run", runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedData{
		WorkspaceID: "ws-typed-appended", RunID: "run-typed-appended", GoalID: "goal-typed-appended", DoDID: "dod-typed-appended", State: common.RunStatePending,
	})
	if result := rawHub.Send(event); result == nil || result.Error() != nil {
		t.Fatalf("send typed run event: %v", result)
	}

	select {
	case data := <-received:
		if data.Record.ID == "" || data.Record.RunID != "run-typed-appended" {
			t.Fatalf("appended data = %+v", data)
		}
		payload := data.Record.Payload.Map()
		if payload[events.PayloadKeyState] != string(common.RunStatePending) {
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
	observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(_ magicevent.Event, result magicevent.Result) {
		appended <- struct{}{}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) })
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Stop)

	result := rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCreated,
		"blocks/run",
		runevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.RunStateChangedData{WorkspaceID: "ws-append-failure", RunID: "run-append-failure", State: common.RunStatePending},
	))
	if result == nil || result.Error() == nil || !errors.Is(result.Error().Cause, common.ErrConflict) {
		t.Fatalf("append failure result = %+v, want conflict", result)
	}
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
	records, err := baseRepo.List(ctx, repository.EventRecordQuery{WorkspaceID: "ws-append-failure"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Revision != 1 {
		t.Fatalf("records after retry = %+v, want one record at revision 1", records)
	}
}
