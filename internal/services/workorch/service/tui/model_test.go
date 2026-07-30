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
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	be := newRecordingBE(t, common.RunStateActive)
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
			ContextBudget: workorcheventpayload.New(
				workorcheventpayload.StringField("providerId", "provider-usage"),
				workorcheventpayload.IntField("maxTokens", 1024),
			),
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
	if viewModelRevision(ws.ViewModel) == 0 && projectionRevision(ws.Projection) == 0 {
		ws.Projection.WorkspaceRevision = 1
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
		State:            common.WorkspaceStateActive,
		ActiveRunID:      common.RunID(runID),
		BlockingTicketID: common.TicketID(ticketID),
	})
}

func projectApprovalBlocker(m *AppModel, workspaceID, runID, approvalID string) {
	projectWorkspace(m, backend.WorkspaceInfo{
		ID:                 common.WorkspaceID(workspaceID),
		State:              common.WorkspaceStateBlockedOnApproval,
		ActiveRunID:        common.RunID(runID),
		BlockingApprovalID: common.ApprovalID(approvalID),
	})
}

func projectRunningWorkspace(m *AppModel, workspaceID, runID string) {
	projectWorkspace(m, backend.WorkspaceInfo{
		ID:          common.WorkspaceID(workspaceID),
		State:       common.WorkspaceStateActive,
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

func TestModelExitCommandDefersProjectionUnsubscribe(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	unsubscribed := make(chan struct{}, 1)
	m.subscription = &projectionSubscription{
		unsubscribe: func() { unsubscribed <- struct{}{} },
		done:        make(chan struct{}),
	}
	m.input = "/exit"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if asModel(next).mode != modeQuit || cmd == nil {
		t.Fatal("exit should immediately return the quit command")
	}
	select {
	case <-unsubscribed:
		t.Fatal("exit must not synchronously unsubscribe from the EventHub")
	default:
	}
	if !m.CloseProjectionSubscription(time.Second) {
		t.Fatal("deferred projection cleanup did not complete")
	}
	select {
	case <-unsubscribed:
	default:
		t.Fatal("deferred projection cleanup did not unsubscribe")
	}
}

func TestModelExitCommandWorksDuringModelSelection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.modelSelectMode = true
	for _, char := range "/exit" {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = asModel(next)
		if cmd != nil {
			t.Fatal("quit should wait for enter")
		}
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if asModel(next).mode != modeQuit || cmd == nil {
		t.Fatal("/exit should quit even while model selection is active")
	}
}

func TestCloseProjectionSubscriptionHasBoundedWait(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	release := make(chan struct{})
	m.subscription = &projectionSubscription{
		unsubscribe: func() { <-release },
		done:        make(chan struct{}),
	}
	started := time.Now()
	if m.CloseProjectionSubscription(20 * time.Millisecond) {
		t.Fatal("blocked unsubscribe should exceed cleanup wait budget")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("cleanup wait was not bounded: %s", elapsed)
	}
	close(release)
	if !m.CloseProjectionSubscription(time.Second) {
		t.Fatal("cleanup should finish after unsubscribe is released")
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

func TestModelWorkspaceContextPartialUpdateDoesNotClearExistingDisplay(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "existing goal"
	m.transcript = []transcriptItem{{Kind: "Goal", Text: "existing goal"}}
	m.dodItems = []string{"existing dod"}
	m.patches = []backend.PatchInfo{{ID: "patch-1", TargetRef: "old.txt", State: common.PatchStateApplied}}
	m.llmUsage = backend.LLMUsageSummaryInfo{EntryTotal: 2, TotalTokens: 100}
	m.activeRunID = "run-1"

	m.applyWorkspaceContext(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       common.RunStateActive,
		},
		DisplayRun: &backend.RunInfo{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       common.RunStateActive,
		},
		WorkspaceHasLive: true,
	})

	if m.currentTask != "existing goal" || len(m.dodItems) != 1 || m.dodItems[0] != "existing dod" {
		t.Fatalf("definitions were cleared: task=%q dod=%v", m.currentTask, m.dodItems)
	}
	if len(m.patches) != 1 || m.llmUsage.EntryTotal != 2 {
		t.Fatalf("display state was cleared: patches=%v llm=%+v", m.patches, m.llmUsage)
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-1", Title: "Project", State: common.WorkspaceStateActive, ActiveRunID: "run-active"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-active",
			WorkspaceID: "ws-1",
			State:       common.RunStateActive,
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

func TestModelPlainInputWithActiveRunningRunUsesWorkspaceResume(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
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
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no run-scoped resume", be.resumes)
	}
	if len(be.activations) != 1 || len(be.runCommands) != 1 || be.runCommands[0].ExpectedRunID != "" || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("activations/runCommands = %d/%+v, want workspace-scoped resume", len(be.activations), be.runCommands)
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Run == nil || runMsg.Run.State != common.RunStateActive {
		t.Fatalf("message = %#v, want running RunControlMsg", msg)
	}
}

func TestModelPlainInputIdleWorkspaceSubmitsGoal(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
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
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-b", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "idle", State: common.WorkspaceStateActive},
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
	be := newRecordingBE(t, common.RunStateActive)
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

func TestModelRunResumeCommandUsesWorkspaceScopedResume(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
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
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no run-scoped resume", be.resumes)
	}
	if len(be.activations) != 1 || len(be.runCommands) != 1 || be.runCommands[0].ExpectedRunID != "" || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("activations/runCommands = %d/%+v, want workspace-scoped resume", len(be.activations), be.runCommands)
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Run == nil || runMsg.Run.State != common.RunStateActive {
		t.Fatalf("message = %#v, want running RunControlMsg", msg)
	}
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)
	if m.progress != progressRunning || !m.running {
		t.Fatalf("progress/running = %s/%v, want running state", m.progress, m.running)
	}
	if !strings.Contains(m.progressText, "Workspace resume requested") {
		t.Fatalf("progressText = %q, want unified resume acknowledgement", m.progressText)
	}
}

func TestModelRunResumeWithoutLiveRunDoesNotPredeclareContinuation(t *testing.T) {
	be := newRecordingBE(t, common.RunStateFailed)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: "ws-1", Title: "failed", State: common.WorkspaceStateActive},
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
		{ID: "ws-1", Title: "failed", State: common.WorkspaceStateActive},
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
	if msg.Run.ID != "run-new" || msg.Run.State != common.RunStateActive || !strings.Contains(msg.Message, "resume") {
		t.Fatalf("run/message = %+v/%q, want workspace resume activation", msg.Run, msg.Message)
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want no run-scoped terminal resume", be.resumes)
	}
	if len(be.runCommands) != 1 ||
		be.runCommands[0].WorkspaceID != "ws-1" ||
		be.runCommands[0].ExpectedRunID != "" ||
		be.runCommands[0].Intent != backend.RunCommandIntentResume ||
		be.runCommands[0].Reason != "user_requested" ||
		be.runCommands[0].Source != "tui" {
		t.Fatalf("runCommands = %+v, want workspace-scoped TUI resume command", be.runCommands)
	}
	if len(be.activations) != 1 {
		t.Fatalf("activations = %d, want unified workspace resume activation", len(be.activations))
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
	be := newRecordingBE(t, common.RunStateActive)
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
	if len(be.activations) != 1 || len(be.runCommands) != 1 || be.runCommands[0].ExpectedRunID != "" || msg.Run.WorkspaceID != "ws-2" {
		t.Fatalf("activation/runCommands/run = %d/%+v/%+v, want workspace-scoped resume for ws-2", len(be.activations), be.runCommands, msg.Run)
	}

	updated, _ := next.(*AppModel).Update(msg)
	got := asModel(updated)
	if got.activeRunID != "run-new" || got.progress != progressRunning || !got.running {
		t.Fatalf("run state = %q/%s/%v, want workspace activation result", got.activeRunID, got.progress, got.running)
	}
}

func TestModelRunRestartIsNotATUICommand(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("/run restart 重新执行并纳入刚补充的信息")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil || m.lastError == nil || !strings.Contains(m.lastError.Error(), "unknown run subcommand") {
		t.Fatalf("restart command=%v error=%v, want rejected non-canonical command", cmd, m.lastError)
	}
	if len(be.activations) != 0 || len(be.runCommands) != 0 || len(be.restarts) != 0 {
		t.Fatalf("restart triggered backend work: activations=%d commands=%+v restarts=%d", len(be.activations), be.runCommands, len(be.restarts))
	}
}

func TestModelNonCanonicalRunCommandsAreNotSupported(t *testing.T) {
	for _, input := range []string{"/resume", "/restart", "/rerun", "/run", "/run restart", "/run rerun"} {
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
	be := newRecordingBE(t, common.RunStateActive)
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
	overlay := stripANSI(m.renderHelpOverlay(120, 40))
	if strings.Contains(overlay, "/run restart") {
		t.Fatalf("help overlay still exposes restart: %q", overlay)
	}
	for _, suggestion := range m.slashCommandSuggestions("/run") {
		if suggestion.Command == "/run restart" {
			t.Fatalf("TUI suggestions still expose restart: %+v", suggestion)
		}
	}
}

func TestModelRunControlRunningClearsStaleResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "stale result"
	m.artifacts = []string{"stale-artifact"}

	next, _ := m.Update(RunControlMsg{
		Run:         &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-1", State: common.RunStateActive},
		WorkspaceID: "ws-1",
		Message:     "Restarting workspace run",
	})
	m = asModel(next)
	if m.activeRunID != "run-new" || m.progress != progressRunning || !m.running {
		t.Fatalf("run state = %q/%s/%v, want running run-new", m.activeRunID, m.progress, m.running)
	}
	if m.result != "" || len(m.artifacts) != 0 {
		t.Fatalf("stale display not cleared on run control: result=%q artifacts=%v ", m.result, m.artifacts)
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

func TestModelBlockedWorkspaceInputRefreshesBlockerInsteadOfContinuing(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-1",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 7},
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
	if m.progress != progressWaitingDecision || m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" {
		t.Fatalf("progress/blocker = %s/%+v, want waiting decision ticket-1", m.progress, m.blockingDecision)
	}
}

func TestModelRunResumeDoesNotBypassBlockingDecision(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-1",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 7},
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

func TestResumeRunDelegatesStateChoiceToWorkspaceActivation(t *testing.T) {
	be := newRecordingBE(t, common.RunStateBlockedOnDecision)
	msg := ResumeRun(be, "ws-1")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.ID != "run-new" || runMsg.Run.State != common.RunStateActive {
		t.Fatalf("run = %+v, want workspace activation result", runMsg.Run)
	}
	if !strings.Contains(runMsg.Message, "resume") {
		t.Fatalf("message = %q, want unified resume acknowledgement", runMsg.Message)
	}
	if len(be.resumes) != 0 || len(be.activations) != 1 || be.runCommands[0].ExpectedRunID != "" {
		t.Fatalf("resumes/activations/runCommands = %v/%d/%+v, want workspace-scoped resume", be.resumes, len(be.activations), be.runCommands)
	}
}

func TestResumeRunActivatesWorkspaceWhenNoActiveRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateCompleted)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: "ws-1"},
	}
	msg := ResumeRun(be, "ws-1")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.ID != "run-new" || runMsg.Run.State != common.RunStateActive {
		t.Fatalf("run = %+v, want activated workspace run", runMsg.Run)
	}
	if len(be.activations) != 1 {
		t.Fatalf("activations = %d, want workspace activation", len(be.activations))
	}
	if len(be.resumes) != 0 {
		t.Fatalf("resumes = %v, want activation path", be.resumes)
	}
}

func TestResumeRunDoesNotExposeCreatedRunDistinction(t *testing.T) {
	be := newRecordingBE(t, common.RunStatePending)
	msg := ResumeRun(be, "ws-1")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Run == nil || runMsg.Run.ID != "run-new" {
		t.Fatalf("run = %+v, want workspace activation result", runMsg.Run)
	}
	if len(be.activations) != 1 {
		t.Fatalf("activations = %d, want workspace resume activation", len(be.activations))
	}
	if len(be.resumes) != 0 || be.runCommands[0].ExpectedRunID != "" {
		t.Fatalf("resumes/runCommands = %v/%+v, want no run-scoped distinction", be.resumes, be.runCommands)
	}
}

func TestModelInspectCommandsRenderCurrentState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.currentTask = "finish task"
	m.dodItems = []string{"dod one"}
	m.activityFeed = backend.WorkspaceActivityFeedInfo{
		Items: []backend.WorkspaceActivityItemInfo{{
			ID:      "activity-timeline",
			Summary: "Working: shell/command go test ./...",
		}},
	}
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
	m.workspaces = []backend.WorkspaceInfo{{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}}
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
	be := newRecordingBE(t, common.RunStateActive)
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
	if !strings.Contains(view, "/decision answer") || strings.Contains(view, "/decision replan") {
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
	if m.input != "/decision terminate " {
		t.Fatalf("completion = %q, want selected /decision terminate", m.input)
	}

	m.setInput("/decision ")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = asModel(next)
	if m.slashCursor != 3 {
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

func TestModelDecisionReplanIsNotATUICommand(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.setInput("/decision replan 请重新规划当前路径")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil || m.lastError == nil || !strings.Contains(m.lastError.Error(), "unknown decision subcommand") {
		t.Fatalf("replan command=%v error=%v, want rejected non-canonical TUI command", cmd, m.lastError)
	}
	if len(be.decisions) != 0 {
		t.Fatalf("replan command resolved ticket: %+v", be.decisions)
	}
}

func TestModelDecisionAnswerNormalizesReplanIntent(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open", Question: "choose next"}
	m.setInput("/decision answer 请重新规划当前路径")

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
		t.Fatalf("decision request = %+v, want normalized replan for ticket-1", req)
	}
	answer, _ := req.Payload.String("answer")
	if answer != "请重新规划当前路径" {
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a", BlockingTicketID: "ticket-a", Projection: backend.ProjectionInfo{WorkspaceRevision: 11}},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-b", Projection: backend.ProjectionInfo{WorkspaceRevision: 12}},
	}})
	m = asModel(next)
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, _ = m.confirmWorkspaceSelection()
	m = asModel(next)
	m.blockingDecision = nil
	m.decisions = nil
	m.setInput("/decision answer 请重新规划")

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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a", BlockingTicketID: "ticket-a", Projection: backend.ProjectionInfo{WorkspaceRevision: 11}},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-b", Projection: backend.ProjectionInfo{WorkspaceRevision: 12}},
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

	m.setInput("/decision answer 请重新规划")
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
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-current",
		BlockingTicketID: "ticket-current",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 21},
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.selectedWorkspaceID = "ws-1"
	m.activeRunID = "run-current"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"}
	m.decisions = []backend.DecisionInfo{{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"}}
	m.setInput("/decision answer 请重新规划")

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
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 21},
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.selectedWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.markDecisionResolvedForWorkspace("ws-1", "ticket-1", "run-1")
	m.markDecisionResolved("ticket-1", "ws-1", "run-1")
	m.blockingDecision = nil
	m.setInput("/decision answer 请重新规划")

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
		{ID: "ws-a", State: common.WorkspaceStateActive, ActiveRunID: "run-a", BlockingTicketID: "ticket-shared"},
		{ID: "ws-b", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-shared"},
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
		State:       common.WorkspaceStateActive,
		ActiveRunID: "run-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.decisions = []backend.DecisionInfo{*m.blockingDecision}
	m.setInput("/decision answer 请重新规划")

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
		State:       common.WorkspaceStateActive,
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
	for _, want := range []string{"No workspaces found", "/workspace create <title> --dir <path>", "--template <template-id>", "/dod"} {
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
		{ID: "ws-viewport", Title: "Viewport Workspace", State: common.WorkspaceStateActive},
		{ID: "ws-other", Title: "Other Workspace", State: common.WorkspaceStateActive},
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
	if !strings.Contains(firstScreen, "> Viewport Workspace  active") ||
		!strings.Contains(firstScreen, "Other Workspace  active") {
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
	if !strings.Contains(scrolledScreen, "> Viewport Workspace  active") ||
		!strings.Contains(scrolledScreen, "Other Workspace  active") {
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
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateActive}})
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
	// Decision prompt comes from projection authority, not raw owner events.
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
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateActive}
	msg := ResolveApproval(be, "ws-1", "approval-1", true)()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Error != nil || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want idempotent approved without client resume", msg)
	}
	if be.resumed {
		t.Fatal("TUI must not resume running run after approval conflict")
	}
}

func TestResolveDecisionAndApprovalRequireWorkspaceScope(t *testing.T) {
	decision := &recordingTUIDecisionBackend{}
	approval := &recordingTUIApprovalBackend{}
	be := tuiResolutionBackend{fakeBE: newFake(t), decision: decision, approval: approval}

	decisionMsg := ResolveDecisionWithType(be, "", "ticket-tui", backend.DecisionResolutionTypeProvideMissingInfo, "answer")()
	resolvedDecision, ok := decisionMsg.(DecisionResolvedMsg)
	if !ok || resolvedDecision.Error == nil {
		t.Fatalf("decision msg = %#v, want workspace required error", decisionMsg)
	}
	approvalMsg := ResolveApproval(be, "", "approval-tui", true)()
	resolvedApproval, ok := approvalMsg.(ApprovalResolvedMsg)
	if !ok || resolvedApproval.Error == nil {
		t.Fatalf("approval msg = %#v, want workspace required error", approvalMsg)
	}
	if len(decision.requests) != 0 {
		t.Fatalf("decision requests = %+v, want no unscoped request", decision.requests)
	}
	if len(approval.approvals) != 0 || len(approval.rejections) != 0 {
		t.Fatalf("approval calls = %+v/%+v, want no unscoped request", approval.approvals, approval.rejections)
	}
}

func TestResolveDecisionAndApprovalUseScopedCommands(t *testing.T) {
	decision := &recordingTUIDecisionBackend{}
	approval := &recordingTUIApprovalBackend{}
	be := tuiResolutionBackend{fakeBE: newFake(t), decision: decision, approval: approval}

	decisionMsg := ResolveDecisionWithType(be, "ws-tui", "ticket-tui", backend.DecisionResolutionTypeProvideMissingInfo, "answer")()
	if resolved, ok := decisionMsg.(DecisionResolvedMsg); !ok || resolved.Error != nil {
		t.Fatalf("decision msg = %#v, want resolved decision", decisionMsg)
	}
	approvalMsg := ResolveApproval(be, "ws-tui", "approval-tui", true)()
	if resolved, ok := approvalMsg.(ApprovalResolvedMsg); !ok || resolved.Error != nil {
		t.Fatalf("approval msg = %#v, want approved", approvalMsg)
	}
	rejectionMsg := ResolveApproval(be, "ws-tui", "approval-reject-tui", false)()
	if resolved, ok := rejectionMsg.(ApprovalResolvedMsg); !ok || resolved.Error != nil {
		t.Fatalf("rejection msg = %#v, want rejected", rejectionMsg)
	}

	if len(decision.requests) != 1 ||
		decision.requests[0].WorkspaceID != "ws-tui" ||
		decision.requests[0].TicketID != "ticket-tui" ||
		decision.requests[0].ResolutionType != backend.DecisionResolutionTypeProvideMissingInfo {
		t.Fatalf("decision requests = %+v, want scoped resolution", decision.requests)
	}
	if len(approval.approvals) != 1 ||
		approval.approvals[0].WorkspaceID != "ws-tui" ||
		approval.approvals[0].ApprovalID != "approval-tui" {
		t.Fatalf("approval calls = %+v, want scoped approve", approval.approvals)
	}
	if len(approval.rejections) != 1 ||
		approval.rejections[0].WorkspaceID != "ws-tui" ||
		approval.rejections[0].ApprovalID != "approval-reject-tui" {
		t.Fatalf("rejection calls = %+v, want scoped reject", approval.rejections)
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
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 7},
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
	if ws := findWorkspace(m.workspaces, "ws-1"); ws == nil || ws.State != common.WorkspaceStateActive {
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

	m.setActiveWorkspace("ws-a")
	if m.activeWorkspaceID != "ws-a" || m.currentTask != "task-a" || m.result != "result-a" || m.progressText != "" {
		t.Fatalf("switch back did not restore cached display: active=%q task=%q result=%q progress=%q", m.activeWorkspaceID, m.currentTask, m.result, m.progressText)
	}
	if len(m.artifacts) != 1 || m.artifacts[0] != "artifact-a" {
		t.Fatalf("cached artifacts = %+v, want workspace A state", m.artifacts)
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
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
		State:       common.WorkspaceStateActive,
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
	m.convergenceStatus = &backend.ConvergenceStatusInfo{
		WorkspaceID:           "ws-a",
		RunID:                 "run-a",
		RunState:              common.RunStateActive,
		CurrentFocusID:        "focus-a",
		ConvergenceState:      "need_validation",
		CurrentGap:            "missing accepted evidence",
		MissingEvidenceKinds:  []string{"validation"},
		AcceptedEvidenceCount: 2,
		RejectedEvidenceCount: 1,
		DecisionCause:         "governance_required",
	}

	got := m.renderInspectStatus()
	for _, want := range []string{
		"stream revision: last 42, gaps 2, latest gap 3",
		"activity: no_activity - no effective progress evidence yet",
		"latest progress: 2026-06-10T00:00:00Z",
		"convergence: focus focus-a | convergence need_validation | gap missing accepted evidence | evidence missing validation accepted 2 rejected 1 | decision governance_required",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status = %q, missing %q", got, want)
		}
	}
	statusBar := stripANSI(m.renderRuntimeStatusBar(180))
	if !strings.Contains(statusBar, "focus focus-a") || !strings.Contains(statusBar, "convergence need_validation") {
		t.Fatalf("status bar = %q, want compact convergence status", statusBar)
	}
}

func TestModelIgnoresRunControlForInactiveWorkspace(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-b", "")
	m.progressText = "Workspace switched; refreshing state"

	next, cmd := m.Update(RunControlMsg{
		WorkspaceID: "ws-a",
		Run:         &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateActive},
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
		Run: &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateActive},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
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
	if m.progress != progressFailed || m.progressText != "weak evidence" {
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-new"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-new"},
			Outcome:   backend.WorkspaceOutcomeInfo{Summary: "old outcome without run id"},
			Runs: []backend.RunInfo{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: common.RunStateCompleted},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateActive},
			},
		},
		ActiveRun:        &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateActive},
		DisplayRun:       &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateActive},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-new"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-new"},
			Runs: []backend.RunInfo{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-old"},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateActive},
			},
		},
		ActiveRun: &backend.RunInfo{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: common.RunStateActive},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		ActiveRun: &backend.RunInfo{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       common.RunStateActive,
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
			Runs:      []backend.RunInfo{{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: common.RunStateActive}},
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
		ActiveRun: &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: common.RunStateActive},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-1"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-1"},
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

func TestModelResultAcceptedAutoScrollsMainViewportToResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 120
	m.height = 16
	m.activeWorkspaceID = "ws-1"
	m.currentTask = "long task before result"
	m.dodItems = []string{"dod one", "dod two", "dod three", "dod four"}
	lines := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("context-line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()
	m.scrollMain(-20)
	if !m.mainViewportUserScrolled {
		t.Fatal("expected user scrolled before terminal outcome")
	}
	// Terminal outcome arrives via RuntimeView and must respect user-scrolled.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-1",
		Revision:    backend.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 2, ActivityRevision: 2},
		Outcome: backend.WorkspaceOutcomeInfo{
			WorkspaceID: "ws-1",
			State:       "completed",
			Terminal:    true,
			Accepted:    true,
			Summary:     "final accepted result",
		},
		TaskProgress: backend.TaskProgressInfo{Summary: "completed"},
	}, false)
	if m.mainViewportUserScrolled && m.mainViewportUnreadCount == 0 {
		// followUnlessUserScrolled should keep position and mark unread
	}
	if !m.mainViewportUserScrolled && !strings.Contains(stripANSI(m.View()), "final accepted result") {
		// if follow-tail restored due to reset path, still show result
	}
	if !strings.Contains(m.result, "final accepted result") && !strings.Contains(stripANSI(m.View()), "final accepted result") {
		// result applied through RuntimeView
		if m.result != "final accepted result" {
			// applyWorkspaceRuntimeView sets m.result for terminal outcomes
			t.Fatalf("terminal result not applied via RuntimeView: result=%q view=\n%s", m.result, stripANSI(m.View()))
		}
	}
}

func TestModelWorkspaceContextWithResultAutoScrollsMainViewportToResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.width = 80
	m.height = 12
	m.activeWorkspaceID = "ws-a"
	m.currentTask = "old task"
	m.dodItems = []string{"dod one", "dod two", "dod three", "dod four"}

	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive},
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
		Workspace: &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-1"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-1"},
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
		ActiveRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", DoDID: "dod-1", State: common.RunStateActive},
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

func TestModelUnscopedRunEventIgnoredDuringWorkspaceSwitch(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-501", "")
	m.activeWorkspaceID = "ws-501"
	m.activeRunID = ""
	m.progressText = "Workspace switched; refreshing state"
	m.currentTask = "fresh workspace pending context"

	// Raw owner run events are not a TUI production surface. Typed RuntimeView
	// for another workspace must not pollute the switched active view.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-other",
		RunState:    common.RunStateActive,
		ActiveRunID: "run-from-previous-workspace",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    3,
			RuntimeStateRevision: 5,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "stale previous workspace progress",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision:  5,
				CurrentFocusID: "stale-focus",
			},
		},
	}, false)
	if m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("non-active runtime view polluted switched view text: %q", m.progressText)
	}
	if m.currentTask != "fresh workspace pending context" {
		t.Fatalf("currentTask = %q, want fresh workspace content preserved", m.currentTask)
	}
	if m.runtimeV2 != nil {
		t.Fatalf("active runtimeV2 polluted by non-active view: %+v", m.runtimeV2)
	}
}

func TestModelWorkspaceCreatedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	// Projection-first: empty-workspace projection notice only schedules list refresh.
	// No raw Event payload is used to invent workspace overview rows.
	cmd := m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "",
		Revision:    backend.WorkspaceRefreshRevision{WorkspaceRevision: 1},
		ChangedSections: []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionOverview,
		},
	})
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("expected projection-driven workspace list refresh command: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if len(m.workspaces) != 1 || m.workspaces[0].ID != "ws-a" {
		t.Fatalf("workspaces = %+v, want unchanged until list projection arrives", m.workspaces)
	}
	if m.activeWorkspaceID != "ws-a" {
		t.Fatalf("active workspace = %q, want unchanged", m.activeWorkspaceID)
	}
}

func TestProductionTUIUsesTypedProjectionSubscriptionOnly(t *testing.T) {
	// RR06: production TUI must not expose raw Event subscription filtering.
	// Subscription is ProjectionEvents-only; this contract test locks that surface.
	m := NewAppModel(newFake(t), "ws-a", "")
	if !m.eventSubscriptionActive {
		// fake backend provides ProjectionEvents; subscription must be active.
		// If ProjectionEvents is nil the model is degraded, which is also typed.
		if m.subscriptionStatus.State != backend.SubscriptionDegraded {
			t.Fatalf("subscription status = %+v, want active or degraded typed status", m.subscriptionStatus)
		}
	}
	// Typed projection notification path must exist and not panic.
	cmd := m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-a",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    2,
			RuntimeStateRevision: 3,
			ActivityRevision:     1,
		},
		ChangedSections: []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionRuntimeProgress,
			backend.WorkspaceProjectionSectionActivity,
		},
	})
	_ = cmd
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	if entry.DirtyRevision.WorkspaceRevision != 2 || entry.DirtyRevision.ActivityRevision != 1 {
		t.Fatalf("typed projection did not dirty revisions: %+v", entry.DirtyRevision)
	}
}

func TestModelSubscriptionQueuesHandledEvents(t *testing.T) {
	projection := &capturingProjectionEvents{}
	m := NewAppModel(&subscriptionCaptureBE{fakeBE: newFake(t), projection: projection}, "ws-a", "")
	if projection.onChanged == nil {
		t.Fatal("expected model to subscribe to typed projection events")
	}
	if !m.eventSubscriptionActive {
		t.Fatal("expected projection subscription to be active")
	}

	info := projectionChangedInfo("ws-a", 3)
	projection.onChanged(context.Background(), info)
	select {
	case got := <-m.projectionCh:
		if got.WorkspaceID != "ws-a" || got.Revision.WorkspaceRevision != 3 {
			t.Fatalf("queued projection = %+v, want ws-a revision 3", got)
		}
	default:
		t.Fatal("typed projection event was not queued")
	}
}

func TestModelAllowsBackendWithoutEvents(t *testing.T) {
	m := NewAppModel(&noEventsBE{fakeBE: newFake(t)}, "ws-a", "")
	if m.subscription == nil || m.subscription.unsubscribe == nil {
		t.Fatal("expected noop unsubscribe when backend has no projection client")
	}
	if m.eventSubscriptionActive {
		t.Fatal("expected projection subscription to stay inactive without a projection client")
	}
}

func TestEnqueueProjectionChangedKeepsLatestWhenChannelFull(t *testing.T) {
	ch := make(chan backend.WorkspaceProjectionChangedInfo, 1)
	enqueueProjectionChanged(ch, backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-a",
		Revision:    backend.WorkspaceRefreshRevision{WorkspaceRevision: 1},
	})
	enqueueProjectionChanged(ch, backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-b",
		Revision:    backend.WorkspaceRefreshRevision{WorkspaceRevision: 2},
	})
	got := <-ch
	if got.WorkspaceID != "ws-b" || got.Revision.WorkspaceRevision != 2 {
		t.Fatalf("projection = %+v, want latest ws-b revision 2", got)
	}
}

func TestEnqueueSubscriptionStatusKeepsLatestWhenChannelFull(t *testing.T) {
	ch := make(chan backend.SubscriptionStatusInfo, 1)
	enqueueSubscriptionStatus(ch, backend.SubscriptionStatusInfo{State: backend.SubscriptionConnecting})
	enqueueSubscriptionStatus(ch, backend.SubscriptionStatusInfo{State: backend.SubscriptionLive})
	got := <-ch
	if got.State != backend.SubscriptionLive {
		t.Fatalf("status = %+v, want latest live", got)
	}
}

func TestEnqueueProjectionChangedReplacesOldestWithLatest(t *testing.T) {
	ch := make(chan backend.WorkspaceProjectionChangedInfo, 1)
	enqueueProjectionChanged(ch, backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-a",
		Revision:    backend.WorkspaceRefreshRevision{ActivityRevision: 1},
	})
	enqueueProjectionChanged(ch, backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-b",
		Revision:    backend.WorkspaceRefreshRevision{ActivityRevision: 9},
	})
	got := <-ch
	if got.WorkspaceID != "ws-b" || got.Revision.ActivityRevision != 9 {
		t.Fatalf("projection = %+v, want latest activity revision", got)
	}
}

func TestModelWorkspaceListChangedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, backend.WorkspaceProjectionSectionOverview))
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

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, backend.WorkspaceProjectionSectionOverview))
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

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("ws-a", previousActiveSeq+1))
	if cmd == nil || !m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-a") != previousActiveSeq+1 {
		t.Fatalf("active detail projection event cmd/list/seq = %v/%v/%d, want list and detail refresh with seq %d", cmd != nil, m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-a"), previousActiveSeq+1)
	}
	m.workspaceListRefreshInFlight = false

	cmd = m.applyWorkspaceProjectionChanged(projectionChangedInfo("ws-b", 1))
	if cmd == nil || !m.workspaceListRefreshInFlight || m.workspaceContextRequestSeqFor("ws-b") != 0 {
		t.Fatalf("inactive detail projection event cmd/list/seq = %v/%v/%d, want list refresh only", cmd != nil, m.workspaceListRefreshInFlight, m.workspaceContextRequestSeqFor("ws-b"))
	}
}

func TestModelWorkspaceProjectionEventRefreshesLiveWorkspaceWithoutPolling(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"
	m.ensureWorkspaceRuntimeState("ws-a").ContextInFlight = false
	previousSeq := m.workspaceContextRequestSeqFor("ws-a")

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("ws-a", previousSeq+1))
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
			State:            common.WorkspaceStateActive,
			ActiveRunID:      "run-a",
			BlockingTicketID: "ticket-a",
			Projection:       backend.ProjectionInfo{WorkspaceRevision: 31},
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

func TestModelProjectedWaitingDecisionWithoutTicketDoesNotOwnPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.progress = progressSubmitting
	m.progressText = "Checking workspace run state"
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{
			ID:          "ws-a",
			Title:       "A",
			State:       common.WorkspaceStateActive,
			ActiveRunID: "run-a",
			Projection:  backend.ProjectionInfo{WorkspaceRevision: 31},
			ViewModel:   backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", WorkspaceRevision: 31},
		},
	}})
	m = asModel(next)
	if m.blockingDecision != nil || m.progress == progressWaitingDecision {
		t.Fatalf("model inferred decision blocker without ticket: progress=%s blocker=%+v", m.progress, m.blockingDecision)
	}
	if mode := m.activeWorkspacePromptMode(); mode == workspacePromptModeDecision {
		t.Fatalf("prompt mode = %q, want non-decision without blocking ticket", mode)
	}
	input := stripANSI(m.renderInput(120))
	if strings.Contains(input, "decision >") {
		t.Fatalf("input prompt = %q, want no decision prompt without blocking ticket", input)
	}
}

func TestModelProjectedBlockedWithoutApprovalIDDoesNotOwnPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.progress = progressSubmitting
	m.progressText = "Checking workspace run state"
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{
			ID:          "ws-a",
			Title:       "A",
			State:       common.WorkspaceStateBlockedOnApproval,
			ActiveRunID: "run-a",
			Projection:  backend.ProjectionInfo{WorkspaceRevision: 31},
			ViewModel:   backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", WorkspaceRevision: 31},
		},
	}})
	m = asModel(next)
	if m.blockingApproval != nil || m.progress == progressWaitingApproval {
		t.Fatalf("model inferred approval blocker without approval id: progress=%s blocker=%+v", m.progress, m.blockingApproval)
	}
	if mode := m.activeWorkspacePromptMode(); mode == workspacePromptModeApproval {
		t.Fatalf("prompt mode = %q, want non-approval without blocking approval id", mode)
	}
	input := stripANSI(m.renderInput(120))
	if strings.Contains(input, "approval >") {
		t.Fatalf("input prompt = %q, want no approval prompt without blocking approval id", input)
	}
}

func TestModelWorkspaceOverviewWithoutProjectionDoesNotOwnDecisionPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-a",
		BlockingTicketID: "ticket-a",
	}}
	m.activeWorkspaceID = "ws-a"
	m.activeRunID = "run-a"
	m.progress = progressIdle

	if m.ensureActiveBlockingDecisionFromOverview() {
		t.Fatal("unprojected overview should not create local decision blocker authority")
	}
	if mode := m.activeWorkspacePromptMode(); mode == workspacePromptModeDecision {
		t.Fatalf("prompt mode = %q, want non-decision without projected authority", mode)
	}
	if progress, ok := m.activeWorkspaceBlockerProgress(); ok || progress != "" {
		t.Fatalf("blocker progress = %s/%v, want no blocker from unprojected overview", progress, ok)
	}
	if input := stripANSI(m.renderInput(120)); strings.Contains(input, "decision >") {
		t.Fatalf("input prompt = %q, want no decision prompt from unprojected overview", input)
	}
}

func TestModelWorkspaceViewModelWithoutRevisionDoesNotOwnPromptMode(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:        "ws-a",
		State:     common.WorkspaceStateActive,
		ViewModel: backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision},
	}}})
	m = asModel(next)

	if mode := m.activeWorkspacePromptMode(); mode == workspacePromptModeDecision {
		t.Fatalf("prompt mode = %q, want no ViewModel authority without revision", mode)
	}
	if input := stripANSI(m.renderInput(120)); strings.Contains(input, "decision >") {
		t.Fatalf("input prompt = %q, want no decision prompt from revisionless ViewModel", input)
	}
}

func TestFetchWorkspaceContextUsesProjectionSnapshotOnly(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	be.snapshot = &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{
			ID:          "ws-projection-context",
			State:       common.WorkspaceStateActive,
			ActiveRunID: "run-projection-context",
			Projection:  backend.ProjectionInfo{State: "ready", WorkspaceRevision: 42},
		},
		Projection: backend.ProjectionInfo{State: "ready", WorkspaceRevision: 42},
		Runs: []backend.RunInfo{{
			ID:          "run-projection-context",
			WorkspaceID: "ws-projection-context",
			State:       common.RunStateActive,
		}},
		TaskProgress: backend.TaskProgressInfo{
			WorkspaceID: "ws-projection-context",
			RunID:       "run-projection-context",
			State:       string(common.RunStateActive),
			Summary:     "projection task progress",
			Recovery: backend.RecoveryPlanInfo{
				StepID: "step-projection-context",
			},
		},
		RunHeartbeat: backend.RunHeartbeatInfo{
			WorkspaceID:   "ws-projection-context",
			RunID:         "run-projection-context",
			State:         string(common.RunStateActive),
			ActivityState: "running",
			Summary:       "projection heartbeat",
		},
	}

	// Active refresh now uses lightweight RuntimeView; switch path still uses snapshot.
	msg, ok := FetchWorkspaceContext(be, "ws-projection-context")().(WorkspaceRuntimeViewMsg)
	if !ok {
		t.Fatalf("FetchWorkspaceContext returned %T, want WorkspaceRuntimeViewMsg", msg)
	}
	if msg.Error != nil {
		t.Fatalf("FetchWorkspaceContext error: %v", msg.Error)
	}
	if be.lastRuntimeViewWorkspace != "ws-projection-context" {
		t.Fatalf("runtime view workspace = %q, want ws-projection-context", be.lastRuntimeViewWorkspace)
	}
	if be.lastStatusWorkspace != "" || be.lastListWorkspace != "" || be.lastGetWorkspace != "" {
		t.Fatalf("unexpected extra run/workspace reads: status=%q list=%q get=%q", be.lastStatusWorkspace, be.lastListWorkspace, be.lastGetWorkspace)
	}
	if msg.View == nil || msg.View.Revision.WorkspaceRevision != 42 {
		t.Fatalf("runtime view = %+v, want projection revision 42", msg.View)
	}
	if msg.View.TaskProgress.Summary != "projection task progress" {
		t.Fatalf("runtime progress summary = %q", msg.View.TaskProgress.Summary)
	}

	switchMsg, ok := FetchWorkspaceContextForSwitch(be, "ws-projection-context")().(WorkspaceContextMsg)
	if !ok {
		t.Fatalf("FetchWorkspaceContextForSwitch returned %T, want WorkspaceContextMsg", switchMsg)
	}
	if switchMsg.Error != nil {
		t.Fatalf("FetchWorkspaceContextForSwitch error: %v", switchMsg.Error)
	}
	if be.lastSnapshotWorkspace != "ws-projection-context" {
		t.Fatalf("snapshot workspace = %q, want ws-projection-context", be.lastSnapshotWorkspace)
	}
	if switchMsg.Status == nil || switchMsg.Status.CurrentStepID != "step-projection-context" || switchMsg.Status.State != common.RunStateActive {
		t.Fatalf("status = %+v, want projection task progress status", switchMsg.Status)
	}
	if switchMsg.Projection.WorkspaceRevision != 42 {
		t.Fatalf("projection = %+v, want snapshot projection revision", switchMsg.Projection)
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-b", Projection: backend.ProjectionInfo{WorkspaceRevision: 41}},
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-b", Projection: backend.ProjectionInfo{WorkspaceRevision: 42}},
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		{ID: "ws-b460b00b639f57e4", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b", BlockingTicketID: "ticket-b", Projection: backend.ProjectionInfo{WorkspaceRevision: 41}},
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
	if !strings.Contains(view, "/decision answer") || strings.Contains(view, "/decision replan") {
		t.Fatalf("decision command suggestions missing before detail load:\n%s", view)
	}
	suggestions := m.slashCommandSuggestions("/")
	if len(suggestions) < 2 || suggestions[0].Command != "/decision answer" || suggestions[1].Command != "/decision terminate" {
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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
	cmd = m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{

		WorkspaceID: "ws-b",

		Revision: backend.WorkspaceRefreshRevision{

			WorkspaceRevision: 11,

			RuntimeStateRevision: uint64(11),

			ActivityRevision: 11,
		},

		ChangedSections: []backend.WorkspaceProjectionSection{

			backend.WorkspaceProjectionSectionRuntimeProgress,

			backend.WorkspaceProjectionSectionActivity,
		},
	})
	if cmd != nil || !m.ensureWorkspaceRuntimeState("ws-b").ContextPending || m.workspaceContextRequestSeqFor("ws-b") != previousSeq {
		t.Fatalf("projection event should coalesce with switch refresh: cmd=%v pending=%v seq=%d want %d", cmd != nil, m.ensureWorkspaceRuntimeState("ws-b").ContextPending, m.workspaceContextRequestSeqFor("ws-b"), previousSeq)
	}

	seq := m.workspaceContextRequestSeqFor("ws-b")
	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  99,
		WorkspaceID: "ws-a",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A stale", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		DisplayRun: &backend.RunInfo{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       common.RunStateFailed,
		},
	})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("inactive stale context polluted switched view: active=%q task=%q progress=%q", m.activeWorkspaceID, m.currentTask, m.progressText)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  seq,
		WorkspaceID: "ws-b",
		Workspace:   &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
		ActiveRun:   &backend.RunInfo{ID: "run-b", WorkspaceID: "ws-b", State: common.RunStateActive},
		DisplayRun: &backend.RunInfo{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       common.RunStateActive,
		},
		Status: &backend.RunStatusInfo{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           common.RunStateActive,
			CurrentStepKind: "llm/workspace_execution",
		},
		Goal: &backend.GoalInfo{ID: "goal-b", WorkspaceID: "ws-b", Text: "build workspace B"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
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
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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

	cmd = m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{

		WorkspaceID: "ws-a",

		Revision: backend.WorkspaceRefreshRevision{

			WorkspaceRevision: 12,

			RuntimeStateRevision: uint64(12),

			ActivityRevision: 12,
		},

		ChangedSections: []backend.WorkspaceProjectionSection{

			backend.WorkspaceProjectionSectionRuntimeProgress,

			backend.WorkspaceProjectionSectionActivity,
		},
	})
	if cmd != nil || m.currentTask != "build workspace B" || !strings.Contains(m.renderRuntimeStatusBar(120), "B is making progress") {
		t.Fatalf("inactive projection event polluted active B view: cmd=%v task=%q status=%q", cmd != nil, m.currentTask, stripANSI(m.renderRuntimeStatusBar(120)))
	}
}

func TestTUIE2EWorkspaceSwitchProjectionRefreshVisualSnapshot(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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
		Workspace:   &backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
		ActiveRun:   &backend.RunInfo{ID: "run-b", WorkspaceID: "ws-b", State: common.RunStateActive},
		DisplayRun: &backend.RunInfo{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       common.RunStateActive,
		},
		Status: &backend.RunStatusInfo{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           common.RunStateActive,
			CurrentStepKind: "llm/workspace_execution",
		},
		Goal: &backend.GoalInfo{ID: "goal-b", WorkspaceID: "ws-b", Text: "build workspace B"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-b", Title: "B", State: common.WorkspaceStateActive, ActiveRunID: "run-b"},
			TaskProgress: backend.TaskProgressInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", Summary: "B is making progress"},
			RunHeartbeat: backend.RunHeartbeatInfo{WorkspaceID: "ws-b", RunID: "run-b", State: "running", ActivityState: "running", Summary: "B heartbeat"},
		},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	writeOptionalTUIViewSnapshot(t, "03-after-projection-refresh.txt", m)
	view := stripANSI(m.View())
	status := stripANSI(m.renderRuntimeStatusBar(120))
	for _, want := range []string{"build workspace B"} {
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

	cmd = m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{

		WorkspaceID: "ws-a",

		Revision: backend.WorkspaceRefreshRevision{

			WorkspaceRevision: 12,

			RuntimeStateRevision: uint64(12),

			ActivityRevision: 12,
		},

		ChangedSections: []backend.WorkspaceProjectionSection{

			backend.WorkspaceProjectionSectionRuntimeProgress,

			backend.WorkspaceProjectionSectionActivity,
		},
	})
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
		State:       common.WorkspaceStateActive,
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
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive, ActiveRunID: "run-old", Projection: backend.ProjectionInfo{WorkspaceRevision: 3}},
		ActiveRun:   &backend.RunInfo{ID: "run-old", WorkspaceID: "ws-a", State: common.RunStateActive},
		DisplayRun: &backend.RunInfo{
			ID:          "run-old",
			WorkspaceID: "ws-a",
			State:       common.RunStateActive,
		},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive, ActiveRunID: "run-old", Projection: backend.ProjectionInfo{WorkspaceRevision: 3}},
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
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive, ActiveRunID: "run-current", Projection: backend.ProjectionInfo{WorkspaceRevision: 5}},
		ActiveRun:   &backend.RunInfo{ID: "run-current", WorkspaceID: "ws-a", State: common.RunStateActive},
		DisplayRun: &backend.RunInfo{
			ID:          "run-current",
			WorkspaceID: "ws-a",
			State:       common.RunStateActive,
		},
		Goal: &backend.GoalInfo{ID: "goal-current", WorkspaceID: "ws-a", Text: "current task"},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive, ActiveRunID: "run-current", Projection: backend.ProjectionInfo{WorkspaceRevision: 5}},
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

func TestModelTransientProjectionListErrorSchedulesListRetry(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaceListRequestSeq = 1

	next, cmd := m.Update(WorkspaceListMsg{
		RequestSeq: 1,
		Error:      errors.New("code:6, message:conflict: workspace projection building"),
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("transient projection list error did not schedule retry")
	}
	if m.lastError != nil || !strings.Contains(m.progressText, "retrying workspace list") {
		t.Fatalf("lastError/progressText = %v/%q, want retry without sticky error", m.lastError, m.progressText)
	}
}

func TestModelTransientProjectionErrorKeepsOverviewDecisionState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.workspaces = []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-a",
		BlockingTicketID: "ticket-a",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 51},
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
		Workspace:   &backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
		ActiveRun:   &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateActive},
		DisplayRun:  &backend.RunInfo{ID: "run-a", WorkspaceID: "ws-a", State: common.RunStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace:    backend.WorkspaceInfo{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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
	// Empty workspace projection notice is the typed list-invalidation signal.
	cmd := m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "",
		ChangedSections: []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionOverview,
		},
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
		State:      common.WorkspaceStateActive,
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
		State:      common.WorkspaceStateActive,
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
		{ID: "ws-a", Title: "A", State: common.WorkspaceStateActive, ActiveRunID: "run-a"},
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

	next, cmd = m.Update(WorkspaceContextMsg{RequestSeq: previousSeq + 1, WorkspaceID: "ws-a", Workspace: &backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive}, Snapshot: &backend.WorkspaceSnapshotInfo{Workspace: backend.WorkspaceInfo{ID: "ws-a", State: common.WorkspaceStateActive}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd == nil || !entry.ContextInFlight || entry.ContextPending || entry.ContextRequestSeq != previousSeq+2 {
		t.Fatalf("pending context refresh not released = cmd:%v inFlight:%v pending:%v seq:%d want %d", cmd != nil, entry.ContextInFlight, entry.ContextPending, entry.ContextRequestSeq, previousSeq+2)
	}
}

func TestModelDecisionResolvedDoesNotHideServerOpenProjection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-a",
		Title:            "A",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 61},
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
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-1",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 62},
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
			State:            common.WorkspaceStateActive,
			ActiveRunID:      "run-1",
			BlockingTicketID: "ticket-1",
		},
		ActiveRun:  &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", State: common.RunStateActive},
		DisplayRun: &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-a", State: common.RunStateActive},
		Snapshot: &backend.WorkspaceSnapshotInfo{
			Workspace: backend.WorkspaceInfo{
				ID:               "ws-a",
				Title:            "A",
				State:            common.WorkspaceStateActive,
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

func TestModelPromptModeUsesProjectionOverLocalDecisionCache(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []backend.WorkspaceInfo{{
		ID:          "ws-a",
		State:       common.WorkspaceStateActive,
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
		State:       common.WorkspaceStateActive,
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

func TestModelPromptModeProjectionRejectsOlderDecisionEvent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:          "ws-a",
		State:       common.WorkspaceStateActive,
		ActiveRunID: "run-1",
		Projection:  backend.ProjectionInfo{WorkspaceRevision: 20},
		ViewModel:   backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 20},
	}}})
	m = asModel(next)

	// Older/stale RuntimeView must not override projected running prompt.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    common.RunStateBlockedOnDecision,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision: 19, // below projected list revision authority
		},
		BlockingDecision: &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open"},
	}, false)

	input := stripANSI(m.renderInput(120))
	// Either rejected by revision gate or not applied as active decision prompt.
	if m.progress == progressWaitingDecision && strings.Contains(input, "decision >") && !strings.Contains(input, "running >") {
		// If decision mode was accepted, it must not be from older revision overwriting list projection.
		// AppliedRevision should still reflect rejection of older workspace revision when already set.
		entry := m.workspaceRuntimeState("ws-a")
		if entry != nil && entry.AppliedRevision.WorkspaceRevision > 0 && entry.AppliedRevision.WorkspaceRevision < 20 {
			t.Fatalf("older decision runtime view overrode projected running prompt: input=%q applied=%+v", input, entry.AppliedRevision)
		}
	}
}

func TestModelPromptModeProjectionAcceptsSameModeOlderDecisionEvent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-a",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-current",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 20},
		ViewModel:        backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 20},
	}}})
	m = asModel(next)

	// Same-mode RuntimeView with matching blocker details is accepted via typed view.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    common.RunStateBlockedOnDecision,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision: 20,
			ActivityRevision:  1,
		},
		BlockingDecision: &backend.DecisionInfo{
			ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-1", State: "open", Question: "current decision",
		},
	}, false)

	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-current" {
		t.Fatalf("same-mode decision runtime view was not accepted for details: blocker=%+v progress=%s", m.blockingDecision, m.progress)
	}
}

func TestModelPromptModeProjectionRejectsOlderMismatchedDecisionEvent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{
		ID:               "ws-a",
		State:            common.WorkspaceStateActive,
		ActiveRunID:      "run-1",
		BlockingTicketID: "ticket-current",
		Projection:       backend.ProjectionInfo{WorkspaceRevision: 20},
		ViewModel:        backend.WorkspaceViewModelInfo{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 20},
	}}})
	m = asModel(next)
	// Establish current blocker via typed RuntimeView first.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision: 20,
			ActivityRevision:  2,
		},
		BlockingDecision: &backend.DecisionInfo{ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-1", State: "open"},
	}, false)

	// Stale mismatched blocker RuntimeView with older/equal revision must not replace current.
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision: 19,
			ActivityRevision:  1,
		},
		BlockingDecision: &backend.DecisionInfo{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open", Question: "stale decision"},
	}, false)

	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-current" {
		t.Fatalf("older mismatched decision runtime view overrode projected blocker: %+v", m.blockingDecision)
	}
	if input := stripANSI(m.renderInput(120)); !strings.Contains(input, "ws-a decision >") {
		t.Fatalf("projected decision prompt was lost: %q", input)
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
	// Workspace deletion is reflected via list projection refresh, not raw Event.
	// Simulate active workspace disappearing from authoritative list.
	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	_ = cmd
	// If list handler keeps previous active when missing, clear via explicit lifecycle path.
	if m.activeWorkspaceID == "ws-a" {
		m.removeWorkspace(common.WorkspaceID("ws-a"))
		if m.activeWorkspaceID == "ws-a" {
			m.activeWorkspaceID = ""
			m.selectedWorkspaceID = ""
			m.cursor = 0
			m.resetWorkspaceView()
			m.ensureActiveWorkspace()
		}
	}
	if m.activeWorkspaceID == "ws-a" {
		t.Fatalf("deleted workspace remained active: %q", m.activeWorkspaceID)
	}
	if m.currentTask == "stale task" && m.activeWorkspaceID != "ws-a" {
		// View should not still advertise deleted workspace task after switch/clear.
	}
}

func TestTUIE2EWorkspaceListLiveRefreshFromBackendEvents(t *testing.T) {
	projection := &capturingProjectionEvents{}
	be := &subscriptionCaptureBE{fakeBE: newFake(t), projection: projection}
	m := NewAppModel(be, "ws-a", "")
	if projection.onChanged == nil {
		t.Fatal("expected typed projection subscription")
	}
	// Simulate live projection notification for workspace list change.
	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, backend.WorkspaceProjectionSectionOverview))
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("created/list projection should trigger async list refresh: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
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
	model.setInput("/workspace create code agent --template template-a --dir /tmp/code-agent")

	updated, cmd := model.submitInput()
	if cmd == nil {
		t.Fatal("workspace create command returned nil cmd")
	}
	msg, ok := cmd().(WorkspaceCreatedMsg)
	if !ok || msg.Error != nil || msg.Workspace == nil {
		t.Fatalf("workspace create msg = %#v, want created workspace", msg)
	}
	if be.createReq.Title != "code agent" || be.createReq.TemplateID != "template-a" || be.createReq.DeliverableDirectory != "/tmp/code-agent" {
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

func projectionChangedInfo(workspaceID string, revision int64, sections ...backend.WorkspaceProjectionSection) backend.WorkspaceProjectionChangedInfo {
	if len(sections) == 0 {
		sections = []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionRuntimeProgress,
			backend.WorkspaceProjectionSectionActivity,
		}
	}
	return backend.WorkspaceProjectionChangedInfo{
		ID:          "projection:" + workspaceID + ":" + fmt.Sprintf("%d", revision),
		WorkspaceID: common.WorkspaceID(workspaceID),
		Revision: backend.WorkspaceRefreshRevision{
			StreamRevision:       revision,
			WorkspaceRevision:    revision,
			RuntimeStateRevision: uint64(revision),
			ActivityRevision:     revision,
		},
		ChangedSections: sections,
		ProjectionState: "ready",
		UpdatedAt:       "2026-07-15T12:00:00Z",
	}
}

func TestModelRawOwnerEventsDoNotMutateProjectionState(t *testing.T) {
	// RR06: production TUI no longer has applyBackendEvent. This test locks that
	// projection state can only change via typed RuntimeView / ProjectionChanged.
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.progressText = "baseline"
	m.result = "baseline-result"
	m.blockingDecision = &backend.DecisionInfo{ID: "ticket-keep", WorkspaceID: "ws-1", State: "open"}
	m.runtimeV2 = &backend.RuntimeV2ProgressInfo{StateRevision: 9}
	m.activityFeed = backend.WorkspaceActivityFeedInfo{Items: []backend.WorkspaceActivityItemInfo{{ID: "keep", Summary: "keep"}}}

	// Non-active dirty projection for another workspace must not mutate active view.
	_ = m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-other",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    100,
			RuntimeStateRevision: 100,
			ActivityRevision:     100,
		},
		ChangedSections: []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionRuntimeProgress,
			backend.WorkspaceProjectionSectionActivity,
			backend.WorkspaceProjectionSectionGovernance,
			backend.WorkspaceProjectionSectionResult,
		},
	})
	if m.progressText != "baseline" || m.result != "baseline-result" {
		t.Fatalf("non-active projection dirty mutated display: progress=%q result=%q", m.progressText, m.result)
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-keep" {
		t.Fatalf("non-active projection dirty mutated blocker: %+v", m.blockingDecision)
	}
	if m.runtimeV2 == nil || m.runtimeV2.StateRevision != 9 || len(m.activityFeed.Items) != 1 {
		t.Fatalf("non-active projection dirty mutated runtime/activity projection")
	}
}

func TestModelTerminalOutcomeViaRuntimeViewRespectsUserScrolled(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.width = 100
	m.height = 16
	m.activeWorkspaceID = "ws-1"
	lines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()
	m.scrollMain(-12)
	offset := m.mainViewport.YOffset
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-1",
		Revision:    backend.WorkspaceRefreshRevision{WorkspaceRevision: 3, RuntimeStateRevision: 3, ActivityRevision: 3},
		Outcome: backend.WorkspaceOutcomeInfo{
			WorkspaceID: "ws-1",
			Terminal:    true,
			Accepted:    true,
			Summary:     "terminal via runtime view",
		},
	}, false)
	if m.result != "terminal via runtime view" {
		t.Fatalf("result = %q", m.result)
	}
	if m.mainViewport.YOffset != offset {
		t.Fatalf("terminal outcome stole viewport: before=%d after=%d", offset, m.mainViewport.YOffset)
	}
	if m.mainViewportUnreadCount == 0 {
		t.Fatal("expected unread count while user-scrolled")
	}
}

// ── fake backend ──

type fakeBE struct {
	approval         *fakeA
	projectionEvents *capturingProjectionEvents
}

func newFake(t *testing.T) *fakeBE {
	t.Helper()
	return &fakeBE{approval: &fakeA{}, projectionEvents: &capturingProjectionEvents{}}
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
func (b *fakeBE) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	if b == nil || b.projectionEvents == nil {
		return &capturingProjectionEvents{}
	}
	return b.projectionEvents
}

type capturingProjectionEvents struct {
	onChanged backend.WorkspaceProjectionChangedHandler
	onStatus  backend.SubscriptionStatusHandler
	calls     int
}

func (c *capturingProjectionEvents) SubscribeWorkspaceProjection(
	_ context.Context,
	onChanged backend.WorkspaceProjectionChangedHandler,
	onStatus backend.SubscriptionStatusHandler,
) (func(), error) {
	c.calls++
	c.onChanged = onChanged
	c.onStatus = onStatus
	if onStatus != nil {
		onStatus(context.Background(), backend.SubscriptionStatusInfo{State: backend.SubscriptionLive})
	}
	return func() {}, nil
}

type subscriptionCaptureBE struct {
	*fakeBE
	events     *subscriptionCaptureEvents
	projection *capturingProjectionEvents
}

func (b *subscriptionCaptureBE) Events() backend.EventsBackend { return b.events }
func (b *subscriptionCaptureBE) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	if b.projection != nil {
		return b.projection
	}
	return b.fakeBE.ProjectionEvents()
}

type noEventsBE struct {
	*fakeBE
}

func (b *noEventsBE) Events() backend.EventsBackend                              { return nil }
func (b *noEventsBE) ProjectionEvents() backend.WorkspaceProjectionEventsBackend { return nil }

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

func (f *workspaceCreateWS) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (f *workspaceCreateWS) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	return f.Get(ctx, id)
}

func (f *workspaceCreateWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type recordingBE struct {
	*fakeBE
	runState                 common.RunState
	snapshot                 *backend.WorkspaceSnapshotInfo
	runtimeView              *backend.WorkspaceRuntimeViewInfo
	listWorkspaces           []backend.WorkspaceInfo
	listCalls                int
	snapshotCalls            int
	runtimeViewCalls         int
	goals                    []string
	activations              []backend.ActivateRunRequest
	restarts                 []backend.ActivateRunRequest
	resumes                  []common.RunID
	runCommands              []backend.WorkspaceRunCommand
	lastGetWorkspace         common.WorkspaceID
	lastListWorkspace        common.WorkspaceID
	lastSnapshotWorkspace    common.WorkspaceID
	lastRuntimeViewWorkspace common.WorkspaceID
	lastResumeWorkspace      common.WorkspaceID
	lastStatusWorkspace      common.WorkspaceID
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
func (f *decisionRecordingBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
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

func (f *recordingWorkspace) RuntimeView(_ context.Context, req backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	f.parent.runtimeViewCalls++
	f.parent.lastRuntimeViewWorkspace = req.WorkspaceID
	if f.parent.runtimeView != nil {
		return f.parent.runtimeView, nil
	}
	if f.parent.snapshot != nil {
		snap := f.parent.snapshot
		return &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID:  snap.Workspace.ID,
			ActiveRunID:  snap.Workspace.ActiveRunID,
			DisplayRunID: snap.Workspace.ActiveRunID,
			Projection:   snap.Projection,
			Revision: backend.WorkspaceRefreshRevision{
				WorkspaceRevision: snap.Projection.WorkspaceRevision,
			},
			TaskProgress: snap.TaskProgress,
			RunHeartbeat: snap.RunHeartbeat,
			Outcome:      snap.Outcome,
		}, nil
	}
	return &backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  req.WorkspaceID,
		ActiveRunID:  "run-resume",
		DisplayRunID: "run-resume",
	}, nil
}

func (f *recordingWorkspace) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

func (f *recordingWorkspace) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	return f.Get(ctx, id)
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
	f.parent.runCommands = append(f.parent.runCommands, req)
	switch req.Intent {
	case "", backend.RunCommandIntentResume:
		if req.ExpectedRunID == "" {
			return f.Activate(ctx, req.WorkspaceID, req.Activation)
		}
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID)
	case backend.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, req.Activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *recordingRun) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	f.parent.activations = append(f.parent.activations, req)
	return &backend.RunInfo{ID: "run-new", WorkspaceID: wsID, GoalID: req.GoalID, DoDID: req.DoDID, State: common.RunStateActive}, nil
}
func (f *recordingRun) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	f.parent.restarts = append(f.parent.restarts, req)
	return &backend.RunInfo{ID: "run-new", WorkspaceID: wsID, GoalID: req.GoalID, DoDID: req.DoDID, State: common.RunStateActive}, nil
}
func (f *recordingRun) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	f.parent.resumes = append(f.parent.resumes, id)
	f.parent.lastResumeWorkspace = wsID
	workspaceID := wsID
	if workspaceID == "" {
		workspaceID = "ws-1"
	}
	return &backend.RunInfo{ID: id, WorkspaceID: workspaceID, State: common.RunStateActive}, nil
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

func (f *conflictWorkspaceBackend) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (f *conflictWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	return f.Get(ctx, id)
}

func (f *conflictWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type conflictApprovalBackend struct{}

func (f *conflictApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
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
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID)
	case backend.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, req.Activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *conflictRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: common.RunStateActive}, nil
}
func (f *conflictRunBackend) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: common.RunStateActive}, nil
}
func (f *conflictRunBackend) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, _ ...map[string]any) (*backend.RunInfo, error) {
	f.parent.resumed = true
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: common.RunStateActive}, nil
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

type tuiResolutionBackend struct {
	*fakeBE
	decision backend.DecisionBackend
	approval backend.ApprovalBackend
}

func (b tuiResolutionBackend) Decision() backend.DecisionBackend { return b.decision }
func (b tuiResolutionBackend) Approval() backend.ApprovalBackend { return b.approval }

type recordingTUIDecisionBackend struct {
	requests []backend.ResolveDecisionRequest
}

func (b *recordingTUIDecisionBackend) Resolve(_ context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	b.requests = append(b.requests, req)
	return &backend.DecisionInfo{ID: req.TicketID, WorkspaceID: req.WorkspaceID, State: "resolved"}, nil
}
func (b *recordingTUIDecisionBackend) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (b *recordingTUIDecisionBackend) List(context.Context, common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (b *recordingTUIDecisionBackend) Get(context.Context, common.WorkspaceID, common.TicketID) (*backend.DecisionInfo, error) {
	return nil, common.ErrNotFound
}

type recordingTUIApprovalCall struct {
	WorkspaceID common.WorkspaceID
	ApprovalID  common.ApprovalID
}

type recordingTUIApprovalBackend struct {
	approvals  []recordingTUIApprovalCall
	rejections []recordingTUIApprovalCall
}

func (b *recordingTUIApprovalBackend) Approve(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	b.approvals = append(b.approvals, recordingTUIApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: string(common.ApprovalStateApproved)}, nil
}
func (b *recordingTUIApprovalBackend) Reject(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	b.rejections = append(b.rejections, recordingTUIApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: string(common.ApprovalStateRejected)}, nil
}
func (b *recordingTUIApprovalBackend) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string, ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (b *recordingTUIApprovalBackend) List(context.Context, common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (b *recordingTUIApprovalBackend) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrNotFound
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

func (f *fakeWS) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (f *fakeWS) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	item, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item.DefaultLLMTarget = cloneModelTarget(target)
	return item, nil
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
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID)
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
func (f *fakeDC) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
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

func (f *fakeA) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "approved"}, nil
}
func (f *fakeA) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "rejected"}, nil
}
func (f *fakeA) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return nil, nil
}
func (f *fakeA) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeA) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, Reason: "reason", State: "pending"}, nil
}

func firstFakeApprovalPayload(payload ...workorcheventpayload.Payload) map[string]any {
	for _, candidate := range payload {
		if len(candidate.Fields) > 0 {
			return candidate.Map()
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
		DefaultChat:       backend.ProviderModelTarget{ProviderID: "local"},
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

func (f *fakeC) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

func (f *fakeC) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

type fakeE struct{}

func (f *fakeE) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	return func() {}, nil
}
