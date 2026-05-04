package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
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

func TestModelInit(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-default", "")
	if m.mode != modeAgentSession {
		t.Fatal("expected agent session mode")
	}
}

func TestModelQuit(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if asModel(next).mode != modeQuit {
		t.Fatal("expected quit mode")
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
	if !strings.Contains(view, "> workorch") {
		t.Fatalf("workspace panel missing selected workspace name:\n%s", view)
	}
	if !strings.Contains(view, "ws-f2e4da0  completed") {
		t.Fatalf("workspace panel missing complete id/state line:\n%s", view)
	}
	if strings.Contains(view, "c...") {
		t.Fatalf("workspace panel still truncates state:\n%s", view)
	}
}

func TestModelSubmitGoalStartsRun(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{{ID: "ws-1", Title: "Project", State: "active"}}})
	m = asModel(next)
	m.input = "finish task"
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

func TestModelRunStartedDoesNotOverwriteStepProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventStepDispatched,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"kind": "llm/workspace_execution"},
	})
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	if m.progressText != "Working: llm/workspace_execution" {
		t.Fatalf("progress text = %q, want step progress preserved", m.progressText)
	}
}

func TestModelRunStartedDoesNotOverwriteDecisionProgress(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.applyBackendEvent(backend.Event{
		Type:        backend.EventDecisionCreated,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"ticketId": "ticket-1", "reason": "缺少城市"},
	})
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	if m.progress != progressWaitingDecision || m.progressText != "缺少城市" {
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
		{ID: "ws-1", Title: "Project", WorkingDirectory: "/tmp/project", State: "active"},
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
	view := stripANSI(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("view too short:\n%s", view)
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
	if view := stripANSI(m.View()); !strings.Contains(view, "WorkOrch Help") || !strings.Contains(view, "Ctrl+P / Ctrl+N") {
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

func TestModelCtrlCRequiresSecondPressToQuitWhenIdle(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = asModel(next)
	if cmd != nil || m.mode == modeQuit {
		t.Fatalf("first ctrl+c should not quit, cmd=%v mode=%v", cmd != nil, m.mode)
	}
	if !strings.Contains(m.progressText, "Press Ctrl+C again") {
		t.Fatalf("first ctrl+c hint = %q", m.progressText)
	}
	m.lastCtrlC = time.Now()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = asModel(next)
	if cmd == nil || m.mode != modeQuit {
		t.Fatalf("second ctrl+c should quit, cmd=%v mode=%v", cmd != nil, m.mode)
	}
}

func TestModelEmptyWorkspaceGuidance(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	m.width = 90
	m.height = 18
	view := stripANSI(m.View())
	for _, want := range []string{"No workspaces found", "/workspace create", "--template software-dev", "/dod"} {
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

func TestModelResultMarkdownRendering(t *testing.T) {
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

func TestModelRunningEmptyResultText(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	next, _ := m.Update(RunStartedMsg{Run: &backend.RunInfo{ID: "run-1", State: common.RunStateRunning}})
	m = asModel(next)
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "waiting for progress or final result") {
		t.Fatalf("running progress text missing:\n%s", view)
	}
	if !strings.Contains(view, "Long tasks may stay quiet while backend works") {
		t.Fatalf("running empty result text missing:\n%s", view)
	}
}

func TestModelSubmittedInputRendersAsTaskContext(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "查询当前天气情况"
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Task: 查询当前天气情况") {
		t.Fatalf("view missing submitted input context:\n%s", view)
	}
}

func TestModelGoalRendersBeforeTask(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.currentTask = "张雪机车昨天夺冠了吗"
	m.appendTranscript("Goal", "张雪机车昨天夺冠了吗")
	m.appendTranscript("Decision", "需要我提供哪些信息")
	m.width = 100
	m.height = 20
	view := stripANSI(m.View())
	goalIndex := strings.Index(view, "Goal: 张雪机车昨天夺冠了吗")
	taskIndex := strings.Index(view, "Task: 张雪机车昨天夺冠了吗")
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
		Payload:     map[string]any{"ticketId": "ticket-1", "question": "缺少天气数据源"},
	})
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
			"reason":           "缺少城市",
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
	next, _ := m.Update(DecisionListMsg{Decisions: []backend.DecisionInfo{{
		ID:          "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Question:    "请问要查询哪个城市？",
		State:       "open",
	}}})
	m = asModel(next)
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" {
		t.Fatalf("blocking decision = %+v, want ticket-1", m.blockingDecision)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "请问要查询哪个城市？") {
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
	next, _ = m.Update(DecisionDetailMsg{Decision: &backend.DecisionInfo{
		ID:       "ticket-1",
		RunID:    "run-1",
		Question: "请问要查询哪个城市？",
		State:    "open",
	}})
	m = asModel(next)
	if m.progressText != "请问要查询哪个城市？" {
		t.Fatalf("progressText = %q, want decision question", m.progressText)
	}
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "请问要查询哪个城市？") || !strings.Contains(view, "ws-1 decision >") {
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
	m.width = 80
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "Approval required") || !strings.Contains(view, "ws-1 approval >") {
		t.Fatalf("view missing approval input context:\n%s", view)
	}
}

func TestApprovalInputUsesSemanticIntent(t *testing.T) {
	positive := []string{
		"同意继续调用天气接口",
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

	if approve, ok := parseApprovalInput("天气看起来还不错"); ok {
		t.Fatalf("parseApprovalInput ambiguous = %v/%v, want no decision", approve, ok)
	}

	reason := "在调用天气查询 MCP 工具前需要确认目标城市和参数"
	for _, input := range []string{"查询江苏南京天气", "江苏南京"} {
		approve, ok := parseApprovalInputWithContext(input, reason)
		if !ok || !approve {
			t.Fatalf("parseApprovalInputWithContext(%q) = %v/%v, want contextual approve", input, approve, ok)
		}
	}
	if approve, ok := parseApprovalInputWithContext("不要查询南京天气", reason); !ok || approve {
		t.Fatalf("contextual negative = %v/%v, want reject", approve, ok)
	}
}

func TestModelApprovalSemanticApproveSubmitsApprove(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.input = "同意继续调用天气接口"
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
	if len(m.transcript) != 1 || m.transcript[0].Text != "同意继续调用天气接口" {
		t.Fatalf("transcript = %+v, want original approval text", m.transcript)
	}
}

func TestModelApprovalAmbiguousInputDoesNotReject(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.blockingApproval = &backend.ApprovalInfo{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.input = "天气接口"
	m.inputCursor = len([]rune(m.input))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd != nil {
		t.Fatal("ambiguous approval input should not submit")
	}
	if m.blockingApproval == nil || m.input != "天气接口" {
		t.Fatalf("approval/input = %+v/%q, want preserved", m.blockingApproval, m.input)
	}
	if !strings.Contains(m.progressText, "intent unclear") {
		t.Fatalf("progressText = %q, want unclear intent hint", m.progressText)
	}
}

func TestModelApprovalContextualCityInputApproves(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.blockingApproval = &backend.ApprovalInfo{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "在调用天气查询 MCP 工具前需要确认目标城市和参数",
		State:       "pending",
	}
	m.input = "查询江苏南京天气"
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
	if len(m.transcript) != 1 || m.transcript[0].Text != "查询江苏南京天气" {
		t.Fatalf("transcript = %+v, want contextual approval text", m.transcript)
	}
}

func TestResolveApprovalIsIdempotentWhenAlreadyApproved(t *testing.T) {
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateBlockedOnApproval}
	msg := ResolveApproval(be, "approval-1", true)()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Error != nil || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want approved without error", msg)
	}
	if !be.resumed {
		t.Fatal("expected blocked run to resume after already-approved conflict")
	}
}

func TestResolveApprovalSkipsResumeWhenRunAlreadyRunning(t *testing.T) {
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateRunning}
	msg := ResolveApproval(be, "approval-1", true)()
	resolved, ok := msg.(ApprovalResolvedMsg)
	if !ok || resolved.Error != nil || resolved.Approval == nil || resolved.Approval.State != "approved" {
		t.Fatalf("approval msg = %#v, want approved without error", msg)
	}
	if be.resumed {
		t.Fatal("running run should not be resumed again")
	}
}

func TestModelConversationRendersMultipleSubmissions(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.appendTranscript("Goal", "查询当前天气")
	m.appendTranscript("Decision", "江苏南京")
	m.appendTranscript("Approval", "同意继续调用天气接口")
	m.width = 100
	m.height = 20
	view := stripANSI(m.View())
	for _, want := range []string{
		"Conversation",
		"Goal: 查询当前天气",
		"Decision: 江苏南京",
		"Approval: 同意继续调用天气接口",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
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
	m.width = 100
	m.height = 18
	view := stripANSI(m.View())
	if strings.Contains(view, "old task") || strings.Contains(view, "old result") || !strings.Contains(view, "fresh result") || !strings.Contains(view, "fresh goal") {
		t.Fatalf("view not fully refreshed:\n%s", view)
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
			Runs:      []backend.RunInfo{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: common.RunStateCompleted}},
			Results:   []backend.ResultInfo{{ID: "result-1", RunID: "run-1", Summary: "目标已完成，成功查询到远程服务器配置信息。"}},
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
	m.input = "run active workspace"
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
			"title":            "B",
			"workingDirectory": "/tmp/b",
			"state":            "active",
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
	if be.createReq.Title != "code agent" || be.createReq.TemplateID != "software-dev" || be.createReq.WorkingDirectory != "/tmp/code-agent" {
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

// ── fake backend ──

type fakeBE struct{}

func newFake(t *testing.T) *fakeBE                    { t.Helper(); return &fakeBE{} }
func (b *fakeBE) Workspace() backend.WorkspaceBackend { return &fakeWS{} }
func (b *fakeBE) Goal() backend.GoalBackend           { return &fakeG{} }
func (b *fakeBE) DoD() backend.DoDBackend             { return &fakeD{} }
func (b *fakeBE) Run() backend.RunBackend             { return &fakeR{} }
func (b *fakeBE) Decision() backend.DecisionBackend   { return &fakeDC{} }
func (b *fakeBE) Approval() backend.ApprovalBackend   { return &fakeA{} }
func (b *fakeBE) Soul() backend.SoulBackend           { return &fakeS{} }
func (b *fakeBE) Skill() backend.SkillBackend         { return &fakeSK{} }
func (b *fakeBE) MCP() backend.MCPBackend             { return &fakeM{} }
func (b *fakeBE) Memory() backend.MemoryBackend       { return &fakeMM{} }
func (b *fakeBE) Template() backend.TemplateBackend   { return &fakeT{} }
func (b *fakeBE) Config() backend.ConfigBackend       { return &fakeC{} }
func (b *fakeBE) Events() backend.EventsBackend       { return &fakeE{} }

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
		ID:               "ws-created",
		Title:            req.Title,
		Description:      req.Description,
		WorkingDirectory: req.WorkingDirectory,
		State:            common.WorkspaceStateActive,
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
func (f *workspaceCreateWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type conflictApprovalBE struct {
	*fakeBE
	runState common.RunState
	resumed  bool
}

func (b *conflictApprovalBE) Approval() backend.ApprovalBackend { return &conflictApprovalBackend{} }
func (b *conflictApprovalBE) Run() backend.RunBackend           { return &conflictRunBackend{parent: b} }

type conflictApprovalBackend struct{}

func (f *conflictApprovalBackend) Approve(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) Reject(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) Get(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, WorkspaceID: "ws-1", RunID: "run-1", State: "approved"}, nil
}

type conflictRunBackend struct{ parent *conflictApprovalBE }

func (f *conflictRunBackend) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: wsID, State: common.RunStateRunning}, nil
}
func (f *conflictRunBackend) Resume(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	f.parent.resumed = true
	return &backend.RunInfo{ID: id, State: common.RunStateRunning}, nil
}
func (f *conflictRunBackend) Get(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, State: f.parent.runState}, nil
}
func (f *conflictRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}
func (f *conflictRunBackend) Status(ctx context.Context, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, State: f.parent.runState}, nil
}

type fakeWS struct{}

func (f *fakeWS) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-new", Title: req.Title, State: "active"}, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: id}, nil
}
func (f *fakeWS) List(ctx context.Context) ([]backend.WorkspaceInfo, error)           { return nil, nil }
func (f *fakeWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error { return nil }

type fakeG struct{}

func (f *fakeG) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: "g-1", Text: text}, nil
}
func (f *fakeG) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.GoalInfo, error) {
	return nil, nil
}
func (f *fakeG) Get(ctx context.Context, id common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id}, nil
}
func (f *fakeG) Update(ctx context.Context, id common.GoalID, text string) (*backend.GoalInfo, error) {
	return f.Get(ctx, id)
}

type fakeD struct{}

func (f *fakeD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: "d-1", Items: items}, nil
}
func (f *fakeD) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DoDInfo, error) {
	return nil, nil
}
func (f *fakeD) Get(ctx context.Context, id common.DoDID) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: id}, nil
}
func (f *fakeD) Update(ctx context.Context, id common.DoDID, items []string) (*backend.DoDInfo, error) {
	return f.Get(ctx, id)
}

type fakeR struct{}

func (f *fakeR) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", State: "running"}, nil
}
func (f *fakeR) Resume(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id}, nil
}
func (f *fakeR) Get(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id}, nil
}
func (f *fakeR) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}
func (f *fakeR) Status(ctx context.Context, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, State: "running"}, nil
}

type fakeDC struct{}

func (f *fakeDC) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *fakeDC) BatchResolve(ctx context.Context, ids []common.TicketID, rt string, p map[string]any) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDC) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDC) Get(ctx context.Context, id common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: id, Question: "?", State: "open"}, nil
}

type fakeA struct{}

func (f *fakeA) Approve(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, State: "approved"}, nil
}
func (f *fakeA) Reject(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, State: "rejected"}, nil
}
func (f *fakeA) BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeA) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeA) Get(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, Reason: "reason", State: "pending"}, nil
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
func (f *fakeMM) Get(ctx context.Context, id common.MemoryID) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{ID: id}, nil
}
func (f *fakeMM) Update(ctx context.Context, id common.MemoryID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return m, nil
}
func (f *fakeMM) Delete(ctx context.Context, id common.MemoryID) error {
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
	return &backend.ServiceConfig{}, nil
}

type fakeE struct{}

func (f *fakeE) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	return func() {}, nil
}
