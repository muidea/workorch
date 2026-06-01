package appservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestCreateRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateRunWithSession(context.Background(), nil, "ws-1", "run-1", "", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateRunWithBackendReturnsCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run created workspace", "", "workspaces/run created workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "created goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", []string{"created dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run-created", goal.ID, dod.ID)
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if item.ID != "run-created" || item.WorkspaceID != "ws-run-created" {
		t.Fatalf("run = %+v, want created run", item)
	}
	if item.State != "created" || item.GoalID != goal.ID || item.DoDID != dod.ID {
		t.Fatalf("run fields = %+v, want created state with goal/dod", item)
	}
	goalContext, _ := item.ExecutionContext["goal"].(map[string]any)
	dodContext, _ := item.ExecutionContext["dod"].(map[string]any)
	if goalContext["text"] != "- created goal" {
		t.Fatalf("execution goal context = %+v, want created goal", goalContext)
	}
	if dodContext["state"] != "ready" {
		t.Fatalf("execution dod context = %+v, want ready dod", dodContext)
	}
	dodItems := normalizeStringIDsFromAny(dodContext["items"])
	if len(dodItems) != 1 || dodItems[0] != "created dod" {
		t.Fatalf("execution dod items = %+v, want created dod", dodContext["items"])
	}
}

func normalizeStringIDsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func runCompressedContext(t *testing.T, item Run) map[string]any {
	t.Helper()
	compressed, ok := item.ExecutionContext["compressedContext"].(map[string]any)
	if !ok || len(compressed) == 0 {
		t.Fatalf("executionContext = %+v, want compressedContext map", item.ExecutionContext)
	}
	return compressed
}

func TestStartRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := StartRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestResumeRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ResumeRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCompressedContextForRunStartUsesLightweightCreatedRunProjection(t *testing.T) {
	compressed := compressedContextForRunStart(&runmodel.Run{
		ID:          "run-lightweight-start",
		WorkspaceID: "ws-lightweight-start",
		State:       common.RunStateCreated,
	})
	if compressed == nil {
		t.Fatalf("compressedContext nil, want lightweight projection")
	}
	if compressed["version"] != "workorch.compressedContext.v1" ||
		compressed["mode"] != "deterministic_recovery_summary" ||
		compressed["runId"] != "run-lightweight-start" ||
		compressed["workspaceId"] != "ws-lightweight-start" {
		t.Fatalf("compressedContext = %+v, want created run identity", compressed)
	}
	if compressed["currentMilestone"] != "execution_steps" {
		t.Fatalf("currentMilestone = %v, want execution_steps", compressed["currentMilestone"])
	}
	if compressedContextForRunStart(&runmodel.Run{
		ID:          "run-lightweight-start",
		WorkspaceID: "ws-lightweight-start",
		State:       common.RunStateRunning,
	}) != nil {
		t.Fatalf("running run should use full snapshot-based compressed context")
	}
}

func TestCompressedContextForBlockedRunResumeUsesLightweightGovernanceProjection(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	deps := readModelDependencies{
		tickets:     ticketRepo,
		resolutions: resolutionRepo,
		approvals:   approvalRepo,
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          "ticket-lightweight-resume",
		WorkspaceID: "ws-lightweight-decision-resume",
		RunID:       "run-lightweight-decision-resume",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-lightweight-resume",
		TicketID: "ticket-lightweight-resume",
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          "approval-lightweight-resume",
		WorkspaceID: "ws-lightweight-approval-resume",
		RunID:       "run-lightweight-approval-resume",
		State:       common.ApprovalStateApproved,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	decision, err := compressedContextForBlockedRunResume(ctx, deps, &runmodel.Run{
		ID:               "run-lightweight-decision-resume",
		WorkspaceID:      "ws-lightweight-decision-resume",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-lightweight-resume",
	})
	if err != nil {
		t.Fatalf("decision compressedContext: %v", err)
	}
	if decision == nil {
		t.Fatalf("decision compressedContext nil, want lightweight blocked resume projection")
	}
	if decision["runId"] != "run-lightweight-decision-resume" ||
		decision["workspaceId"] != "ws-lightweight-decision-resume" ||
		decision["currentMilestone"] != "execution_steps" {
		t.Fatalf("decision compressedContext = %+v, want run identity and execution milestone", decision)
	}
	decisionRecovery, _ := decision["recovery"].(map[string]any)
	if decisionRecovery["blockingTicketId"] != "ticket-lightweight-resume" ||
		decisionRecovery["nextAction"] != "continue_run" {
		t.Fatalf("decision recovery = %+v, want resolved ticket resume", decisionRecovery)
	}

	approval, err := compressedContextForBlockedRunResume(ctx, deps, &runmodel.Run{
		ID:                 "run-lightweight-approval-resume",
		WorkspaceID:        "ws-lightweight-approval-resume",
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-lightweight-resume",
	})
	if err != nil {
		t.Fatalf("approval compressedContext: %v", err)
	}
	if approval == nil {
		t.Fatalf("approval compressedContext nil, want lightweight blocked resume projection")
	}
	approvalRecovery, _ := approval["recovery"].(map[string]any)
	if approvalRecovery["blockingApprovalId"] != "approval-lightweight-resume" ||
		approvalRecovery["nextAction"] != "continue_run" {
		t.Fatalf("approval recovery = %+v, want approved approval resume", approvalRecovery)
	}

	nonBlocked, err := compressedContextForBlockedRunResume(ctx, deps, &runmodel.Run{
		ID:          "run-lightweight-running",
		WorkspaceID: "ws-lightweight-running",
		State:       common.RunStateRunning,
	})
	if err != nil {
		t.Fatalf("running compressedContext: %v", err)
	}
	if nonBlocked != nil {
		t.Fatalf("running run should use snapshot-based compressed context")
	}
}

func TestCompressedContextPatchRecordsRecoveryMemoryRecallTrace(t *testing.T) {
	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	defer session.Close(ctx)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-recovery-memory")
	runID := common.RunID("run-recovery-memory")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "recovery memory", State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.MemoryRecallTraces.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 "trace-recovery-source",
		WorkspaceID:        workspaceID,
		RunID:              runID,
		Stage:              "planning",
		Source:             "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-recovery-source"},
		EvidenceRef:        "trace-recovery-source",
	}); err != nil {
		t.Fatalf("seed memory recall trace: %v", err)
	}

	patch, _, err := compressedContextPatchForRun(ctx, session, runID)
	if err != nil {
		t.Fatalf("compressedContextPatchForRun: %v", err)
	}
	compressed, _ := patch["compressedContext"].(map[string]any)
	if !containsString(stringSliceFromAny(compressed["sourceSectionRefs"]), "memory_recall") {
		t.Fatalf("compressed context = %+v, want memory_recall source section", compressed)
	}
	traces, err := exports.Repositories.MemoryRecallTraces.List(ctx, repository.MemoryRecallTraceQuery{RunID: runID, Source: "recovery"})
	if err != nil {
		t.Fatalf("list recovery traces: %v", err)
	}
	if len(traces) != 1 ||
		traces[0].Policy != "inherit_compressed_context" ||
		traces[0].Status != "inherited" ||
		len(traces[0].RetrievedMemoryIDs) != 1 ||
		traces[0].RetrievedMemoryIDs[0] != "memory-recovery-source" {
		t.Fatalf("recovery traces = %+v, want inherited memory recall trace", traces)
	}
}

func TestCompressedContextPatchForRunRefreshesCumulativeGoalAndDoD(t *testing.T) {
	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	defer session.Close(ctx)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-recovery-definitions")
	runID := common.RunID("run-recovery-definitions")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "recovery definitions", State: common.WorkspaceStateBlocked}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	for _, goal := range []*workspacemodel.Goal{
		{ID: "goal-1", WorkspaceID: workspaceID, Text: "initial goal"},
		{ID: "goal-2", WorkspaceID: workspaceID, Text: "additional goal"},
	} {
		if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
			t.Fatalf("seed goal %s: %v", goal.ID, err)
		}
	}
	for _, dod := range []*workspacemodel.DoD{
		{ID: "dod-1", WorkspaceID: workspaceID, Items: []string{"initial dod"}, State: common.DoDStateReady},
		{ID: "dod-2", WorkspaceID: workspaceID, Items: []string{"additional dod"}, State: common.DoDStateReady},
	} {
		if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
			t.Fatalf("seed dod %s: %v", dod.ID, err)
		}
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-1",
		DoDID:            "dod-1",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-recovery-definitions",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          "ticket-recovery-definitions",
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := exports.Repositories.Resolutions.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-recovery-definitions",
		TicketID: "ticket-recovery-definitions",
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	patch, _, err := compressedContextPatchForRun(ctx, session, runID)
	if err != nil {
		t.Fatalf("compressedContextPatchForRun: %v", err)
	}
	if patch["goalId"] != "goal-2" || patch["doDId"] != "dod-2" {
		t.Fatalf("definition patch = %+v, want latest goal/dod IDs", patch)
	}
	goalContext, _ := patch["goal"].(map[string]any)
	goalText, _ := goalContext["text"].(string)
	if !strings.Contains(goalText, "initial goal") || !strings.Contains(goalText, "additional goal") {
		t.Fatalf("goal context = %+v, want cumulative goals", goalContext)
	}
	dodContext, _ := patch["dod"].(map[string]any)
	dodItems := normalizeStringIDsFromAny(dodContext["items"])
	if !stringSliceContains(dodItems, "initial dod") || !stringSliceContains(dodItems, "additional dod") {
		t.Fatalf("dod context = %+v, want cumulative dods", dodContext)
	}
	if _, ok := patch["compressedContext"].(map[string]any); !ok {
		t.Fatalf("definition patch = %+v, want compressedContext retained", patch)
	}
	revision, _ := patch["definitionRevision"].(map[string]any)
	if revision["policy"] != "workspace_goal_dod_cumulative" {
		t.Fatalf("definitionRevision = %+v, want cumulative policy", revision)
	}
}

func TestCreateRunWithBackendReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	if _, err := CreateRunWithBackend(context.Background(), "file", t.TempDir(), "ws-missing", "run-created", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestReadRunWithBackendReturnsPersistedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run read workspace", "", "workspaces/run read workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run-read", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := ReadRunWithBackend(context.Background(), "file", dataRoot, "run-read")
	if err != nil {
		t.Fatalf("ReadRunWithBackend: %v", err)
	}
	if item.ID != "run-read" || item.WorkspaceID != "ws-run-read" || item.State != "created" {
		t.Fatalf("run = %+v, want persisted run", item)
	}
}

func TestListRunsWithBackendReturnsFilteredRuns(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run workspace a", "", "workspaces/run workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-a: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run workspace b", "", "workspaces/run workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run-b", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run-a", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-a: %v", err)
	}

	items, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "")
	if err != nil {
		t.Fatalf("ListRunsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "run-a" || items[1].ID != "run-b" {
		t.Fatalf("run ids = %q/%q, want run-a/run-b", items[0].ID, items[1].ID)
	}

	filtered, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "ws-run-b")
	if err != nil {
		t.Fatalf("ListRunsWithBackend filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-b" {
		t.Fatalf("filtered runs = %+v, want run-b only", filtered)
	}
}

func TestStartRunWithBackendBlocksWhenDefaultLLMProviderUnavailable(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run start workspace", "", "workspaces/run start workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run-start", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start")
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if item.ID != "run-start" || item.State != "blocked_on_decision" || item.BlockingTicketID == "" || item.FailureReason != "" {
		t.Fatalf("run = %+v, want run-start blocked on provider decision without failure reason", item)
	}
	compressed := runCompressedContext(t, item)
	if compressed["version"] != "workorch.compressedContext.v1" || compressed["mode"] != "deterministic_recovery_summary" {
		t.Fatalf("compressedContext = %+v, want deterministic recovery summary", compressed)
	}
	if compressed["runId"] != "run-start" || compressed["workspaceId"] != "ws-run-start" {
		t.Fatalf("compressedContext identity = %+v, want run-start/ws-run-start", compressed)
	}
}

func TestStartRunWithBackendReturnsConflictForBlockedProviderRunAfterReload(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run start restart workspace", "", "workspaces/run start restart workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run-start-restart", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart"); err != nil {
		t.Fatalf("first StartRunWithBackend: %v", err)
	}

	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second StartRunWithBackend err = %v, want conflict for blocked provider run", err)
	}
}

func TestStartRunWithBackendReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	item := prepareDecisionRun(t, dataRoot, "ws-run-start-blocked-decision", "run-start-blocked-decision")
	if item.State != "blocked_on_decision" || item.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", item)
	}
}

func TestResumeRunWithBackendReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-rejected"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-approval-rejected", runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}
	if item.State != "failed" || item.BlockingApprovalID != "" || item.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason and cleared approval", item)
	}
	compressed := runCompressedContext(t, item)
	if compressed["runId"] != runID || compressed["workspaceId"] != "ws-run-resume-approval-rejected" {
		t.Fatalf("compressedContext identity = %+v, want resumed approval run", compressed)
	}
	if _, ok := compressed["recentEvidence"].([]any); !ok {
		t.Fatalf("compressedContext = %+v, want recent evidence for resumed run", compressed)
	}
	if refs, ok := compressed["sourceSectionRefs"].([]any); !ok || len(refs) == 0 {
		t.Fatalf("compressedContext = %+v, want source section refs for resumed run", compressed)
	}
}

func TestResumeRunWithBackendReturnsConflictForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-rejected-restart"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-failed", runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("first ResumeRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second ResumeRunWithBackend err = %v, want conflict for failed run", err)
	}
}

func TestStartRunWithBackendReturnsConflictForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-failed-start-conflict"
	started := prepareApprovalRun(t, dataRoot, "ws-run-start-failed", runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}

	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResumeRunWithBackendReturnsBlockedWhenDependencyPending(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-pending"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-pending", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("err = %v, want blocked", err)
	}
}

func TestResumeRunWithBackendReturnsConflictForCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run resume conflict workspace", "", "workspaces/run resume conflict workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run-resume-conflict", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, "run-resume-conflict"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestRecoverStuckRunningRunsWithBackendFailsRunningRunWithoutContinuationCheckpoint(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-stuck-running", "stuck running workspace", "", "workspaces/stuck-running"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend stuck: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-continuation-running", "continuation running workspace", "", "workspaces/continuation-running"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend continuation: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-stuck-running", "run-stuck-running", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend stuck: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-continuation-running", "run-continuation-running", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend continuation: %v", err)
	}

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	for _, runID := range []common.RunID{"run-stuck-running", "run-continuation-running"} {
		item, err := exports.Repositories.Runs.Get(ctx, runID)
		if err != nil {
			t.Fatalf("get run %s: %v", runID, err)
		}
		item.State = common.RunStateRunning
		if err := exports.Repositories.Runs.Update(ctx, item); err != nil {
			t.Fatalf("update run %s: %v", runID, err)
		}
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               "checkpoint-continuation-running",
		WorkspaceID:      "ws-continuation-running",
		RunID:            "run-continuation-running",
		CheckpointType:   common.CheckpointTypeRecovery,
		RunStateSnapshot: common.RunStateRunning,
		OccurredAt:       "2026-04-30T13:00:00Z",
		Data:             map[string]any{"continuationAction": "resume_from_checkpoint"},
	}); err != nil {
		t.Fatalf("create continuation checkpoint: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	recovered, err := RecoverStuckRunningRunsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunsWithBackend: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != "run-stuck-running" || recovered[0].State != "failed" {
		t.Fatalf("recovered = %+v, want only run-stuck-running failed", recovered)
	}
	if recovered[0].FailureReason != stuckRunningRunFailureReason {
		t.Fatalf("failure reason = %q, want %q", recovered[0].FailureReason, stuckRunningRunFailureReason)
	}
	continued, err := ReadRunWithBackend(ctx, "file", dataRoot, "run-continuation-running")
	if err != nil {
		t.Fatalf("ReadRunWithBackend continuation: %v", err)
	}
	if continued.State != "running" {
		t.Fatalf("continued run = %+v, want running", continued)
	}
}

func TestRecoverStuckRunningRunsWithBackendClearsTerminalActiveWorkspaceRun(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-terminal-active", "terminal active workspace", "", "workspaces/terminal-active"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-terminal-active", "run-terminal-active", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, "ws-terminal-active")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.State = common.WorkspaceStateRunning
	workspaceItem.ActiveRunID = "run-terminal-active"
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, "run-terminal-active")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateSuperseded
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	recovered, err := RecoverStuckRunningRunsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunsWithBackend: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered runs = %+v, want no running run recovery", recovered)
	}
	workspace, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, "ws-terminal-active")
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if workspace.State != string(common.WorkspaceStateFailed) || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want failed with no terminal active run", workspace)
	}
}

func TestRecoverStuckRunningRunsWithBackendRedispatchesOrphanDispatchedStep(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-orphan-dispatched", "orphan dispatched workspace", "", "workspaces/orphan-dispatched"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-orphan-dispatched", "run-orphan-dispatched", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, "run-orphan-dispatched")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateRunning
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    "run-orphan-dispatched/follow-up",
		RunID: "run-orphan-dispatched",
		Kind:  filecommon.StepKindList,
		State: common.StepStateDispatched,
		Input: map[string]any{"path": ".", "maxEntries": 1, "previousStepId": "run-orphan-dispatched/z-lexically-newer"},
	}); err != nil {
		t.Fatalf("create dispatched step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     "run-orphan-dispatched/z-lexically-newer",
		RunID:  "run-orphan-dispatched",
		Kind:   filecommon.StepKindList,
		State:  common.StepStateCompleted,
		Input:  map[string]any{"path": ".", "maxEntries": 1},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("create previous completed step: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	recovered, err := RecoverStuckRunningRunsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunsWithBackend: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != "run-orphan-dispatched" || recovered[0].State != "completed" {
		t.Fatalf("recovered = %+v, want completed run after redispatched step", recovered)
	}
	verifySession, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend verify: %v", err)
	}
	verifyExports, err := verifySession.Exports()
	if err != nil {
		t.Fatalf("Exports verify: %v", err)
	}
	step, err := verifyExports.Repositories.Steps.Get(ctx, "run-orphan-dispatched/follow-up")
	if err != nil {
		t.Fatalf("get redispatched step: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Output["status"] != "completed" {
		t.Fatalf("step = %+v, want redispatched completed step", step)
	}
	runAfter, err := verifyExports.Repositories.Runs.Get(ctx, "run-orphan-dispatched")
	if err != nil {
		t.Fatalf("get run after: %v", err)
	}
	if runAfter.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed after redispatch", runAfter.State)
	}
	_ = verifySession.Close(ctx)
}

func TestReadRunWithSessionReturnsFailureReason(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	if err := session.Bootstrap.Exports().Services.WorkspaceService.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:          "ws-run-failure",
		Title:                "run failure workspace",
		DeliverableDirectory: "workspaces/ws-run-failure",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := session.Bootstrap.Exports().Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:       "run-failure",
		WorkspaceID: "ws-run-failure",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := session.Bootstrap.Exports().Services.RunService.FailRun(ctx, runmodule.FailRunCommand{
		RunID:  "run-failure",
		Reason: "manual fail",
	}); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	item, err := ReadRunWithSession(ctx, session, "run-failure")
	if err != nil {
		t.Fatalf("ReadRunWithSession: %v", err)
	}
	if item.State != "failed" || item.FailureReason != "manual fail" {
		t.Fatalf("run = %+v, want failed/manual fail", item)
	}
}
