package governance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestServiceAutoResolvesTicketFromMemoryStructuredContext(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
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

	var resolvedTicket common.TicketID
	var resolvedPayload map[string]any
	var resumedRun common.RunID
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(_ context.Context, id common.TicketID, _ string, payload map[string]any) error {
			resolvedTicket = id
			resolvedPayload = cloneMap(payload)
			return resolutionRepo.Create(context.Background(), &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id) + "-auto-governance"),
				TicketID: id,
				Type:     "provide_missing_info",
				Payload:  cloneMap(payload),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(_ context.Context, id common.RunID) error {
			resumedRun = id
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	if answer, _ := resolvedPayload["answer"].(string); answer != "staging" {
		t.Fatalf("resolved answer = %q, want staging", answer)
	}
	if source, _ := resolvedPayload["source"].(string); source != "auto_governance" {
		t.Fatalf("resolved source = %q, want auto_governance", source)
	}
	if resumedRun != runID {
		t.Fatalf("resumed run = %s, want %s", resumedRun, runID)
	}

	traceID := common.AutoGovernanceTraceID(string(ticketID) + "/auto-governance")
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
	hub := events.NewMemoryHub()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
	mustNoErr(t, ticketRepo.Create(ctx, ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       runID,
		kind:        "provide_missing_info",
		question:    "请提供要连接的外部系统地址",
		reason:      "缺少目标地址",
		state:       common.DecisionTicketStateOpen,
	})))

	var resolveCount int
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          memory.NewDecisionResolutionRepository(),
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(context.Context, common.TicketID, string, map[string]any) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	if resumeCount != 0 {
		t.Fatalf("resume count = %d, want 0", resumeCount)
	}

	traceID := common.AutoGovernanceTraceID(string(ticketID) + "/auto-governance")
	trace, err := traceRepo.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "escalated" {
		t.Fatalf("trace status = %s, want escalated", trace.Status)
	}
	if trace.PolicyBlocked == "" {
		t.Fatalf("trace policy blocked is empty")
	}
}

func TestServiceAutoReplansRuntimeRepairExhaustionTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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
	mustNoErr(t, traceRepo.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID:                common.AutoGovernanceTraceID(string(ticketID) + "/auto-governance"),
		WorkspaceID:       workspaceID,
		RunID:             runID,
		ItemType:          "ticket",
		ItemID:            string(ticketID),
		Action:            "escalate",
		Status:            "escalated",
		PolicyBlocked:     "human_confirmation_required",
		PromptFingerprint: compactPromptKey("Provide guidance, choose replan, or terminate current path.\nruntime repair attempts exhausted after 3 attempts"),
		OccurredAt:        nowUTC(),
	}))

	var resolvedTicket common.TicketID
	var resolvedResolutionType string
	var resolvedPayload map[string]any
	var resumedRun common.RunID
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(_ context.Context, id common.TicketID, resolutionType string, payload map[string]any) error {
			resolvedTicket = id
			resolvedResolutionType = resolutionType
			resolvedPayload = cloneMap(payload)
			return resolutionRepo.Create(ctx, &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id)),
				TicketID: id,
				Type:     resolutionType,
				Payload:  cloneMap(payload),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(_ context.Context, id common.RunID) error {
			resumedRun = id
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	if resolvedResolutionType != "replan" {
		t.Fatalf("resolved type = %s, want replan", resolvedResolutionType)
	}
	if resolvedPayload["answer"] != "replan" || resolvedPayload["recoveryAction"] != "replan" {
		t.Fatalf("resolved payload = %+v, want replan recovery action", resolvedPayload)
	}
	semantic, _ := resolvedPayload["governanceSemantic"].(map[string]any)
	if semantic["intent"] != governancecommon.GovernanceIntentRequestReplan {
		t.Fatalf("semantic = %+v, want request_replan", semantic)
	}
	if resumedRun != runID {
		t.Fatalf("resumed run = %s, want %s", resumedRun, runID)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.PolicyMatched != "runtime_repair_exhausted_auto_replan" {
		t.Fatalf("trace = %+v, want auto replan resolved", trace)
	}
}

func TestServiceAutoResolvesReplanDecisionTicketWithoutMissingFields(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	var resolvedTicket common.TicketID
	var resolvedResolutionType string
	var resolvedPayload map[string]any
	var resumedRun common.RunID
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(_ context.Context, id common.TicketID, resolutionType string, payload map[string]any) error {
			resolvedTicket = id
			resolvedResolutionType = resolutionType
			resolvedPayload = cloneMap(payload)
			return resolutionRepo.Create(ctx, &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id)),
				TicketID: id,
				Type:     resolutionType,
				Payload:  cloneMap(payload),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(_ context.Context, id common.RunID) error {
			resumedRun = id
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	if resolvedResolutionType != "replan" {
		t.Fatalf("resolved type = %s, want replan", resolvedResolutionType)
	}
	if resolvedPayload["answer"] != "replan" || resolvedPayload["recoveryAction"] != "replan" {
		t.Fatalf("resolved payload = %+v, want replan recovery action", resolvedPayload)
	}
	if resumedRun != runID {
		t.Fatalf("resumed run = %s, want %s", resumedRun, runID)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.PolicyMatched != "decision_kind_replan_auto_resolve" {
		t.Fatalf("trace = %+v, want generic replan auto-resolve", trace)
	}
}

func TestServiceEscalatesReplanDecisionTicketWhenMissingFieldsRemain(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	var resolveCount int
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          memory.NewDecisionResolutionRepository(),
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(context.Context, common.TicketID, string, map[string]any) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount != 0 || resumeCount != 0 {
		t.Fatalf("resolve/resume count = %d/%d, want 0/0", resolveCount, resumeCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "missing_replan_fields" {
		t.Fatalf("trace = %+v, want missing_replan_fields escalation", trace)
	}
}

func TestServiceEscalatesRuntimeRepairExhaustionWhenExternalRecoveryRequired(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          memory.NewDecisionResolutionRepository(),
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(context.Context, common.TicketID, string, map[string]any) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	if resolveCount != 0 || resumeCount != 0 {
		t.Fatalf("resolve/resume count = %d/%d, want 0/0", resolveCount, resumeCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
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

func TestServiceEscalatesRuntimeRepairExhaustionAfterAutoReplanLineage(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-replan-lineage-budget")
	predecessorID := common.RunID("run-replan-lineage-budget")
	successorID := common.RunID("run-replan-lineage-budget-replan-001")
	ticketID := common.TicketID("ticket-replan-lineage-budget")

	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:          predecessorID,
		WorkspaceID: workspaceID,
		State:       common.RunStateSuperseded,
	}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID:          successorID,
		WorkspaceID: workspaceID,
		State:       common.RunStateBlockedOnDecision,
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId":  string(predecessorID),
				"resolutionType":    "replan",
				"resolutionPayload": map[string]any{"recoveryAction": "replan"},
			},
		},
	}))
	ticket := ticket(&ticketFixture{
		id:          ticketID,
		workspaceID: workspaceID,
		runID:       successorID,
		kind:        "provide_missing_info",
		question:    "Choose the recovery path.",
		reason:      "runtime repair attempts exhausted after automatic replan",
		context: map[string]any{
			"recoveryPolicy":         recoveryPolicyRuntimeRepairExhausted,
			"allowedRecoveryActions": []any{"retry_same_step", "replan", "provide_information", "terminate"},
		},
		state: common.DecisionTicketStateOpen,
	})
	svc := &Service{runs: runRepo}

	handled, eval := svc.evaluateRuntimeRepairExhaustionTicket(ctx, ticket, &runmodel.Run{
		ID:          successorID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId":  string(predecessorID),
				"resolutionType":    "replan",
				"resolutionPayload": map[string]any{"recoveryAction": "replan"},
			},
		},
	}, ticketEvaluation{evidenceRefs: []string{string(ticketID)}})
	if !handled {
		t.Fatalf("repair exhaustion ticket was not handled")
	}
	if eval.canAutoResolve || eval.policyBlocked != "replan_lineage_budget_exhausted" {
		t.Fatalf("eval = %+v, want replan lineage budget escalation", eval)
	}
}

func TestServiceAutoResolvesTicketFromStructuredCandidateContext(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
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

	var resolvedTicket common.TicketID
	var resolvedPayload map[string]any
	var resumedRun common.RunID
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(_ context.Context, id common.TicketID, _ string, payload map[string]any) error {
			resolvedTicket = id
			resolvedPayload = cloneMap(payload)
			return resolutionRepo.Create(context.Background(), &decisionmodel.Resolution{
				ID:       common.ResolutionID("resolution-" + string(id) + "-auto-governance"),
				TicketID: id,
				Type:     "provide_missing_info",
				Payload:  cloneMap(payload),
			})
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(_ context.Context, id common.RunID) error {
			resumedRun = id
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolvedTicket != ticketID {
		t.Fatalf("resolved ticket = %s, want %s", resolvedTicket, ticketID)
	}
	answer, _ := resolvedPayload["answer"].(string)
	if answer != "target=artifact, source=workspace" {
		t.Fatalf("resolved answer = %q, want structured answer", answer)
	}
	if resumedRun != runID {
		t.Fatalf("resumed run = %s, want %s", resumedRun, runID)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "resolved" || trace.Confidence != 0.93 {
		t.Fatalf("trace = %+v, want resolved with candidate confidence", trace)
	}
}

func TestInferMissingInfoAnswerSelectsContinuableCandidate(t *testing.T) {
	answer, confidence, reason, _ := inferMissingInfoAnswer(
		map[string]any{
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
		},
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
		map[string]any{
			"candidateValues": map[string]any{
				"workspaceSource": []any{"提供现有项目文件到当前 workspace", "提供代码仓库内容/打包文件后再继续"},
			},
			"missingFields":       []any{"workspaceSource"},
			"requiresHumanFields": []any{"workspaceSource"},
		},
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
	hub := events.NewMemoryHub()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
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
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          memory.NewDecisionResolutionRepository(),
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(context.Context, common.TicketID, string, map[string]any) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun:       func(context.Context, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.Status != "escalated" || trace.PolicyBlocked != "low_confidence" {
		t.Fatalf("trace = %+v, want low confidence escalation", trace)
	}
}

func TestServiceAutoApprovesReadOnlyApproval(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-3/step-1"),
		Reason:      "需要确认是否执行只读配置查询",
		Action:      "read configuration and list files",
		Context: map[string]any{
			"risk": "read_only",
		},
		State: common.ApprovalStatePending,
	}))

	var resolveCount int
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision:      func(context.Context, common.TicketID, string, map[string]any) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID) error {
			resolveCount++
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   string(approvalID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", resolveCount)
	}
	if resumeCount != 1 {
		t.Fatalf("resume count = %d, want 1", resumeCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(approvalID)+"/auto-governance"))
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
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
		Context: map[string]any{
			"impactScope": []any{"server.conf", "config.yaml"},
			"riskLevel":   "medium",
		},
		State: common.ApprovalStatePending,
	}))

	var resolveCount atomic.Int64
	var resumeCount atomic.Int64
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision:      func(context.Context, common.TicketID, string, map[string]any) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID) error {
			resolveCount.Add(1)
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount.Add(1)
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resolveCount.Load() == 1 && resumeCount.Load() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := resolveCount.Load(); got != 1 {
		t.Fatalf("resolve count = %d, want 1", got)
	}
	if got := resumeCount.Load(); got != 1 {
		t.Fatalf("resume count = %d, want 1", got)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(approvalID)+"/auto-governance"))
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
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-dependency-metadata/repair-1"),
		Reason:      "workspace build failed because dependency metadata is missing",
		Action:      "update manifest and checksum files inside the workspace to add the missing dependency metadata, then rerun bounded verification",
		Context: map[string]any{
			"affectedFiles": []any{"manifest", "checksum"},
			"risk":          "low_risk_workspace_dependency_metadata",
		},
		State: common.ApprovalStatePending,
	}))

	var resolveCount atomic.Int64
	var resumeCount atomic.Int64
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision:      func(context.Context, common.TicketID, string, map[string]any) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID) error {
			resolveCount.Add(1)
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount.Add(1)
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   string(approvalID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount.Load() != 1 {
		t.Fatalf("resolve count = %d, want 1", resolveCount.Load())
	}
	if resumeCount.Load() != 1 {
		t.Fatalf("resume count = %d, want 1", resumeCount.Load())
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(approvalID)+"/auto-governance"))
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
		Context: map[string]any{
			"risk": "low_risk_workspace_dependency_metadata",
			"kind": "patch_approval_required",
		},
	}
	riskLevel, signals := classifyApprovalRisk(approval)
	if riskLevel != "L2" {
		t.Fatalf("riskLevel = %s, want L2; signals=%v", riskLevel, signals)
	}
	if !containsAnyToken(signals, []string{"workspace_dependency_metadata", "low_risk_workspace_dependency_metadata"}) {
		t.Fatalf("signals = %v, want dependency metadata low-risk signal", signals)
	}
}

func TestServiceStartReturnsWithoutWaitingForBlockedRunResume(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
		Context: map[string]any{
			"impactScope": []any{"server.conf", "config.yaml"},
			"riskLevel":   "medium",
		},
		State: common.ApprovalStatePending,
	}))

	resumeStarted := make(chan struct{}, 1)
	releaseResume := make(chan struct{})
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision:      func(context.Context, common.TicketID, string, map[string]any) error { return nil },
		ResolveApproval: func(_ context.Context, id common.ApprovalID) error {
			item, err := approvalRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			return approvalRepo.Update(ctx, item)
		},
		ResumeRun: func(context.Context, common.RunID) error {
			select {
			case resumeStarted <- struct{}{}:
			default:
			}
			<-releaseResume
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})

	startedAt := time.Now()
	mustNoErr(t, svc.Start())
	defer func() {
		close(releaseResume)
		svc.Stop()
	}()
	if elapsed := time.Since(startedAt); elapsed > 200*time.Millisecond {
		t.Fatalf("Start elapsed = %v, want <= 200ms", elapsed)
	}

	select {
	case <-resumeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("background reconcile did not attempt resume")
	}
}

func TestServiceEscalatesWorkspaceWriteWhenAutoApprovalPolicyDisallowsIt(t *testing.T) {
	ctx := context.Background()
	approval := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-policy-disabled"),
		WorkspaceID: common.WorkspaceID("ws-policy-disabled"),
		RunID:       common.RunID("run-policy-disabled"),
		Reason:      "update local workspace configuration",
		Action:      "edit config file inside workspace",
		Context:     map[string]any{"riskLevel": "medium"},
		State:       common.ApprovalStatePending,
	}
	enabled := true
	allowReadOnly := true
	allowWorkspaceWrite := false
	svc := NewService(Dependencies{AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository()})
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
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateRunning}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-4/step-1"),
		Reason:      "需要确认是否执行远程写操作",
		Action:      "ssh write config to remote server",
		Context: map[string]any{
			"risk": "ssh_write",
		},
		State: common.ApprovalStatePending,
	}))

	var resolveCount int
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision:      func(context.Context, common.TicketID, string, map[string]any) error { return nil },
		ResolveApproval: func(context.Context, common.ApprovalID) error {
			resolveCount++
			return nil
		},
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 5),
	})
	mustNoErr(t, svc.Start())
	defer svc.Stop()

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   string(approvalID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))

	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", resolveCount)
	}
	if resumeCount != 0 {
		t.Fatalf("resume count = %d, want 0", resumeCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(approvalID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
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
	hub := events.NewMemoryHub()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
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
		State:       common.RunStateRunning,
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
		ID:                common.AutoGovernanceTraceID("ticket-budget-previous/auto-governance"),
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
	var resumeCount int
	svc := NewService(Dependencies{
		Tickets:              ticketRepo,
		Resolutions:          resolutionRepo,
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		Goals:                goalRepo,
		DoDs:                 dodRepo,
		MemoryItems:          memoryRepo,
		AutoGovernanceTraces: traceRepo,
		EventHub:             hub,
		ResolveDecision: func(context.Context, common.TicketID, string, map[string]any) error {
			resolveCount++
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID) error { return nil },
		ResumeRun: func(context.Context, common.RunID) error {
			resumeCount++
			return nil
		},
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

	mustNoErr(t, hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectType: "ticket",
		SubjectID:   string(ticketID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	if resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0 when repeated prompt budget exhausted", resolveCount)
	}
	if resumeCount != 0 {
		t.Fatalf("resume count = %d, want 0 when repeated prompt budget exhausted", resumeCount)
	}
	trace, err := traceRepo.Get(ctx, common.AutoGovernanceTraceID(string(ticketID)+"/auto-governance"))
	if err != nil {
		t.Fatalf("get trace: %v", err)
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
	return &decisionmodel.Ticket{
		ID:           v.id,
		WorkspaceID:  v.workspaceID,
		RunID:        v.runID,
		Reason:       v.reason,
		DecisionKind: v.kind,
		Question:     v.question,
		Context:      cloneMap(v.context),
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

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
