package governance

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"

	magictask "github.com/muidea/magicCommon/task"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustGovernanceServicePayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type governanceDecisionCapture struct {
	mu             sync.Mutex
	ticketID       common.TicketID
	resolutionType string
	payload        workorcheventpayload.Payload
}

func (c *governanceDecisionCapture) recordDecision(ticketID common.TicketID, resolutionType string, payload workorcheventpayload.Payload) {
	c.mu.Lock()
	c.ticketID = ticketID
	c.resolutionType = resolutionType
	c.payload = payload.Clone()
	c.mu.Unlock()
}

func (c *governanceDecisionCapture) completed(ticketID common.TicketID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticketID == ticketID
}

func (c *governanceDecisionCapture) snapshot() (common.TicketID, string, workorcheventpayload.Payload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticketID, c.resolutionType, c.payload.Clone()
}

func publishGovernanceTestTicketCreated(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID, workspaceID common.WorkspaceID, runID common.RunID) error {
	event := magicevent.NewEventWithContext(
		decisionevents.TopicTicketCreated,
		"blocks/decision",
		decisionevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		decisionevents.TicketChangedData{
			TicketID:    ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       common.DecisionTicketStateOpen,
		},
	)
	hub.Post(event)
	return nil
}

type testWorkspaceDefinitions struct {
	goals interface {
		Get(context.Context, common.GoalID) (*workspacemodel.Goal, error)
	}
	dods interface {
		Get(context.Context, common.DoDID) (*workspacemodel.DoD, error)
	}
}

func (r testWorkspaceDefinitions) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	return r.goals.Get(ctx, id)
}

func (r testWorkspaceDefinitions) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	return r.dods.Get(ctx, id)
}

type recordingWorkspaceDefinitions struct {
	inner         testWorkspaceDefinitions
	goalScope     common.WorkspaceID
	dodScope      common.WorkspaceID
	goalReadCount atomic.Int32
	dodReadCount  atomic.Int32
}

func (r *recordingWorkspaceDefinitions) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	r.goalReadCount.Add(1)
	r.goalScope = common.RuntimeWorkspaceIDFromContext(ctx)
	return r.inner.GetGoal(ctx, id)
}

func (r *recordingWorkspaceDefinitions) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	r.dodReadCount.Add(1)
	r.dodScope = common.RuntimeWorkspaceIDFromContext(ctx)
	return r.inner.GetDoD(ctx, id)
}

type countingGovernanceRunReader struct {
	inner                   repository.RunRepository
	getCount                atomic.Int32
	getByWorkspaceIDCount   atomic.Int32
	lastGetByWorkspaceIDRun common.RunID
	lastGetByWorkspaceID    common.WorkspaceID
}

type countingGovernanceTicketReader struct {
	getCount  atomic.Int32
	lastScope common.WorkspaceID
}

func (r *countingGovernanceTicketReader) Get(ctx context.Context, _ common.TicketID) (*decisionmodel.Ticket, error) {
	r.getCount.Add(1)
	r.lastScope = common.RuntimeWorkspaceIDFromContext(ctx)
	return nil, common.ErrNotFound
}

type countingGovernanceApprovalReader struct {
	getCount  atomic.Int32
	lastScope common.WorkspaceID
}

func (r *countingGovernanceApprovalReader) GetScoped(ctx context.Context, _ common.WorkspaceID, _ common.RunID, _ common.ApprovalID) (*approvalmodel.Approval, error) {
	r.getCount.Add(1)
	r.lastScope = common.RuntimeWorkspaceIDFromContext(ctx)
	return nil, common.ErrNotFound
}

type replayGovernanceRunReader struct {
	items []*runmodel.Run
}

func (r replayGovernanceRunReader) Get(context.Context, common.RunID) (*runmodel.Run, error) {
	return nil, common.ErrNotFound
}

func (r replayGovernanceRunReader) GetByWorkspaceID(context.Context, common.WorkspaceID, common.RunID) (*runmodel.Run, error) {
	return nil, common.ErrNotFound
}

func (r replayGovernanceRunReader) List(context.Context) ([]*runmodel.Run, error) {
	return append([]*runmodel.Run(nil), r.items...), nil
}

func (r *countingGovernanceRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingGovernanceRunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	r.getByWorkspaceIDCount.Add(1)
	r.lastGetByWorkspaceID = workspaceID
	r.lastGetByWorkspaceIDRun = id
	return r.inner.GetByWorkspaceID(ctx, workspaceID, id)
}

func (r *countingGovernanceRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

type recordingBackgroundRoutine struct {
	asyncFunctions atomic.Int32
	failAsyncAt    int32
	mu             sync.Mutex
	done           []chan struct{}
}

func (r *recordingBackgroundRoutine) AsyncTask(task magictask.Task) error {
	return r.AsyncFunction(task.Run)
}

func (r *recordingBackgroundRoutine) SyncTask(task magictask.Task) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) SyncTaskWithTimeOut(task magictask.Task, _ time.Duration) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) AsyncFunction(function func()) error {
	call := r.asyncFunctions.Add(1)
	if r.failAsyncAt > 0 && call == r.failAsyncAt {
		return errors.New("injected background registration failure")
	}
	done := make(chan struct{})
	r.mu.Lock()
	r.done = append(r.done, done)
	r.mu.Unlock()
	go func() {
		defer close(done)
		function()
	}()
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunction(function func()) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunctionWithTimeOut(function func(), _ time.Duration) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) Timer(_ context.Context, task magictask.Task, _, _ time.Duration) error {
	return r.AsyncTask(task)
}

func (r *recordingBackgroundRoutine) Shutdown(ctx context.Context) bool {
	r.mu.Lock()
	done := append([]chan struct{}(nil), r.done...)
	r.mu.Unlock()
	for _, item := range done {
		select {
		case <-item:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func TestServiceStartRollsBackEverySubscriptionAndWorkerWhenReconciliationRegistrationFails(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	routine := &recordingBackgroundRoutine{failAsyncAt: 2}
	svc := NewService(servicePorts{
		Tickets:              memory.NewDecisionTicketRepository(),
		Approvals:            memory.NewApprovalRepository(),
		Runs:                 memory.NewRunRepository(),
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: memory.NewGoalRepository(), dods: memory.NewDoDRepository()},
		MemoryItems:          memory.NewMemoryItemRepository(),
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval:      func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:           testGovernanceConfig(true, 0.82, 3),
		NotificationHub:      hub,
		BackgroundRoutine:    routine,
	})
	if err := svc.Start(); err == nil {
		t.Fatal("Start succeeded despite injected reconciliation registration failure")
	}
	if len(svc.topics) != 0 || len(svc.unsubscribes) != 0 || svc.workCtx != nil || svc.workCancel != nil || svc.workQueue != nil || svc.workDone != nil || svc.reconcileCancel != nil || svc.reconcileDone != nil {
		t.Fatalf("failed Start retained lifecycle state: topics=%v unsubscribes=%d workCtx=%v workCancel=%v workQueue=%v workDone=%v reconcileCancel=%v reconcileDone=%v", svc.topics, len(svc.unsubscribes), svc.workCtx, svc.workCancel, svc.workQueue, svc.workDone, svc.reconcileCancel, svc.reconcileDone)
	}
	// The injected failure is one-shot. A retry must establish exactly one fresh
	// worker/reconciliation pair rather than inheriting partial Start state.
	if err := svc.Start(); err != nil {
		t.Fatalf("retry Start after full rollback: %v", err)
	}
	svc.Stop()
	if !routine.Shutdown(ctx) {
		t.Fatal("background routine tasks did not stop after retry")
	}
	hub.Terminate(ctx)
}

func TestServiceUsesBackgroundRoutineForWorkerAndReconciliation(t *testing.T) {
	ctx := context.Background()
	routine := &recordingBackgroundRoutine{}
	svc := NewService(servicePorts{
		Tickets:              memory.NewDecisionTicketRepository(),
		Approvals:            memory.NewApprovalRepository(),
		Runs:                 memory.NewRunRepository(),
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: memory.NewGoalRepository(), dods: memory.NewDoDRepository()},
		MemoryItems:          memory.NewMemoryItemRepository(),
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval:      func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:           testGovernanceConfig(true, 0.82, 3),
		BackgroundRoutine:    routine,
	})
	mustNoErr(t, svc.Start())
	svc.Stop()
	if !routine.Shutdown(ctx) {
		t.Fatal("background routine tasks did not stop")
	}
	if got := routine.asyncFunctions.Load(); got < 2 {
		t.Fatalf("background async functions = %d, want at least worker and reconciliation tasks", got)
	}
}

func TestProcessPendingTicketUsesWorkspaceScopedRunLookup(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-governance-scoped-run")
	runID := common.RunID("run-governance-scoped-run")
	goalID := common.GoalID("goal-governance-scoped-run")
	dodID := common.DoDID("dod-governance-scoped-run")
	ticketID := common.TicketID("ticket-governance-scoped-run")
	ticketRepo := memory.NewDecisionTicketRepository()
	runRepo := memory.NewRunRepository()
	countingRuns := &countingGovernanceRunReader{inner: runRepo}
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	definitions := &recordingWorkspaceDefinitions{inner: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo}}
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		DoDID:       dodID,
		State:       common.RunStateBlockedOnDecision,
	}))
	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "complete the requested outcome"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"final result satisfies the request"}, State: common.DoDStateReady}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "provide missing input",
		reason:      "missing input",
		state:       common.DecisionTicketStateOpen,
	})))
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Runs:                 countingRuns,
		WorkspaceDefinitions: definitions,
		MemoryItems:          memory.NewMemoryItemRepository(),
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval:      func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:           testGovernanceConfig(true, 0.82, 3),
	})

	// Background reconciliation must be given the workspace by the owner fact
	// that scheduled it.  It must never discover scope by reading a ticket ID.
	scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		t.Fatalf("bind workspace context: %v", err)
	}
	svc.processPendingTicket(scopedCtx, ticketID)

	if got := countingRuns.getCount.Load(); got != 0 {
		t.Fatalf("run Get count = %d, want 0 for ticket-scoped governance lookup", got)
	}
	if got := countingRuns.getByWorkspaceIDCount.Load(); got == 0 {
		t.Fatalf("run GetByWorkspaceID count = %d, want scoped ticket governance lookup", got)
	}
	if countingRuns.lastGetByWorkspaceID != workspaceID || countingRuns.lastGetByWorkspaceIDRun != runID {
		t.Fatalf("scoped lookup = (%s, %s), want (%s, %s)", countingRuns.lastGetByWorkspaceID, countingRuns.lastGetByWorkspaceIDRun, workspaceID, runID)
	}
	if definitions.goalReadCount.Load() == 0 || definitions.dodReadCount.Load() == 0 {
		t.Fatalf("definition reads goal=%d dod=%d, want both", definitions.goalReadCount.Load(), definitions.dodReadCount.Load())
	}
	if definitions.goalScope != workspaceID || definitions.dodScope != workspaceID {
		t.Fatalf("definition scopes goal=%s dod=%s, want %s", definitions.goalScope, definitions.dodScope, workspaceID)
	}
}

func TestProcessPendingWorkRejectsUnscopedIdentifierBeforeOwnerIO(t *testing.T) {
	ctx := context.Background()
	tickets := &countingGovernanceTicketReader{}
	approvals := &countingGovernanceApprovalReader{}
	svc := NewService(servicePorts{
		Tickets:    tickets,
		Approvals:  approvals,
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})

	// Ticket and approval identifiers are only workspace-local.  A background
	// worker without an immutable source fact must not turn either into an
	// unscoped read through another owner.
	svc.processPendingTicket(ctx, "ticket-local")
	svc.processPendingApproval(ctx, "run-local", "approval-local")

	if got := tickets.getCount.Load(); got != 0 {
		t.Fatalf("unscoped ticket reads = %d, want 0", got)
	}
	if got := approvals.getCount.Load(); got != 0 {
		t.Fatalf("unscoped approval reads = %d, want 0", got)
	}
}

func TestGovernanceOwnerFactsPropagateWorkspaceScopeToBackgroundWork(t *testing.T) {
	ctx := context.Background()
	tickets := &countingGovernanceTicketReader{}
	approvals := &countingGovernanceApprovalReader{}
	svc := NewService(servicePorts{
		Tickets:              tickets,
		Approvals:            approvals,
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		Runs: replayGovernanceRunReader{items: []*runmodel.Run{
			{ID: "run-ticket", WorkspaceID: "ws-ticket", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-local"},
			{ID: "run-approval", WorkspaceID: "ws-approval", State: common.RunStateBlockedOnApproval, BlockingApprovalID: "approval-local"},
		}},
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})

	// Notification and restart replay both begin from an owner fact containing
	// workspace scope.  The worker may never infer that scope from ticket or
	// approval IDs, which are intentionally only workspace-local.
	svc.processTicketCreated(ctx, decisionevents.TicketChangedData{
		TicketID: "ticket-event", WorkspaceID: "ws-ticket-event", RunID: "run-ticket-event", State: common.DecisionTicketStateOpen,
	})
	approvalCtx, err := common.BindRuntimeWorkspaceContext(ctx, "ws-approval-event")
	if err != nil {
		t.Fatalf("bind approval scope: %v", err)
	}
	svc.processPendingApproval(approvalCtx, "run-approval-event", "approval-event")
	if tickets.lastScope != "ws-ticket-event" || approvals.lastScope != "ws-approval-event" {
		t.Fatalf("notification scopes ticket=%q approval=%q", tickets.lastScope, approvals.lastScope)
	}

	svc.reconcileBlockedRuns(ctx)
	if tickets.lastScope != "ws-ticket" || approvals.lastScope != "ws-approval" {
		t.Fatalf("reconciliation scopes ticket=%q approval=%q", tickets.lastScope, approvals.lastScope)
	}
}

func TestProcessPendingApprovalUsesWorkspaceScopedRunLookup(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-governance-approval-scoped-run")
	runID := common.RunID("run-governance-approval-scoped-run")
	goalID := common.GoalID("goal-governance-approval-scoped-run")
	dodID := common.DoDID("dod-governance-approval-scoped-run")
	approvalID := common.ApprovalID("approval-governance-scoped-run")
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	countingRuns := &countingGovernanceRunReader{inner: runRepo}
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	definitions := &recordingWorkspaceDefinitions{inner: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo}}
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		DoDID:       dodID,
		State:       common.RunStateBlockedOnApproval,
	}))
	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "complete the requested outcome"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"final result satisfies the request"}, State: common.DoDStateReady}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(string(runID) + "/step-1"),
		Reason:      "approval required",
		Action:      "continue requested operation",
		Context:     mustGovernanceServicePayload(t, map[string]any{"risk": "read_only"}),
		State:       common.ApprovalStatePending,
	}))
	svc := NewService(servicePorts{
		Tickets:              memory.NewDecisionTicketRepository(),
		Approvals:            approvalRepo,
		Runs:                 countingRuns,
		WorkspaceDefinitions: definitions,
		MemoryItems:          memory.NewMemoryItemRepository(),
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval:      func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:           testGovernanceConfig(true, 0.82, 3),
	})

	// See the ticket counterpart above: the event/reconciliation source owns
	// workspace scope, not the locally-unique approval identifier.
	scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		t.Fatalf("bind workspace context: %v", err)
	}
	svc.processPendingApproval(scopedCtx, runID, approvalID)

	if got := countingRuns.getCount.Load(); got != 0 {
		t.Fatalf("run Get count = %d, want 0 for approval-scoped governance lookup", got)
	}
	if got := countingRuns.getByWorkspaceIDCount.Load(); got == 0 {
		t.Fatalf("run GetByWorkspaceID count = %d, want scoped approval governance lookup", got)
	}
	if countingRuns.lastGetByWorkspaceID != workspaceID || countingRuns.lastGetByWorkspaceIDRun != runID {
		t.Fatalf("scoped lookup = (%s, %s), want (%s, %s)", countingRuns.lastGetByWorkspaceID, countingRuns.lastGetByWorkspaceIDRun, workspaceID, runID)
	}
	if definitions.goalReadCount.Load() == 0 || definitions.dodReadCount.Load() == 0 {
		t.Fatalf("definition reads goal=%d dod=%d, want both", definitions.goalReadCount.Load(), definitions.dodReadCount.Load())
	}
	if definitions.goalScope != workspaceID || definitions.dodScope != workspaceID {
		t.Fatalf("definition scopes goal=%s dod=%s, want %s", definitions.goalScope, definitions.dodScope, workspaceID)
	}
}

func TestPublishGovernanceEventCarriesTraceScope(t *testing.T) {
	ctx := context.Background()
	hub := newRecordingGovernanceHub(t)
	svc := NewService(servicePorts{NotificationHub: hub})
	trace := &governancemodel.AutoGovernanceTrace{
		ID:                "trace-scope",
		WorkspaceID:       "ws-governance-event-scope",
		RunID:             "run-governance-event-scope",
		StepID:            "run-governance-event-scope/step-1",
		ItemType:          "approval",
		ItemID:            "approval-governance-event-scope",
		Action:            "escalate",
		RiskLevel:         "L2",
		Confidence:        0.91,
		PolicyMatched:     "workspace_write_allowed",
		PolicyBlocked:     "",
		BudgetBefore:      1,
		BudgetAfter:       2,
		PromptFingerprint: "prompt-key",
		Status:            "evaluated",
		Reason:            "scope contract",
	}
	for _, topic := range []string{
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	} {
		if err := svc.publishGovernanceEvent(ctx, topic, trace, "manual_escalation"); err != nil {
			t.Fatalf("publish %s: %v", topic, err)
		}
	}
	published := hub.Posted()
	if len(published) != 4 {
		t.Fatalf("published events = %d, want 4", len(published))
	}
	for _, event := range published {
		data, ok := event.Data().(governanceevents.GovernanceData)
		if !ok {
			t.Fatalf("governance data type = %T", event.Data())
		}
		if data.Trace.WorkspaceID != trace.WorkspaceID || data.Trace.RunID != trace.RunID || data.Trace.StepID != trace.StepID || data.Trace.ID != trace.ID {
			t.Fatalf("governance data missing scope: %+v", data)
		}
		if data.Trace.ItemType != trace.ItemType || data.Trace.ItemID != trace.ItemID || data.EscalationReason != "manual_escalation" {
			t.Fatalf("governance data missing trace refs: %+v", data)
		}
	}
}

func TestPublishGovernanceEventUsesConcreteEventHubNotification(t *testing.T) {
	ctx := context.Background()
	hub := newRecordingGovernanceHub(t)
	svc := NewService(servicePorts{NotificationHub: hub})
	trace := &governancemodel.AutoGovernanceTrace{
		ID:          "trace-async",
		WorkspaceID: "ws-governance-async",
		RunID:       "run-governance-async",
		StepID:      "run-governance-async/step-1",
		ItemType:    "ticket",
		ItemID:      "ticket-governance-async",
		Status:      "evaluated",
	}

	if err := svc.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEvaluated, trace, ""); err != nil {
		t.Fatalf("publish governance event: %v", err)
	}

	published := hub.Posted()
	if len(published) != 1 || published[0].ID() != governanceevents.TopicGovernanceAutoEvaluated {
		t.Fatalf("published = %+v, want governance auto evaluated notification", published)
	}
	if _, ok := published[0].Data().(governanceevents.GovernanceData); !ok {
		t.Fatalf("published data type = %T", published[0].Data())
	}
}

func TestServiceAutoResolvesTicketFromMemoryStructuredContext(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	goalID := common.GoalID("goal-1")
	dodID := common.DoDID("dod-1")
	ticketID := common.TicketID("ticket-1")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "执行配置读取任务"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"输出配置结果"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, memoryRepo.Create(ctx, &memorymodel.MemoryItem{
		ID:          common.MemoryID("memory-target-env"),
		WorkspaceID: workspaceID,
		Summary:     "target_env: staging",
		Content:     "deployment config target_env=staging",
	}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "请提供 target_env",
		reason:      "缺少目标环境",
		state:       common.DecisionTicketStateOpen,
	})))

	capture := &governanceDecisionCapture{}
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(_ context.Context, id common.TicketID, _ string, payload workorcheventpayload.Payload) error {
			capture.recordDecision(id, "provide_missing_info", payload)
			return resolutionRepo.Create(context.Background(), &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id) + "-auto-governance"),
				TicketID: id,
				Type:     "provide_missing_info",
				Payload:  payload.Clone(),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	waitForGovernanceCondition(t, func() bool {
		return capture.completed(ticketID)
	})
	resolvedTicket, _, resolvedPayload := capture.snapshot()

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	if answer, _ := resolvedPayload.String("answer"); answer != "staging" {
		t.Fatalf("resolved answer = %q, want staging", answer)
	}
	if source, _ := resolvedPayload.String("source"); source != "auto_governance" {
		t.Fatalf("resolved source = %q, want auto_governance", source)
	}

	traceID := autoGovernanceTraceID(workspaceID, "ticket", string(ticketID))
	trace, err := traceRepo.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Action != "answer" {
		t.Fatalf("trace status/action = %s/%s, want resolved/answer", trace.Status, trace.Action)
	}
	if trace.PolicyMatched != "goal_dod_memory_structured_inference" {
		t.Fatalf("trace policy matched = %s, want goal_dod_memory_structured_inference", trace.PolicyMatched)
	}
	if len(trace.EvidenceRefs) < 2 {
		t.Fatalf("trace evidence refs = %v, want ticket + goal/memory refs", trace.EvidenceRefs)
	}
}

func TestServiceEscalatesTicketWhenContextInsufficient(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-2")
	runID := common.RunID("run-2")
	goalID := common.GoalID("goal-2")
	dodID := common.DoDID("dod-2")
	ticketID := common.TicketID("ticket-2")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "处理用户请求"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"确认后执行"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "请提供要连接的外部系统地址",
		reason:      "缺少目标地址",
		state:       common.DecisionTicketStateOpen,
	})))

	var resolveCount atomic.Int64
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount.Add(1)
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	traceID := autoGovernanceTraceID(workspaceID, "ticket", string(ticketID))
	trace := waitForGovernanceTrace(t, ctx, traceRepo, traceID, func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})

	if resolveCount.Load() != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount.Load())
	}

	if trace.Status != "escalated" {
		t.Fatalf("trace status = %s, want escalated", trace.Status)
	}
	if trace.PolicyBlocked == "" {
		t.Fatalf("trace policy blocked is empty")
	}
}

func TestServiceEscalatesRuntimeRepairExhaustionTicket(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-repair-auto-replan")
	runID := common.RunID("run-repair-auto-replan")
	goalID := common.GoalID("goal-repair-auto-replan")
	dodID := common.DoDID("dod-repair-auto-replan")
	ticketID := common.TicketID("ticket-repair-auto-replan")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "Complete the requested outcome"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"Final result satisfies DoD"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "Provide guidance, choose replan, or terminate current path.",
		reason:      "runtime repair attempts exhausted after 3 attempts",
		context: map[string]any{
			"recoveryPolicy":         recoveryPolicyRuntimeRepairExhausted,
			"allowedRecoveryActions": []any{"retry_same_step", "replan", "provide_information", "terminate"},
			"failureClass":           "runtime_failed",
			"failureSignature":       "class=runtime_failed|kind=llm/workspace_execution",
		},
		state: common.DecisionTicketStateOpen,
	})))
	capture := &governanceDecisionCapture{}
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(_ context.Context, id common.TicketID, resolutionType string, payload workorcheventpayload.Payload) error {
			capture.recordDecision(id, resolutionType, payload)
			return resolutionRepo.Create(ctx, &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id)),
				TicketID: id,
				Type:     resolutionType,
				Payload:  payload.Clone(),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	waitForGovernanceCondition(t, func() bool {
		_, err := traceRepo.Get(ctx, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)))
		return err == nil
	})
	if capture.completed(ticketID) {
		t.Fatal("runtime repair exhaustion must not auto-resolve to a successor replan")
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "explicit_recovery_strategy_required" {
		t.Fatalf("trace = %+v, want explicit recovery strategy escalation", trace)
	}
}

func TestServiceAutoResolvesReplanDecisionTicketWithoutMissingFields(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-replan-generic")
	runID := common.RunID("run-replan-generic")
	goalID := common.GoalID("goal-replan-generic")
	dodID := common.DoDID("dod-replan-generic")
	ticketID := common.TicketID("ticket-replan-generic")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "完成目标输出"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"完成补丁并通过测试"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "replan",
		question:    "自动修复未满足 DoD，请允许重新规划。",
		reason:      "需要先读取关键文件后再生成补丁。",
		context: map[string]any{
			"missingFields": []any{},
		},
		state: common.DecisionTicketStateOpen,
	})))

	capture := &governanceDecisionCapture{}
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(_ context.Context, id common.TicketID, resolutionType string, payload workorcheventpayload.Payload) error {
			capture.recordDecision(id, resolutionType, payload)
			return resolutionRepo.Create(ctx, &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id)),
				TicketID: id,
				Type:     resolutionType,
				Payload:  payload.Clone(),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	waitForGovernanceCondition(t, func() bool {
		return capture.completed(ticketID)
	})
	resolvedTicket, resolvedResolutionType, resolvedPayload := capture.snapshot()

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	if resolvedResolutionType != "replan" {
		t.Fatalf("resolved type = %s, want replan", resolvedResolutionType)
	}
	answer, _ := resolvedPayload.String("answer")
	recoveryAction, _ := resolvedPayload.String("recoveryAction")
	if answer != "replan" || recoveryAction != "replan" {
		t.Fatalf("resolved payload = %+v, want replan recovery action", resolvedPayload)
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.PolicyMatched != "decision_kind_replan_auto_resolve" {
		t.Fatalf("trace = %+v, want generic replan auto-resolve", trace)
	}
}

func TestServiceEscalatesReplanDecisionTicketWhenMissingFieldsRemain(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-replan-missing")
	runID := common.RunID("run-replan-missing")
	goalID := common.GoalID("goal-replan-missing")
	dodID := common.DoDID("dod-replan-missing")
	ticketID := common.TicketID("ticket-replan-missing")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "完成目标输出"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"完成补丁并通过测试"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "replan",
		question:    "自动修复未满足 DoD，请允许重新规划。",
		reason:      "还缺少必要输入。",
		context: map[string]any{
			"missingFields": []any{"workspaceSource"},
		},
		state: common.DecisionTicketStateOpen,
	})))

	var resolveCount atomic.Int64
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount.Add(1)
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	trace := waitForGovernanceTrace(t, ctx, traceRepo, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)), func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})

	if resolveCount.Load() != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount.Load())
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "missing_replan_fields" {
		t.Fatalf("trace = %+v, want missing_replan_fields escalation", trace)
	}
}

func TestServiceEscalatesRuntimeRepairExhaustionWhenExternalRecoveryRequired(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-repair-provider")
	runID := common.RunID("run-repair-provider")
	goalID := common.GoalID("goal-repair-provider")
	dodID := common.DoDID("dod-repair-provider")
	ticketID := common.TicketID("ticket-repair-provider")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "Complete the requested outcome"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"Final result satisfies DoD"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "Fix or switch the LLM provider in system configuration first.",
		reason:      "runtime repair attempts exhausted after provider retry",
		context: map[string]any{
			"recoveryPolicy":         recoveryPolicyRuntimeRepairExhausted,
			"allowedRecoveryActions": []any{"retry_same_step", "replan", "provide_information", "terminate"},
			"failureClass":           "provider_timeout",
		},
		state: common.DecisionTicketStateOpen,
	})))

	var resolveCount int
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	trace := waitForGovernanceTrace(t, ctx, traceRepo, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)), func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})
	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "external_recovery_required" {
		t.Fatalf("trace = %+v, want external recovery escalation", trace)
	}
}

func TestServiceEscalatesRuntimeRepairExhaustionWhenManualRequired(t *testing.T) {
	ticket := ticket(&ticketFixture{
		id:          common.TicketID("ticket-repair-manual"),
		workspaceID: common.WorkspaceID("ws-repair-manual"),
		runID:       common.RunID("run-repair-manual"),
		kind:        "provide_missing_info",
		question:    "Choose the recovery path.",
		reason:      "runtime repair attempts exhausted after 3 attempts",
		context: map[string]any{
			"recoveryPolicy":         recoveryPolicyRuntimeRepairExhausted,
			"allowedRecoveryActions": []any{"retry_same_step", "replan"},
			"requiresHumanFields":    []any{"operatorDecision"},
		},
		state: common.DecisionTicketStateOpen,
	})
	handled, eval := (*Service)(nil).evaluateRuntimeRepairExhaustionTicket(context.Background(), ticket, nil, ticketEvaluation{
		evidenceRefs: []string{string(ticket.ID)},
	})
	if !handled {
		t.Fatalf("repair exhaustion ticket was not handled")
	}
	if eval.canAutoResolve || eval.policyBlocked != "manual_recovery_required" {
		t.Fatalf("eval = %+v, want manual escalation", eval)
	}
}

func TestServiceAutoResolvesTicketFromStructuredCandidateContext(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-structured-candidate")
	runID := common.RunID("run-structured-candidate")
	goalID := common.GoalID("goal-structured-candidate")
	dodID := common.DoDID("dod-structured-candidate")
	ticketID := common.TicketID("ticket-structured-candidate")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "完成受治理查询"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"参数完整后继续执行"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "请补充 source 和 target",
		reason:      "缺少执行参数",
		context: map[string]any{
			"knownValues": map[string]any{
				"source": "workspace",
			},
			"missingFields": []any{"target"},
			"candidateValues": map[string]any{
				"target": "artifact",
			},
			"confidenceByField": map[string]any{
				"target": 0.93,
			},
		},
		state: common.DecisionTicketStateOpen,
	})))

	capture := &governanceDecisionCapture{}
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(_ context.Context, id common.TicketID, _ string, payload workorcheventpayload.Payload) error {
			capture.recordDecision(id, "provide_missing_info", payload)
			return resolutionRepo.Create(context.Background(), &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id) + "-auto-governance"),
				TicketID: id,
				Type:     "provide_missing_info",
				Payload:  payload.Clone(),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	waitForGovernanceCondition(t, func() bool {
		return capture.completed(ticketID)
	})
	resolvedTicket, _, resolvedPayload := capture.snapshot()

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	answer, _ := resolvedPayload.String("answer")
	if answer != "target=artifact, source=workspace" {
		t.Fatalf("resolved answer = %q, want structured answer", answer)
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Confidence != 0.93 {
		t.Fatalf("trace = %+v, want resolved with candidate confidence", trace)
	}
}

func TestInferMissingInfoAnswerSelectsContinuableCandidate(t *testing.T) {
	answer, confidence, reason, _ := inferMissingInfoAnswer(
		mustGovernanceServicePayload(t, map[string]any{
			"candidateValues": map[string]any{
				"initializationPermission": []any{"允许在空目录从零初始化 Go 网关项目", "不允许初始化，仅等待现有代码同步"},
				"workspaceSource":          []any{"提供现有项目文件到当前 workspace"},
			},
			"missingFields":       []any{"workspaceSource", "initializationPermission"},
			"requiresHumanFields": []any{"workspaceSource", "initializationPermission"},
			"confidenceByField": map[string]any{
				"initializationPermission": 0.93,
				"workspaceSource":          0.95,
			},
		}),
		"请提供下一步工作目标",
		"当前工作区为空",
		"",
		&workspacemodel.DoD{Items: []string{"usage.csv 包含：时间, 模型, 输入Token"}},
		nil,
	)
	if answer != "initializationPermission=允许在空目录从零初始化 Go 网关项目" ||
		confidence != 0.93 ||
		reason != "answer inferred from continuable ticket candidate" {
		t.Fatalf("answer=%q confidence=%v reason=%q, want continuable candidate", answer, confidence, reason)
	}
}

func TestInferMissingInfoAnswerDoesNotUseGoalOrDoDTextForCandidateTicket(t *testing.T) {
	answer, confidence, reason, _ := inferMissingInfoAnswer(
		mustGovernanceServicePayload(t, map[string]any{
			"candidateValues": map[string]any{
				"workspaceSource": []any{"提供现有项目文件到当前 workspace", "提供代码仓库内容/打包文件后再继续"},
			},
			"missingFields":       []any{"workspaceSource"},
			"requiresHumanFields": []any{"workspaceSource"},
		}),
		"请提供 workspaceSource",
		"当前工作区为空",
		"workspaceSource: 从目标描述推断",
		&workspacemodel.DoD{Items: []string{"workspaceSource: 时间"}},
		nil,
	)
	if answer != "" || confidence != 0 || reason != "" {
		t.Fatalf("answer=%q confidence=%v reason=%q, want no guessed answer for candidate ticket", answer, confidence, reason)
	}
}

func TestServiceEscalatesTicketWhenStructuredCandidateRequiresHuman(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-structured-human")
	runID := common.RunID("run-structured-human")
	goalID := common.GoalID("goal-structured-human")
	dodID := common.DoDID("dod-structured-human")
	ticketID := common.TicketID("ticket-structured-human")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "处理目标"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"确认后执行"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "请提供 target",
		reason:      "目标参数缺少",
		context: map[string]any{
			"knownValues": map[string]any{
				"target": "candidate",
			},
			"requiresHumanFields": []any{"target"},
			"confidence":          "high",
		},
		state: common.DecisionTicketStateOpen,
	})))

	var resolveCount int
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	trace := waitForGovernanceTrace(t, ctx, traceRepo, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)), func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})

	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "low_confidence" {
		t.Fatalf("trace = %+v, want low confidence escalation", trace)
	}
}

func TestServiceAutoApprovesReadOnlyApproval(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-3")
	runID := common.RunID("run-3")
	goalID := common.GoalID("goal-3")
	dodID := common.DoDID("dod-3")
	approvalID := common.ApprovalID("approval-1")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "查询配置并返回结果"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"完成只读查询"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-3/step-1"),
		Reason:      "需要确认是否执行只读配置查询",
		Action:      "read configuration and list files",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"risk": "read_only",
		}),
		State: common.ApprovalStatePending,
	}))

	var resolveCount atomic.Int64
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID, _ common.RunID) error {
			resolveCount.Add(1)
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		t.Fatalf("bind approval scope: %v", err)
	}
	svc.processPendingApproval(scopedCtx, runID, approvalID)
	waitForGovernanceCondition(t, func() bool {
		return resolveCount.Load() == 1
	})

	if resolveCount.Load() != 1 {
		t.Fatalf("resolve count = %d, want 1", resolveCount.Load())
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceApprovalTraceID(workspaceID, runID, approvalID))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Action != "approve" {
		t.Fatalf("trace = %+v, want resolved approve", trace)
	}
	if trace.RiskLevel != "L1" || trace.PolicyMatched != "read_only" {
		t.Fatalf("trace risk/policy = %s/%s, want L1/read_only", trace.RiskLevel, trace.PolicyMatched)
	}
}

func TestServiceAutoApprovesWorkspaceWriteOnStartupWhenCreatedEventWasMissed(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-approval-run-state")
	runID := common.RunID("run-approval-run-state")
	goalID := common.GoalID("goal-approval-run-state")
	dodID := common.DoDID("dod-approval-run-state")
	approvalID := common.ApprovalID("approval-run-state")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "update workspace files"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"workspace changes are applied"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
	}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-approval-run-state/step-1"),
		Reason:      "update local workspace file server.conf with a reversible patch",
		Action:      "edit server.conf and config.yaml inside the workspace",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"impactScope": []any{"server.conf", "config.yaml"},
			"riskLevel":   "medium",
		}),
		State: common.ApprovalStatePending,
	}))

	var resolveCount atomic.Int64
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID, _ common.RunID) error {
			resolveCount.Add(1)
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resolveCount.Load() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := resolveCount.Load(); got != 1 {
		t.Fatalf("resolve count = %d, want 1", got)
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceApprovalTraceID(workspaceID, runID, approvalID))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Action != "approve" {
		t.Fatalf("trace = %+v, want resolved approve", trace)
	}
	if trace.RiskLevel != "L2" || trace.PolicyMatched != "workspace_low_risk_write" {
		t.Fatalf("trace risk/policy = %s/%s, want L2/workspace_low_risk_write", trace.RiskLevel, trace.PolicyMatched)
	}
	for _, ref := range trace.EvidenceRefs {
		if ref == "remote_operation" {
			t.Fatalf("trace evidence refs = %v, server.* file names must not imply remote_operation", trace.EvidenceRefs)
		}
	}
}

func TestServiceAutoApprovesLowRiskWorkspaceDependencyMetadataApproval(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-dependency-metadata")
	runID := common.RunID("run-dependency-metadata")
	goalID := common.GoalID("goal-dependency-metadata")
	dodID := common.DoDID("dod-dependency-metadata")
	approvalID := common.ApprovalID("approval-dependency-metadata")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "produce a buildable workspace deliverable"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"workspace deliverable builds"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-dependency-metadata/repair-1"),
		Reason:      "workspace build failed because dependency metadata is missing",
		Action:      "update manifest and checksum files inside the workspace to add the missing dependency metadata, then rerun bounded verification",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"affectedFiles": []any{"manifest", "checksum"},
			"risk":          "low_risk_workspace_dependency_metadata",
		}),
		State: common.ApprovalStatePending,
	}))

	var resolveCount atomic.Int64
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID, _ common.RunID) error {
			resolveCount.Add(1)
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		t.Fatalf("bind approval scope: %v", err)
	}
	svc.processPendingApproval(scopedCtx, runID, approvalID)
	waitForGovernanceCondition(t, func() bool {
		return resolveCount.Load() == 1
	})

	if resolveCount.Load() != 1 {
		t.Fatalf("resolve count = %d, want 1", resolveCount.Load())
	}
	trace, err := traceRepo.Get(ctx, autoGovernanceApprovalTraceID(workspaceID, runID, approvalID))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Action != "approve" {
		t.Fatalf("trace = %+v, want resolved approve", trace)
	}
	if trace.RiskLevel != "L2" || trace.PolicyMatched != "workspace_low_risk_write" {
		t.Fatalf("trace risk/policy = %s/%s, want L2/workspace_low_risk_write", trace.RiskLevel, trace.PolicyMatched)
	}
}

func TestClassifyApprovalRiskTreatsRuntimeDependencyGovernanceAsL2(t *testing.T) {
	approval := &approvalmodel.Approval{
		Action: "update workspace dependency metadata and manifest files to satisfy runtime dependency governance before rerunning bounded validation",
		Reason: "workspace build failed because dependency metadata is missing",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"risk": "low_risk_workspace_dependency_metadata",
			"kind": "patch_approval_required",
		}),
	}
	riskLevel, signals := classifyApprovalRisk(approval)
	if riskLevel != "L2" {
		t.Fatalf("riskLevel = %s, want L2; signals=%v", riskLevel, signals)
	}
	if !containsAnyToken(signals, []string{"workspace_dependency_metadata", "low_risk_workspace_dependency_metadata"}) {
		t.Fatalf("signals = %v, want dependency metadata low-risk signal", signals)
	}
}

func TestServiceStartReconcilesBlockedApprovalAsynchronously(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-start-nonblocking")
	runID := common.RunID("run-start-nonblocking")
	goalID := common.GoalID("goal-start-nonblocking")
	dodID := common.DoDID("dod-start-nonblocking")
	approvalID := common.ApprovalID("approval-start-nonblocking")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "update workspace files"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"workspace changes are applied"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
	}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-start-nonblocking/step-1"),
		Reason:      "update local workspace file server.conf with a reversible patch",
		Action:      "edit server.conf and config.yaml inside the workspace",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"impactScope": []any{"server.conf", "config.yaml"},
			"riskLevel":   "medium",
		}),
		State: common.ApprovalStatePending,
	}))

	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID, _ common.RunID) error {
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})

	startedAt := time.Now()
	mustNoErr(t, svc.Start())
	defer svc.Stop()
	if elapsed := time.Since(startedAt); elapsed > 200*time.Millisecond {
		t.Fatalf("Start elapsed = %v, want <= 200ms", elapsed)
	}

	waitForGovernanceCondition(t, func() bool {
		item, err := approvalRepo.Get(ctx, approvalID)
		return err == nil && item != nil && item.State == common.ApprovalStateApproved
	})
}

func TestServiceEscalatesWorkspaceWriteWhenAutoApprovalPolicyDisallowsIt(t *testing.T) {
	ctx := context.Background()
	approval := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-policy-disabled"),
		WorkspaceID: common.WorkspaceID("ws-policy-disabled"),
		RunID:       common.RunID("run-policy-disabled"),
		Reason:      "update local workspace configuration",
		Action:      "edit config file inside workspace",
		Context:     mustGovernanceServicePayload(t, map[string]any{"riskLevel": "medium"}),
		State:       common.ApprovalStatePending,
	}
	enabled := true
	allowReadOnly := true
	allowWorkspaceWrite := false
	svc := NewService(servicePorts{AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository()})
	eval := svc.evaluateApproval(ctx, approval, nil, nil, workorchconfig.GovernanceConfig{
		AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
			Enabled:             &enabled,
			MinConfidence:       0.9,
			MaxPerRun:           3,
			AllowReadOnly:       &allowReadOnly,
			AllowWorkspaceWrite: &allowWorkspaceWrite,
		},
	})
	if eval.canAutoApprove {
		t.Fatalf("canAutoApprove = true, want false")
	}
	if eval.riskLevel != "L2" || eval.policyBlocked != "workspace_write_auto_disabled" {
		t.Fatalf("eval risk/block = %s/%s, want L2/workspace_write_auto_disabled", eval.riskLevel, eval.policyBlocked)
	}
}

func TestServiceEscalatesHighRiskApproval(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-4")
	runID := common.RunID("run-4")
	goalID := common.GoalID("goal-4")
	dodID := common.DoDID("dod-4")
	approvalID := common.ApprovalID("approval-2")

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "查询系统信息"}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"仅允许只读操作"}, State: common.DoDStateReady}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateActive}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-4/step-1"),
		Reason:      "需要确认是否执行远程写操作",
		Action:      "ssh write config to remote server",
		Context: mustGovernanceServicePayload(t, map[string]any{
			"risk": "ssh_write",
		}),
		State: common.ApprovalStatePending,
	}))

	var resolveCount int
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error {
			resolveCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		t.Fatalf("bind approval scope: %v", err)
	}
	svc.processPendingApproval(scopedCtx, runID, approvalID)
	trace := waitForGovernanceTrace(t, ctx, traceRepo, autoGovernanceApprovalTraceID(workspaceID, runID, approvalID), func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})

	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	if trace.Status != "escalated" || trace.Action != "escalate" {
		t.Fatalf("trace = %+v, want escalated", trace)
	}
	if trace.RiskLevel != "L3" && trace.RiskLevel != "L4" {
		t.Fatalf("trace risk = %s, want L3/L4", trace.RiskLevel)
	}
}

func TestServiceEscalatesTicketWhenRepeatedPromptBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewAutoGovernanceTraceRepository()

	workspaceID := common.WorkspaceID("ws-budget-ticket")
	runID := common.RunID("run-budget-ticket")
	goalID := common.GoalID("goal-budget-ticket")
	dodID := common.DoDID("dod-budget-ticket")
	ticketID := common.TicketID("ticket-budget-ticket")
	question := "请提供 default_test_command"
	reason := "缺少默认测试命令"
	promptKey := compactPromptKey(question + "\n" + reason)

	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "执行代码任务 default_test_command=go test ./...",
	}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"输出默认测试命令"},
		State:       common.DoDStateReady,
	}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		DoDID:       dodID,
		State:       common.RunStateActive,
	}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    question,
		reason:      reason,
		state:       common.DecisionTicketStateOpen,
	})))
	mustNoErr(t, traceRepo.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID:                autoGovernanceTraceID(workspaceID, "ticket", "ticket-budget-previous"),
		WorkspaceID:       workspaceID,
		RunID:             runID,
		ItemType:          "ticket",
		ItemID:            "ticket-budget-previous",
		Action:            "escalate",
		Status:            "escalated",
		PromptFingerprint: promptKey,
		OccurredAt:        nowUTC(),
	}))

	var resolveCount int
	svc := NewService(servicePorts{
		Tickets:              ticketRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig: func() (workorchconfig.Config, error) {
			enabledVal := true
			allowReadOnly := true
			allowWorkspaceWrite := true
			return workorchconfig.Config{
				Governance: workorchconfig.GovernanceConfig{
					AutoTicket: workorchconfig.GovernanceAutoTicketConfig{
						Enabled:       &enabledVal,
						MinConfidence: 0.82,
						MaxPerRun:     5,
					},
					AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
						Enabled:             &enabledVal,
						MinConfidence:       0.9,
						MaxPerRun:           3,
						AllowReadOnly:       &allowReadOnly,
						AllowWorkspaceWrite: &allowWorkspaceWrite,
					},
					Budget: workorchconfig.GovernanceBudgetConfig{
						MaxConsecutiveFailures: 3,
						MaxRepeatedSamePrompt:  1,
					},
				},
			}, nil
		},
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, publishGovernanceTestTicketCreated(ctx, svc.EventHub(), ticketID, workspaceID, runID))
	trace := waitForGovernanceTrace(t, ctx, traceRepo, autoGovernanceTraceID(workspaceID, "ticket", string(ticketID)), func(trace *governancemodel.AutoGovernanceTrace) bool {
		return trace.Status == "escalated"
	})
	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0 when repeated prompt budget exhausted", resolveCount)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "repeated_prompt_exhausted" {
		t.Fatalf("trace = %+v, want repeated prompt exhaustion", trace)
	}
}

type ticketFixture struct {
	id          common.TicketID
	workspaceID common.WorkspaceID
	runID       common.RunID
	kind        string
	question    string
	reason      string
	state       common.DecisionTicketState
	context     map[string]any
}

func ticket(v *ticketFixture) *decisionmodel.Ticket {
	contextPayload, err := workorcheventpayload.FromMap(v.context)
	if err != nil {
		panic(err)
	}
	return &decisionmodel.Ticket{
		ID:           v.id,
		WorkspaceID:  v.workspaceID,
		RunID:        v.runID,
		Reason:       v.reason,
		DecisionKind: v.kind,
		Question:     v.question,
		Context:      contextPayload,
		State:        v.state,
	}
}

func testGovernanceConfig(enabled bool, minConfidence float64, maxPerRun int) func() (workorchconfig.Config, error) {
	return func() (workorchconfig.Config, error) {
		enabledVal := enabled
		allowReadOnly := true
		allowWorkspaceWrite := true
		return workorchconfig.Config{
			Governance: workorchconfig.GovernanceConfig{
				AutoTicket: workorchconfig.GovernanceAutoTicketConfig{
					Enabled:       &enabledVal,
					MinConfidence: minConfidence,
					MaxPerRun:     maxPerRun,
				},
				AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
					Enabled:             &enabledVal,
					MinConfidence:       0.9,
					MaxPerRun:           3,
					AllowReadOnly:       &allowReadOnly,
					AllowWorkspaceWrite: &allowWorkspaceWrite,
					RequireHumanFor:     []string{"ssh_write", "secret_access", "production_operation"},
				},
				Budget: workorchconfig.GovernanceBudgetConfig{
					MaxConsecutiveFailures: 2,
					MaxRepeatedSamePrompt:  1,
				},
			},
		}, nil
	}
}

func waitForGovernanceCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition was not satisfied before timeout")
}

func waitForGovernanceTrace(
	t *testing.T,
	ctx context.Context,
	repo interface {
		Get(context.Context, common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error)
	},
	traceID common.AutoGovernanceTraceID,
	condition func(*governancemodel.AutoGovernanceTrace) bool,
) *governancemodel.AutoGovernanceTrace {
	t.Helper()
	var last *governancemodel.AutoGovernanceTrace
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		trace, err := repo.Get(ctx, traceID)
		if err == nil && trace != nil {
			last = trace
			if condition == nil || condition(trace) {
				return trace
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("trace %s did not satisfy condition before timeout: %+v", traceID, last)
	}
	t.Fatalf("trace %s was not created before timeout", traceID)
	return nil
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordingGovernanceHub struct {
	magicevent.Hub
	mu     sync.Mutex
	posted []magicevent.Event
}

func newRecordingGovernanceHub(t *testing.T) *recordingGovernanceHub {
	t.Helper()
	rawHub := magicevent.NewHub(32)
	hub := &recordingGovernanceHub{Hub: rawHub}
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	return hub
}

func (h *recordingGovernanceHub) Post(event magicevent.Event) {
	h.mu.Lock()
	h.posted = append(h.posted, event)
	h.mu.Unlock()
	h.Hub.Post(event)
}

func (h *recordingGovernanceHub) Posted() []magicevent.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]magicevent.Event(nil), h.posted...)
}
