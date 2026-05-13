package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type countingAppDecisionTicketRepository struct {
	base     *memory.DecisionTicketRepository
	getCount atomic.Int64
}

func newCountingAppDecisionTicketRepository() *countingAppDecisionTicketRepository {
	return &countingAppDecisionTicketRepository{base: memory.NewDecisionTicketRepository()}
}

func (r *countingAppDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Create(ctx, item)
}

func (r *countingAppDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	r.getCount.Add(1)
	return r.base.Get(ctx, id)
}

func (r *countingAppDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	return r.base.List(ctx)
}

func (r *countingAppDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return r.base.ListByRunID(ctx, runID)
}

func (r *countingAppDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingAppDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Update(ctx, item)
}

func (r *countingAppDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	return r.base.Delete(ctx, id)
}

type countingAppDecisionResolutionRepository struct {
	base               *memory.DecisionResolutionRepository
	getByTicketIDCount atomic.Int64
}

func newCountingAppDecisionResolutionRepository() *countingAppDecisionResolutionRepository {
	return &countingAppDecisionResolutionRepository{base: memory.NewDecisionResolutionRepository()}
}

func (r *countingAppDecisionResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	return r.base.Create(ctx, item)
}

func (r *countingAppDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	r.getByTicketIDCount.Add(1)
	return r.base.GetByTicketID(ctx, ticketID)
}

func (r *countingAppDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	return r.base.DeleteByTicketID(ctx, ticketID)
}

func newCountingDecisionRuntimeSession(t *testing.T) (*RuntimeSession, *countingAppDecisionTicketRepository, *countingAppDecisionResolutionRepository) {
	t.Helper()

	tickets := newCountingAppDecisionTicketRepository()
	resolutions := newCountingAppDecisionResolutionRepository()
	decisionModule := decisionmodule.New()
	if err := decisionModule.Setup(decisionmodule.Dependencies{Tickets: tickets, Resolutions: resolutions}); err != nil {
		t.Fatalf("setup decision module: %v", err)
	}
	workspaceModule := workspacemodule.New()
	if err := workspaceModule.Setup(workspacemodule.Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
	}); err != nil {
		t.Fatalf("setup workspace module: %v", err)
	}
	approvalModule := approvalmodule.New()
	if err := approvalModule.Setup(approvalmodule.Dependencies{Approvals: memory.NewApprovalRepository()}); err != nil {
		t.Fatalf("setup approval module: %v", err)
	}

	session, err := NewRuntimeSessionFromExports(bootstrap.Exports{
		Repositories: bootstrap.RepositoryExports{
			Workspaces:         memory.NewWorkspaceRepository(),
			Runs:               memory.NewRunRepository(),
			Steps:              memory.NewStepRepository(),
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			EventRecords:       memory.NewEventRecordRepository(),
			MCPCapabilities:    memory.NewMCPCapabilityRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			Tickets:            tickets,
			Resolutions:        resolutions,
			Approvals:          memory.NewApprovalRepository(),
		},
		Services: bootstrap.ServiceExports{
			WorkspaceService: workspaceModule.Service(),
			RunService:       runservice.New(runservice.Dependencies{}),
			DecisionService:  decisionModule.Service(),
			ApprovalService:  approvalModule.Service(),
		},
	})
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	return session, tickets, resolutions
}

func TestReadDecisionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadDecisionWithSession(context.Background(), nil, "ticket-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestListDecisionsWithBackendReturnsFilteredTickets(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-list-a", "run-decision-list-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-list-b", "run-decision-list-b")

	items, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != first.BlockingTicketID || items[1].ID != second.BlockingTicketID {
		t.Fatalf("ticket ids = %q/%q, want %q/%q", items[0].ID, items[1].ID, first.BlockingTicketID, second.BlockingTicketID)
	}

	filteredByRun, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "run-decision-list-b")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by run: %v", err)
	}
	if len(filteredByRun) != 1 || filteredByRun[0].ID != second.BlockingTicketID {
		t.Fatalf("filteredByRun = %+v, want ticket %s", filteredByRun, second.BlockingTicketID)
	}

	filteredByWorkspace, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "ws-decision-list-a", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by workspace: %v", err)
	}
	if len(filteredByWorkspace) != 1 || filteredByWorkspace[0].ID != first.BlockingTicketID {
		t.Fatalf("filteredByWorkspace = %+v, want ticket %s", filteredByWorkspace, first.BlockingTicketID)
	}
}

func TestReadDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-read", "run-decision-read")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	item, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.ID != started.BlockingTicketID || item.State != "resolved" {
		t.Fatalf("decision = %+v, want resolved decision", item)
	}
	if item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionType = %q, want provide_missing_info", item.ResolutionType)
	}
}

func TestReadDecisionWithBackendIncludesAutoGovernanceSummary(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-decision-autogov"
	runID := "run-decision-autogov"
	started := prepareDecisionRun(t, dataRoot, workspaceID, runID)

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	ctx := context.Background()
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("session exports: %v", err)
	}
	if err := exports.Repositories.AutoGovernanceTraces.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID:                common.AutoGovernanceTraceID("trace-decision-autogov"),
		WorkspaceID:       common.WorkspaceID(workspaceID),
		RunID:             common.RunID(runID),
		ItemType:          "ticket",
		ItemID:            started.BlockingTicketID,
		Action:            "answer",
		RiskLevel:         "L1",
		Confidence:        0.92,
		PolicyMatched:     "goal_context",
		Reason:            "derived from structured context",
		EvidenceRefs:      []string{"memory-recall-1"},
		PromptFingerprint: "fingerprint-1",
		Status:            "resolved",
		OccurredAt:        "2026-05-06T10:00:00Z",
	}); err != nil {
		t.Fatalf("upsert auto governance trace: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	item, err := ReadDecisionWithBackend(ctx, "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.AutoGovernance == nil ||
		item.AutoGovernance.Action != "answer" ||
		item.AutoGovernance.Status != "resolved" ||
		item.AutoGovernance.RiskLevel != "L1" {
		t.Fatalf("auto governance summary = %+v, want resolved ticket summary", item.AutoGovernance)
	}
}

func TestResolveDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-resolve", "run-decision-resolve")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{"operator": "tester"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionID = %q, want resolution-%s-%s", item.ResolutionID, started.BlockingTicketID, decisioncommon.ResolutionTypeProvideMissingInfo)
	}
	if item.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("resolutionPayload = %+v, want operator tester", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendNormalizesSemanticIntent(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-semantic", "run-decision-semantic")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: "auto",
		ResolutionPayload: map[string]any{
			"answer":   "请重新规划执行方案",
			"operator": "tui",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("decision = %+v, want resolved replan", item)
	}
	semantic, _ := item.ResolutionPayload["governanceSemantic"].(map[string]any)
	if semantic["intent"] != "request_replan" || semantic["source"] != "tui" || semantic["rawInput"] != "请重新规划执行方案" {
		t.Fatalf("semantic payload = %+v, want normalized intent/source/rawInput", semantic)
	}
}

func TestResolveDecisionWithSessionSelectsContinuationCandidateFromApproveIntent(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-candidate-continue")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  common.WorkspaceID("ws-candidate-continue"),
		RunID:        common.RunID("run-candidate-continue"),
		StepID:       common.StepID("run-candidate-continue/step-1"),
		Reason:       "execution target choice is missing",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Continue in the current workspace or provide another target?",
		Context: map[string]any{
			"candidateValues": map[string]any{
				"executionTargetMode": []any{"init_from_scratch", "use_existing_project"},
			},
			"missingFields":        []any{"executionTargetMode"},
			"requiresHumanFields":  []any{"executionTargetMode"},
			"confidenceByField":    map[string]any{"executionTargetMode": 0.98},
			"knownValues":          map[string]any{"workspaceState": "empty"},
			"decisionContextScope": "target_workspace",
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{
			"answer":   "继续处理",
			"operator": "tui",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	if item.ResolutionPayload["executionTargetMode"] != "init_from_scratch" ||
		item.ResolutionPayload["candidateSelectionMode"] != "semantic_continue" {
		t.Fatalf("resolutionPayload = %+v, want selected continuation candidate", item.ResolutionPayload)
	}
	fields, ok := item.ResolutionPayload["selectedCandidateFields"].([]any)
	if ok {
		if len(fields) != 1 || fields[0] != "executionTargetMode" {
			t.Fatalf("selectedCandidateFields = %+v, want executionTargetMode", fields)
		}
	} else if fieldsString, ok := item.ResolutionPayload["selectedCandidateFields"].([]string); !ok || len(fieldsString) != 1 || fieldsString[0] != "executionTargetMode" {
		t.Fatalf("selectedCandidateFields = %+v, want executionTargetMode", item.ResolutionPayload["selectedCandidateFields"])
	}
}

func TestResolveDecisionWithSessionRejectsInlineProviderConfigForProviderRecoveryTicket(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-provider-recovery")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-provider-recovery"),
		RunID:       common.RunID("run-provider-recovery"),
		StepID:      common.StepID("run-provider-recovery/step-1"),
		Reason:      "LLM provider retry attempts exhausted after 2 attempts",
		Question:    "Fix or switch the LLM provider in system configuration first.",
		Context: map[string]any{
			"failureClass":                   "provider_timeout",
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	_, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{
			"answer": "provider = compatible\nbaseURL = https://api.example.com\napiKey = sk-secret\nmodel = model-a",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ResolveDecisionWithSession err = %v, want invalid argument", err)
	}
	item, getErr := tickets.Get(ctx, ticketID)
	if getErr != nil {
		t.Fatalf("get ticket: %v", getErr)
	}
	if item.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", item.State)
	}
	if _, getErr = resolutions.GetByTicketID(ctx, ticketID); !errors.Is(getErr, common.ErrNotFound) {
		t.Fatalf("resolution err = %v, want not found", getErr)
	}
}

func TestResolveDecisionWithSessionAllowsProviderRecoveryConfirmationWithoutSecrets(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-provider-recovery-confirmed")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-provider-recovery-confirmed"),
		RunID:       common.RunID("run-provider-recovery-confirmed"),
		StepID:      common.StepID("run-provider-recovery-confirmed/step-1"),
		Reason:      "LLM provider retry attempts exhausted after 2 attempts",
		Question:    "Fix or switch the LLM provider in system configuration first.",
		Context: map[string]any{
			"failureClass":                   "provider_timeout",
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{
			"answer":   "已在系统配置中切换默认 LLM provider，继续执行",
			"operator": "console",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provider recovery confirmation", item)
	}
	if item.ResolutionPayload["recoveryAction"] != "retry_same_step" {
		t.Fatalf("resolutionPayload = %+v, want retry_same_step recovery action", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionAllowsTargetWorkspaceExternalDependencyConfig(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-target-workspace-test-config")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-target-workspace-test-config"),
		RunID:       common.RunID("run-target-workspace-test-config"),
		StepID:      common.StepID("run-target-workspace-test-config/step-1"),
		Reason:      "integration test configuration is missing",
		Question:    "Provide the target workspace external dependency test configuration.",
		Context: map[string]any{
			decisionContextKeyScope: decisionContextScopeTargetWorkspace,
			"dependencyKind":        "external_service",
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{
			"answer": "provider = compatible\nbaseURL = https://api.example.com\napiKey = sk-secret\nmodel = model-a",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want target workspace config accepted", item)
	}
	if rendered := fmt.Sprintf("%#v", item.ResolutionPayload); strings.Contains(strings.ToLower(rendered), "sk-secret") {
		t.Fatalf("resolutionPayload = %+v, want secret redacted", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionInfersRepairExhaustionRetryFromContinue(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-repair-exhaustion-continue")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-continue"),
		RunID:       common.RunID("run-repair-exhaustion-continue"),
		StepID:      common.StepID("run-repair-exhaustion-continue/step-1"),
		Reason:      "runtime repair attempts exhausted after 3 attempts",
		Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: "auto",
		ResolutionPayload: map[string]any{
			"answer":   "继续",
			"operator": "tui",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved retry as provide_missing_info", item)
	}
	if item.ResolutionPayload["recoveryAction"] != "retry_same_step" {
		t.Fatalf("resolutionPayload = %+v, want inferred retry_same_step", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionAllowsExplicitRepairExhaustionRecoveryAction(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-repair-exhaustion-explicit")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-explicit"),
		RunID:       common.RunID("run-repair-exhaustion-explicit"),
		StepID:      common.StepID("run-repair-exhaustion-explicit/step-1"),
		Reason:      "runtime repair attempts exhausted after 3 attempts",
		Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{
			"recoveryAction":    "retry_same_step",
			"resumeInstruction": "retry the failed verification after applying the clarified constraint",
			"operator":          "tui",
		},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved explicit recovery action", item)
	}
}

func TestResolveDecisionWithSessionMapsRepairExhaustionReplanAndTerminate(t *testing.T) {
	for _, tc := range []struct {
		name               string
		answer             string
		wantResolutionType string
		wantAction         string
	}{
		{name: "replan", answer: "请重新规划修复路径", wantResolutionType: decisioncommon.ResolutionTypeReplan, wantAction: "replan"},
		{name: "terminate", answer: "不继续，终止当前路径", wantResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath, wantAction: "terminate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, tickets, _ := newCountingDecisionRuntimeSession(t)
			ctx := context.Background()
			ticketID := common.TicketID("ticket-repair-exhaustion-" + tc.name)
			if err := tickets.Create(ctx, &decisionmodel.Ticket{
				ID:          ticketID,
				WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-" + tc.name),
				RunID:       common.RunID("run-repair-exhaustion-" + tc.name),
				StepID:      common.StepID("run-repair-exhaustion-" + tc.name + "/step-1"),
				Reason:      "runtime repair attempts exhausted after 3 attempts",
				Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
				Context: map[string]any{
					decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
				},
				State: common.DecisionTicketStateOpen,
			}); err != nil {
				t.Fatalf("create ticket: %v", err)
			}

			item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), ResolveDecisionCommand{
				ResolutionType: "auto",
				ResolutionPayload: map[string]any{
					"answer":   tc.answer,
					"operator": "tui",
				},
			})
			if err != nil {
				t.Fatalf("ResolveDecisionWithSession: %v", err)
			}
			if item.ResolutionType != tc.wantResolutionType || item.ResolutionPayload["recoveryAction"] != tc.wantAction {
				t.Fatalf("decision = %+v, want type=%s action=%s", item, tc.wantResolutionType, tc.wantAction)
			}
		})
	}
}

func TestResolveDecisionWithSessionReusesReturnedEntitiesForResponse(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()

	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-app-return-loaded"),
		WorkspaceID: common.WorkspaceID("ws-app-return-loaded"),
		RunID:       common.RunID("run-app-return-loaded"),
		StepID:      common.StepID("run-app-return-loaded/step-1"),
		Reason:      "need operator decision",
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	tickets.getCount.Store(0)
	resolutions.getByTicketIDCount.Store(0)

	item, err := ResolveDecisionWithSession(ctx, session, "ticket-app-return-loaded", ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{"operator": "tester"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo || item.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("decision = %+v, want resolved response from returned entities", item)
	}
	if got := tickets.getCount.Load(); got != 2 {
		t.Fatalf("ticket Get count = %d, want validation read plus service resolve read without app response reread", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 0 {
		t.Fatalf("resolution GetByTicketID count = %d, want no app response reread on first resolve", got)
	}
}

func TestResolveDecisionWithBackendSupportsRejectCurrentPath(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-reject", "run-decision-reject")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeRejectCurrentPath,
		ResolutionPayload: map[string]any{"operator": "tester", "reason": "not viable"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("resolutionID = %q, want formal reject resolution id", item.ResolutionID)
	}
	if item.ResolutionPayload["reason"] != "not viable" {
		t.Fatalf("resolutionPayload = %+v, want reject reason", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendReturnsConflictAfterResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-conflict", "run-decision-conflict")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResolveDecisionWithBackendRejectsUnsupportedResolutionType(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-invalid", "run-decision-invalid")

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: "unsupported_resolution",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendResolvesTicketsInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-a", "run-decision-batch-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-b", "run-decision-batch-b")

	items, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:         []string{second.BlockingTicketID, first.BlockingTicketID},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: map[string]any{"operator": "batch-tester"},
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.BlockingTicketID || items[1].ID != first.BlockingTicketID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan with payload", item)
		}
	}
}

func TestBatchResolveDecisionsWithSessionReusesReturnedEntitiesAfterPrevalidation(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()

	for _, ticketID := range []common.TicketID{"ticket-app-batch-a", "ticket-app-batch-b"} {
		if err := tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          ticketID,
			WorkspaceID: common.WorkspaceID("ws-app-batch"),
			RunID:       common.RunID("run-app-batch"),
			StepID:      common.StepID("run-app-batch/step"),
			Reason:      "need operator decision",
			State:       common.DecisionTicketStateOpen,
		}); err != nil {
			t.Fatalf("create ticket %s: %v", ticketID, err)
		}
	}
	tickets.getCount.Store(0)
	resolutions.getByTicketIDCount.Store(0)

	items, err := BatchResolveDecisionsWithSession(ctx, session, BatchResolveDecisionCommand{
		TicketIDs:         []string{"ticket-app-batch-b", "ticket-app-batch-a"},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: map[string]any{"operator": "batch-tester"},
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithSession: %v", err)
	}
	if len(items) != 2 || items[0].ID != "ticket-app-batch-b" || items[1].ID != "ticket-app-batch-a" {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan response", item)
		}
	}
	if got := tickets.getCount.Load(); got != 4 {
		t.Fatalf("ticket Get count = %d, want prevalidation plus one service read per ticket", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 0 {
		t.Fatalf("resolution GetByTicketID count = %d, want no response reread on first batch resolve", got)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsDuplicateTicketIDs(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-batch-dup", "run-decision-batch-dup")

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{started.BlockingTicketID, started.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsAlreadyResolvedTicketBeforeMutation(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-a", "run-decision-batch-conflict-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-b", "run-decision-batch-conflict-b")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{first.BlockingTicketID, second.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeReplan,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
	firstDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if firstDecision.State != "open" {
		t.Fatalf("first decision = %+v, want still open after prevalidation conflict", firstDecision)
	}
}
