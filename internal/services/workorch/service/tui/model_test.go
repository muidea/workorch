package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func asModel(m tea.Model) AppModel {
	switch v := m.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("not AppModel")
}

func TestSchedulerDiagnosticsStatusShowsWatchdogTimerRegistration(t *testing.T) {
	text := schedulerDiagnosticsStatusText(&backend.SchedulerDiagnosticsInfo{
		Watchdog: backend.WatchdogDiagnosticsInfo{TimerRegistrations: 2},
	})
	if !strings.Contains(text, "- watchdog: registered x2") {
		t.Fatalf("status text = %q, want watchdog registration summary", text)
	}
}

func TestSchedulerDiagnosticsStatusShowsReadback(t *testing.T) {
	text := schedulerDiagnosticsStatusText(&backend.SchedulerDiagnosticsInfo{
		LatestContinuation: &backend.ContinuationDiagnosticInfo{
			Kind:   "run_continuation",
			State:  "failed",
			Reason: "owner_readback_not_advanced",
			Readback: &backend.ContinuationReadbackDiagnosticInfo{
				Action:              "resume",
				Advanced:            false,
				Reason:              "blocked_on_resolved_decision",
				OldRunState:         "blocked_on_decision",
				NewRunState:         "blocked_on_decision",
				NewBlockingTicketID: "ticket-readback",
				ProjectionRevision:  9,
			},
		},
	})
	for _, want := range []string{
		"scheduler readback",
		"resume",
		"advanced=false",
		"blocked_on_resolved_decision",
		"ticket=ticket-readback",
		"projection=9",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text = %q, want %q", text, want)
		}
	}
}

func TestFetchAllUsesWorkspaceListWithoutSnapshot(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	be.listWorkspaces = []backend.WorkspaceInfo{{
		ID:    "ws-list-only",
		Title: "List Only",
		State: common.WorkspaceStateActive,
		Projection: backend.ProjectionInfo{
			GlobalRevision: 7,
		},
	}}

	msg := FetchAll(be)()
	got, ok := msg.(WorkspaceListMsg)
	if !ok {
		t.Fatalf("message = %T, want WorkspaceListMsg", msg)
	}
	if got.Error != nil {
		t.Fatalf("FetchAll error = %v", got.Error)
	}
	if be.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", be.listCalls)
	}
	if be.snapshotCalls != 0 || be.lastSnapshotWorkspace != "" {
		t.Fatalf("snapshot calls = %d workspace=%q, want none for main view fetch", be.snapshotCalls, be.lastSnapshotWorkspace)
	}
	if len(got.Workspaces) != 1 || got.Workspaces[0].ID != "ws-list-only" || got.Projection.GlobalRevision != 7 {
		t.Fatalf("workspace list msg = %+v, want compact projection from list", got)
	}
}

func TestLatestLLMContextBudgetPrefersProjectedBudgetTrace(t *testing.T) {
	got := latestLLMContextBudget([]backend.LLMContextBudgetTraceInfo{
		{
			StepID:               common.StepID("step-budget"),
			ProviderID:           "provider-budget",
			Model:                "model-budget",
			MaxTokens:            8192,
			EstimatedInputTokens: 4096,
			ShrinkLevel:          2,
			IncludedSections:     []string{"p0", "p1"},
			ProfileMode:          "auto",
			CapabilitySource:     "provider",
			PromptCaptured:       true,
		},
	}, []backend.LLMUsageTraceInfo{
		{
			ID: "usage-fallback",
			ContextBudget: map[string]any{
				"providerId": "provider-usage",
				"maxTokens":  1024,
			},
		},
	})
	if got["providerId"] != "provider-budget" || got["model"] != "model-budget" || got["maxTokens"] != 8192 {
		t.Fatalf("context budget = %+v, want projected budget trace", got)
	}
	included, ok := got["includedSections"].([]string)
	if !ok || len(included) != 2 || included[0] != "p0" {
		t.Fatalf("includedSections = %+v, want projected sections", got["includedSections"])
	}
}

func projectWorkspace(m *AppModel, ws backend.WorkspaceInfo) {
	if m == nil {
		return
	}
	m.upsertWorkspace(ws)
	if m.activeWorkspaceID == "" {
		m.activeWorkspaceID = string(ws.ID)
		m.selectedWorkspaceID = string(ws.ID)
	}
	if string(ws.ID) == m.activeWorkspaceID && ws.ActiveRunID != "" {
		m.activeRunID = string(ws.ActiveRunID)
	}
}

func projectDecisionBlocker(m *AppModel, workspaceID, runID, ticketID string) {
	projectWorkspace(m, backend.WorkspaceInfo{
		ID:               common.WorkspaceID(workspaceID),
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      common.RunID(runID),
		BlockingTicketID: common.TicketID(ticketID),
	})
}

func projectApprovalBlocker(m *AppModel, workspaceID, runID, approvalID string) {
	projectWorkspace(m, backend.WorkspaceInfo{
		ID:                 common.WorkspaceID(workspaceID),
		State:              common.WorkspaceStateBlocked,
		ActiveRunID:        common.RunID(runID),
		BlockingApprovalID: common.ApprovalID(approvalID),
	})
}

func projectRunningWorkspace(m *AppModel, workspaceID, runID string) {
	projectWorkspace(m, backend.WorkspaceInfo{
		ID:          common.WorkspaceID(workspaceID),
		State:       common.WorkspaceStateRunning,
		ActiveRunID: common.RunID(runID),
	})
}

func TestModelInit(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-default", "")
	if m.mode != modeAgentSession {
		t.Fatal("expected agent session mode")
	}
}

func TestRunControlIntentUsesResumeAsCanonicalEnglishVerb(t *testing.T) {
	for _, input := range []string{"resume", "resume run", "resume execution", "继续执行"} {
		if !isRunControlIntent(input) {
			t.Fatalf("%q not recognized as run-control intent", input)
		}
	}
	for _, input := range []string{"hello", "show status", "list workspaces"} {
		if isRunControlIntent(input) {
			t.Fatalf("%q recognized as run-control intent, want non-canonical", input)
		}
	}
}

func TestModelCtrlQDoesNotQuit(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = asModel(next)
	if m.mode == modeQuit || cmd != nil {
		t.Fatalf("ctrl+q should not quit: mode=%v cmd=%v", m.mode, cmd != nil)
	}
}

func TestModelPlainQIsInputNotQuit(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = asModel(next)
	if m.mode == modeQuit || cmd != nil {
		t.Fatalf("plain q should not quit: mode=%v cmd=%v", m.mode, cmd != nil)
	}
	if m.input != "q" {
		t.Fatalf("input = %q, want q", m.input)
	}
}

func TestModelExitCommandQuits(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.input = "/exit"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if asModel(next).mode != modeQuit {
		t.Fatal("expected quit mode")
	}
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestModelOnlyExitSlashCommandQuits(t *testing.T) {
	for _, input := range []string{"/q", "/quit"} {
		m := NewAppModel(newFake(t), "ws-1", "")
		m.setInput(input)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = asModel(next)
		if m.mode == modeQuit || cmd != nil {
			t.Fatalf("%s should not quit: mode=%v cmd=%v", input, m.mode, cmd != nil)
		}
		if m.lastError == nil {
			t.Fatalf("%s should report unknown command", input)
		}
	}
}

func TestModelWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	msg := WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "running"},
	}}
	next, _ := m.Update(msg)
	if len(asModel(next).workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(asModel(next).workspaces))
	}
}

func TestModelError(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	next, _ := m.Update(ErrorMsg{Error: errors.New("boom")})
	if asModel(next).lastError == nil {
		t.Fatal("expected error")
	}
}

func TestModelView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "Project", State: "active"},
	}})
	m = asModel(next)
	m.width = 80
	m.height = 24
	if m.View() == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestModelRendersArtifactContinuationProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 32
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-001"
	m.progress = progressRunning
	m.artifactContinuations = []backend.ArtifactContinuationInfo{{
		ID:                "artifact-continuation-001",
		WorkspaceID:       "ws-1",
		RunID:             "run-001",
		ArtifactID:        "artifact-001",
		TargetRef:         "deliverable.txt",
		State:             common.ArtifactContinuationStateChunking,
		ExpectedSequence:  3,
		ReceivedSequences: []int{1, 2},
		BytesWritten:      128,
	}}

	view := m.View()
	if !strings.Contains(view, "Artifact Continuations") || !strings.Contains(view, "deliverable.txt") {
		t.Fatalf("view missing artifact continuation progress:\n%s", view)
	}
	status := m.renderRuntimeStatusBar(120)
	if !strings.Contains(status, "artifact deliverable.txt 2/3") {
		t.Fatalf("status = %q, want active artifact progress", status)
	}
}

func TestModelHandlesArtifactContinuationEvent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-001"
	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventArtifactContinuationChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-001",
		Payload: map[string]any{
			"state":             "chunking",
			"targetRef":         "deliverable.txt",
			"expectedSequence":  4,
			"receivedSequences": []any{1, 2, 3},
			"bytesWritten":      512,
		},
	})
	if !strings.Contains(m.progressText, "Artifact chunking deliverable.txt seq 3/4 512B") {
		t.Fatalf("progressText = %q, want artifact continuation progress", m.progressText)
	}
	if cmd != nil {
		t.Fatalf("artifact continuation event should update incrementally without workspace refresh")
	}
	if len(m.artifactContinuations) != 1 || m.artifactContinuations[0].TargetRef != "deliverable.txt" {
		t.Fatalf("artifactContinuations = %+v, want incrementally updated item", m.artifactContinuations)
	}
}

func TestModelWorkspaceContextPartialUpdateDoesNotClearExistingDisplay(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "existing goal"
	m.transcript = []transcriptItem{{Kind: "Goal", Text: "existing goal"}}
	m.dodItems = []string{"existing dod"}
	m.runtimeEvents = []string{"existing runtime event"}
	m.patches = []backend.PatchInfo{{ID: "patch-1", TargetRef: "old.txt", State: common.PatchStateApplied}}
	m.llmUsage = backend.LLMUsageSummaryInfo{EntryTotal: 2, TotalTokens: 100}
	m.activeRunID = "run-1"

	m.applyWorkspaceContext(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-1", State: common.WorkspaceStateRunning, ActiveRunID: "run-1"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       common.RunStateRunning,
		},
		DisplayRun: &backend.RunInfo{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       common.RunStateRunning,
		},
		WorkspaceHasLive: true,
	})

	if m.currentTask != "existing goal" || len(m.dodItems) != 1 || m.dodItems[0] != "existing dod" {
		t.Fatalf("definitions were cleared: task=%q dod=%v", m.currentTask, m.dodItems)
	}
	if len(m.runtimeEvents) != 1 || len(m.patches) != 1 || m.llmUsage.EntryTotal != 2 {
		t.Fatalf("display state was cleared: events=%v patches=%v llm=%+v", m.runtimeEvents, m.patches, m.llmUsage)
	}
}

func TestModelWorkspacePanelKeepsStateVisible(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-f2e4da0c7199a047", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:    "ws-f2e4da0c7199a047",
		Title: "workorch",
		State: "completed",
	}}})
	m = asModel(next)
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "> workorch  completed") {
		t.Fatalf("workspace panel missing selected workspace name:\n%s", view)
	}
	if strings.Contains(view, "c...") {
		t.Fatalf("workspace panel still truncates state:\n%s", view)
	}
}

func TestModelWorkspacePanelShowsActiveRunWhenPresent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-1",
		Title:       "Project",
		State:       "running",
		ActiveRunID: "run-016-replan-001",
	}}})
	m = asModel(next)
	m.width = 120
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "running  run-016-re") {
		t.Fatalf("workspace panel missing active run:\n%s", view)
	}
	if !strings.Contains(view, "current ws: 0 decisions") || !strings.Contains(view, "current ws: 0 approvals") {
		t.Fatalf("workspace panel blocker scope is unclear:\n%s", view)
	}
}

func TestModelWorkspacePanelCountsOnlyActiveRunBlockers(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-1", Title: "Project", State: common.WorkspaceStateRunning, ActiveRunID: "run-active"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-active",
			WorkspaceID: "ws-1",
			State:       common.RunStateRunning,
		},
		Decisions: []backend.DecisionInfo{
			{ID: "ticket-active", WorkspaceID: "ws-1", RunID: "run-active", State: "open"},
			{ID: "ticket-old", WorkspaceID: "ws-1", RunID: "run-old", State: "open"},
		},
		Approvals: []backend.ApprovalInfo{
			{ID: "approval-old", WorkspaceID: "ws-1", RunID: "run-old", State: "pending"},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	m.width = 120
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "current ws: 1 decisions") || !strings.Contains(view, "current ws: 0 approvals") {
		t.Fatalf("workspace panel blocker counts include stale run blockers:\n%s", view)
	}
}

func TestModelWorkspacePanelDisambiguatesDuplicateNames(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1111111111111111", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-1111111111111111", Title: "llm acceptance workspace", State: "completed"},
		{ID: "ws-2222222222222222", Title: "llm acceptance workspace", State: "failed"},
	}})
	m = asModel(next)
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	for _, want := range []string{
		"> ll... ws-1111111  completed",
		"  llm a... ws-2222222  failed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("workspace panel missing disambiguated line %q:\n%s", want, view)
		}
	}
}

func TestModelSubmitGoalStartsRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{ID: "ws-1", Title: "Project", State: "active"}}})
	m = asModel(next)
	m.input = "/goal finish task"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	msg := cmd()
	next, _ = asModel(next).Update(msg)
	m = asModel(next)
	if m.activeRunID != "run-1" || !m.running {
		t.Fatalf("run state = %q/%v, want run-1/running", m.activeRunID, m.running)
	}
	if m.currentTask != "finish task" {
		t.Fatalf("current task = %q, want submitted input", m.currentTask)
	}
}

func TestModelPlainInputWithActiveRunningRunDoesNotResume(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("继续处理")

	next, cmd := m.submitInput()
	m = asModel(next)

	if cmd == nil {
		t.Fatal("plain input with active run should resume run")
	}
	msg := cmd()
	_, _ = m.Update(msg)
	if len(be.goals) != 0 {
		t.Fatalf("goals = %v, want no goal created for plain input", be.goals)
	}
	if be.lastSnapshotWorkspace != "ws-1" || be.lastGetWorkspace != "" || be.lastListWorkspace != "" || be.lastResumeWorkspace != "" {
		t.Fatalf("snapshot/get/list/resume workspace scope = %q/%q/%q/%q, want ws-1/empty/empty/empty", be.lastSnapshotWorkspace, be.lastGetWorkspace, be.lastListWorkspace, be.lastResumeWorkspace)
	}
	if len(be.resumes) != 0 || len(be.resumePatches) != 0 {
		t.Fatalf("resumes/patches = %v/%v, want no resume for already-running run", be.resumes, be.resumePatches)
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Run == nil || runMsg.Run.State != common.RunStateRunning {
		t.Fatalf("message = %#v, want running RunControlMsg", msg)
	}
}

func TestModelPlainInputIdleWorkspaceSubmitsGoal(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = ""
	m.running = false
	m.setInput("补充新的目标")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("plain input in idle workspace should submit goal")
	}
	msg := cmd()
	_, _ = m.Update(msg)

	if len(be.goals) != 1 || be.goals[0] != "补充新的目标" {
		t.Fatalf("goals = %v, want plain input as goal", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume for idle goal input", be.resumes)
	}
}

func TestModelPlainInputUsesCurrentWorkspaceLiveRunOnly(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-b", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive},
	}
	m.activeWorkspaceID = "ws-b"
	m.selectedWorkspaceID = "ws-b"
	m.activeRunID = "run-a"
	m.running = true
	m.progress = progressRunning
	m.setInput("启动 B 工作区")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error != nil || msg.Run == nil {
		t.Fatalf("submit msg = %#v, want scoped RunStartedMsg", msg)
	}
	if msg.WorkspaceID != "ws-b" || msg.Run.WorkspaceID != "ws-b" {
		t.Fatalf("run scope = msg:%q run:%q, want ws-b", msg.WorkspaceID, msg.Run.WorkspaceID)
	}
	_, _ = next.(*AppModel).Update(msg)

	if len(be.goals) != 1 || be.goals[0] != "启动 B 工作区" {
		t.Fatalf("goals = %v, want new goal for current workspace", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume from another workspace running state", be.resumes)
	}
}

func TestModelPlainInputFailedWorkspaceIgnoresStaleActiveRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "failed", State: common.WorkspaceStateFailed, ActiveRunID: "run-stale"},
	}
	m.activeRunID = "run-stale"
	m.running = true
	m.progress = progressRunning
	m.setInput("重新整理目标")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected goal submission command for failed workspace")
	}
	if m.progressText == "Continuing active run" {
		t.Fatalf("progressText = %q, want non-continuation submission text", m.progressText)
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error != nil || msg.Run == nil {
		t.Fatalf("submit msg = %#v, want RunStartedMsg", msg)
	}
	_, _ = m.Update(msg)
	if len(be.goals) != 1 || be.goals[0] != "重新整理目标" {
		t.Fatalf("goals = %v, want failed workspace input submitted as new goal", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no stale run resume", be.resumes)
	}
}

func TestModelIgnoresRunStartedErrorForInactiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.progressText = "active workspace"

	next, cmd := m.Update(RunStartedMsg{
		WorkspaceID: "ws-a",
		Error:       errors.New("stale activation failed"),
	})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale run started error should not trigger command")
	}
	if m.lastError != nil || m.progress == progressFailed || m.progressText != "active workspace" {
		t.Fatalf("stale error polluted active state: err=%v progress=%s text=%q", m.lastError, m.progress, m.progressText)
	}
}

func TestModelGoalSlashCommandCanSubmitRunControlWordsAsGoal(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("/goal 继续处理")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected goal submit command")
	}
	msg := cmd()
	_, _ = next.(*AppModel).Update(msg)

	if len(be.goals) != 1 || be.goals[0] != "继续处理" {
		t.Fatalf("goals = %v, want explicit /goal text", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume for explicit /goal", be.resumes)
	}
}

func TestModelRunResumeCommandDoesNotResumeAlreadyRunningRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("/run resume 补充约束：优先修复当前失败项")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected resume command")
	}
	msg := cmd()
	_, _ = next.(*AppModel).Update(msg)

	if len(be.goals) != 0 {
		t.Fatalf("goals = %v, want no goal for active-run operator input", be.goals)
	}
	if len(be.resumes) != 0 || len(be.resumePatches) != 0 {
		t.Fatalf("resumes/patches = %v/%v, want no resume for already-running run", be.resumes, be.resumePatches)
	}
	if len(be.activations) != 0 {
		t.Fatalf("activations = %d, want no activation for already-running run", len(be.activations))
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Run == nil || runMsg.Run.State != common.RunStateRunning {
		t.Fatalf("message = %#v, want running RunControlMsg", msg)
	}
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)
	if m.progress != progressRunning || !m.running {
		t.Fatalf("progress/running = %s/%v, want running state", m.progress, m.running)
	}
	if !strings.Contains(m.progressText, "already running") {
		t.Fatalf("progressText = %q, want already-running guidance", m.progressText)
	}
}

func TestModelRunResumeWithoutLiveRunDoesNotPredeclareContinuation(t *testing.T) {
	be := newRecordingBE(t, common.RunStateFailed)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: "ws-1", Title: "failed", State: common.WorkspaceStateFailed},
		Outcome: backend.WorkspaceOutcomeInfo{
			WorkspaceID: "ws-1",
			State:       string(common.RunStateFailed),
			RunID:       "run-failed",
			Terminal:    true,
			Reason:      "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
		},
		TaskProgress: backend.TaskProgressInfo{
			WorkspaceID: "ws-1",
			RunID:       "run-failed",
			State:       string(common.RunStateFailed),
			Recovery: backend.RecoveryPlanInfo{
				Reason:     "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
				NextAction: "inspect_failure_reason_and_evidence",
			},
		},
		Runs: []backend.RunInfo{{
			ID:            "run-failed",
			WorkspaceID:   "ws-1",
			State:         common.RunStateFailed,
			FailureReason: "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
		}},
	}
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "failed", State: common.WorkspaceStateFailed},
	}
	m.setInput("/run resume retry with current workspace")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected run resume command")
	}
	if m.progressText != "Checking workspace run state" {
		t.Fatalf("progressText = %q, want backend state check", m.progressText)
	}
	msg, ok := cmd().(RunControlMsg)
	if !ok || msg.Error != nil || msg.Run == nil {
		t.Fatalf("run resume msg = %#v, want RunControlMsg with resumed run", msg)
	}
	if msg.Run.ID != "run-failed" || msg.Run.State != common.RunStateRunning || !strings.Contains(msg.Message, "Resuming") {
		t.Fatalf("run/message = %+v/%q, want resumed failed run", msg.Run, msg.Message)
	}
	if len(be.resumes) != 1 || be.resumes[0] != "run-failed" {
		t.Fatalf("resumes = %v, want terminal run resume through backend", be.resumes)
	}
	if len(be.activations) != 0 {
		t.Fatalf("activations = %d, want no implicit activation for /run resume", len(be.activations))
	}
	next, _ = m.Update(msg)
	m = asModel(next)
	if m.progress != progressRunning || !m.running {
		t.Fatalf("progress/running = %s/%v, want running after backend resume", m.progress, m.running)
	}
	if strings.Contains(m.progressText, "use /run restart") {
		t.Fatalf("progressText = %q, want no restart guidance for resume path", m.progressText)
	}
}

func TestModelRunResumeIgnoresStaleRunIDFromAnotherWorkspace(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-2", "")
	m.activeRunID = "run-from-ws-1"
	m.setInput("/run resume 切换后继续当前工作区")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected resume command")
	}
	msg, ok := cmd().(RunControlMsg)
	if !ok || msg.Error != nil || msg.Run == nil {
		t.Fatalf("resume msg = %#v, want RunControlMsg with run", msg)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume for already-running current workspace run", be.resumes)
	}
	if be.lastSnapshotWorkspace != "ws-2" || be.lastGetWorkspace != "" || be.lastListWorkspace != "" || be.lastResumeWorkspace != "" {
		t.Fatalf("snapshot/get/list/resume workspace scope = %q/%q/%q/%q, want ws-2/empty/empty/empty", be.lastSnapshotWorkspace, be.lastGetWorkspace, be.lastListWorkspace, be.lastResumeWorkspace)
	}
	if len(be.activations) != 0 || msg.Run.WorkspaceID != "ws-2" {
		t.Fatalf("activation/run = %d/%+v, want current run for ws-2", len(be.activations), msg.Run)
	}

	updated, _ := next.(*AppModel).Update(msg)
	got := asModel(updated)
	if got.activeRunID != "run-resume" || got.progress != progressRunning || !got.running {
		t.Fatalf("run state = %q/%s/%v, want current workspace run", got.activeRunID, got.progress, got.running)
	}
}

func TestModelRunRestartCommandRestartsWorkspaceWithOperatorInput(t *testing.T) {
	be := newRecordingBE(t, common.RunStateCompleted)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.currentTask = "已有目标"
	m.result = "old final result"
	m.artifacts = []string{"old-artifact"}
	m.runtimeEvents = []string{"old event"}
	m.setInput("/run restart 补充信息：继续基于现有目标修复")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected restart command")
	}
	msg := cmd()
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)

	if len(be.goals) != 0 {
		t.Fatalf("goals = %v, want no goal created for restart input", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want terminal run to activate a new run", be.resumes)
	}
	if len(be.activations) != 0 {
		t.Fatalf("activations = %d, want no activation for restart command", len(be.activations))
	}
	if len(be.restarts) != 1 {
		t.Fatalf("restarts = %d, want one restart", len(be.restarts))
	}
	queue, ok := be.restarts[0].ExecutionContextPatch["operatorInputQueue"].([]map[string]any)
	if !ok || len(queue) != 1 || queue[0]["content"] != "补充信息：继续基于现有目标修复" {
		t.Fatalf("operatorInputQueue = %#v, want queued restart input", be.restarts[0].ExecutionContextPatch["operatorInputQueue"])
	}
	if m.activeRunID != "run-new" || !m.running {
		t.Fatalf("run state = %q/%v, want restarted run-new/running", m.activeRunID, m.running)
	}
	if m.result != "" || len(m.artifacts) != 0 || len(m.runtimeEvents) != 0 {
		t.Fatalf("old run display not cleared: result=%q artifacts=%v events=%v", m.result, m.artifacts, m.runtimeEvents)
	}
}

func TestModelRunRestartSlashCommandActivatesWorkspaceWithoutCreatingGoal(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.currentTask = "已有目标"
	m.setInput("/run restart 重新执行并纳入刚补充的信息")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected restart command")
	}
	msg := cmd()
	_, _ = next.(*AppModel).Update(msg)

	if len(be.goals) != 0 {
		t.Fatalf("goals = %v, want no goal created for /run restart", be.goals)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want /run restart to activate directly", be.resumes)
	}
	if len(be.activations) != 0 {
		t.Fatalf("activations = %d, want no activation for /run restart", len(be.activations))
	}
	if len(be.restarts) != 1 {
		t.Fatalf("restarts = %d, want one restart", len(be.restarts))
	}
	latest, ok := be.restarts[0].ExecutionContextPatch["latestOperatorInput"].(map[string]any)
	if !ok || latest["content"] != "重新执行并纳入刚补充的信息" {
		t.Fatalf("latestOperatorInput = %#v, want restart dialog input", be.restarts[0].ExecutionContextPatch["latestOperatorInput"])
	}
}

func TestModelNonCanonicalRunCommandsAreNotSupported(t *testing.T) {
	for _, input := range []string{"/resume", "/restart", "/rerun", "/run", "/run rerun"} {
		m := NewAppModel(newFake(t), "ws-1", "")
		m.activeRunID = "run-1"
		m.progress = progressRunning
		m.setInput(input)

		next, cmd := m.submitInput()
		m = asModel(next)
		if cmd != nil {
			t.Fatalf("%s returned command, want unsupported", input)
		}
		if m.lastError == nil || !strings.Contains(m.lastError.Error(), "unknown") {
			t.Fatalf("%s error = %v, want unsupported command error", input, m.lastError)
		}
	}
}

func TestModelRunResumeCommandIsSupported(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("/run resume")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected /run resume command")
	}
	if m.lastError != nil {
		t.Fatalf("lastError = %v, want nil", m.lastError)
	}
	if m.progress != progressSubmitting || !strings.Contains(m.progressText, "Resuming active run") {
		t.Fatalf("progress = %s %q, want resuming submit state", m.progress, m.progressText)
	}
}

func TestModelHelpPrefersRunResume(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	help := stripANSI(m.renderHelp(160))
	if !strings.Contains(help, "/run resume") {
		t.Fatalf("help = %q, want /run resume", help)
	}
}

func TestModelRunControlRunningClearsStaleResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "stale result"
	m.artifacts = []string{"stale-artifact"}
	m.runtimeEvents = []string{"stale event"}

	next, _ := m.Update(RunControlMsg{
		Run:         &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-1", State: common.RunStateRunning},
		WorkspaceID: "ws-1",
		Message:     "Restarting workspace run",
	})
	m = asModel(next)
	if m.activeRunID != "run-new" || m.progress != progressRunning || !m.running {
		t.Fatalf("run state = %q/%s/%v, want running run-new", m.activeRunID, m.progress, m.running)
	}
	if m.result != "" || len(m.artifacts) != 0 || len(m.runtimeEvents) != 0 {
		t.Fatalf("stale display not cleared on run control: result=%q artifacts=%v events=%v", m.result, m.artifacts, m.runtimeEvents)
	}
}

func TestModelClearRunDisplayStateSyncsViewportHeightAfterResultClear(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 100
	m.height = 18
	m.currentTask = "goal"
	m.dodItems = []string{"dod one", "dod two", "dod three"}
	m.result = strings.Repeat("result line\n", 30)
	m.mainViewport = newMainViewport()
	m.mainViewport.Height = 3
	m.mainViewport.Width = 20

	m.clearRunDisplayState()

	width, height := m.contentDimensions()
	if m.result != "" {
		t.Fatalf("result not cleared: %q", m.result)
	}
	if m.mainViewport.Height != height || m.mainViewport.Width != width {
		t.Fatalf("viewport size = %d/%d, want synced %d/%d", m.mainViewport.Width, m.mainViewport.Height, width, height)
	}
}

func TestModelRunStartedDoesNotOverwriteStepProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"kind": "llm/workspace-plan"},
	})
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	if m.progressText != "Working: llm/workspace_execution" {
		t.Fatalf("progress text = %q, want step progress preserved", m.progressText)
	}
}

func TestModelStepProgressShowsPathCommandAndMCPDetails(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 18

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "file/read",
			"input": map[string]any{
				"summary": map[string]any{"path": "stats.go"},
			},
		},
	})
	if m.progressText != "Working: file/read stats.go" {
		t.Fatalf("file/read progress = %q", m.progressText)
	}

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "shell/command",
			"input": map[string]any{
				"summary": map[string]any{
					"command": "go",
					"args":    []string{"test", "./..."},
				},
			},
		},
	})
	if m.progressText != "Working: shell/command go test ./..." {
		t.Fatalf("shell progress = %q", m.progressText)
	}

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "mcp/tool_call",
			"input": map[string]any{
				"summary": map[string]any{
					"serverId": "mcp-lsp-bridge",
					"toolName": "textDocument/definition",
				},
			},
		},
	})
	if m.progressText != "Working: mcp/tool_call mcp-lsp-bridge:textDocument/definition" {
		t.Fatalf("mcp progress = %q", m.progressText)
	}
}

func TestModelStepProgressAppendsRuntimeEvents(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 16

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "mcp/tool_call",
			"input": map[string]any{
				"summary": map[string]any{"serverId": "mcp-lsp-bridge", "toolName": "textDocument/references"},
			},
		},
	})

	view := stripANSI(m.View())
	if !strings.Contains(view, "Runtime Events") || !strings.Contains(view, "Working: mcp/tool_call mcp-lsp-bridge:textDocument/references") {
		t.Fatalf("view missing runtime event log:\n%s", view)
	}
}

func TestModelPatchMemoryAndLLMUsageEventsUpdateIncrementally(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	entry := m.ensureWorkspaceRuntimeState("ws-1")

	if cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventPatchApplied,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"patchId":      "patch-1",
			"state":        string(common.PatchStateApplied),
			"targetRef":    "main.go",
			"summary":      "updated target",
			"addedLines":   3,
			"deletedLines": 1,
		},
	}); cmd != nil {
		t.Fatal("patch event should not fetch full workspace context")
	}
	if len(m.patches) != 1 || m.patches[0].ID != "patch-1" || m.patches[0].AddedLines != 3 {
		t.Fatalf("patches = %+v, want incremental patch", m.patches)
	}

	if cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventMemoryRecallRecorded,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"traceId":            "trace-1",
			"status":             "selected",
			"source":             "recall",
			"stage":              "planning",
			"retrievedMemoryIds": []any{"mem-1", "mem-2"},
		},
	}); cmd == nil {
		t.Fatal("memory recall event should refresh workspace context after incremental update")
	}
	if len(m.memoryRecalls) != 1 || len(m.memoryRecalls[0].RetrievedMemoryIDs) != 2 {
		t.Fatalf("memoryRecalls = %+v, want incremental recall", m.memoryRecalls)
	}

	entry.ContextInFlight = false
	entry.ContextPending = false
	if cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventLLMUsageRecorded,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"providerId":   "provider-a",
			"model":        "model-a",
			"inputTokens":  10,
			"outputTokens": 5,
			"totalTokens":  15,
			"contextBudget": map[string]any{
				"maxTokens": 2048,
			},
		},
	}); cmd == nil {
		t.Fatal("llm usage event should refresh workspace context after incremental update")
	}
	if m.llmUsage.EntryTotal != 1 || m.llmUsage.TotalTokens != 15 || m.llmContext["maxTokens"] != 2048 {
		t.Fatalf("llm usage/context = %+v/%+v, want incremental usage", m.llmUsage, m.llmContext)
	}
}

func TestModelBlockedWorkspaceInputRefreshesBlockerInsteadOfContinuing(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:          "ws-1",
		State:       common.WorkspaceStateWaitingDecision,
		ActiveRunID: "run-1",
	}}
	m.activeRunID = "run-1"
	m.input = "resume"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("blocked workspace input should refresh blocker details")
	}
	if len(be.resumes) != 0 || len(be.goals) != 0 {
		t.Fatalf("unexpected backend calls: resumes=%v goals=%v", be.resumes, be.goals)
	}
	if m.progress != progressWaitingDecision || !strings.Contains(m.progressText, "Waiting for") {
		t.Fatalf("progress = %s %q, want waiting decision", m.progress, m.progressText)
	}
	if m.input != "resume" {
		t.Fatalf("input = %q, want preserved answer text", m.input)
	}
}

func TestModelRunResumeDoesNotBypassBlockingDecision(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-1",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
	}}
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.input = "/run resume"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("run resume with decision should refresh blocker details")
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume while decision is open", be.resumes)
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" || m.progress != progressWaitingDecision {
		t.Fatalf("blocking decision/progress = %+v/%s", m.blockingDecision, m.progress)
	}
}

func TestResumeRunReturnsBlockedRunWithoutResume(t *testing.T) {
	be := newRecordingBE(t, common.RunStateBlockedOnDecision)
	msg := ResumeRun(be, "ws-1", "run-1", "resume")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.State != common.RunStateBlockedOnDecision {
		t.Fatalf("run = %+v, want blocked decision", runMsg.Run)
	}
	if !strings.Contains(runMsg.Message, "waiting for decision") {
		t.Fatalf("message = %q, want waiting decision", runMsg.Message)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no resume", be.resumes)
	}
}

func TestResumeRunActivatesWorkspaceWhenNoActiveRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateCompleted)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: "ws-1"},
	}
	msg := ResumeRun(be, "ws-1", "", "resume from tui")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.ID != "run-new" || runMsg.Run.State != common.RunStateRunning {
		t.Fatalf("run = %+v, want activated workspace run", runMsg.Run)
	}
	if len(be.activations) != 1 {
		t.Fatalf("activations = %d, want workspace activation", len(be.activations))
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want activation path", be.resumes)
	}
	if got := latestOperatorInputFromPatch(be.activations[0].ExecutionContextPatch); got != "resume from tui" {
		t.Fatalf("operator input = %q, want resume text", got)
	}
}

func TestResumeRunActivatesCreatedRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateCreated)
	msg := ResumeRun(be, "ws-1", "run-resume", "start created run")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.ID != "run-resume" {
		t.Fatalf("run = %+v, want resumed created run", runMsg.Run)
	}
	if len(be.activations) != 0 {
		t.Fatalf("activations = %d, want resume command path", len(be.activations))
	}
	if len(be.resumes) != 1 || be.resumes[0] != "run-resume" {
		t.Fatalf("resumes = %v, want created run resume", be.resumes)
	}
	if got := latestOperatorInputFromPatch(be.resumePatches[0]); got != "start created run" {
		t.Fatalf("operator input = %q, want created run input", got)
	}
}

func latestOperatorInputFromPatch(patch map[string]any) string {
	if len(patch) == 0 {
		return ""
	}
	latest, _ := patch["latestOperatorInput"].(map[string]any)
	content, _ := latest["content"].(string)
	return content
}

func TestModelInspectCommandsRenderCurrentState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.currentTask = "finish task"
	m.dodItems = []string{"dod one"}
	m.runtimeEvents = []string{"Working: shell/command go test ./..."}
	m.patches = []backend.PatchInfo{{ID: "patch-1", TargetRef: "main.go", State: common.PatchStateApplied}}
	m.artifacts = []string{"result.md"}

	for _, tc := range []struct {
		input string
		want  string
	}{
		{"/status", "finish task"},
		{"/coverage", "dod one"},
		{"/timeline", "go test ./..."},
		{"/patches", "main.go"},
		{"/artifacts", "result.md"},
	} {
		m.setInput(tc.input)
		next, cmd := m.submitInput()
		m = asModel(next)
		if cmd != nil {
			t.Fatalf("%s returned command, want local render", tc.input)
		}
		if !strings.Contains(m.result, tc.want) {
			t.Fatalf("%s result = %q, want %q", tc.input, m.result, tc.want)
		}
	}
}

func TestModelStatusRendersSchedulerDiagnosticsWithoutTriggeringCommands(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.schedulerDiagnostics = &backend.SchedulerDiagnosticsInfo{
		LatestContinuation: &backend.ContinuationDiagnosticInfo{
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			Kind:               "replan_continuation",
			State:              "failed",
			Reason:             "owner_command_failed",
			Detail:             "owner command failed after accepted scheduler event",
			Recoverability:     "manual_retry_required",
			RecommendedActions: []string{"run_resume", "workspace_restart"},
		},
		Watchdog: backend.WatchdogDiagnosticsInfo{TimerRegistrations: 1},
	}

	m.setInput("/status")
	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil {
		t.Fatal("/status returned command, want local projection render only")
	}
	for _, want := range []string{
		"replan_continuation failed owner_command_failed",
		"manual_retry_required run_resume,workspace_restart",
		"watchdog: registered",
	} {
		if !strings.Contains(m.result, want) {
			t.Fatalf("/status result = %q, missing %q", m.result, want)
		}
	}
	if len(be.decisions) != 0 || be.approval.lastPayload != nil {
		t.Fatalf("/status triggered governance command: decisions=%+v approvalPayload=%+v", be.decisions, be.approval.lastPayload)
	}
	if m.progressText != "Showing workspace status" {
		t.Fatalf("progressText = %q, want status display text", m.progressText)
	}
}

func TestModelStepProgressAutoScrollsToLatest(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 16
	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("tail-line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()
	if m.mainViewport == nil {
		t.Fatal("expected viewport initialized")
	}
	before := m.mainViewport.YOffset
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "shell/command",
			"input": map[string]any{
				"summary": map[string]any{"command": "go", "args": []string{"test", "./..."}},
			},
		},
	})
	_ = m.View()
	if m.mainViewport.YOffset < before {
		t.Fatalf("viewport offset regressed: before=%d after=%d", before, m.mainViewport.YOffset)
	}
	if m.mainViewport.YOffset == 0 {
		t.Fatalf("viewport should follow latest content, got offset=%d", m.mainViewport.YOffset)
	}
}

func TestModelRuntimeUpdateDoesNotStealManuallyScrolledViewport(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 16
	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("manual-scroll-line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = asModel(next)
	offsetAfterScroll := m.mainViewport.YOffset
	if offsetAfterScroll == 0 {
		t.Fatal("expected manual scroll to move viewport")
	}

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"kind": "shell/command",
			"input": map[string]any{
				"summary": map[string]any{"command": "go", "args": []string{"test", "./..."}},
			},
		},
	})
	_ = m.View()
	if m.mainViewport.YOffset != offsetAfterScroll {
		t.Fatalf("runtime update stole viewport: before=%d after=%d", offsetAfterScroll, m.mainViewport.YOffset)
	}
	if !m.mainViewportPendingUpdate {
		t.Fatal("expected pending update marker after runtime update while manually scrolled")
	}
	if !strings.Contains(stripANSI(m.View()), "new updates") {
		t.Fatalf("status bar missing new update marker:\n%s", stripANSI(m.View()))
	}
}

func TestModelRunStartedDoesNotOverwriteDecisionProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventDecisionCreated,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"ticketId": "ticket-1", "reason": "缺少必要参数"},
	})
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	if m.progress != progressWaitingDecision || m.progressText != "缺少必要参数" {
		t.Fatalf("progress = %s/%q, want decision state preserved", m.progress, m.progressText)
	}
}

func TestModelWorkspaceStateChangedUpdatesMainProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeRunID = "run-1"
	next, _ := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"state": string(common.WorkspaceStateFailed), "activeRunId": "run-1"},
	}})
	m = asModel(next)
	if m.running {
		t.Fatal("expected running=false after workspace failed event")
	}
	if m.progress != progressFailed {
		t.Fatalf("progress = %s, want failed", m.progress)
	}
}

func TestModelBackendRunCompletionCallback(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"state": "completed"},
	}})
	m = asModel(next)
	if m.running {
		t.Fatal("expected running=false after completion event")
	}
	if m.progress != progressCompleted {
		t.Fatalf("progress = %s, want completed", m.progress)
	}
}

func TestModelRunFailurePrefersActionableHintOverTimeout(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepCompleted,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"state": "failed",
			"kind":  "llm/workspace_execution",
			"output": map[string]any{
				"reason": "invalid argument: ssh/command requires password or privateKey/privateKeyPath",
			},
		},
	})
	next, _ := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"state":         string(common.RunStateFailed),
			"failureReason": "context deadline exceeded (Client.Timeout or context cancellation while reading body)",
		},
	}})
	m = asModel(next)
	if strings.Contains(strings.ToLower(m.failureReason), "context deadline exceeded") {
		t.Fatalf("failure reason should hide timeout noise:\n%s", m.failureReason)
	}
	if !strings.Contains(m.failureReason, "SSH 命令缺少 password 或 privateKey/privateKeyPath") {
		t.Fatalf("failure reason should surface actionable hint, got %q", m.failureReason)
	}
}

func TestModelViewIsResultFirstAndSuppressesEventNoise(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "Project", DeliverableDirectory: "/tmp/project", State: "active"},
	}})
	m = asModel(next)
	m.width = 100
	m.height = 24
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"kind": "llm/workspace_execution"},
	})
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventResultAccepted,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"summary": "查询完成：杭州多云"},
	})
	view := stripANSI(m.View())
	for _, forbidden := range []string{"Ready.", "run:", "step:", "goal:", "answer:"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("view contains forbidden event noise %q:\n%s", forbidden, view)
		}
	}
	if !strings.Contains(view, "查询完成") {
		t.Fatalf("view missing result summary:\n%s", view)
	}
}

func TestModelInputRendersAtBottom(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{ID: "ws-1", Title: "Project", State: "active"}}})
	m = asModel(next)
	m.width = 80
	m.height = 12
	m.progress = progressRunning
	m.progressText = "runtime status"
	view := stripANSI(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("view too short:\n%s", view)
	}
	if !strings.HasPrefix(lines[len(lines)-3], "Running - runtime status") {
		t.Fatalf("expected runtime status immediately above input, got %q in:\n%s", lines[len(lines)-3], view)
	}
	if !strings.HasPrefix(lines[len(lines)-2], "ws-1 >") {
		t.Fatalf("expected bottom input above help, got %q in:\n%s", lines[len(lines)-2], view)
	}
}

func TestModelInputPromptUsesActiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-f2e4da0c7199a047", "")
	m.width = 80
	m.height = 12
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-f2e4da0 >") {
		t.Fatalf("view missing workspace input prompt:\n%s", view)
	}
}

func TestModelInputPromptReflectsLiveActiveRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-1", State: common.WorkspaceStateRunning, ActiveRunID: "run-1"}}
	m.activeRunID = "run-1"
	m.progress = progressRunning
	m.running = true
	m.width = 80
	m.height = 12
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-1 running >") {
		t.Fatalf("view missing running workspace input prompt:\n%s", view)
	}
	if !strings.Contains(view, "run run-1") {
		t.Fatalf("status bar missing active run:\n%s", view)
	}
}

func TestModelTerminalRunWithRetainedRunIDReturnsGoalPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-97549a108e19d500", "")
	next, _ := m.Update(RunControlMsg{
		Run:         &backend.RunInfo{ID: "run-completed", WorkspaceID: "ws-97549a108e19d500", State: common.RunStateCompleted},
		WorkspaceID: "ws-97549a108e19d500",
		Message:     "Completed",
	})
	m = asModel(next)
	m.width = 100
	m.height = 14

	if m.activeRunID != "run-completed" {
		t.Fatalf("activeRunID = %q, want retained terminal run id for status context", m.activeRunID)
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "ws-97549a1 resume >") {
		t.Fatalf("terminal run should not render resume prompt:\n%s", view)
	}
	if !strings.Contains(view, "ws-97549a1 > |") {
		t.Fatalf("terminal run should return to neutral prompt:\n%s", view)
	}
}

func TestModelRunResumeCommandClearsInputAndReturnsRunningPrompt(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	m := NewAppModel(be, "ws-97549a108e19d500", "")
	m.activeRunID = "run-resume"
	m.progress = progressRunning
	m.running = true
	projectRunningWorkspace(&m, "ws-97549a108e19d500", "run-resume")
	m.setInput("/run resume")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected run resume command")
	}
	msg := cmd()
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)
	m.width = 100
	m.height = 14

	if m.input != "" || m.inputCursor != 0 {
		t.Fatalf("input = %q cursor=%d, want cleared", m.input, m.inputCursor)
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "ws-97549a1 resume >") || strings.Contains(view, "ws-97549a1 run >") {
		t.Fatalf("view should not keep /run resume command text as prompt:\n%s", view)
	}
	if !strings.Contains(view, "ws-97549a1 running > |") {
		t.Fatalf("live run should return to running input prompt after /run resume:\n%s", view)
	}
}

func TestModelInputEditingSupportsSpacesAndArrowKeys(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("abc")},
		{Type: tea.KeyLeft},
		{Type: tea.KeySpace, Runes: []rune(" ")},
		{Type: tea.KeyRunes, Runes: []rune("X")},
		{Type: tea.KeyEnd},
		{Type: tea.KeyRunes, Runes: []rune("Y")},
	} {
		next, _ := m.Update(msg)
		m = asModel(next)
	}
	if m.input != "ab XcY" {
		t.Fatalf("input = %q, want edited text", m.input)
	}
	if m.inputCursor != len([]rune(m.input)) {
		t.Fatalf("cursor = %d, want end", m.inputCursor)
	}
	m.width = 80
	m.height = 12
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-1 > ab XcY|") {
		t.Fatalf("view missing edited input and cursor:\n%s", view)
	}
}

func TestModelInputHistoryAndMultilineEditing(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.input = "first goal"
	m.inputCursor = len([]rune(m.input))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	m.input = "second goal"
	m.inputCursor = len([]rune(m.input))
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = asModel(next)
	if m.input != "second goal" {
		t.Fatalf("history previous = %q, want second goal", m.input)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = asModel(next)
	if m.input != "first goal" {
		t.Fatalf("history previous twice = %q, want first goal", m.input)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = asModel(next)
	if m.input != "second goal" {
		t.Fatalf("history next = %q, want second goal", m.input)
	}

	m.clearInput()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line one")})
	m = asModel(next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = asModel(next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line two")})
	m = asModel(next)
	if m.input != "line one\nline two" {
		t.Fatalf("multiline input = %q", m.input)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "line one") || !strings.Contains(view, "line two") {
		t.Fatalf("view missing multiline input:\n%s", view)
	}
}

func TestModelLargeMultilinePasteRendersCollapsedInputPreview(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{ID: "ws-1", Title: "Project", State: "active"}}})
	m = asModel(next)
	var pasted strings.Builder
	for i := 1; i <= 20; i++ {
		if i > 1 {
			pasted.WriteString("\n")
		}
		fmt.Fprintf(&pasted, "pasted-line-%02d with detailed text", i)
	}
	text := pasted.String()

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	m = asModel(next)
	if m.input != text {
		t.Fatalf("input was modified by folded preview")
	}
	m.width = 100
	m.height = 14
	view := stripANSI(m.View())
	if !strings.Contains(view, "[folded input: 20 lines") || !strings.Contains(view, "pasted-line-20") {
		t.Fatalf("view missing folded paste preview:\n%s", view)
	}
	if !strings.Contains(view, "head: pasted-line-01") {
		t.Fatalf("folded preview should include paste head:\n%s", view)
	}
	if strings.Contains(view, "pasted-line-10") {
		t.Fatalf("folded preview leaked hidden paste body:\n%s", view)
	}

	m.setInput("/goal " + text)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	msg := cmd()
	next, _ = asModel(next).Update(msg)
	m = asModel(next)
	if m.currentTask != text {
		t.Fatalf("submitted task length = %d, want original pasted text length %d", len(m.currentTask), len(text))
	}
}

func TestModelSlashCompletionHelpAndClearCommands(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.input = "/he"
	m.inputCursor = len([]rune(m.input))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(next)
	if m.input != "/help" {
		t.Fatalf("completion = %q, want /help", m.input)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd != nil || !m.helpOpen {
		t.Fatalf("help command state cmd/help = %v/%v, want open help without cmd", cmd != nil, m.helpOpen)
	}
	m.width = 80
	m.height = 20
	if view := stripANSI(m.View()); !strings.Contains(view, "WorkOrch Help") || !strings.Contains(view, "/status") || !strings.Contains(view, "/coverage") {
		t.Fatalf("help overlay missing content:\n%s", view)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = asModel(next)
	if m.helpOpen {
		t.Fatal("esc should close help overlay")
	}

	m.result = "old result"
	m.appendTranscript("Goal", "old goal")
	m.input = "/clear"
	m.inputCursor = len([]rune(m.input))
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if m.result != "" || len(m.transcript) != 0 {
		t.Fatalf("clear command did not clear display result/transcript = %q/%+v", m.result, m.transcript)
	}
}

func TestModelSlashCompletionSuggestionsVisible(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.input = "/w"
	m.inputCursor = len([]rune(m.input))
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Commands:") || !strings.Contains(view, "/workspace") {
		t.Fatalf("slash completion suggestions missing:\n%s", view)
	}
}

func TestModelConfigCommandShowsLLMOutputBudget(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.setInput("/config")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected config command")
	}
	msg := cmd()
	next, _ = asModel(next).Update(msg)
	m = asModel(next)
	if !strings.Contains(m.result, "LLM Output Budget") || !strings.Contains(m.result, "maxGraphBytes: 32768") {
		t.Fatalf("config result missing llm output budget:\n%s", m.result)
	}
}

func TestModelDecisionSlashCompletionSuggestsContextSubcommands(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", State: "open", Question: "choose next"}
	m.progress = progressWaitingDecision
	m.input = "/"
	m.inputCursor = len([]rune(m.input))
	m.width = 90
	m.height = 16

	view := stripANSI(m.View())
	if !strings.Contains(view, "/decision answer") || !strings.Contains(view, "/decision replan") {
		t.Fatalf("decision suggestions missing contextual subcommands:\n%s", view)
	}

	m.input = "/decision "
	m.inputCursor = len([]rune(m.input))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(next)
	if m.input != "/decision answer " {
		t.Fatalf("completion = %q, want /decision answer with trailing space", m.input)
	}
}

func TestModelSlashCompletionArrowSelectionControlsCompletedCommand(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", State: "open", Question: "choose next"}
	m.progress = progressWaitingDecision
	m.setInput("/decision ")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = asModel(next)
	if m.slashCursor != 1 {
		t.Fatalf("slashCursor = %d, want 1", m.slashCursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(next)
	if m.input != "/decision replan " {
		t.Fatalf("completion = %q, want selected /decision replan", m.input)
	}

	m.setInput("/decision ")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = asModel(next)
	if m.slashCursor != 4 {
		t.Fatalf("slashCursor = %d, want wrap to last suggestion", m.slashCursor)
	}
}

func TestModelApprovalSlashCompletionSuggestsContextSubcommands(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-1", State: "pending", Reason: "run shell command"}
	m.progress = progressWaitingApproval
	m.input = "/"
	m.inputCursor = len([]rune(m.input))
	m.width = 90
	m.height = 16

	view := stripANSI(m.View())
	if !strings.Contains(view, "/approval approve") || !strings.Contains(view, "/approval reject") {
		t.Fatalf("approval suggestions missing contextual subcommands:\n%s", view)
	}
}

func TestModelDecisionReplanSlashSubcommandUsesExplicitResolutionType(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open", Question: "choose next"}
	m.setInput("/decision replan 重新规划当前路径")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command")
	}
	msg := cmd()
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)

	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	req := be.decisions[0]
	if req.TicketID != "ticket-1" || req.ResolutionType != backend.DecisionResolutionTypeReplan {
		t.Fatalf("decision request = %+v, want explicit replan for ticket-1", req)
	}
	if req.Payload["answer"] != "重新规划当前路径" {
		t.Fatalf("payload = %+v, want note as answer", req.Payload)
	}
	if m.blockingDecision != nil {
		t.Fatalf("blockingDecision = %+v, want cleared after resolution", m.blockingDecision)
	}
}

func TestModelDecisionReplanAfterWorkspaceSwitchUsesOverviewBlocker(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-b"},
	}})
	m = asModel(next)
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, _ = m.confirmWorkspaceSelection()
	m = asModel(next)
	m.blockingDecision = nil
	m.decisions = nil
	m.setInput("/decision replan")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command for switched workspace")
	}
	msg := cmd()
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)

	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	req := be.decisions[0]
	if req.WorkspaceID != "ws-b" || req.TicketID != "ticket-b" || req.ResolutionType != backend.DecisionResolutionTypeReplan {
		t.Fatalf("decision request = %+v, want ws-b ticket-b replan", req)
	}
	if m.lastError != nil {
		t.Fatalf("lastError = %v, want nil", m.lastError)
	}
}

func TestModelDecisionReplanAfterWorkspaceSwitchReplacesStaleBlocker(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-b"},
	}})
	m = asModel(next)

	m.activeWorkspaceID = "ws-b"
	m.selectedWorkspaceID = "ws-b"
	m.activeRunID = "run-b"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}}
	m.syncBlockingDecisionFromList()
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-b" {
		t.Fatalf("blocking decision = %+v, want ticket-b from active overview", m.blockingDecision)
	}

	m.setInput("/decision replan")
	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command for active workspace")
	}
	_ = next
	_ = cmd()
	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	req := be.decisions[0]
	if req.WorkspaceID != "ws-b" || req.TicketID != "ticket-b" {
		t.Fatalf("decision request = %+v, want ws-b ticket-b", req)
	}
}

func TestModelDecisionReplanUsesCurrentOverviewTicketOverStaleLocalBlocker(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-1",
		Title:            "A",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-current",
		BlockingTicketID: "ticket-current",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.selectedWorkspaceID = "ws-1"
	m.activeRunID = "run-current"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"}}
	m.setInput("/decision replan")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command")
	}
	_ = next
	_ = cmd()
	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	req := be.decisions[0]
	if req.WorkspaceID != "ws-1" || req.TicketID != "ticket-current" || req.ResolutionType != backend.DecisionResolutionTypeReplan {
		t.Fatalf("decision request = %+v, want current server-projected replan ticket", req)
	}
}

func TestModelDecisionReplanUsesServerProjectionEvenAfterOptimisticResolvedMark(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-1",
		Title:            "A",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.selectedWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.markDecisionResolvedForWorkspace("ws-1", "ticket-1", "run-1")
	m.markDecisionResolved("ticket-1", "ws-1", "run-1")
	m.blockingDecision = nil
	m.setInput("/decision replan")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command")
	}
	_ = next
	_ = cmd()
	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	req := be.decisions[0]
	if req.WorkspaceID != "ws-1" || req.TicketID != "ticket-1" || req.ResolutionType != backend.DecisionResolutionTypeReplan {
		t.Fatalf("decision request = %+v, want server-projected ticket despite optimistic resolved mark", req)
	}
}

func TestModelResolvedMarkersAreScopedAcrossDuplicateIDs(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-a", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-a", BlockingTicketID: "ticket-shared"},
		{ID: "ws-b", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-shared"},
	}
	m.activeWorkspaceID = "ws-b"
	m.activeRunID = "run-b"
	m.decisions = []backend.DecisionInfo{
		{ID: "ticket-shared", WorkspaceID: "ws-a", RunID: "run-a", State: "open"},
		{ID: "ticket-shared", WorkspaceID: "ws-b", RunID: "run-b", State: "open"},
	}
	m.approvals = []backend.ApprovalInfo{
		{ID: "approval-shared", WorkspaceID: "ws-a", RunID: "run-a", State: "pending"},
		{ID: "approval-shared", WorkspaceID: "ws-b", RunID: "run-b", State: "pending"},
	}

	m.markDecisionResolved("ticket-shared", "ws-b", "run-b")
	m.markApprovalResolved("approval-shared", "ws-b", "run-b")

	if m.decisions[0].State != "open" || m.decisions[1].State != "resolved" {
		t.Fatalf("decision states = %+v, want only ws-b resolved", m.decisions)
	}
	if m.approvals[0].State != "pending" || m.approvals[1].State != "resolved" {
		t.Fatalf("approval states = %+v, want only ws-b resolved", m.approvals)
	}
}

func TestModelDecisionReplanDoesNotUseLocalBlockerWhenServerProjectionIsLive(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-1",
		Title:       "A",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.setInput("/decision replan")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale local blocker produced a decision command")
	}
	if len(be.decisions) != 0 {
		t.Fatalf("resolved decisions = %d, want none", len(be.decisions))
	}
	if m.lastError == nil || !strings.Contains(m.lastError.Error(), "no blocking decision") {
		t.Fatalf("lastError = %v, want no blocking decision", m.lastError)
	}
}

func TestModelApprovalApproveDoesNotUseLocalBlockerWhenServerProjectionIsLive(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-1",
		Title:       "A",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-stale", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.approvals = []backend.ApprovalInfo{*m.blockingApproval}
	m.setInput("/approval approve")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale local blocker produced an approval command")
	}
	if m.lastError == nil || !strings.Contains(m.lastError.Error(), "no blocking approval") {
		t.Fatalf("lastError = %v, want no blocking approval", m.lastError)
	}
}

func TestModelCtrlCOnlyInterruptsOrHintsExitCommand(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = asModel(next)
	if cmd != nil || m.mode == modeQuit {
		t.Fatalf("ctrl+c should not quit, cmd=%v mode=%v", cmd != nil, m.mode)
	}
	if !strings.Contains(m.progressText, "Use /exit to quit") {
		t.Fatalf("ctrl+c hint = %q", m.progressText)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = asModel(next)
	if cmd != nil || m.mode == modeQuit {
		t.Fatalf("second ctrl+c should not quit, cmd=%v mode=%v", cmd != nil, m.mode)
	}
}

func TestModelEmptyWorkspaceGuidance(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	m.width = 90
	m.height = 18
	view := stripANSI(m.View())
	for _, want := range []string{"No workspaces found", "/workspace create <title> --dir <path>", "--template generic-governed", "--template software-dev", "/dod"} {
		if !strings.Contains(view, want) {
			t.Fatalf("empty workspace guidance missing %q:\n%s", want, view)
		}
	}
}

func TestModelScrollableResultViewport(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, "result line "+string(rune('A'+i)))
	}
	m.result = strings.Join(lines, "\n")
	m.width = 80
	m.height = 14
	initial := stripANSI(m.View())
	if strings.Contains(initial, "result line Z") {
		t.Fatalf("initial viewport should not show later result lines:\n%s", initial)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = asModel(next)
	scrolled := stripANSI(m.View())
	if !strings.Contains(scrolled, "result line G") {
		t.Fatalf("pgdown should scroll main content:\n%s", scrolled)
	}
}

func TestModelScrollableResultKeepsWorkspacePanelFixed(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "Project", State: "active"},
		{ID: "ws-2", Title: "Other", State: "completed"},
	}})
	m = asModel(next)
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "result line "+string(rune('A'+i)))
	}
	m.result = strings.Join(lines, "\n")
	m.width = 120
	m.height = 16
	initial := stripANSI(m.View())
	if !strings.Contains(initial, "> Project  active") || !strings.Contains(initial, "  Other  completed") {
		t.Fatalf("initial view missing fixed workspace panel:\n%s", initial)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = asModel(next)
	scrolled := stripANSI(m.View())
	if !strings.Contains(scrolled, "result line G") {
		t.Fatalf("pgdown should scroll main content:\n%s", scrolled)
	}
	if !strings.Contains(scrolled, "> Project  active") || !strings.Contains(scrolled, "  Other  completed") {
		t.Fatalf("workspace panel should remain visible after main scroll:\n%s", scrolled)
	}
	if m.mainViewport == nil || m.mainViewport.MouseWheelEnabled {
		t.Fatalf("main viewport must not enable mouse wheel reporting")
	}
}

func TestTUIE2EViewportKeyboardScrollAndMouseSelectionContract(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-viewport", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-viewport", Title: "Viewport Workspace", State: common.WorkspaceStateCompleted},
		{ID: "ws-other", Title: "Other Workspace", State: common.WorkspaceStateRunning},
	}})
	m = asModel(next)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 16})
	m = asModel(next)

	lines := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("viewport-result-line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")

	firstScreen := stripANSI(m.View())
	if !strings.Contains(firstScreen, "viewport-result-line-00") {
		t.Fatalf("first screen missing initial result line:\n%s", firstScreen)
	}
	if strings.Contains(firstScreen, "viewport-result-line-35") {
		t.Fatalf("first screen should not expose later result lines before scrolling:\n%s", firstScreen)
	}
	if !strings.Contains(firstScreen, "> Viewport Work...  completed") ||
		!strings.Contains(firstScreen, "Other Workspace  running") {
		t.Fatalf("first screen missing workspace monitor:\n%s", firstScreen)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = asModel(next)
	scrolledScreen := stripANSI(m.View())
	if !strings.Contains(scrolledScreen, "viewport-result-line-06") {
		t.Fatalf("PgDown should scroll the main display area:\n%s", scrolledScreen)
	}
	if strings.Contains(scrolledScreen, "viewport-result-line-00") {
		t.Fatalf("PgDown should move the main display viewport away from the first line:\n%s", scrolledScreen)
	}
	if !strings.Contains(scrolledScreen, "> Viewport Work...  completed") ||
		!strings.Contains(scrolledScreen, "Other Workspace  running") {
		t.Fatalf("workspace monitor must stay fixed while main display scrolls:\n%s", scrolledScreen)
	}
	if m.mainViewport == nil || m.mainViewport.MouseWheelEnabled {
		t.Fatalf("TUI e2e contract requires viewport mouse wheel reporting disabled")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = asModel(next)
	topScreen := stripANSI(m.View())
	if !strings.Contains(topScreen, "viewport-result-line-00") {
		t.Fatalf("PgUp should return the main display area to the top:\n%s", topScreen)
	}
}

func TestModelResultMarkdownRendering(t *testing.T) {
	t.Setenv(markdownRendererEnv, "plain")
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "# Final Result\n\n**Done** with `code`.\n\n```text\nstdout payload\n```"
	m.width = 80
	m.height = 18
	view := stripANSI(m.View())
	for _, want := range []string{"# Final Result", "Done with code.", "stdout payload"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "Code") || !strings.Contains(view, "| stdout payload") {
		t.Fatalf("code block rendering missing visual marker:\n%s", view)
	}
	for _, forbidden := range []string{"**", "```"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("view still contains raw markdown %q:\n%s", forbidden, view)
		}
	}
}

func TestModelResultGlamourMarkdownRendering(t *testing.T) {
	t.Setenv(markdownRendererEnv, "glamour")
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "# Final Result\n\n**Done** with `code`.\n\n```text\nstdout payload\n```"
	m.width = 80
	m.height = 18
	view := stripANSI(m.View())
	for _, want := range []string{"Final Result", "Done", "code", "stdout payload"} {
		if !strings.Contains(view, want) {
			t.Fatalf("glamour view missing %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{"**", "```"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("glamour view still contains raw markdown %q:\n%s", forbidden, view)
		}
	}
	if strings.Contains(view, "##") || strings.Contains(view, "Final Result\n\n") {
		t.Fatalf("glamour view should be compact and suppress heading markers:\n%s", view)
	}
}

func TestModelResultGlamourChineseHeadingsAreCompact(t *testing.T) {
	t.Setenv(markdownRendererEnv, "glamour")
	rendered, ok := renderGlamourRichText("## 目标成果\n\nMemory recall DoD satisfied.", 88, 20)
	if !ok {
		t.Fatal("glamour renderer failed")
	}
	plainRendered := stripANSI(rendered)
	if strings.Contains(plainRendered, "目标成果\n\n") || !strings.Contains(plainRendered, "目标成果\nMemory recall DoD satisfied.") {
		t.Fatalf("raw glamour render should be compact:\n%q", plainRendered)
	}
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "## 目标成果\n\nMemory recall DoD satisfied.\n\n## 实现摘要\n\nMemory recall DoD satisfied.\n\n## 验证摘要\n\n验收结论由 1 个证据引用支撑。"
	m.width = 90
	m.height = 24
	view := stripANSI(m.View())
	for _, forbidden := range []string{"##", "目标成果\n\n", "实现摘要\n\n", "验证摘要\n\n"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("glamour Chinese headings should be compact, found %q:\n%s", forbidden, view)
		}
	}
	for _, want := range []string{"目标成果\nMemory recall DoD satisfied.", "实现摘要\nMemory recall DoD satisfied.", "验证摘要\n验收结论由 1 个证据引用支撑。"} {
		if !strings.Contains(view, want) {
			t.Fatalf("compact Chinese heading view missing %q:\n%s", want, view)
		}
	}
}

func TestDetectThemeHonorsEnvironment(t *testing.T) {
	t.Setenv("WORKORCH_THEME", "light")
	if DetectTheme().Primary != lightTheme().Primary {
		t.Fatal("WORKORCH_THEME=light should select light theme")
	}
	t.Setenv("WORKORCH_THEME", "dark")
	if DetectTheme().Primary != darkTheme().Primary {
		t.Fatal("WORKORCH_THEME=dark should select dark theme")
	}
	t.Setenv("WORKORCH_THEME", "auto")
	t.Setenv("COLORFGBG", "0;15")
	if DetectTheme().Primary != lightTheme().Primary {
		t.Fatal("WORKORCH_THEME=auto should use terminal color detection")
	}
}

func TestModelRunningWithoutResultDoesNotRenderEmptyResultSection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "waiting for progress or final result") {
		t.Fatalf("running progress text missing:\n%s", view)
	}
	if strings.Contains(view, "Result") {
		t.Fatalf("empty result section should be hidden while run has no result:\n%s", view)
	}
	if strings.Contains(view, "Long tasks may stay quiet while backend works") {
		t.Fatalf("empty result placeholder should be hidden while run has no result:\n%s", view)
	}
}

func TestModelSubmittedInputRendersAsTaskContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "处理当前目标"
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Task: 处理当前目标") {
		t.Fatalf("view missing submitted input context:\n%s", view)
	}
}

func TestModelGoalRendersBeforeTask(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "完成目标并输出结果"
	m.appendTranscript("Goal", "完成目标并输出结果")
	m.appendTranscript("Decision", "需要我提供哪些信息")
	m.width = 100
	m.height = 20
	view := stripANSI(m.View())
	goalIndex := strings.Index(view, "Goal: 完成目标并输出结果")
	taskIndex := strings.Index(view, "Task: 完成目标并输出结果")
	if goalIndex < 0 || taskIndex < 0 {
		t.Fatalf("view missing goal/task:\n%s", view)
	}
	if goalIndex > taskIndex {
		t.Fatalf("goal should render before task:\n%s", view)
	}
}

func TestModelDecisionUsesBottomInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventDecisionCreated,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"ticketId": "ticket-1", "question": "缺少必要信息"},
	})
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if !strings.HasPrefix(lines[len(lines)-2], "ws-1 decision >") {
		t.Fatalf("expected decision input context at bottom, got %q", lines[len(lines)-2])
	}
	if strings.Contains(view, "answer:") {
		t.Fatalf("view should not include answer history:\n%s", view)
	}
}

func TestModelDecisionCreatedAcceptsBlockingTicketIDPayload(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventDecisionCreated,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"blockingTicketId": "ticket-from-run-payload",
			"reason":           "缺少必要参数",
		},
	})
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-from-run-payload" {
		t.Fatalf("blocking decision = %+v, want id from blockingTicketId", m.blockingDecision)
	}
}

func TestModelRunStartedBlockedOnDecisionCreatesInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(RunStartedMsg{Run: &backend.RunInfo{
		ID:               "run-1",
		WorkspaceID:      "ws-1",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-1",
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected blocker detail fetch command")
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" {
		t.Fatalf("blocking decision = %+v, want ticket-1", m.blockingDecision)
	}
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "ticket-1") {
		t.Fatalf("view missing ticket id:\n%s", view)
	}
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if !strings.HasPrefix(lines[len(lines)-2], "ws-1 decision >") {
		t.Fatalf("expected decision input context, got %q", lines[len(lines)-2])
	}
}

func TestModelDecisionListRestoresBlockingInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	next, _ := m.Update(DecisionListMsg{Decisions: []backend.DecisionInfo{{
		ID:          "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Question:    "请补充目标参数。",
		State:       "open",
	}}})
	m = asModel(next)
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" {
		t.Fatalf("blocking decision = %+v, want ticket-1", m.blockingDecision)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "请补充目标参数。") {
		t.Fatalf("view missing decision question:\n%s", view)
	}
	if !strings.Contains(view, "ws-1 decision >") {
		t.Fatalf("view missing decision input context:\n%s", view)
	}
}

func TestModelDecisionDetailReplacesBlockingTicketPlaceholder(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{
		ID:               "run-1",
		WorkspaceID:      "ws-1",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-1",
	}})
	m = asModel(next)
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	next, _ = m.Update(DecisionDetailMsg{Decision: &backend.DecisionInfo{
		ID:       "ticket-1",
		RunID:    "run-1",
		Question: "请补充目标参数。",
		State:    "open",
	}})
	m = asModel(next)
	if m.progressText != "请补充目标参数。" {
		t.Fatalf("progressText = %q, want decision question", m.progressText)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "请补充目标参数。") || !strings.Contains(view, "ws-1 decision >") {
		t.Fatalf("view missing detailed decision context:\n%s", view)
	}
}

func TestModelRunStateEventBlockedOnDecisionCreatesInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeRunID = "run-1"
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"state":            "blocked_on_decision",
			"blockingTicketId": "ticket-from-run-state",
		},
	})
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-from-run-state" {
		t.Fatalf("blocking decision = %+v, want ticket-from-run-state", m.blockingDecision)
	}
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-from-run-state")
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Decision required") || !strings.Contains(view, "ticket-from-run-state") {
		t.Fatalf("view missing decision prompt:\n%s", view)
	}
	if !strings.Contains(view, "ws-1 decision >") {
		t.Fatalf("view missing decision input:\n%s", view)
	}
}

func TestModelRunStartedBlockedOnApprovalCreatesInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(RunStartedMsg{Run: &backend.RunInfo{
		ID:                 "run-1",
		WorkspaceID:        "ws-1",
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-1",
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected blocker detail fetch command")
	}
	if m.blockingApproval == nil || m.blockingApproval.ID != "approval-1" {
		t.Fatalf("blocking approval = %+v, want approval-1", m.blockingApproval)
	}
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Approval required") || !strings.Contains(view, "ws-1 approval >") {
		t.Fatalf("view missing approval input context:\n%s", view)
	}
}

func TestApprovalInputUsesSemanticIntent(t *testing.T) {
	positive := []string{
		"同意继续调用外部接口",
		"可以，确认执行",
		"没问题，请继续",
		"I approve this request",
		"please go ahead",
	}
	for _, input := range positive {
		approve, ok := parseApprovalInput(input)
		if !ok || !approve {
			t.Fatalf("parseApprovalInput(%q) = %v/%v, want approve", input, approve, ok)
		}
	}

	negative := []string{
		"不同意调用外部接口",
		"拒绝本次操作",
		"不要执行",
		"please reject it",
		"not approve",
	}
	for _, input := range negative {
		approve, ok := parseApprovalInput(input)
		if !ok || approve {
			t.Fatalf("parseApprovalInput(%q) = %v/%v, want reject", input, approve, ok)
		}
	}

	if approve, ok := parseApprovalInput("这里补充一些上下文"); ok {
		t.Fatalf("parseApprovalInput ambiguous = %v/%v, want no decision", approve, ok)
	}

	reason := "在调用外部工具前需要确认目标参数"
	for _, input := range []string{"使用参数 value-1 调用工具", "value-1"} {
		approve, ok := parseApprovalInputWithContext(input, reason)
		if !ok || !approve {
			t.Fatalf("parseApprovalInputWithContext(%q) = %v/%v, want contextual approve", input, approve, ok)
		}
	}
	if approve, ok := parseApprovalInputWithContext("不要使用该参数", reason); !ok || approve {
		t.Fatalf("contextual negative = %v/%v, want reject", approve, ok)
	}
}

func TestModelApprovalSemanticApproveSubmitsApprove(t *testing.T) {
	be := newFake(t)
	m := NewAppModel(be, "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.input = "同意继续调用外部接口"
	m.inputCursor = len([]rune(m.input))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected approval resolve command")
	}
	msg := cmd()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want approved", msg)
	}
	if len(m.transcript) != 1 || m.transcript[0].Text != "同意继续调用外部接口" {
		t.Fatalf("transcript = %+v, want original approval text", m.transcript)
	}
	if be.approval.lastPayload["rawInput"] != "同意继续调用外部接口" || be.approval.lastPayload["source"] != "tui" {
		t.Fatalf("approval payload = %#v, want raw input from tui", be.approval.lastPayload)
	}
}

func TestModelApprovalAmbiguousInputDoesNotReject(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.input = "外部上下文"
	m.inputCursor = len([]rune(m.input))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("ambiguous approval input should not submit")
	}
	if m.blockingApproval == nil || m.input != "外部上下文" {
		t.Fatalf("approval/input = %+v/%q, want preserved", m.blockingApproval, m.input)
	}
	if !strings.Contains(m.progressText, "intent unclear") {
		t.Fatalf("progressText = %q, want unclear intent hint", m.progressText)
	}
}

func TestModelApprovalContextualParameterInputApproves(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.blockingApproval = &backend.ApprovalInfo{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "在调用外部工具前需要确认目标参数",
		State:       "pending",
	}
	m.input = "使用参数 value-1 调用工具"
	m.inputCursor = len([]rune(m.input))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected contextual approval command")
	}
	msg := cmd()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want approved", msg)
	}
	if len(m.transcript) != 1 || m.transcript[0].Text != "使用参数 value-1 调用工具" {
		t.Fatalf("transcript = %+v, want contextual approval text", m.transcript)
	}
}

func TestResolveApprovalIsIdempotentWhenAlreadyApproved(t *testing.T) {
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateBlockedOnApproval}
	msg := ResolveApproval(be, "ws-1", "approval-1", true)()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Error != nil || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want idempotent approved without client resume", msg)
	}
	if be.resumed {
		t.Fatal("TUI must not resume blocked run after approval conflict")
	}
}

func TestResolveApprovalSkipsResumeWhenRunAlreadyRunning(t *testing.T) {
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateRunning}
	msg := ResolveApproval(be, "ws-1", "approval-1", true)()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Error != nil || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want idempotent approved without client resume", msg)
	}
	if be.resumed {
		t.Fatal("TUI must not resume running run after approval conflict")
	}
}

func TestModelConversationRendersMultipleSubmissions(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.appendTranscript("Goal", "完成当前目标")
	m.appendTranscript("Decision", "补充参数 value-1")
	m.appendTranscript("Approval", "同意继续调用外部接口")
	m.width = 100
	m.height = 20
	view := stripANSI(m.View())
	for _, want := range []string{
		"Conversation",
		"Goal: 完成当前目标",
		"Decision: 补充参数 value-1",
		"Approval: 同意继续调用外部接口",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModelDecisionResolvedRefreshesWorkspaceState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-1",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
	}}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}}
	m.blockingDecision = &m.decisions[0]
	m.input = "补充上下文"
	m.inputCursor = len([]rune(m.input))

	next, cmd := m.Update(DecisionResolvedMsg{Decision: &backend.DecisionInfo{
		ID:          "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		State:       "resolved",
	}})
	m = asModel(next)

	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("expected workspace refresh after decision resolve: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if m.blockingDecision != nil || m.input != "" || m.activeRunID != "run-1" {
		t.Fatalf("state not cleared/refreshed: blocker=%+v input=%q activeRun=%q", m.blockingDecision, m.input, m.activeRunID)
	}
	if ws := findWorkspace(m.workspaces, "ws-1"); ws == nil || ws.State != common.WorkspaceStateWaitingDecision {
		t.Fatalf("workspace overview after resolve = %+v, want unchanged until server projection updates", ws)
	}
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-1 decision >") || strings.Contains(view, "ws-1 submitting >") {
		t.Fatalf("prompt should follow server waiting projection until refresh converges:\n%s", view)
	}
	if m.decisions[0].State != "resolved" {
		t.Fatalf("decision state = %q, want resolved", m.decisions[0].State)
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("refresh command should batch workspace context and list refresh")
	}
}

func TestModelApprovalResolvedRefreshesWorkspaceState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.approvals = []backend.ApprovalInfo{{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}}
	m.blockingApproval = &m.approvals[0]
	m.input = "同意"
	m.inputCursor = len([]rune(m.input))

	next, cmd := m.Update(ApprovalResolvedMsg{Approval: &backend.ApprovalInfo{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		State:       "approved",
	}})
	m = asModel(next)

	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("expected workspace refresh after approval resolve: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if m.blockingApproval != nil || m.input != "" || m.activeRunID != "run-1" {
		t.Fatalf("state not cleared/refreshed: blocker=%+v input=%q activeRun=%q", m.blockingApproval, m.input, m.activeRunID)
	}
	if m.progress != progressSubmitting || !strings.Contains(m.progressText, "waiting for server continuation") {
		t.Fatalf("approval resolve progress = %s/%q, want server continuation wait", m.progress, m.progressText)
	}
	if m.approvals[0].State != "resolved" {
		t.Fatalf("approval state = %q, want resolved", m.approvals[0].State)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("refresh command should batch workspace context, list refresh, and approval refresh")
	}
	if len(batch) < 3 {
		t.Fatalf("approval refresh batch size = %d, want workspace state plus approval list refresh", len(batch))
	}
}

func TestModelApprovalListClearsResolvedBlockingApproval(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.blockingApproval = &backend.ApprovalInfo{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "需要审批",
		State:       "pending",
	}
	m.progress = progressWaitingApproval
	next, _ := m.Update(ApprovalListMsg{Approvals: []backend.ApprovalInfo{{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "需要审批",
		State:       "rejected",
	}}})
	m = asModel(next)
	if m.blockingApproval != nil {
		t.Fatalf("blocking approval = %+v, want cleared", m.blockingApproval)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if strings.Contains(view, "ws-1 approval >") {
		t.Fatalf("view should allow normal goal input after rejected approval:\n%s", view)
	}
	if !strings.Contains(view, "ws-1 >") {
		t.Fatalf("view missing normal workspace input:\n%s", view)
	}
}

func TestModelFailedRunClearsBlockingInputContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeRunID = "run-1"
	m.blockingApproval = &backend.ApprovalInfo{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "需要审批",
		State:       "pending",
	}
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"state":         "failed",
			"failureReason": "approval rejected",
		},
	})
	if m.blockingApproval != nil || m.blockingDecision != nil {
		t.Fatalf("blockers = decision:%+v approval:%+v, want cleared", m.blockingDecision, m.blockingApproval)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if strings.Contains(view, "approval >") || !strings.Contains(view, "approval rejected") {
		t.Fatalf("view should show failure and normal input:\n%s", view)
	}
}

func TestModelWorkspaceSwitchCallback(t *testing.T) {
	var selected string
	m := NewAppModelWithOptions(newFake(t), "ws-a", "", Options{
		OnActiveWorkspaceChanged: func(workspaceID string) { selected = workspaceID },
	})
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	next, _ = asModel(next).Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	next, _ = asModel(next).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-a" || m.selectedWorkspaceID != "ws-b" || !m.workspaceSelectMode || selected != "" {
		t.Fatalf("active/selected/mode/callback after navigate = %q/%q/%v/%q, want ws-a/ws-b/true/empty", m.activeWorkspaceID, m.selectedWorkspaceID, m.workspaceSelectMode, selected)
	}
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "> B") || !strings.Contains(view, "Workspace: A") {
		t.Fatalf("view should show pending selection while active remains A:\n%s", view)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-b" || m.workspaceSelectMode || selected != "ws-b" {
		t.Fatalf("active/mode/callback = %q/%v/%q, want ws-b/false/ws-b", m.activeWorkspaceID, m.workspaceSelectMode, selected)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after workspace confirmation")
	}
}

func TestModelWorkspaceSwitchClearsStaleViewBeforeRefresh(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.currentTask = "old task"
	m.result = "old result"
	m.failureReason = "old failure"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", State: "open"}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-old", State: "open"}}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	next, _ = asModel(next).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	next, cmd := asModel(next).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected refresh command")
	}
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.result != "" || m.failureReason != "" || m.blockingDecision != nil || len(m.decisions) != 0 {
		t.Fatalf("switch state not fully reset: active=%q task=%q result=%q failure=%q blocker=%+v decisions=%d", m.activeWorkspaceID, m.currentTask, m.result, m.failureReason, m.blockingDecision, len(m.decisions))
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatal("workspace switch should batch list and selected workspace context refresh")
	}
}

func TestModelWorkspaceSwitchRestoresCachedDisplayState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "task-a"
	m.result = "result-a"
	m.artifacts = []string{"artifact-a"}
	m.runtimeEvents = []string{"event-a"}
	m.progress = progressRunning
	m.progressText = "progress-a"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-a", WorkspaceID: "ws-a", State: "open"}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-a", WorkspaceID: "ws-a", State: "open"}}

	m.setActiveWorkspace("ws-b")
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.result != "" || m.blockingDecision != nil {
		t.Fatalf("switch to uncached workspace did not clear active display: active=%q task=%q result=%q blocker=%+v", m.activeWorkspaceID, m.currentTask, m.result, m.blockingDecision)
	}
	m.currentTask = "task-b"
	m.result = "result-b"
	m.progressText = "progress-b"
	m.runtimeEvents = []string{"event-b"}

	m.setActiveWorkspace("ws-a")
	if m.activeWorkspaceID != "ws-a" || m.currentTask != "task-a" || m.result != "result-a" || m.progressText != "" {
		t.Fatalf("switch back did not restore cached display: active=%q task=%q result=%q progress=%q", m.activeWorkspaceID, m.currentTask, m.result, m.progressText)
	}
	if len(m.artifacts) != 1 || m.artifacts[0] != "artifact-a" || len(m.runtimeEvents) != 1 || m.runtimeEvents[0] != "event-a" {
		t.Fatalf("cached artifacts/events = %+v/%+v, want workspace A state", m.artifacts, m.runtimeEvents)
	}
	if m.blockingDecision != nil || len(m.decisions) != 0 {
		t.Fatalf("cached blocker/decisions = %+v/%+v, want no authoritative blocker cache", m.blockingDecision, m.decisions)
	}

	m.setActiveWorkspace("ws-b")
	if m.currentTask != "task-b" || m.result != "result-b" || m.progressText != "" {
		t.Fatalf("workspace B cache = task:%q result:%q progress:%q, want restored B display", m.currentTask, m.result, m.progressText)
	}
}

func TestModelWorkspaceSwitchUsesProjectedStateBeforeCachedBlocker(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateRunning, ActiveRunID: "run-b"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-b"
	m.activeRunID = "run-b"
	m.progress = progressWaitingDecision
	m.progressText = "Waiting for decision"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-b-old", WorkspaceID: "ws-b", RunID: "run-b", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.saveActiveWorkspaceDisplayState()

	m.activeWorkspaceID = "ws-a"
	m.selectedWorkspaceID = "ws-b"
	next, cmd := m.confirmWorkspaceSelection()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected workspace switch refresh command")
	}
	input := stripANSI(m.renderInput(120))
	if strings.Contains(input, "decision >") || !strings.Contains(input, "ws-b running >") {
		t.Fatalf("projected running state did not override cached blocker prompt: %q", input)
	}
	if decision := m.activeBlockingDecision(); decision != nil {
		t.Fatalf("active decision = %+v, want none for projected running workspace", decision)
	}
}

func TestModelSubmitInputIgnoresStaleWorkspaceDecision(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:          "ws-a",
		Title:       "A",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-a",
	}}
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-b", WorkspaceID: "ws-b", RunID: "run-b", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.setInput("resume normal work")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected normal run command")
	}
	_ = cmd()
	if len(be.decisions) != 0 {
		t.Fatalf("stale decision was resolved: %+v", be.decisions)
	}
	if m.progress != progressSubmitting || !strings.Contains(m.progressText, "Resuming active run") {
		t.Fatalf("progress = %s/%q, want normal run resume", m.progress, m.progressText)
	}
}

func TestModelInspectStatusIncludesRunHeartbeat(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"
	m.progress = progressRunning
	state := m.ensureWorkspaceRuntimeState("ws-a")
	state.LastRevision = 42
	state.RevisionGapCount = 2
	state.LastRevisionGap = 3
	m.runHeartbeat = backend.RunHeartbeatInfo{
		WorkspaceID:               "ws-a",
		RunID:                     "run-a",
		ActivityState:             "no_activity",
		Summary:                   "no effective progress evidence yet",
		LatestEffectiveProgressAt: "2026-06-10T00:00:00Z",
		Stalled:                   true,
	}
	m.runtimeExecution = &backend.RuntimeExecutionInfo{
		StepID:              "step-a",
		StepKind:            "llm/workspace_execution",
		StepState:           "running",
		RequestID:           "request-a",
		Pending:             true,
		Gap:                 "worker_lost",
		LastProgressSummary: "worker heartbeat lost",
	}
	m.convergenceStatus = &backend.ConvergenceStatusInfo{
		WorkspaceID:                 "ws-a",
		RunID:                       "run-a",
		RunState:                    common.RunStateRunning,
		CurrentFocusID:              "focus-a",
		NextActionKind:              "validate",
		CurrentGap:                  "missing accepted evidence",
		MissingEvidenceKinds:        []string{"validation"},
		AcceptedEvidenceCount:       2,
		RejectedEvidenceCount:       1,
		LatestContractViolationRule: "coverage_assessment_incomplete",
		LatestRecovery:              "recompute_authority",
		LatestRejectedDiagnosticRef: "run-a/step-a/rejected-output",
		DecisionCause:               "retry_exhaustion",
		RetryAttempt:                2,
		RetryMaxAttempts:            2,
		NarrowedRetryAttempted:      true,
		ManagedValidationAvailable:  true,
	}

	got := m.renderInspectStatus()
	for _, want := range []string{
		"stream revision: last 42, gaps 2, latest gap 3",
		"activity: no_activity - no effective progress evidence yet",
		"latest progress: 2026-06-10T00:00:00Z",
		"runtime execution: runtime pending | gap worker_lost | step step-a llm/workspace_execution running",
		"runtime progress: worker heartbeat lost",
		"convergence: focus focus-a | next validate | gap missing accepted evidence | evidence missing validation accepted 2 rejected 1 | violation coverage_assessment_incomplete recompute_authority | decision retry_exhaustion | retry 2/2 narrowed managedValidation rejected run-a/step-a/rejected-output",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status = %q, missing %q", got, want)
		}
	}
	statusBar := stripANSI(m.renderRuntimeStatusBar(180))
	if !strings.Contains(statusBar, "focus focus-a") || !strings.Contains(statusBar, "next validate") {
		t.Fatalf("status bar = %q, want compact convergence status", statusBar)
	}
}

func TestModelIgnoresRunControlForInactiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.progressText = "Workspace switched; refreshing state"

	next, cmd := m.Update(RunControlMsg{
		WorkspaceID: "ws-a",
		Run:         &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateRunning},
		Message:     "Resuming active run",
	})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale run control should not trigger refresh")
	}
	if m.activeRunID != "" || m.running || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("stale run control polluted state: activeRun=%q running=%v progress=%q", m.activeRunID, m.running, m.progressText)
	}
}

func TestModelIgnoresStaleWorkspaceContextError(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.activeWorkspaceID = "ws-b"
	m.progressText = "Workspace switched; refreshing state"

	next, cmd := m.Update(WorkspaceContextMsg{
		WorkspaceID: "ws-a",
		Error:       errors.New("old workspace snapshot failed"),
	})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale context error should not trigger follow-up command")
	}
	if m.lastError != nil || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("stale context error polluted state: err=%v progress=%q", m.lastError, m.progressText)
	}
}

func TestModelIgnoresStaleBlockerListsForInactiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.activeWorkspaceID = "ws-b"

	next, cmd := m.Update(DecisionListMsg{
		WorkspaceID: "ws-a",
		Decisions:   []backend.DecisionInfo{{ID: "ticket-old", State: "open"}},
	})
	m = asModel(next)
	if cmd != nil || len(m.decisions) != 0 || m.blockingDecision != nil {
		t.Fatalf("stale decisions polluted state: cmd=%v decisions=%+v blocker=%+v", cmd != nil, m.decisions, m.blockingDecision)
	}

	next, cmd = m.Update(ApprovalListMsg{
		WorkspaceID: "ws-a",
		Approvals:   []backend.ApprovalInfo{{ID: "approval-old", State: "pending"}},
	})
	m = asModel(next)
	if cmd != nil || len(m.approvals) != 0 || m.blockingApproval != nil {
		t.Fatalf("stale approvals polluted state: cmd=%v approvals=%+v blocker=%+v", cmd != nil, m.approvals, m.blockingApproval)
	}

	next, cmd = m.Update(DecisionDetailMsg{
		WorkspaceID: "ws-a",
		Decision:    &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", State: "open"},
	})
	m = asModel(next)
	if cmd != nil || len(m.decisions) != 0 || m.blockingDecision != nil {
		t.Fatalf("stale decision detail polluted state: cmd=%v decisions=%+v blocker=%+v", cmd != nil, m.decisions, m.blockingDecision)
	}

	next, cmd = m.Update(ApprovalDetailMsg{
		WorkspaceID: "ws-a",
		Approval:    &backend.ApprovalInfo{ID: "approval-old", WorkspaceID: "ws-a", State: "pending"},
	})
	m = asModel(next)
	if cmd != nil || len(m.approvals) != 0 || m.blockingApproval != nil {
		t.Fatalf("stale approval detail polluted state: cmd=%v approvals=%+v blocker=%+v", cmd != nil, m.approvals, m.blockingApproval)
	}
}

func TestModelWorkspaceSwitchContextResetsScrolledMainViewport(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.width = 120
	m.height = 16
	m.activeWorkspaceID = "ws-b"
	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("old-workspace-line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()
	m.scrollMain(10)
	if m.mainViewport == nil || m.mainViewport.YOffset == 0 {
		t.Fatal("expected old workspace view to be scrolled before switch context")
	}

	next, _ := m.Update(WorkspaceContextMsg{
		WorkspaceID:       "ws-b",
		ResetMainViewport: true,
		Workspace:         &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive},
		Goal:              &backend.GoalInfo{ID: "goal-b", WorkspaceID: "ws-b", Text: "fresh workspace goal"},
	})
	m = asModel(next)
	view := stripANSI(m.View())
	if m.mainViewport.YOffset != 0 {
		t.Fatalf("viewport offset = %d, want reset to top", m.mainViewport.YOffset)
	}
	if !strings.Contains(view, "fresh workspace goal") || strings.Contains(view, "old-workspace-line") {
		t.Fatalf("view not refreshed after workspace switch context:\n%s", view)
	}
}

func TestModelIgnoresRunStartedForInactiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")

	next, cmd := m.Update(RunStartedMsg{
		Run: &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateRunning},
	})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("stale run started should not fetch blockers")
	}
	if m.activeRunID != "" || m.running {
		t.Fatalf("stale run started polluted state: activeRun=%q running=%v", m.activeRunID, m.running)
	}
}

func TestModelWorkspaceContextHydratesFreshSnapshot(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-b"
	m.currentTask = "old task"
	m.result = "old result"
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-old", State: "pending"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateCompleted, ActiveRunID: "run-b"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateCompleted, ActiveRunID: "run-b"},
			Outcome:   backend.WorkspaceOutcomeInfo{RunID: "run-b", Summary: "fresh result", ArtifactRefs: []string{"artifact-ref"}},
			Runs:      []backend.RunInfo{{ID: "run-b", WorkspaceID: "ws-b", GoalID: "goal-b", State: common.RunStateCompleted}},
			Results:   []backend.ResultInfo{{ID: "result-b", RunID: "run-b", Summary: "fresh result", ArtifactRefs: []string{"result-artifact"}}},
			Artifacts: []backend.ArtifactInfo{{ID: "artifact-b", RunID: "run-b", Name: "artifact name"}},
			Patches:   []backend.PatchInfo{{ID: "patch-b", RunID: "run-b", TargetRef: "main.go", State: common.PatchStateApplied, AddedLines: 3, DeletedLines: 1, ApprovalID: "approval-b"}},
		},
		ActiveRun: &backend.RunInfo{ID: "run-b", WorkspaceID: "ws-b", GoalID: "goal-b", State: common.RunStateCompleted},
		Goal:      &backend.GoalInfo{ID: "goal-b", Text: "fresh goal"},
	})
	m = asModel(next)
	if m.currentTask != "fresh goal" || m.result != "fresh result" || m.progress != progressCompleted || m.blockingApproval != nil {
		t.Fatalf("hydrated state = task:%q result:%q progress:%s approval:%+v", m.currentTask, m.result, m.progress, m.blockingApproval)
	}
	for _, want := range []string{"artifact-b", "artifact-ref", "result-artifact"} {
		found := false
		for _, got := range m.artifacts {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("artifacts = %+v, missing %q", m.artifacts, want)
		}
	}
	if len(m.patches) != 1 || m.patches[0].ID != "patch-b" {
		t.Fatalf("patches = %+v, want hydrated patch", m.patches)
	}
	m.width = 100
	m.height = 24
	view := stripANSI(m.View())
	if strings.Contains(view, "old task") || strings.Contains(view, "old result") || !strings.Contains(view, "fresh result") || !strings.Contains(view, "fresh goal") || !strings.Contains(view, "main.go") {
		t.Fatalf("view not fully refreshed:\n%s", view)
	}
}

func TestModelWorkspaceContextDoesNotTreatOutcomeRunAsActive(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-old"
	m.running = true
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateFailed},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateFailed},
			Outcome:   backend.WorkspaceOutcomeInfo{RunID: "run-1", Summary: "failed result", Reason: "weak evidence"},
			Runs:      []backend.RunInfo{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateFailed, FailureReason: "weak evidence"}},
		},
		DisplayRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateFailed, FailureReason: "weak evidence"},
		Goal:       &backend.GoalInfo{ID: "goal-1", Text: "goal"},
	})
	m = asModel(next)
	if m.activeRunID != "" || m.running {
		t.Fatalf("active run state = %q/%v, want no live run", m.activeRunID, m.running)
	}
	if m.progress != progressFailed || m.progressText != "Workspace failed" {
		t.Fatalf("progress = %s/%q, want failed workspace state", m.progress, m.progressText)
	}
	if m.currentTask != "goal" || m.result != "failed result" {
		t.Fatalf("display state = task:%q result:%q, want terminal run details retained", m.currentTask, m.result)
	}
}

func TestModelWorkspaceContextDoesNotTreatAcceptedRunCommandAsRunning(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-old"
	m.running = true
	m.progress = progressSubmitting
	m.progressText = "Submitting run command"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
			RunCommand: &backend.RunCommandStatusInfo{
				WorkspaceID: "ws-a",
				RunID:       "run-accepted",
				State:       "accepted",
				Intent:      "replan",
				RequestID:   "request-accepted",
				Source:      "tui",
				Revision:    3,
			},
		},
	})
	m = asModel(next)
	if m.activeRunID != "" || m.running || m.progress == progressRunning {
		t.Fatalf("run state = %q/%v/%s, want accepted command metadata without live run", m.activeRunID, m.running, m.progress)
	}
	if m.progress == progressSubmitting {
		t.Fatalf("progress = %s/%q, accepted command projection must not leave stale submitting state", m.progress, m.progressText)
	}
	status := stripANSI(m.renderRuntimeStatusBar(120))
	if !strings.Contains(status, "command accepted replan") {
		t.Fatalf("status bar = %q, want accepted command projection", status)
	}
}

func TestModelWorkspaceContextDisplaysRejectedRunCommandProjection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.progress = progressSubmitting
	m.progressText = "Submitting run command"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
			RunCommand: &backend.RunCommandStatusInfo{
				WorkspaceID:    "ws-a",
				RunID:          "run-rejected",
				State:          "rejected",
				Intent:         "resume",
				RequestID:      "request-rejected",
				Source:         "tui",
				RejectedAt:     "2026-06-21T00:01:00Z",
				Error:          "scheduler command was not queued",
				IdempotencyKey: "resume/ws-a/run-rejected",
				Revision:       4,
			},
		},
	})
	m = asModel(next)
	if m.progress == progressSubmitting {
		t.Fatalf("progress = %s/%q, rejected command projection must not leave stale submitting state", m.progress, m.progressText)
	}
	status := stripANSI(m.renderRuntimeStatusBar(120))
	for _, want := range []string{"command rejected resume", "scheduler command was not queued"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status bar = %q, missing %q", status, want)
		}
	}
}

func TestModelWorkspaceContextDoesNotRestoreOldOutcomeForLiveRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.result = "stale result"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-new"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-new"},
			Outcome:   backend.WorkspaceOutcomeInfo{Summary: "old outcome without run id"},
			Runs: []backend.RunInfo{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: common.RunStateCompleted},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateRunning},
			},
		},
		ActiveRun:        &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateRunning},
		DisplayRun:       &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateRunning},
		Goal:             &backend.GoalInfo{ID: "goal-new", Text: "new goal"},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if m.activeRunID != "run-new" || m.progress != progressRunning || !m.running {
		t.Fatalf("live run state = %q/%s/%v, want running run-new", m.activeRunID, m.progress, m.running)
	}
	if m.result != "" {
		t.Fatalf("workspace context restored stale result for live run: %q", m.result)
	}
}

func TestModelWorkspaceContextDoesNotRestoreOldRunBlockersForLiveRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-old"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-new"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-new"},
			Runs: []backend.RunInfo{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-old"},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateRunning},
			},
		},
		ActiveRun: &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateRunning},
		Decisions: []backend.DecisionInfo{{
			ID:          "ticket-old",
			WorkspaceID: "ws-a",
			RunID:       "run-old",
			Question:    "old question",
			State:       "open",
		}},
		Approvals: []backend.ApprovalInfo{{
			ID:          "approval-old",
			WorkspaceID: "ws-a",
			RunID:       "run-old",
			Action:      "old action",
			State:       "pending",
		}},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if m.activeRunID != "run-new" || m.progress != progressRunning || !m.running {
		t.Fatalf("live run state = %q/%s/%v, want running run-new", m.activeRunID, m.progress, m.running)
	}
	if m.blockingDecision != nil || m.blockingApproval != nil {
		t.Fatalf("old blockers restored for live run: decision=%+v approval=%+v", m.blockingDecision, m.blockingApproval)
	}
}

func TestModelWorkspaceContextUsesEffectiveGoalAndDoDFromRunContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			GoalID:      "goal-latest",
			DoDID:       "dod-latest",
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"goal": map[string]any{
					"id":   "goal-latest",
					"text": "完成目标 A\n补充约束 B",
				},
				"goals": []any{
					map[string]any{"id": "goal-base", "text": "完成目标 A"},
					map[string]any{"id": "goal-latest", "text": "补充约束 B"},
				},
				"dod": map[string]any{
					"id":    "dod-latest",
					"items": []any{"验收项一满足", "验收项二满足"},
				},
			},
		},
		Goal: &backend.GoalInfo{ID: "goal-latest", Text: "补充约束 B"},
		DoD:  &backend.DoDInfo{ID: "dod-latest", Items: []string{"验收项二满足"}},
	})
	m = asModel(next)
	if m.currentTask != "完成目标 A\n补充约束 B" {
		t.Fatalf("current task = %q, want effective cumulative goal", m.currentTask)
	}
	if len(m.transcript) != 2 || m.transcript[0].Text != "完成目标 A" || m.transcript[1].Text != "补充约束 B" {
		t.Fatalf("transcript = %+v, want individual cumulative goals", m.transcript)
	}
	if len(m.dodItems) != 2 || m.dodItems[0] != "验收项一满足" || m.dodItems[1] != "验收项二满足" {
		t.Fatalf("dod items = %+v, want effective cumulative DoD", m.dodItems)
	}
}

func TestModelWorkspaceContextShowsWorkspaceGoalAndDoDWithoutRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
			Goals:     []backend.GoalInfo{{ID: "goal-1", WorkspaceID: "ws-a", Text: "准备工作区目标"}},
			DoDs:      []backend.DoDInfo{{ID: "dod-1", WorkspaceID: "ws-a", Items: []string{"验收项已定义"}, State: "ready"}},
		},
		Goal: &backend.GoalInfo{ID: "goal-1", WorkspaceID: "ws-a", Text: "准备工作区目标"},
		DoD:  &backend.DoDInfo{ID: "dod-1", WorkspaceID: "ws-a", Items: []string{"验收项已定义"}, State: "ready"},
	})
	m = asModel(next)
	if m.currentTask != "准备工作区目标" {
		t.Fatalf("current task = %q, want workspace goal", m.currentTask)
	}
	if !reflect.DeepEqual(m.dodItems, []string{"验收项已定义"}) {
		t.Fatalf("dod items = %+v, want workspace DoD", m.dodItems)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Goal: 准备工作区目标") || !strings.Contains(view, "验收项已定义") {
		t.Fatalf("view missing workspace definitions:\n%s", view)
	}
}

func TestModelWorkspaceContextPartialRefreshKeepsKnownGoalAndDoD(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.currentTask = "已确认目标"
	m.transcript = []transcriptItem{{Kind: "Goal", Text: "已确认目标"}}
	m.dodItems = []string{"已确认验收项"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       common.RunStateRunning,
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if m.currentTask != "已确认目标" {
		t.Fatalf("current task = %q, want known goal retained", m.currentTask)
	}
	if !reflect.DeepEqual(m.dodItems, []string{"已确认验收项"}) {
		t.Fatalf("dod items = %+v, want known DoD retained", m.dodItems)
	}
	if len(m.transcript) != 1 || m.transcript[0].Text != "已确认目标" {
		t.Fatalf("transcript = %+v, want known transcript retained", m.transcript)
	}
}

func TestModelWorkspaceContextCompleteSnapshotClearsMissingGoalAndDoD(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.currentTask = "旧目标"
	m.transcript = []transcriptItem{{Kind: "Goal", Text: "旧目标"}}
	m.dodItems = []string{"旧验收项"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		},
	})
	m = asModel(next)
	if m.currentTask != "" || len(m.dodItems) != 0 || len(m.transcript) != 0 {
		t.Fatalf("definitions = task:%q dod:%+v transcript:%+v, want cleared by complete empty snapshot", m.currentTask, m.dodItems, m.transcript)
	}
}

func TestInferDecisionResolutionTypeUsesGovernanceIntent(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "replan english", text: "please replan this path", want: "replan"},
		{name: "rebuild chinese", text: "要求重新构建项目", want: "replan"},
		{name: "reject chinese", text: "拒绝当前路径", want: "reject_current_path"},
		{name: "missing info", text: "使用本地配置继续执行", want: "provide_missing_info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferDecisionResolutionType(tc.text); got != tc.want {
				t.Fatalf("inferDecisionResolutionType(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestModelMemoryCommandShowsRecallSummary(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
			Runs:      []backend.RunInfo{{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: common.RunStateRunning}},
			MemoryRecallTraces: []backend.MemoryRecallTraceInfo{{
				ID:                 "trace-a",
				WorkspaceID:        "ws-a",
				RunID:              "run-a",
				StepID:             "run-a/step-1",
				Stage:              "planning",
				Source:             "activation",
				Status:             "selected",
				RetrievedMemoryIDs: []common.MemoryID{"memory-a"},
				RetrievedMemoryHits: []backend.MemoryRecallHitInfo{{
					ID:      "memory-a",
					Summary: "project context",
				}},
			}},
		},
		ActiveRun: &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: common.RunStateRunning},
		Goal:      &backend.GoalInfo{ID: "goal-a", Text: "use memory"},
	})
	m = asModel(next)
	m.input = "/memory"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("memory command should render from current snapshot without command")
	}
	m = asModel(next)
	if !strings.Contains(m.result, "activation/planning: selected") || !strings.Contains(m.result, "memory-a: project context") {
		t.Fatalf("memory result = %q, want compact recall summary", m.result)
	}
}

func TestModelMemoryRecallEventRefreshesContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"
	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventMemoryRecallRecorded,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Payload: map[string]any{
			"status":             "selected",
			"retrievedMemoryIds": []string{"memory-a", "memory-b"},
		},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("memory recall event should refresh workspace context")
	}
	if m.progressText != "Memory recall: selected 2" {
		t.Fatalf("progress text = %q, want memory recall summary", m.progressText)
	}
}

func TestModelGoalAndDoDEventsRefreshActiveWorkspaceContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive}}

	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventGoalDefined,
		WorkspaceID: "ws-a",
		Payload:     map[string]any{"goalId": "goal-a", "text": "updated goal"},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("goal event should refresh active workspace context")
	}
	if m.progressText != "Workspace definitions changed; refreshing context" {
		t.Fatalf("progress text = %q, want definition refresh status", m.progressText)
	}

	next, cmd = m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventDoDDefined,
		WorkspaceID: "ws-a",
		Payload:     map[string]any{"doDId": "dod-a", "items": []string{"tests pass"}},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("dod event should refresh active workspace context")
	}
	if m.progressText != "Workspace definitions changed; refreshing context" {
		t.Fatalf("progress text = %q, want definition refresh status", m.progressText)
	}
}

func TestModelGovernanceResolvedEventShowsCompactStatus(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"

	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventGovernanceResolved,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Payload: map[string]any{
			"itemType": "approval",
			"action":   "approve",
			"status":   "resolved",
			"risk":     "L1",
		},
	}})
	m = asModel(next)
	_ = cmd
	if m.progressText != "Auto governance resolved: approval/approve (L1)" {
		t.Fatalf("progress text = %q, want compact governance resolved status", m.progressText)
	}
}

func TestModelGovernanceEscalatedEventRefreshesBlockers(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"

	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventGovernanceEscalated,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Payload: map[string]any{
			"itemType": "ticket",
			"action":   "escalate",
			"status":   "escalated",
		},
	}})
	m = asModel(next)
	_ = cmd
	if m.progressText != "Auto governance escalated: ticket/escalate" {
		t.Fatalf("progress text = %q, want compact governance escalated status", m.progressText)
	}
}

func TestModelWorkspaceContextPrefersFinalSummaryArtifactOutput(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	finalSummary := `# 已验收目标成果摘要

## 目标成果

已通过 SSH 查询到远程主机 192.168.3.56 的服务配置信息。

## 验证摘要

命令执行成功（exit code 0），输出包含所有要求的配置细节。

## 证据输出

### run-1/block/query_server_config

标准输出:

` + "```text" + `
Architecture:                            x86_64
CPU(s):                                  12
Vendor ID:                               GenuineIntel
Model name:                              Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz
Flags:                                   very long flags line should not dominate the TUI result
---
               total        used        free
Mem:            15Gi       1.5Gi        11Gi
Swap:          4.0Gi          0B       4.0Gi
---
Filesystem                         Size  Used Avail Use% Mounted on
/dev/mapper/ubuntu--vg-ubuntu--lv  466G   23G  419G   6% /
---
Docker version 29.2.1, build a5c7197
` + "```"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateCompleted, ActiveRunID: "run-1"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateCompleted, ActiveRunID: "run-1"},
			Outcome:   backend.WorkspaceOutcomeInfo{RunID: "run-1", Summary: "目标已完成，成功查询到远程服务器配置信息。"},
			TaskProgress: backend.TaskProgressInfo{
				State: "running",
				Recovery: backend.RecoveryPlanInfo{
					Mode:       "resume",
					NextAction: "resume_run",
				},
				RepairLoop: backend.RepairLoopInfo{
					State:        "repairing",
					AttemptCount: 2,
					LastAttempt:  2,
					NextAction:   "resume_run",
				},
			},
			Runs:    []backend.RunInfo{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateCompleted}},
			Results: []backend.ResultInfo{{ID: "result-1", RunID: "run-1", Summary: "目标已完成，成功查询到远程服务器配置信息。"}},
			Artifacts: []backend.ArtifactInfo{{
				ID:       "run-1/artifact/result-summary",
				RunID:    "run-1",
				Category: "final_summary",
				Name:     "Accepted Result Summary",
				Summary:  finalSummary,
				State:    common.ArtifactStateAvailable,
			}},
		},
		ActiveRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateCompleted},
		Goal:      &backend.GoalInfo{ID: "goal-1", Text: "查询服务配置信息"},
	})
	m = asModel(next)
	for _, want := range []string{
		"Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz",
		"Mem:            15Gi",
		"/dev/mapper/ubuntu--vg-ubuntu--lv  466G",
		"Docker version 29.2.1",
	} {
		if !strings.Contains(m.result, want) {
			t.Fatalf("result missing %q:\n%s", want, m.result)
		}
	}
	if strings.Contains(m.result, "very long flags line") {
		t.Fatalf("result should compact noisy stdout lines:\n%s", m.result)
	}
	if !strings.Contains(stripANSI(m.View()), "repair #2") {
		t.Fatalf("view should render repair loop summary:\n%s", stripANSI(m.View()))
	}
	m.width = 100
	m.height = 34
	view := stripANSI(m.View())
	if !strings.Contains(view, "Docker version 29.2.1") {
		t.Fatalf("view should render final artifact output beyond the old 8-line cap:\n%s", view)
	}
}

func TestModelResultAcceptedRefreshesWorkspaceContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventResultAccepted,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"summary": "完成"},
	}})
	m = asModel(next)
	if m.result != "完成" {
		t.Fatalf("result = %q, want immediate event summary", m.result)
	}
	if cmd == nil {
		t.Fatal("expected result accepted event to refresh workspace snapshot")
	}
}

func TestModelResultAcceptedAutoScrollsMainViewportToResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 80
	m.height = 12
	m.currentTask = "long task before result"
	m.dodItems = []string{"dod one", "dod two", "dod three", "dod four"}
	m.runtimeEvents = []string{
		"event 01", "event 02", "event 03", "event 04",
		"event 05", "event 06", "event 07", "event 08",
	}

	next, _ := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventResultAccepted,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"summary": "最终结果已完成"},
	}})
	m = asModel(next)
	view := stripANSI(m.View())
	if !strings.Contains(view, "最终结果已完成") {
		t.Fatalf("accepted result should auto-scroll into the main viewport:\n%s", view)
	}
}

func TestModelWorkspaceContextWithResultAutoScrollsMainViewportToResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.width = 80
	m.height = 12
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "old task"
	m.dodItems = []string{"dod one", "dod two", "dod three", "dod four"}
	m.runtimeEvents = []string{
		"event 01", "event 02", "event 03", "event 04",
		"event 05", "event 06", "event 07", "event 08",
	}

	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateCompleted},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateCompleted},
			Outcome:   backend.WorkspaceOutcomeInfo{RunID: "run-1", Summary: "快照结果已完成"},
			Runs:      []backend.RunInfo{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateCompleted}},
		},
		DisplayRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateCompleted},
	})
	m = asModel(next)
	view := stripANSI(m.View())
	if !strings.Contains(view, "快照结果已完成") {
		t.Fatalf("workspace snapshot result should auto-scroll into the main viewport:\n%s", view)
	}
}

func TestModelWorkspaceContextRendersBusinessSectionsFromSnapshot(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-1"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-1"},
			TaskProgress: backend.TaskProgressInfo{
				State: "running",
			},
			TaskDecompositions: []backend.TaskDecompositionInfo{
				{
					ID:            "td-1",
					RunID:         "run-1",
					State:         "running",
					Summary:       "分解代码任务并执行验证",
					NodeTotal:     3,
					NodeCompleted: 1,
					NodeRunning:   1,
				},
			},
			TaskPlans: []backend.TaskPlanInfo{
				{
					ID:                "plan-1",
					RunID:             "run-1",
					State:             "blocked_on_decision",
					SourceOutputState: "retrying",
					Summary:           "按计划推进当前收敛项",
					NodeTotal:         3,
					NodeCompleted:     1,
					NodeRunning:       1,
				},
			},
			MemoryRecallTraces: []backend.MemoryRecallTraceInfo{
				{
					ID:                 "mr-1",
					RunID:              "run-1",
					Source:             "vector",
					Stage:              "execution",
					Status:             "selected",
					RetrievedMemoryIDs: []common.MemoryID{"memory-1"},
				},
			},
			LLMUsageSummary: backend.LLMUsageSummaryInfo{
				EntryTotal:            2,
				InputTokens:           1200,
				OutputTokens:          300,
				TotalTokens:           1500,
				ProviderNativeEntries: 1,
				EstimatedEntries:      1,
			},
			Decisions: []backend.DecisionInfo{
				{ID: "ticket-1", RunID: "run-1", Question: "需要补充部署目标环境", State: "open"},
			},
			Approvals: []backend.ApprovalInfo{
				{ID: "approval-1", RunID: "run-1", Reason: "准备执行潜在风险命令", State: "pending"},
			},
		},
		ActiveRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", DoDID: "dod-1", State: common.RunStateRunning},
		Goal:      &backend.GoalInfo{ID: "goal-1", Text: "完成服务配置修复"},
		DoD:       &backend.DoDInfo{ID: "dod-1", Items: []string{"变更最小化", "验证通过"}},
		Decisions: []backend.DecisionInfo{
			{ID: "ticket-1", RunID: "run-1", Question: "需要补充部署目标环境", State: "open"},
		},
		Approvals: []backend.ApprovalInfo{
			{ID: "approval-1", RunID: "run-1", Reason: "准备执行潜在风险命令", State: "pending"},
		},
	})
	m = asModel(next)
	m.width = 140
	m.height = 48
	view := stripANSI(m.View())
	for _, want := range []string{
		"Goal: 完成服务配置修复",
		"DoD",
		"变更最小化",
		"Task Plans",
		"按计划推进当前收敛项",
		"Memory Recall",
		"vector/execution: selected",
		"LLM Usage",
		"tokens 1200/300/1500",
		"Governance",
		"decision",
		"approval",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModelWorkspaceSelectionDoesNotStealTypedSubmit(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = asModel(next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = asModel(next)
	m.input = "/goal run active workspace"
	m.inputCursor = len([]rune(m.input))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-a" {
		t.Fatalf("active workspace = %q, want typed submit to stay on ws-a", m.activeWorkspaceID)
	}
	if m.currentTask != "run active workspace" || m.workspaceSelectMode || cmd == nil {
		t.Fatalf("currentTask/mode/cmd = %q/%v/%v, want submitted goal command and mode off", m.currentTask, m.workspaceSelectMode, cmd != nil)
	}
}

func TestModelWorkspaceNavigationRequiresActivationKey(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-a" || m.selectedWorkspaceID != "ws-a" || m.input != "j" {
		t.Fatalf("active/selected/input = %q/%q/%q, want no workspace navigation before activation", m.activeWorkspaceID, m.selectedWorkspaceID, m.input)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = asModel(next)
	if !m.workspaceSelectMode {
		t.Fatal("expected workspace select mode after ctrl+w")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = asModel(next)
	if m.workspaceSelectMode {
		t.Fatal("expected esc to cancel workspace select mode")
	}
}

func TestModelErrorDeduplicatedInView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(RunStartedMsg{Error: errors.New("command failed once")})
	m = asModel(next)
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if got := strings.Count(view, "command failed once"); got != 1 {
		t.Fatalf("error count = %d, want 1:\n%s", got, view)
	}
}

func TestModelContextDeadlineErrorSuppressedInView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(ErrorMsg{Error: context.DeadlineExceeded})
	m = asModel(next)
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if strings.Contains(view, "context deadline exceeded") || m.lastError != nil {
		t.Fatalf("transient context deadline should be hidden, error=%v view:\n%s", m.lastError, view)
	}
}

func TestModelStaleWorkspaceEventIgnoredByMainView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeRunID = "run-1"
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-other",
		RunID:       "run-other",
		Payload:     map[string]any{"kind": "other-step"},
	})
	if m.progressText == "other-step" {
		t.Fatal("stale workspace event updated active progress")
	}
	if len(m.workspaces) != 1 || m.workspaces[0].ID != "ws-other" {
		t.Fatalf("workspace monitor was not updated by non-active event: %#v", m.workspaces)
	}
}

func TestModelUnscopedRunEventIgnoredDuringWorkspaceSwitch(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-501", "")
	m.activeWorkspaceID = "ws-501"
	m.activeRunID = ""
	m.progressText = "Workspace switched; refreshing state"
	m.currentTask = "fresh workspace pending context"
	m.runtimeEvents = nil

	cmd := m.applyBackendEvent(backend.Event{
		Type:    backend.EventStepDispatched,
		RunID:   "run-from-previous-workspace",
		Payload: map[string]any{"kind": "old workspace step"},
	})
	if cmd != nil {
		t.Fatal("unscoped stale run event should not trigger refresh command")
	}
	if m.progress != progressIdle || m.progressText != "Workspace switched; refreshing state" || len(m.runtimeEvents) != 0 {
		t.Fatalf("unscoped stale run event polluted switched view: progress=%s text=%q events=%v", m.progress, m.progressText, m.runtimeEvents)
	}
	if m.currentTask != "fresh workspace pending context" {
		t.Fatalf("currentTask = %q, want fresh workspace content preserved", m.currentTask)
	}
}

func TestModelWorkspaceCreatedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventWorkspaceCreated,
		WorkspaceID: "ws-b",
		Payload: map[string]any{
			"title":                "B",
			"deliverableDirectory": "/tmp/b",
			"state":                "active",
		},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected event-driven workspace list refresh command")
	}
	if len(m.workspaces) != 2 || m.workspaces[1].ID != "ws-b" || m.workspaces[1].Title != "B" {
		t.Fatalf("workspaces = %+v, want created workspace appended before full refresh", m.workspaces)
	}
	if m.activeWorkspaceID != "ws-a" {
		t.Fatalf("active workspace = %q, want unchanged", m.activeWorkspaceID)
	}
}

func TestShouldConsumeBackendSubscriptionEventMatchesHandledEvents(t *testing.T) {
	handledEvents := []backend.EventType{
		backend.EventWorkspaceCreated,
		backend.EventWorkspaceListChanged,
		backend.EventWorkspaceProjectionChanged,
		backend.EventWorkspaceStateChanged,
		backend.EventWorkspaceDeleted,
		backend.EventGoalDefined,
		backend.EventDoDDefined,
		backend.EventRunStateChanged,
		backend.EventStepDispatched,
		backend.EventStepCompleted,
		backend.EventDecisionCreated,
		backend.EventDecisionResolved,
		backend.EventApprovalCreated,
		backend.EventApprovalResolved,
		backend.EventResultAccepted,
		backend.EventMemoryCreated,
		backend.EventMemoryUpdated,
		backend.EventMemoryDeleted,
		backend.EventMemoryRecallRecorded,
		backend.EventLLMStreamProgress,
		backend.EventLLMUsageRecorded,
		backend.EventPatchPreviewed,
		backend.EventPatchApplied,
		backend.EventPatchFailed,
		backend.EventPatchReverted,
		backend.EventArtifactContinuationChanged,
		backend.EventGovernanceEvaluated,
		backend.EventGovernanceResolved,
		backend.EventGovernanceEscalated,
		backend.EventGovernanceBlocked,
	}
	for _, eventType := range handledEvents {
		if !shouldConsumeBackendSubscriptionEvent(eventType) {
			t.Fatalf("%s should pass subscription filtering", eventType)
		}
		if !isBackendEventQueueCritical(eventType) {
			t.Fatalf("%s should be preserved when the event queue is full", eventType)
		}
	}
	unknown := backend.EventType("custom/non-critical")
	if shouldConsumeBackendSubscriptionEvent(unknown) || isBackendEventQueueCritical(unknown) {
		t.Fatalf("%s should remain filtered/non-critical", unknown)
	}
}

func TestModelSubscriptionQueuesRunProgressEvents(t *testing.T) {
	events := &subscriptionCaptureEvents{}
	m := NewAppModel(&subscriptionCaptureBE{fakeBE: newFake(t), events: events}, "ws-a", "")
	if events.handler == nil {
		t.Fatal("expected model to subscribe to backend events")
	}

	events.handler(context.Background(), backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Payload:     map[string]any{"kind": "shell/command"},
	})
	select {
	case got := <-m.eventCh:
		if got.Type != backend.EventStepDispatched || got.WorkspaceID != "ws-a" || got.RunID != "run-a" {
			t.Fatalf("queued event = %+v, want active step event", got)
		}
	default:
		t.Fatal("step event was filtered before reaching the TUI model queue")
	}

	events.handler(context.Background(), backend.Event{Type: backend.EventType("custom/non-critical")})
	select {
	case got := <-m.eventCh:
		t.Fatalf("unknown event should be filtered, got %+v", got)
	default:
	}
}

func TestModelAllowsBackendWithoutEvents(t *testing.T) {
	m := NewAppModel(&noEventsBE{fakeBE: newFake(t)}, "ws-a", "")
	if m.unsubscribe == nil {
		t.Fatal("expected noop unsubscribe when backend has no event client")
	}
	if m.eventSubscriptionActive {
		t.Fatal("expected event subscription to stay inactive without an event client")
	}
}

func TestEnqueueBackendEventKeepsWorkspaceCreatedWhenChannelFull(t *testing.T) {
	ch := make(chan backend.Event, 1)
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventStepCompleted, WorkspaceID: "ws-a"})
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventWorkspaceCreated, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Type != backend.EventWorkspaceCreated || got.WorkspaceID != "ws-b" {
		t.Fatalf("event = %+v, want workspace created to replace older queued event", got)
	}
}

func TestEnqueueBackendEventDropsNonCriticalWhenChannelFull(t *testing.T) {
	ch := make(chan backend.Event, 1)
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventWorkspaceCreated, WorkspaceID: "ws-a"})
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventType("custom/non-critical"), WorkspaceID: "ws-b"})

	got := <-ch
	if got.Type != backend.EventWorkspaceCreated || got.WorkspaceID != "ws-a" {
		t.Fatalf("event = %+v, want existing critical event preserved", got)
	}
}

func TestEnqueueBackendEventKeepsStepWhenChannelFull(t *testing.T) {
	ch := make(chan backend.Event, 1)
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventStepDispatched, WorkspaceID: "ws-a"})
	enqueueBackendEvent(ch, backend.Event{Type: backend.EventStepCompleted, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Type != backend.EventStepCompleted || got.WorkspaceID != "ws-b" {
		t.Fatalf("event = %+v, want latest critical step event", got)
	}
}

func TestModelWorkspaceListChangedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)

	cmd := m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceListChanged})
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("workspace list changed event should refresh list: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
}

func TestModelWorkspaceProjectionBaselineEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"

	cmd := m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged})
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("projection baseline event should refresh list: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
}

func TestModelWorkspaceProjectionDetailEventRefreshesOnlyActiveDetail(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.ensureWorkspaceRuntimeState("ws-a").ContextInFlight = false
	previousActiveSeq := m.workspaceContextRequestSeqFor("ws-a")

	cmd := m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-a"})
	if cmd == nil || !m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-a") != previousActiveSeq+1 {
		t.Fatalf("active detail projection event cmd/list/seq = %v/%v/%d, want list and detail refresh with seq %d", cmd != nil, m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-a"), previousActiveSeq+1)
	}
	m.workspaceListRefreshInFlight = false

	cmd = m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-b"})
	if cmd == nil || !m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-b") != 0 {
		t.Fatalf("inactive detail projection event cmd/list/seq = %v/%v/%d, want list refresh only", cmd != nil, m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-b"))
	}
}

func TestModelWorkspaceProjectionEventRefreshesLiveWorkspaceWithoutPolling(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.ensureWorkspaceRuntimeState("ws-a").ContextInFlight = false
	previousSeq := m.workspaceContextRequestSeqFor("ws-a")

	cmd := m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-a", Revision: 2})
	if cmd == nil {
		t.Fatal("active workspace projection event should refresh workspace state")
	}
	if !m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-a") != previousSeq+1 {
		t.Fatalf("event refresh state = list:%v seq:%d want list refresh and seq %d", m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-a"), previousSeq+1)
	}
}

func TestModelInitDoesNotScheduleActiveWorkspacePoll(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	previousSeq := m.workspaceContextRequestSeqFor("ws-a")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("init should still fetch initial data and wait for backend events")
	}
	if m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-a") != previousSeq {
		t.Fatalf("init refresh state = list:%v seq:%d want no active polling refresh and seq %d", m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-a"), previousSeq)
	}
}

func TestModelWorkspaceListSynchronizesWaitingDecisionPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.progress = progressSubmitting
	m.progressText = "Checking workspace run state"
	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{
			ID:               "ws-a",
			Title:            "A",
			State:            common.WorkspaceStateWaitingDecision,
			ActiveRunID:      "run-a",
			BlockingTicketID: "ticket-a",
		},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("waiting decision overview should request blocker/context refresh")
	}
	if m.progress != progressWaitingDecision || m.activeRunID != "run-a" || m.blockingDecision == nil || m.blockingDecision.ID != "ticket-a" {
		t.Fatalf("overview sync failed: progress=%s run=%q blocker=%+v", m.progress, m.activeRunID, m.blockingDecision)
	}
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Waiting for decision") || !strings.Contains(view, "ws-a decision >") {
		t.Fatalf("view did not reflect waiting decision overview:\n%s", view)
	}
}

func TestFetchWorkspaceContextUsesProjectionSnapshotOnly(t *testing.T) {
	be := newRecordingBE(t, common.RunStateRunning)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{
			ID:          "ws-projection-context",
			State:       common.WorkspaceStateRunning,
			ActiveRunID: "run-projection-context",
			Projection:  backend.ProjectionInfo{State: "ready", WorkspaceRevision: 42},
		},
		Projection: backend.ProjectionInfo{State: "ready", WorkspaceRevision: 42},
		Runs: []backend.RunInfo{{
			ID:          "run-projection-context",
			WorkspaceID: "ws-projection-context",
			State:       common.RunStateRunning,
		}},
		TaskProgress: backend.TaskProgressInfo{
			WorkspaceID: "ws-projection-context",
			RunID:       "run-projection-context",
			State:       string(common.RunStateRunning),
			Summary:     "projection task progress",
			Recovery: backend.RecoveryPlanInfo{
				StepID: "step-projection-context",
			},
		},
		RunHeartbeat: backend.RunHeartbeatInfo{
			WorkspaceID:   "ws-projection-context",
			RunID:         "run-projection-context",
			State:         string(common.RunStateRunning),
			ActivityState: "running",
			Summary:       "projection heartbeat",
		},
	}

	msg, ok := FetchWorkspaceContext(be, "ws-projection-context")().(WorkspaceContextMsg)
	if !ok {
		t.Fatalf("FetchWorkspaceContext returned %T, want WorkspaceContextMsg", msg)
	}
	if msg.Error != nil {
		t.Fatalf("FetchWorkspaceContext error: %v", msg.Error)
	}
	if be.lastSnapshotWorkspace != "ws-projection-context" {
		t.Fatalf("snapshot workspace = %q, want ws-projection-context", be.lastSnapshotWorkspace)
	}
	if be.lastStatusWorkspace != "" || be.lastListWorkspace != "" || be.lastGetWorkspace != "" {
		t.Fatalf("unexpected extra run/workspace reads: status=%q list=%q get=%q", be.lastStatusWorkspace, be.lastListWorkspace, be.lastGetWorkspace)
	}
	if msg.Status == nil || msg.Status.CurrentStepID != "step-projection-context" || msg.Status.State != common.RunStateRunning {
		t.Fatalf("status = %+v, want projection task progress status", msg.Status)
	}
	if msg.Projection.WorkspaceRevision != 42 {
		t.Fatalf("projection = %+v, want snapshot projection revision", msg.Projection)
	}
}

func TestModelActiveWorkspaceWithoutRunIgnoresHistoricalGovernance(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.width = 100
	m.height = 16
	m.workspaces = []backend.WorkspaceInfo{{
		ID:    "ws-a",
		Title: "A",
		State: common.WorkspaceStateActive,
	}}
	m.activeRunID = ""
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.approvals = []backend.ApprovalInfo{*m.blockingApproval}
	m.progress = progressWaitingDecision

	m.syncBlockingDecisionFromList()
	m.syncBlockingApprovalFromList()

	if m.blockingDecision != nil || m.blockingApproval != nil {
		t.Fatalf("blockers = %+v/%+v, want historical blockers ignored", m.blockingDecision, m.blockingApproval)
	}
	if len(m.activeScopeDecisions()) != 0 || len(m.activeScopeApprovals()) != 0 {
		t.Fatalf("active governance = %+v/%+v, want none without active run", m.activeScopeDecisions(), m.activeScopeApprovals())
	}
	if input := stripANSI(m.renderInput(100)); strings.Contains(input, "decision >") || strings.Contains(input, "approval >") {
		t.Fatalf("input prompt = %q, want active workspace prompt without historical blocker suffix", input)
	}
}

func TestModelWorkspaceSwitchToDecisionWorkspaceShowsDecisionTail(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.width = 120
	m.height = 16
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-b"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "workspace A active task"
	m.result = strings.Repeat("old workspace output\n", 40)
	_ = m.View()
	m.scrollMain(8)

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, _ = m.confirmWorkspaceSelection()
	m = asModel(next)
	if m.mainViewportUserScrolled || m.mainViewportPendingUpdate {
		t.Fatalf("workspace switch should reset viewport scroll flags")
	}

	next, _ = m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-b"},
	}})
	m = asModel(next)
	next, _ = m.Update(DecisionDetailMsg{
		WorkspaceID: "ws-b",
		Decision: &backend.DecisionInfo{
			ID:          "ticket-b",
			WorkspaceID: "ws-b",
			RunID:       "run-b",
			State:       "open",
			Question:    "choose replan or reject",
		},
	})
	m = asModel(next)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Decision required") || !strings.Contains(view, "choose replan or reject") {
		t.Fatalf("decision workspace tail not visible after switch:\n%s", view)
	}
	if strings.Contains(view, "workspace A active task") || strings.Contains(view, "old workspace output") {
		t.Fatalf("decision workspace view still shows previous workspace content:\n%s", view)
	}
}

func TestModelWorkspaceSwitchToDecisionWorkspaceSuggestsDecisionCommandsBeforeDetail(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.width = 120
	m.height = 18
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b460b00b639f57e4", Title: "B", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-b", BlockingTicketID: "ticket-b"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "workspace A active task"
	m.result = "workspace A output"

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b460b00b639f57e4"
	next, cmd := m.confirmWorkspaceSelection()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("workspace switch should request refreshes")
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-b" {
		t.Fatalf("blocking decision = %+v, want ticket-b from workspace overview", m.blockingDecision)
	}
	m.setInput("/")
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-b460b00 decision > /") {
		t.Fatalf("decision prompt missing after switch:\n%s", view)
	}
	if !strings.Contains(view, "/decision answer") || !strings.Contains(view, "/decision replan") {
		t.Fatalf("decision command suggestions missing before detail load:\n%s", view)
	}
	suggestions := m.slashCommandSuggestions("/")
	if len(suggestions) < 2 || suggestions[0].Command != "/decision answer" || suggestions[1].Command != "/decision replan" {
		t.Fatalf("suggestions = %+v, want decision commands first", suggestions)
	}

	m.setInput("/decision ")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = asModel(next)
	if m.input != "/decision answer " {
		t.Fatalf("completion = %q, want /decision answer with trailing space", m.input)
	}
}

func TestModelWorkspaceSwitchThenProjectionRefreshUpdatesMainViewAndStatusBar(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive},
	}})
	m = asModel(next)
	m.width = 120
	m.height = 32
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "task-a"
	m.progress = progressRunning
	m.progressText = "progress-a"

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, cmd := m.confirmWorkspaceSelection()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("workspace selection should request list/context refresh")
	}
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("switch state = active:%q task:%q progress:%q, want cleared ws-b view", m.activeWorkspaceID, m.currentTask, m.progressText)
	}

	previousSeq := m.workspaceContextRequestSeqFor("ws-b")
	cmd = m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-b", Revision: 11})
	if cmd != nil || !m.ensureWorkspaceRuntimeState("ws-b").ContextPending || m.workspaceContextRequestSeqFor("ws-b") != previousSeq {
		t.Fatalf("projection event should coalesce with switch refresh: cmd=%v pending=%v seq=%d want %d", cmd != nil, m.ensureWorkspaceRuntimeState("ws-b").ContextPending, m.workspaceContextRequestSeqFor("ws-b"), previousSeq)
	}

	seq := m.workspaceContextRequestSeqFor("ws-b")
	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  99,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A stale", State: common.WorkspaceStateFailed, ActiveRunID: "run-a"},
		DisplayRun: &backend.RunInfo{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       common.RunStateFailed,
			ExecutionContext: map[string]any{
				"goal": map[string]any{"text": "stale task-a"},
			},
		},
	})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("inactive stale context polluted switched view: active=%q task=%q progress=%q", m.activeWorkspaceID, m.currentTask, m.progressText)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  seq,
		WorkspaceID: "ws-b",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateRunning, ActiveRunID: "run-b"},
		ActiveRun:   &backend.RunInfo{ID: "run-b", WorkspaceID: "ws-b", State: common.RunStateRunning},
		DisplayRun: &backend.RunInfo{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"goal": map[string]any{"text": "build workspace B"},
				"dod":  map[string]any{"items": []any{"B result accepted"}},
			},
		},
		Status: &backend.RunStatusInfo{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           common.RunStateRunning,
			CurrentStepKind: "llm/workspace_execution",
		},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateRunning, ActiveRunID: "run-b"},
			TaskProgress: backend.TaskProgressInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", Summary: "B is making progress"},
			RunHeartbeat: backend.RunHeartbeatInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", ActivityState: "running", Summary: "B heartbeat"},
			SchedulerDiagnostics: &backend.SchedulerDiagnosticsInfo{
				LatestContinuation: &backend.ContinuationDiagnosticInfo{
					WorkspaceID:    "ws-b",
					RunID:          "run-b",
					Kind:           "run_continuation",
					State:          "completed",
					Reason:         "active-diagnostic",
					Recoverability: "no_action_needed",
				},
			},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-b" || m.activeRunID != "run-b" || m.currentTask != "build workspace B" {
		t.Fatalf("active context not applied: active=%q run=%q task=%q", m.activeWorkspaceID, m.activeRunID, m.currentTask)
	}
	status := stripANSI(m.renderRuntimeStatusBar(120))
	for _, want := range []string{"Running - B is making progress", "ws ws-b", "run run-b"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status bar = %q, missing %q", status, want)
		}
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "build workspace B") || strings.Contains(view, "task-a") {
		t.Fatalf("main view not rebound to workspace B:\n%s", view)
	}
	if scheduler := schedulerDiagnosticsStatusText(m.schedulerDiagnostics); !strings.Contains(scheduler, "active-diagnostic") {
		t.Fatalf("scheduler diagnostics = %q, want active workspace diagnostic", scheduler)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateFailed, ActiveRunID: "run-a"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateFailed, ActiveRunID: "run-a"},
			SchedulerDiagnostics: &backend.SchedulerDiagnosticsInfo{
				LatestContinuation: &backend.ContinuationDiagnosticInfo{
					WorkspaceID: "ws-a",
					RunID:       "run-a",
					Kind:        "replan_continuation",
					State:       "failed",
					Reason:      "inactive-diagnostic",
				},
			},
		},
	})
	m = asModel(next)
	if scheduler := schedulerDiagnosticsStatusText(m.schedulerDiagnostics); strings.Contains(scheduler, "inactive-diagnostic") || !strings.Contains(scheduler, "active-diagnostic") {
		t.Fatalf("inactive scheduler diagnostic polluted active view: %q", scheduler)
	}

	cmd = m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-a", Revision: 12})
	if cmd != nil || m.currentTask != "build workspace B" || !strings.Contains(m.renderRuntimeStatusBar(120), "B is making progress") {
		t.Fatalf("inactive projection event polluted active B view: cmd=%v task=%q status=%q", cmd != nil, m.currentTask, stripANSI(m.renderRuntimeStatusBar(120)))
	}
}

func TestTUIE2EWorkspaceSwitchProjectionRefreshVisualSnapshot(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive},
	}})
	m = asModel(next)
	m.width = 120
	m.height = 32
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"
	m.currentTask = "task-a"
	m.progress = progressRunning
	m.progressText = "progress-a"
	writeOptionalTUIViewSnapshot(t, "01-before-switch.txt", m)

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, cmd := m.confirmWorkspaceSelection()
	m = asModel(next)
	if cmd == nil {
		t.Fatal("workspace selection should request refresh")
	}
	writeOptionalTUIViewSnapshot(t, "02-after-switch-before-refresh.txt", m)
	if m.activeWorkspaceID != "ws-b" || strings.Contains(stripANSI(m.View()), "task-a") {
		t.Fatalf("workspace switch did not clear stale workspace A view:\n%s", stripANSI(m.View()))
	}
	if !strings.Contains(stripANSI(m.renderRuntimeStatusBar(120)), "refreshing state") {
		t.Fatalf("status bar missing refresh state after switch: %q", stripANSI(m.renderRuntimeStatusBar(120)))
	}

	seq := m.workspaceContextRequestSeqFor("ws-b")
	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  seq,
		WorkspaceID: "ws-b",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateRunning, ActiveRunID: "run-b"},
		ActiveRun:   &backend.RunInfo{ID: "run-b", WorkspaceID: "ws-b", State: common.RunStateRunning},
		DisplayRun: &backend.RunInfo{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"goal": map[string]any{"text": "build workspace B"},
				"dod":  map[string]any{"items": []any{"B result accepted"}},
			},
		},
		Status: &backend.RunStatusInfo{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           common.RunStateRunning,
			CurrentStepKind: "llm/workspace_execution",
		},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateRunning, ActiveRunID: "run-b"},
			TaskProgress: backend.TaskProgressInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", Summary: "B is making progress"},
			RunHeartbeat: backend.RunHeartbeatInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", ActivityState: "running", Summary: "B heartbeat"},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	writeOptionalTUIViewSnapshot(t, "03-after-projection-refresh.txt", m)
	view := stripANSI(m.View())
	status := stripANSI(m.renderRuntimeStatusBar(120))
	for _, want := range []string{"build workspace B", "B result accepted"} {
		if !strings.Contains(view, want) {
			t.Fatalf("refreshed main view missing %q:\n%s", want, view)
		}
	}
	for _, want := range []string{"Running - B is making progress", "ws ws-b", "run run-b"} {
		if !strings.Contains(status, want) {
			t.Fatalf("refreshed status bar = %q, missing %q", status, want)
		}
	}
	if strings.Contains(view, "task-a") || strings.Contains(status, "run-a") {
		t.Fatalf("refreshed workspace B view/status contains stale workspace A state:\nview:\n%s\nstatus:\n%s", view, status)
	}

	cmd = m.applyBackendEvent(backend.Event{Type: backend.EventWorkspaceProjectionChanged, WorkspaceID: "ws-a", Revision: 12})
	if cmd != nil {
		t.Fatal("inactive workspace projection event should not request active main view refresh")
	}
	writeOptionalTUIViewSnapshot(t, "04-after-inactive-projection-event.txt", m)
	if view := stripANSI(m.View()); !strings.Contains(view, "build workspace B") || strings.Contains(view, "task-a") {
		t.Fatalf("inactive projection event polluted active workspace view:\n%s", view)
	}
}

func TestModelIgnoresStaleWorkspaceListResponse(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-a", Title: "A", State: "active"}}
	m.workspaceListRefreshInFlight = true
	m.workspaceListRequestSeq = 2

	next, cmd := m.Update(WorkspaceListMsg{
		RequestSeq: 1,
		Workspaces: []backend.WorkspaceInfo{
			{ID: "ws-old", Title: "Old", State: "active"},
		},
	})
	m = asModel(next)
	if cmd != nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("stale list response changed refresh state: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if len(m.workspaces) != 1 || m.workspaces[0].ID != "ws-a" {
		t.Fatalf("workspaces = %+v, want unchanged active list", m.workspaces)
	}
}

func TestModelIgnoresStaleWorkspaceContextResponse(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-a", Title: "A", State: "active"}}
	m.activeWorkspaceID = "ws-a"
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 2

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "Old", State: "running"},
	})
	m = asModel(next)
	if cmd != nil {
		t.Fatalf("stale context response returned cmd: %T", cmd())
	}
	if len(m.workspaces) != 1 || m.workspaces[0].Title != "A" {
		t.Fatalf("workspaces = %+v, want unchanged context", m.workspaces)
	}
}

func TestModelRejectsWorkspaceContextOlderThanOverviewRevision(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-a",
		Title:       "A",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-new",
		Projection:  backend.ProjectionInfo{WorkspaceRevision: 5},
	}}})
	m = asModel(next)
	m.currentTask = "new task"
	m.activeRunID = "run-new"
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.ContextInFlight = false
	entry.ContextRequestSeq = 7

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  7,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning, ActiveRunID: "run-old", Projection: backend.ProjectionInfo{WorkspaceRevision: 3}},
		ActiveRun:   &backend.RunInfo{ID: "run-old", WorkspaceID: "ws-a", State: common.RunStateRunning},
		DisplayRun: &backend.RunInfo{
			ID:          "run-old",
			WorkspaceID: "ws-a",
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"goal": map[string]any{"text": "old task"},
			},
		},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning, ActiveRunID: "run-old", Projection: backend.ProjectionInfo{WorkspaceRevision: 3}},
			ViewModel: backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", Revision: 3, WorkspaceRevision: 3},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("older context should schedule projection retry")
	}
	if m.currentTask != "new task" || m.activeRunID != "run-new" || !strings.Contains(m.progressText, "older than list") {
		t.Fatalf("older context applied: task=%q run=%q progress=%q", m.currentTask, m.activeRunID, m.progressText)
	}

	entry = m.ensureWorkspaceRuntimeState("ws-a")
	entry.ContextInFlight = false
	entry.ContextRequestSeq = 8
	next, cmd = m.Update(WorkspaceContextMsg{
		RequestSeq:  8,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning, ActiveRunID: "run-current", Projection: backend.ProjectionInfo{WorkspaceRevision: 5}},
		ActiveRun:   &backend.RunInfo{ID: "run-current", WorkspaceID: "ws-a", State: common.RunStateRunning},
		DisplayRun: &backend.RunInfo{
			ID:          "run-current",
			WorkspaceID: "ws-a",
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"goal": map[string]any{"text": "current task"},
			},
		},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning, ActiveRunID: "run-current", Projection: backend.ProjectionInfo{WorkspaceRevision: 5}},
			ViewModel: backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", Revision: 5, WorkspaceRevision: 5},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("same revision context should continue normal blocker/detail refresh path")
	}
	if m.currentTask != "current task" || m.activeRunID != "run-current" {
		t.Fatalf("same revision context not applied: task=%q run=%q", m.currentTask, m.activeRunID)
	}
}

func TestModelTransientProjectionErrorSchedulesContextRetry(t *testing.T) {
	for _, errorText := range []string{
		"workorch API error (status 409): workspace projection detail not ready",
		"conflict: workspace projection detail not ready",
	} {
		t.Run(errorText, func(t *testing.T) {
			m := NewAppModel(newFake(t), "ws-a", "")
			m.activeWorkspaceID = "ws-a"
			m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 1

			next, cmd := m.Update(WorkspaceContextMsg{
				RequestSeq:  1,
				WorkspaceID: "ws-a",
				Error:       errors.New(errorText),
			})
			m = asModel(next)
			if cmd == nil {
				t.Fatal("transient projection error did not schedule retry")
			}
			if m.lastError != nil || !strings.Contains(m.progressText, "retrying") {
				t.Fatalf("lastError/progressText = %v/%q, want retry without sticky error", m.lastError, m.progressText)
			}
		})
	}
}

func TestModelTransientProjectionErrorKeepsOverviewDecisionState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-a",
		BlockingTicketID: "ticket-a",
	}}
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 1

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-a",
		Error:       errors.New("conflict: workspace projection detail not ready"),
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("transient projection error did not schedule retry")
	}
	if m.progress != progressWaitingDecision || m.blockingDecision == nil || m.blockingDecision.ID != "ticket-a" {
		t.Fatalf("progress/blocker = %s/%+v, want overview decision state", m.progress, m.blockingDecision)
	}
	if strings.Contains(m.progressText, "projection is refreshing") || !strings.Contains(m.progressText, "ticket-a") {
		t.Fatalf("progressText = %q, want active decision blocker instead of projection refresh", m.progressText)
	}
}

func TestModelWorkspaceContextSuccessClearsProjectionError(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.lastError = errors.New("conflict: workspace projection detail not ready")
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 1

	next, _ := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
		ActiveRun:   &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateRunning},
		DisplayRun:  &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateRunning},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
			TaskProgress: backend.TaskProgressInfo{WorkspaceID: "ws-a", RunID: "run-a", State: "running", Summary: "run running"},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if m.lastError != nil {
		t.Fatalf("lastError = %v, want cleared after successful workspace context", m.lastError)
	}
	if m.progress != progressRunning || m.progressText != "run running" {
		t.Fatalf("progress = %s/%q, want running context applied", m.progress, m.progressText)
	}
}

func TestModelWorkspaceContextRequestSeqIsScopedByWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}
	m.activeWorkspaceID = "ws-b"
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 9
	m.ensureWorkspaceRuntimeState("ws-b").ContextRequestSeq = 1

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-b",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-b", Title: "B updated", State: "running"},
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("workspace context response did not return follow-up blocker command")
	}
	if got := findWorkspace(m.workspaces, "ws-b"); got == nil || got.Title != "B updated" {
		t.Fatalf("ws-b context was not applied: %+v", m.workspaces)
	}
}

func TestModelWorkspaceListRefreshCoalescesWhileInFlight(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	m.workspaceListRefreshInFlight = true
	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceCreated,
		WorkspaceID: "ws-b",
		Payload:     map[string]any{"title": "B", "state": "active"},
	})
	if cmd != nil {
		t.Fatal("refresh should be coalesced while list refresh is in flight")
	}
	if !m.workspaceListRefreshPending {
		t.Fatal("expected pending refresh marker")
	}
	next, cmd = m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	if cmd == nil || !m.workspaceListRefreshInFlight || m.workspaceListRefreshPending {
		t.Fatalf("pending refresh not released correctly: cmd=%v inFlight=%v pending=%v", cmd != nil, m.workspaceListRefreshInFlight, m.workspaceListRefreshPending)
	}
}

func TestModelWorkspaceListRefreshDoesNotQueueContextWhileContextInFlight(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.HasDisplay = true
	entry.DisplayRevision = 4
	entry.ContextInFlight = true
	entry.ContextRequestSeq = 7

	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:         "ws-a",
		Title:      "A",
		State:      common.WorkspaceStateRunning,
		Projection: backend.ProjectionInfo{WorkspaceRevision: 5},
	}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd != nil {
		t.Fatalf("workspace list refresh returned cmd while context is already in flight")
	}
	if entry.ContextPending || entry.ContextRequestSeq != 7 {
		t.Fatalf("context refresh queued while in flight: pending=%v seq=%d", entry.ContextPending, entry.ContextRequestSeq)
	}
}

func TestModelWorkspaceListRefreshRequestsContextWhenOverviewOutrunsDisplay(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.HasDisplay = true
	entry.DisplayRevision = 3
	entry.ContextRequestSeq = 2

	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:         "ws-a",
		Title:      "A",
		State:      common.WorkspaceStateRunning,
		Projection: backend.ProjectionInfo{WorkspaceRevision: 5},
	}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd == nil || !entry.ContextInFlight || entry.ContextRequestSeq != 3 {
		t.Fatalf("stale display did not request context refresh: cmd=%v inFlight=%v seq=%d", cmd != nil, entry.ContextInFlight, entry.ContextRequestSeq)
	}
}

func TestModelWorkspaceContextRefreshCoalescesWhileInFlight(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateRunning, ActiveRunID: "run-a"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.ContextInFlight = false
	previousSeq := entry.ContextRequestSeq

	cmd := m.requestWorkspaceContextRefresh(false)
	if cmd == nil || !entry.ContextInFlight || entry.ContextRequestSeq != previousSeq+1 {
		t.Fatalf("first context refresh = cmd:%v inFlight:%v seq:%d want %d", cmd != nil, entry.ContextInFlight, entry.ContextRequestSeq, previousSeq+1)
	}
	cmd = m.requestWorkspaceContextRefresh(false)
	if cmd != nil || !entry.ContextPending || entry.ContextRequestSeq != previousSeq+1 {
		t.Fatalf("second context refresh should coalesce = cmd:%v pending:%v seq:%d want %d", cmd != nil, entry.ContextPending, entry.ContextRequestSeq, previousSeq+1)
	}

	next, cmd = m.Update(WorkspaceContextMsg{RequestSeq: previousSeq + 1, WorkspaceID: "ws-a", Workspace: &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning}, Snapshot: &backend.WorkspaceSnapshotInfo{Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateRunning}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd == nil || !entry.ContextInFlight || entry.ContextPending || entry.ContextRequestSeq != previousSeq+2 {
		t.Fatalf("pending context refresh not released = cmd:%v inFlight:%v pending:%v seq:%d want %d", cmd != nil, entry.ContextInFlight, entry.ContextPending, entry.ContextRequestSeq, previousSeq+2)
	}
}

func TestModelNonActiveWorkspaceStateEventOnlyUpdatesMonitor(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.progressText = "active progress"
	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-b",
		Payload:     map[string]any{"state": "running"},
	})
	if cmd != nil {
		t.Fatal("known non-active workspace state event should not refresh active context")
	}
	if m.workspaces[1].State != "running" || m.progressText != "active progress" {
		t.Fatalf("non-active state update leaked into active view: ws=%+v progress=%q", m.workspaces[1], m.progressText)
	}
}

func TestModelTracksEventRevisionPerWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.ensureWorkspaceRuntimeState("ws-a").ContextInFlight = false

	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-b",
		Revision:    50,
		Payload:     map[string]any{"state": "running"},
	})
	if cmd != nil {
		t.Fatal("inactive workspace revision should not refresh active context")
	}
	cmd = m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Revision:    2,
		Payload:     map[string]any{"kind": "shell/command"},
	})
	if cmd != nil {
		t.Fatal("active workspace first revision should not be compared against inactive workspace revision")
	}
	wsBState := m.workspaceRuntimeState("ws-b")
	wsAState := m.workspaceRuntimeState("ws-a")
	if wsBState == nil || wsBState.LastRevision != 50 || wsAState == nil || wsAState.LastRevision != 2 {
		t.Fatalf("workspace runtime states = %+v, want ws-a revision=2 ws-b revision=50", m.workspaceRuntimeStates)
	}

	cmd = m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Revision:    4,
		Payload:     map[string]any{"kind": "shell/command"},
	})
	if cmd == nil || !strings.Contains(m.progressText, "revision gap") {
		t.Fatalf("active workspace revision gap should refresh context: cmd=%v progress=%q", cmd != nil, m.progressText)
	}
	wsAState = m.workspaceRuntimeState("ws-a")
	if wsAState == nil || wsAState.RevisionGapCount != 1 || wsAState.LastRevisionGap != 1 {
		t.Fatalf("ws-a revision gap state = %+v, want count=1 latest=1", wsAState)
	}

	cmd = m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-b",
		Revision:    53,
		Payload:     map[string]any{"state": "running"},
	})
	if cmd != nil {
		t.Fatal("inactive workspace revision gap should not refresh active context")
	}
	wsBState = m.workspaceRuntimeState("ws-b")
	if wsBState == nil || wsBState.RevisionGapCount != 1 || wsBState.LastRevisionGap != 2 {
		t.Fatalf("ws-b revision gap state = %+v, want count=1 latest=2", wsBState)
	}
}

func TestModelLLMStreamProgressUpdatesStatus(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"

	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventLLMStreamProgress,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Revision:    1,
		Payload: map[string]any{
			backend.PayloadKeyProviderID:    "local",
			backend.PayloadKeyModel:         "verify-model",
			backend.PayloadKeyStatus:        "streaming",
			backend.PayloadKeyChunkCount:    3,
			backend.PayloadKeyReceivedBytes: 128,
		},
	})
	if cmd != nil {
		t.Fatal("llm stream progress should not fetch context")
	}
	if m.progress != progressRunning || !strings.Contains(m.progressText, "LLM streaming") || len(m.runtimeEvents) != 1 {
		t.Fatalf("stream progress not applied: progress=%s text=%q events=%v", m.progress, m.progressText, m.runtimeEvents)
	}
}

func TestModelGovernanceResolvedEventRefreshesWorkspaceState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateBlocked, ActiveRunID: "run-1"},
	}})
	m = asModel(next)
	m.workspaceListRefreshInFlight = false
	m.decisions = []backend.DecisionInfo{{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}}
	m.blockingDecision = &m.decisions[0]
	m.activeRunID = "run-1"

	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventDecisionResolved,
		WorkspaceID: "ws-a",
		RunID:       "run-1",
		Payload:     map[string]any{"ticketId": "ticket-1"},
	})

	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("resolved event should refresh workspace state: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if m.blockingDecision != nil || m.decisions[0].State != "resolved" || m.progress != progressSubmitting {
		t.Fatalf("resolved event not applied: blocker=%+v decision=%+v progress=%s", m.blockingDecision, m.decisions[0], m.progress)
	}
}

func TestModelDecisionResolvedDoesNotHideServerOpenProjection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
	}}})
	m = asModel(next)
	m.width = 120
	m.height = 32
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}

	next, _ = m.Update(DecisionResolvedMsg{Decision: &backend.DecisionInfo{
		ID:          "ticket-1",
		WorkspaceID: "ws-a",
		RunID:       "run-1",
		State:       "resolved",
	}})
	m = asModel(next)
	if m.blockingDecision != nil || !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("resolved prompt/blocker = %q/%+v, want server-projected decision prompt", stripANSI(m.renderInput(120)), m.blockingDecision)
	}

	next, _ = m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
	}}})
	m = asModel(next)
	if !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("server list blocker should remain visible after local resolved marker: input=%q blocker=%+v", stripANSI(m.renderInput(120)), m.blockingDecision)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		WorkspaceID: "ws-a",
		Workspace: &backend.WorkspaceInfo{
			ID:               "ws-a",
			Title:            "A",
			State:            common.WorkspaceStateWaitingDecision,
			ActiveRunID:      "run-1",
			BlockingTicketID: "ticket-1",
		},
		ActiveRun:  &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", State: common.RunStateRunning},
		DisplayRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", State: common.RunStateRunning},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{
				ID:               "ws-a",
				Title:            "A",
				State:            common.WorkspaceStateWaitingDecision,
				ActiveRunID:      "run-1",
				BlockingTicketID: "ticket-1",
			},
			Decisions: []backend.DecisionInfo{{
				ID:          "ticket-1",
				WorkspaceID: "ws-a",
				RunID:       "run-1",
				State:       "open",
			}},
		},
		Decisions:        []backend.DecisionInfo{{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("server snapshot blocker should remain visible after local resolved marker: input=%q blocker=%+v", stripANSI(m.renderInput(120)), m.blockingDecision)
	}
}

func TestModelLiveStateEventClearsStaleDecisionPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-a", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-1", BlockingTicketID: "ticket-1"}}
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.progress = progressWaitingDecision
	m.progressText = "Waiting for decision"
	m.width = 120
	m.height = 32

	cmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-a",
		RunID:       "run-1",
		Payload: map[string]any{
			"state":       "running",
			"activeRunId": "run-1",
		},
	})
	if cmd != nil {
		t.Fatalf("running workspace state event should not require immediate context refresh")
	}
	if m.blockingDecision != nil || m.progress != progressRunning || !strings.Contains(stripANSI(m.renderInput(120)), "ws-a running >") {
		t.Fatalf("workspace running event kept stale decision prompt: input=%q blocker=%+v progress=%s", stripANSI(m.renderInput(120)), m.blockingDecision, m.progress)
	}
}

func TestModelPromptModeUsesProjectionOverLocalDecisionCache(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{
		ID:          "ws-a",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-1",
		Projection:  backend.ProjectionInfo{WorkspaceRevision: 20},
		ViewModel:   backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 20},
	}}
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.PromptMode = workspacePromptModeDecision
	entry.PromptModeRevision = 10
	m.applyWorkspaceViewModel(m.workspaces[0].ViewModel)
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.input = "/"
	m.inputCursor = len([]rune(m.input))

	input := stripANSI(m.renderInput(120))
	if strings.Contains(input, "decision >") || strings.Contains(input, "/decision answer") || !strings.Contains(input, "ws-a running >") {
		t.Fatalf("prompt/suggestions used local decision cache instead of projection: %q", input)
	}
}

func TestModelWorkspaceListViewModelDrivesPromptMode(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.progress = progressWaitingDecision
	m.width = 120
	m.height = 24

	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-a",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-1",
		Projection:  backend.ProjectionInfo{WorkspaceRevision: 12},
		ViewModel:   backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 12},
	}}})
	m = asModel(next)
	m.setInput("/")

	view := stripANSI(m.renderInput(120))
	if strings.Contains(view, "decision >") || !strings.Contains(view, "ws-a running >") {
		t.Fatalf("input prompt = %q, want list ViewModel running prompt", view)
	}
	if matches := m.slashCommandSuggestions("/"); len(matches) == 0 || matches[0].Command != "/run resume" {
		t.Fatalf("slash suggestions = %+v, want running contextual commands from list ViewModel", matches)
	}
}

func TestModelLiveRunStateEventClearsStaleDecisionPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-a", State: common.WorkspaceStateWaitingDecision, ActiveRunID: "run-1", BlockingTicketID: "ticket-1"}}
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.progress = progressWaitingDecision
	m.progressText = "Waiting for decision"
	m.width = 120
	m.height = 32

	m.applyBackendEvent(backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-a",
		RunID:       "run-1",
		Payload:     map[string]any{"state": "running"},
	})
	if m.blockingDecision != nil || m.progress != progressRunning || !strings.Contains(stripANSI(m.renderInput(120)), "ws-a running >") {
		t.Fatalf("run running event kept stale decision prompt: input=%q blocker=%+v progress=%s", stripANSI(m.renderInput(120)), m.blockingDecision, m.progress)
	}
}

func TestModelWorkspaceDeletedEventClearsActiveWorkspaceView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.currentTask = "stale task"
	m.result = "stale result"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-old", State: "open"}
	next, cmd := m.Update(BackendEventMsg{Event: backend.Event{
		Type:        backend.EventWorkspaceDeleted,
		WorkspaceID: "ws-a",
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected refresh command after workspace deletion")
	}
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.result != "" || m.blockingDecision != nil {
		t.Fatalf("active delete did not clear/switch view: active=%q task=%q result=%q blocker=%+v", m.activeWorkspaceID, m.currentTask, m.result, m.blockingDecision)
	}
	if len(m.workspaces) != 1 || m.workspaces[0].ID != "ws-b" {
		t.Fatalf("workspaces = %+v, want deleted workspace removed before full refresh", m.workspaces)
	}
}

func TestTUIE2EWorkspaceListLiveRefreshFromBackendEvents(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
	}})
	m = asModel(next)
	createCmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceCreated,
		WorkspaceID: "ws-b",
		Payload: map[string]any{
			"title":                "B",
			"deliverableDirectory": "/tmp/b",
			"state":                string(common.WorkspaceStateActive),
		},
	})
	if createCmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("created event should trigger async list refresh: cmd=%v inFlight=%v", createCmd != nil, m.workspaceListRefreshInFlight)
	}
	if len(m.workspaces) != 2 || m.workspaces[1].ID != "ws-b" {
		t.Fatalf("created workspace not visible before full refresh: %+v", m.workspaces)
	}

	next, _ = m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive},
	}})
	m = asModel(next)
	m.progressText = "active progress"
	stateCmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceStateChanged,
		WorkspaceID: "ws-b",
		Payload:     map[string]any{"state": string(common.WorkspaceStateRunning)},
	})
	if stateCmd != nil || m.workspaces[1].State != common.WorkspaceStateRunning || m.progressText != "active progress" {
		t.Fatalf("non-active state event leaked into active view: cmd=%v ws=%+v progress=%q", stateCmd != nil, m.workspaces[1], m.progressText)
	}

	deleteCmd := m.applyBackendEvent(backend.Event{
		Type:        backend.EventWorkspaceDeleted,
		WorkspaceID: "ws-b",
	})
	if deleteCmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("deleted event should trigger async list refresh: cmd=%v inFlight=%v", deleteCmd != nil, m.workspaceListRefreshInFlight)
	}
	if len(m.workspaces) != 1 || m.workspaces[0].ID != "ws-a" || m.activeWorkspaceID != "ws-a" {
		t.Fatalf("deleted workspace not removed from monitor: workspaces=%+v active=%q", m.workspaces, m.activeWorkspaceID)
	}
}

func TestModelResultArtifacts(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventResultAccepted,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"summary":   "完成",
			"artifacts": []any{"report.md", map[string]any{"name": "data.json"}},
		},
	})
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "report.md") || !strings.Contains(view, "data.json") {
		t.Fatalf("artifact context missing:\n%s", view)
	}
}

func TestModelResultArtifactsHiddenWhenResultContainsArtifactIndex(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "交付物索引\n• run-1/artifact/result-summary"
	m.artifacts = []string{"run-1/artifact/result-summary"}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if strings.Contains(view, "Artifacts") {
		t.Fatalf("duplicate Artifacts section should be hidden when result has artifact index:\n%s", view)
	}
}

func TestModelResultWebSources(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "完成"
	m.sources = []string{"fetch: Go Downloads - https://go.dev/dl/ (run-1/step-fetch)"}
	m.width = 100
	m.height = 20
	view := stripANSI(m.View())
	if !strings.Contains(view, "Sources") || !strings.Contains(view, "https://go.dev/dl/") || strings.Contains(view, "Download Go snippet") {
		t.Fatalf("source context missing or too verbose:\n%s", view)
	}
}

func TestModelDodSlashCommandSetsPendingDoD(t *testing.T) {
	model := NewAppModel(newFake(t), "ws-1", "")
	model.width = 100
	model.height = 20
	model.setInput("/dod tests pass; diff is delivered")

	updated, _ := model.submitInput()
	got := updated.(*AppModel)
	view := stripANSI(got.View())

	if !strings.Contains(view, "Pending DoD") || !strings.Contains(view, "tests pass") || !strings.Contains(view, "diff is delivered") {
		t.Fatalf("DoD command view missing items:\n%s", view)
	}
}

func TestModelDodSlashCommandNormalizesMarkdownDoD(t *testing.T) {
	model := NewAppModel(newFake(t), "ws-1", "")
	model.width = 120
	model.height = 24
	model.setInput("/dod ### 1. Scope\n- [ ] **Group**:\n  - first logical item\n  - second logical item")

	updated, _ := model.submitInput()
	got := updated.(*AppModel)
	want := []string{
		"Scope / Group： first logical item",
		"Scope / Group： second logical item",
	}
	if len(got.dodItems) != len(want) {
		t.Fatalf("dod items = %#v, want %#v", got.dodItems, want)
	}
	for idx := range want {
		if got.dodItems[idx] != want[idx] {
			t.Fatalf("dod items = %#v, want %#v", got.dodItems, want)
		}
	}
}

func TestModelDodSlashCommandPersistsDoDToActiveWorkspace(t *testing.T) {
	be := &recordingDoDBE{fakeBE: newFake(t)}
	model := NewAppModel(be, "ws-1", "")
	model.width = 120
	model.height = 24
	model.setInput("/dod tests pass; diff is delivered")

	updated, cmd := model.submitInput()
	if cmd == nil {
		t.Fatal("dod command returned nil cmd")
	}
	msg, ok := cmd().(DoDUpdatedMsg)
	if !ok || msg.Error != nil || msg.DoD == nil {
		t.Fatalf("dod command msg = %#v, want DoDUpdatedMsg", msg)
	}
	if be.workspaceID != "ws-1" {
		t.Fatalf("persisted workspace = %q, want ws-1", be.workspaceID)
	}
	want := []string{"tests pass", "diff is delivered"}
	if len(be.items) != len(want) {
		t.Fatalf("persisted items = %#v, want %#v", be.items, want)
	}
	for idx := range want {
		if be.items[idx] != want[idx] {
			t.Fatalf("persisted items = %#v, want %#v", be.items, want)
		}
	}

	next, refresh := updated.(*AppModel).Update(msg)
	if refresh == nil {
		t.Fatal("dod update should refresh workspace context")
	}
	got := next.(AppModel)
	if got.progressText != "DoD saved" {
		t.Fatalf("progress text = %q, want DoD saved", got.progressText)
	}
}

func TestModelWorkspaceCreateCommandUsesTemplateAndActivatesWorkspace(t *testing.T) {
	be := &workspaceCreateFake{fakeBE: newFake(t)}
	model := NewAppModel(be, "", "")
	model.width = 120
	model.height = 24
	model.setInput("/workspace create code agent --template software-dev --dir /tmp/code-agent")

	updated, cmd := model.submitInput()
	if cmd == nil {
		t.Fatal("workspace create command returned nil cmd")
	}
	msg, ok := cmd().(WorkspaceCreatedMsg)
	if !ok || msg.Error != nil || msg.Workspace == nil {
		t.Fatalf("workspace create msg = %#v, want created workspace", msg)
	}
	if be.createReq.Title != "code agent" || be.createReq.TemplateID != "software-dev" || be.createReq.DeliverableDirectory != "/tmp/code-agent" {
		t.Fatalf("create request = %+v, want title/template/dir", be.createReq)
	}

	next, refresh := updated.(*AppModel).Update(msg)
	if refresh == nil {
		t.Fatal("expected workspace create to refresh list/context")
	}
	got := next.(AppModel)
	if got.activeWorkspaceID != "ws-created" {
		t.Fatalf("active workspace = %q, want ws-created", got.activeWorkspaceID)
	}
	view := stripANSI(got.View())
	if !strings.Contains(view, "bindings: planning,execution") {
		t.Fatalf("view missing workspace binding summary:\n%s", view)
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func writeOptionalTUIViewSnapshot(t *testing.T, name string, model AppModel) {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv("WORKORCH_TUI_WORKSPACE_SWITCH_ARTIFACT_DIR"))
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create TUI snapshot dir: %v", err)
	}
	content := "status:\n" + stripANSI(model.renderRuntimeStatusBar(model.width)) + "\n\nview:\n" + stripANSI(model.View())
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write TUI snapshot %s: %v", name, err)
	}
}

// ── fake backend ──

type fakeBE struct {
	approval *fakeA
}

func newFake(t *testing.T) *fakeBE {
	t.Helper()
	return &fakeBE{approval: &fakeA{}}
}
func (b *fakeBE) Workspace() backend.WorkspaceBackend { return &fakeWS{} }
func (b *fakeBE) Goal() backend.GoalBackend           { return &fakeG{} }
func (b *fakeBE) DoD() backend.DoDBackend             { return &fakeD{} }
func (b *fakeBE) Run() backend.RunBackend             { return &fakeR{} }
func (b *fakeBE) Decision() backend.DecisionBackend   { return &fakeDC{} }
func (b *fakeBE) Approval() backend.ApprovalBackend   { return b.approval }
func (b *fakeBE) Soul() backend.SoulBackend           { return &fakeS{} }
func (b *fakeBE) Skill() backend.SkillBackend         { return &fakeSK{} }
func (b *fakeBE) MCP() backend.MCPBackend             { return &fakeM{} }
func (b *fakeBE) Memory() backend.MemoryBackend       { return &fakeMM{} }
func (b *fakeBE) Template() backend.TemplateBackend   { return &fakeT{} }
func (b *fakeBE) Config() backend.ConfigBackend       { return &fakeC{} }
func (b *fakeBE) Events() backend.EventsBackend       { return &fakeE{} }

type subscriptionCaptureBE struct {
	*fakeBE
	events *subscriptionCaptureEvents
}

func (b *subscriptionCaptureBE) Events() backend.EventsBackend { return b.events }

type noEventsBE struct {
	*fakeBE
}

func (b *noEventsBE) Events() backend.EventsBackend { return nil }

type subscriptionCaptureEvents struct {
	handler backend.EventHandler
}

func (f *subscriptionCaptureEvents) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	f.handler = handler
	return func() {}, nil
}

type workspaceCreateFake struct {
	*fakeBE
	createReq backend.CreateWorkspaceRequest
	workspace *backend.WorkspaceInfo
}

func (b *workspaceCreateFake) Workspace() backend.WorkspaceBackend {
	return &workspaceCreateWS{parent: b}
}

type workspaceCreateWS struct{ parent *workspaceCreateFake }

func (f *workspaceCreateWS) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	f.parent.createReq = req
	ws := &backend.WorkspaceInfo{
		ID:                   "ws-created",
		Title:                req.Title,
		Description:          req.Description,
		DeliverableDirectory: req.DeliverableDirectory,
		State:                common.WorkspaceStateActive,
		SoulBindings: map[string]common.SoulID{
			"planning":  "soul-planning",
			"execution": "soul-execution",
		},
	}
	f.parent.workspace = ws
	return ws, nil
}
func (f *workspaceCreateWS) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	if f.parent.workspace != nil && f.parent.workspace.ID == id {
		return f.parent.workspace, nil
	}
	return nil, common.ErrNotFound
}
func (f *workspaceCreateWS) List(ctx context.Context) ([]backend.WorkspaceInfo, error) {
	if f.parent.workspace == nil {
		return nil, nil
	}
	return []backend.WorkspaceInfo{*f.parent.workspace}, nil
}
func (f *workspaceCreateWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	workspace, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &backend.WorkspaceSnapshotInfo{Workspace: *workspace}, nil
}
func (f *workspaceCreateWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type recordingBE struct {
	*fakeBE
	runState              common.RunState
	snapshot              *backend.WorkspaceSnapshotInfo
	listWorkspaces        []backend.WorkspaceInfo
	listCalls             int
	snapshotCalls         int
	goals                 []string
	activations           []backend.ActivateRunRequest
	restarts              []backend.ActivateRunRequest
	resumes               []common.RunID
	resumePatches         []map[string]any
	lastGetWorkspace      common.WorkspaceID
	lastListWorkspace     common.WorkspaceID
	lastSnapshotWorkspace common.WorkspaceID
	lastResumeWorkspace   common.WorkspaceID
	lastStatusWorkspace   common.WorkspaceID
}

func newRecordingBE(t *testing.T, runState common.RunState) *recordingBE {
	t.Helper()
	return &recordingBE{fakeBE: newFake(t), runState: runState}
}

type decisionRecordingBE struct {
	*fakeBE
	decisions []backend.ResolveDecisionRequest
}

func (b *decisionRecordingBE) Decision() backend.DecisionBackend {
	return &decisionRecordingBackend{parent: b}
}

type decisionRecordingBackend struct{ parent *decisionRecordingBE }

func (f *decisionRecordingBackend) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	f.parent.decisions = append(f.parent.decisions, req)
	return &backend.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *decisionRecordingBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p map[string]any) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *decisionRecordingBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *decisionRecordingBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: id, WorkspaceID: wsID, State: "open"}, nil
}

func (b *recordingBE) Goal() backend.GoalBackend { return &recordingGoal{parent: b} }
func (b *recordingBE) Run() backend.RunBackend   { return &recordingRun{parent: b} }
func (b *recordingBE) Workspace() backend.WorkspaceBackend {
	return &recordingWorkspace{parent: b}
}

type recordingWorkspace struct{ parent *recordingBE }

func (f *recordingWorkspace) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return (&fakeWS{}).Create(ctx, req)
}
func (f *recordingWorkspace) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	f.parent.lastGetWorkspace = id
	return &backend.WorkspaceInfo{ID: id, ActiveRunID: "run-resume"}, nil
}
func (f *recordingWorkspace) List(ctx context.Context) ([]backend.WorkspaceInfo, error) {
	f.parent.listCalls++
	if len(f.parent.listWorkspaces) > 0 {
		return append([]backend.WorkspaceInfo(nil), f.parent.listWorkspaces...), nil
	}
	return nil, nil
}
func (f *recordingWorkspace) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	f.parent.snapshotCalls++
	f.parent.lastSnapshotWorkspace = id
	if f.parent.snapshot != nil {
		return f.parent.snapshot, nil
	}
	return &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: id, ActiveRunID: "run-resume"},
		Runs: []backend.RunInfo{{
			ID:          "run-resume",
			WorkspaceID: id,
			State:       f.parent.runState,
		}},
	}, nil
}
func (f *recordingWorkspace) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type recordingGoal struct{ parent *recordingBE }

func (f *recordingGoal) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*backend.GoalInfo, error) {
	f.parent.goals = append(f.parent.goals, text)
	return &backend.GoalInfo{ID: "g-recorded", WorkspaceID: wsID, Text: text}, nil
}
func (f *recordingGoal) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.GoalInfo, error) {
	return nil, nil
}
func (f *recordingGoal) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *recordingGoal) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id, WorkspaceID: wsID, Text: text}, nil
}

type recordingRun struct{ parent *recordingBE }

func (f *recordingRun) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	switch req.Intent {
	case "", backend.RunCommandIntentResume:
		if req.ExpectedRunID == "" {
			activation := req.Activation
			if len(req.ExecutionContextPatch) > 0 {
				activation.ExecutionContextPatch = req.ExecutionContextPatch
			}
			return f.Activate(ctx, req.WorkspaceID, activation)
		}
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID, req.ExecutionContextPatch)
	case backend.RunCommandIntentRestart:
		activation := req.Activation
		if len(req.ExecutionContextPatch) > 0 {
			activation.ExecutionContextPatch = req.ExecutionContextPatch
		}
		return f.Restart(ctx, req.WorkspaceID, activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *recordingRun) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	f.parent.activations = append(f.parent.activations, req)
	return &backend.RunInfo{ID: "run-new", WorkspaceID: wsID, GoalID: req.GoalID, DoDID: req.DoDID, State: common.RunStateRunning}, nil
}
func (f *recordingRun) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	f.parent.restarts = append(f.parent.restarts, req)
	return &backend.RunInfo{ID: "run-new", WorkspaceID: wsID, GoalID: req.GoalID, DoDID: req.DoDID, State: common.RunStateRunning}, nil
}
func (f *recordingRun) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, executionContextPatch ...map[string]any) (*backend.RunInfo, error) {
	f.parent.resumes = append(f.parent.resumes, id)
	f.parent.lastResumeWorkspace = wsID
	if len(executionContextPatch) > 0 {
		f.parent.resumePatches = append(f.parent.resumePatches, executionContextPatch[0])
	}
	workspaceID := wsID
	if workspaceID == "" {
		workspaceID = "ws-1"
	}
	return &backend.RunInfo{ID: id, WorkspaceID: workspaceID, State: common.RunStateRunning}, nil
}
func (f *recordingRun) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	f.parent.lastGetWorkspace = wsID
	if id == "run-from-ws-1" {
		return &backend.RunInfo{ID: id, WorkspaceID: "ws-1", State: f.parent.runState}, nil
	}
	if wsID == "" {
		wsID = "ws-1"
	}
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: f.parent.runState}, nil
}
func (f *recordingRun) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	f.parent.lastListWorkspace = wsID
	return []backend.RunInfo{{ID: "run-resume", WorkspaceID: wsID, State: f.parent.runState}}, nil
}
func (f *recordingRun) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunStatusInfo, error) {
	f.parent.lastStatusWorkspace = wsID
	if wsID == "" {
		wsID = "ws-1"
	}
	return &backend.RunStatusInfo{ID: id, WorkspaceID: wsID, State: f.parent.runState}, nil
}

type conflictApprovalBE struct {
	*fakeBE
	runState common.RunState
	resumed  bool
}

func (b *conflictApprovalBE) Approval() backend.ApprovalBackend { return &conflictApprovalBackend{} }
func (b *conflictApprovalBE) Run() backend.RunBackend           { return &conflictRunBackend{parent: b} }
func (b *conflictApprovalBE) Workspace() backend.WorkspaceBackend {
	return &conflictWorkspaceBackend{parent: b}
}

type conflictWorkspaceBackend struct{ parent *conflictApprovalBE }

func (f *conflictWorkspaceBackend) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return (&fakeWS{}).Create(ctx, req)
}
func (f *conflictWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: id, ActiveRunID: "run-1"}, nil
}
func (f *conflictWorkspaceBackend) List(ctx context.Context) ([]backend.WorkspaceInfo, error) {
	return nil, nil
}
func (f *conflictWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	return &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: id, ActiveRunID: "run-1"},
		Runs: []backend.RunInfo{{
			ID:          "run-1",
			WorkspaceID: id,
			State:       f.parent.runState,
		}},
		Approvals: []backend.ApprovalInfo{{
			ID:          "approval-1",
			WorkspaceID: id,
			RunID:       "run-1",
			State:       string(common.ApprovalStateApproved),
		}},
	}, nil
}
func (f *conflictWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type conflictApprovalBackend struct{}

func (f *conflictApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...map[string]any) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, RunID: "run-1", State: "approved"}, nil
}

type conflictRunBackend struct{ parent *conflictApprovalBE }

func (f *conflictRunBackend) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	switch req.Intent {
	case "", backend.RunCommandIntentResume:
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID, req.ExecutionContextPatch)
	case backend.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, req.Activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *conflictRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: common.RunStateRunning}, nil
}
func (f *conflictRunBackend) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: common.RunStateRunning}, nil
}
func (f *conflictRunBackend) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, _ ...map[string]any) (*backend.RunInfo, error) {
	f.parent.resumed = true
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: common.RunStateRunning}, nil
}
func (f *conflictRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: f.parent.runState}, nil
}
func (f *conflictRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}
func (f *conflictRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, WorkspaceID: wsID, State: f.parent.runState}, nil
}

type fakeWS struct{}

func (f *fakeWS) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-new", Title: req.Title, State: "active"}, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: id}, nil
}
func (f *fakeWS) List(ctx context.Context) ([]backend.WorkspaceInfo, error) { return nil, nil }
func (f *fakeWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	return &backend.WorkspaceSnapshotInfo{Workspace: backend.WorkspaceInfo{ID: id}}, nil
}
func (f *fakeWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error { return nil }

type fakeG struct{}

func (f *fakeG) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: "g-1", Text: text}, nil
}
func (f *fakeG) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.GoalInfo, error) {
	return nil, nil
}
func (f *fakeG) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeG) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*backend.GoalInfo, error) {
	return f.Get(ctx, wsID, id)
}

type fakeD struct{}

func (f *fakeD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: "d-1", Items: items}, nil
}
func (f *fakeD) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DoDInfo, error) {
	return nil, nil
}
func (f *fakeD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*backend.DoDInfo, error) {
	return f.Get(ctx, wsID, id)
}

type recordingDoDBE struct {
	*fakeBE
	workspaceID common.WorkspaceID
	items       []string
}

func (b *recordingDoDBE) DoD() backend.DoDBackend {
	return &recordingDoD{parent: b}
}

type recordingDoD struct{ parent *recordingDoDBE }

func (f *recordingDoD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*backend.DoDInfo, error) {
	f.parent.workspaceID = wsID
	f.parent.items = append([]string(nil), items...)
	return &backend.DoDInfo{ID: "dod-recorded", WorkspaceID: wsID, Items: items, State: "ready"}, nil
}
func (f *recordingDoD) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DoDInfo, error) {
	if f.parent.workspaceID == wsID && len(f.parent.items) > 0 {
		return []backend.DoDInfo{{ID: "dod-recorded", WorkspaceID: wsID, Items: append([]string(nil), f.parent.items...), State: "ready"}}, nil
	}
	return nil, nil
}
func (f *recordingDoD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: id, WorkspaceID: wsID, Items: append([]string(nil), f.parent.items...), State: "ready"}, nil
}
func (f *recordingDoD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*backend.DoDInfo, error) {
	f.parent.items = append([]string(nil), items...)
	return &backend.DoDInfo{ID: id, WorkspaceID: wsID, Items: items, State: "ready"}, nil
}

type fakeR struct{}

func (f *fakeR) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	switch req.Intent {
	case "", backend.RunCommandIntentResume:
		if req.ExpectedRunID == "" {
			return f.Activate(ctx, req.WorkspaceID, req.Activation)
		}
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID, req.ExecutionContextPatch)
	case backend.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, req.Activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *fakeR) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", State: "running"}, nil
}
func (f *fakeR) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: "running"}, nil
}
func (f *fakeR) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, _ ...map[string]any) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeR) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeR) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}
func (f *fakeR) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}

type fakeDC struct{}

func (f *fakeDC) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *fakeDC) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p map[string]any) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDC) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDC) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: id, WorkspaceID: wsID, Question: "?", State: "open"}, nil
}

type fakeA struct {
	lastPayload map[string]any
}

func (f *fakeA) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "approved"}, nil
}
func (f *fakeA) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "rejected"}, nil
}
func (f *fakeA) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...map[string]any) ([]backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return nil, nil
}
func (f *fakeA) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeA) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, Reason: "reason", State: "pending"}, nil
}

func firstFakeApprovalPayload(payload ...map[string]any) map[string]any {
	for _, candidate := range payload {
		if len(candidate) > 0 {
			return candidate
		}
	}
	return nil
}

type fakeS struct{}

func (f *fakeS) Create(ctx context.Context, s *backend.SoulInfo) (*backend.SoulInfo, error) {
	return s, nil
}
func (f *fakeS) Get(ctx context.Context, id common.SoulID) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{ID: id}, nil
}
func (f *fakeS) List(ctx context.Context) ([]backend.SoulInfo, error) { return nil, nil }
func (f *fakeS) Update(ctx context.Context, id common.SoulID, s *backend.SoulInfo) (*backend.SoulInfo, error) {
	return s, nil
}
func (f *fakeS) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSK struct{}

func (f *fakeSK) Create(ctx context.Context, s *backend.SkillInfo) (*backend.SkillInfo, error) {
	return s, nil
}
func (f *fakeSK) Get(ctx context.Context, id common.SkillID) (*backend.SkillInfo, error) {
	return &backend.SkillInfo{ID: id}, nil
}
func (f *fakeSK) List(ctx context.Context) ([]backend.SkillInfo, error) { return nil, nil }

type fakeM struct{}

func (f *fakeM) CreateServer(ctx context.Context, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeM) GetServer(ctx context.Context, id common.MCPServerID) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{ID: id}, nil
}
func (f *fakeM) ListServers(ctx context.Context) ([]backend.MCPServerInfo, error) { return nil, nil }
func (f *fakeM) UpdateServer(ctx context.Context, id common.MCPServerID, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeM) ListCapabilities(ctx context.Context) ([]backend.MCPCapabilityInfo, error) {
	return nil, nil
}

type fakeMM struct{}

func (f *fakeMM) Create(ctx context.Context, wsID common.WorkspaceID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return m, nil
}
func (f *fakeMM) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeMM) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	m.ID = id
	m.WorkspaceID = wsID
	return m, nil
}
func (f *fakeMM) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return nil
}
func (f *fakeMM) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}
func (f *fakeMM) Search(ctx context.Context, wsID common.WorkspaceID, req backend.MemorySearchRequest) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}

type fakeT struct{}

func (f *fakeT) Create(ctx context.Context, t *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return t, nil
}
func (f *fakeT) Get(ctx context.Context, id string) (*backend.TemplateInfo, error) {
	return &backend.TemplateInfo{ID: id}, nil
}
func (f *fakeT) List(ctx context.Context) ([]backend.TemplateInfo, error) { return nil, nil }
func (f *fakeT) Update(ctx context.Context, id string, t *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return t, nil
}
func (f *fakeT) Delete(ctx context.Context, id string) error { return nil }

type fakeC struct{}

func (f *fakeC) Get(ctx context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{
		ServerAddr:        "127.0.0.1:8080",
		Backend:           "file",
		DataDir:           ".workorch-data",
		DefaultProvider:   "local",
		MaxConcurrentRuns: 1,
		LLMOutput: backend.LLMOutputConfig{
			MaxPatchContentBytes:               12288,
			MaxGraphBytes:                      32768,
			MaxGraphFileWrites:                 3,
			SchemaInvalidLargePreviewThreshold: 8192,
			MaxArtifactChunkBytes:              8192,
			PreviousChunkTailBytes:             300,
			MaxChunkRetryAttempts:              3,
			MaxArtifactPlanAttempts:            2,
			MaxArtifactReplanAttempts:          2,
		},
	}, nil
}

type fakeE struct{}

func (f *fakeE) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	return func() {}, nil
}
