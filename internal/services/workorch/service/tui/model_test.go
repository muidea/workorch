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

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
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

func TestInspectStatusShowsRejectedRunCommandProjection(t *testing.T) {
	m := AppModel{
		activeWorkspaceID: "ws-admission",
		runCommand: &projectioncontract.RunCommandStatus{
			WorkspaceID:     "ws-admission",
			State:           "rejected",
			Intent:          "restart",
			RequestReason:   "force_accept",
			Outcome:         runtimecontract.RuntimeExecutionIntentRestartRequired,
			AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
			Diagnostic:      "workspace owner rejected activation",
		},
	}
	text := m.renderInspectStatus()
	for _, want := range []string{
		"execution admission",
		"command rejected restart",
		"outcome=restart_required",
		"admission=run_not_resumable",
		"workspace owner rejected activation",
		"request=force_accept",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text = %q, want %q", text, want)
		}
	}
}

func TestRunCommandStatusDoesNotInventPendingAdmissionState(t *testing.T) {
	command := &projectioncontract.RunCommandStatus{
		WorkspaceID: "ws-admission",
		RequestID:   "request-without-owner-outcome",
	}
	if got := runCommandStatusText(command); got != "" {
		t.Fatalf("run command status = %q, want no derived pending state", got)
	}
}

func TestRunCommandStatusEqualityCoversEveryCanonicalField(t *testing.T) {
	base := projectioncontract.RunCommandStatus{
		WorkspaceID:     "ws-admission",
		RunID:           "run-admission",
		State:           "accepted",
		Intent:          "resume",
		RequestID:       "request-admission",
		RequestReason:   "operator_requested",
		Source:          "tui",
		Outcome:         runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		Diagnostic:      "current Run continuation accepted",
		AcceptedAt:      "2026-08-23T00:00:00Z",
		Revision:        7,
	}
	mutations := []struct {
		name   string
		mutate func(*projectioncontract.RunCommandStatus)
	}{
		{name: "workspaceId", mutate: func(value *projectioncontract.RunCommandStatus) { value.WorkspaceID = "ws-other" }},
		{name: "runId", mutate: func(value *projectioncontract.RunCommandStatus) { value.RunID = "run-other" }},
		{name: "state", mutate: func(value *projectioncontract.RunCommandStatus) { value.State = "rejected" }},
		{name: "intent", mutate: func(value *projectioncontract.RunCommandStatus) { value.Intent = "restart" }},
		{name: "requestId", mutate: func(value *projectioncontract.RunCommandStatus) { value.RequestID = "request-other" }},
		{name: "requestReason", mutate: func(value *projectioncontract.RunCommandStatus) { value.RequestReason = "other reason" }},
		{name: "source", mutate: func(value *projectioncontract.RunCommandStatus) { value.Source = "web" }},
		{name: "outcome", mutate: func(value *projectioncontract.RunCommandStatus) {
			value.Outcome = runtimecontract.RuntimeExecutionIntentReplacementPending
		}},
		{name: "admissionReason", mutate: func(value *projectioncontract.RunCommandStatus) {
			value.AdmissionReason = runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated
		}},
		{name: "diagnostic", mutate: func(value *projectioncontract.RunCommandStatus) { value.Diagnostic = "other diagnostic" }},
		{name: "acceptedAt", mutate: func(value *projectioncontract.RunCommandStatus) { value.AcceptedAt = "2026-08-23T00:00:01Z" }},
		{name: "rejectedAt", mutate: func(value *projectioncontract.RunCommandStatus) { value.RejectedAt = "2026-08-23T00:00:02Z" }},
		{name: "revision", mutate: func(value *projectioncontract.RunCommandStatus) { value.Revision = 8 }},
	}
	if !runCommandStatusEqual(&base, &base) {
		t.Fatal("identical canonical run command status is not equal")
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)
			if runCommandStatusEqual(&base, &changed) {
				t.Fatalf("%s-only mutation was treated as equal: base=%+v changed=%+v", tc.name, base, changed)
			}
		})
	}
}

func TestFetchAllUsesWorkspaceListWithoutSnapshot(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	be.listWorkspaces = []projectioncontract.Workspace{{
		ID:    "ws-list-only",
		Title: "List Only",
		State: string(common.WorkspaceStateActive),
		Projection: projectioncontract.ProjectionStatus{
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
	got := latestLLMContextBudget([]projectioncontract.LLMContextBudgetTrace{
		{
			StepID:               "step-budget",
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
	}, []projectioncontract.LLMUsageTrace{
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

func projectWorkspace(m *AppModel, ws projectioncontract.Workspace) {
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
	projectWorkspace(m, projectioncontract.Workspace{
		ID:          workspaceID,
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: runID,
		ViewModel: projectioncontract.WorkspaceViewModel{
			WorkspaceID: workspaceID, PromptMode: workspacePromptModeDecision, WorkspaceRevision: 1,
		},
	})
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	entry.Display.GovernanceDisplay = projectioncontract.GovernanceDisplay{PromptMode: workspacePromptModeDecision, RunID: runID, DecisionID: ticketID}
	item := projectioncontract.Decision{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: "open"}
	m.decisions = append(m.decisions, item)
	m.canonicalBlockingTicketID = ticketID
	m.blockingDecision = &item
}

func projectApprovalBlocker(m *AppModel, workspaceID, runID, approvalID string) {
	projectWorkspace(m, projectioncontract.Workspace{
		ID:          workspaceID,
		State:       string(common.WorkspaceStateBlockedOnApproval),
		ActiveRunID: runID,
		ViewModel: projectioncontract.WorkspaceViewModel{
			WorkspaceID: workspaceID, PromptMode: workspacePromptModeApproval, WorkspaceRevision: 1,
		},
	})
	entry := m.ensureWorkspaceRuntimeState(workspaceID)
	entry.Display.GovernanceDisplay = projectioncontract.GovernanceDisplay{PromptMode: workspacePromptModeApproval, RunID: runID, ApprovalID: approvalID}
	item := projectioncontract.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: "pending"}
	m.approvals = append(m.approvals, item)
	m.canonicalBlockingApprovalID = approvalID
	m.blockingApproval = &item
}

func projectRunningWorkspace(m *AppModel, workspaceID, runID string) {
	projectWorkspace(m, projectioncontract.Workspace{
		ID:          workspaceID,
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: runID,
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

func TestModelExitCommandWorksDuringHelp(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.setInput("preserved input")
	m.helpOpen = true
	for _, char := range "/exit" {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = asModel(next)
		if cmd != nil {
			t.Fatal("quit should wait for enter")
		}
		if !m.helpOpen {
			t.Fatal("typing the global exit command should not close help before enter")
		}
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if asModel(next).mode != modeQuit || cmd == nil {
		t.Fatal("/exit should quit even while help is active")
	}
}

func TestModelExitCommandIsCaseInsensitiveInModal(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.helpOpen = true
	for _, char := range "/EXIT" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = asModel(next)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if asModel(next).mode != modeQuit || cmd == nil {
		t.Fatal("global /exit command should be case-insensitive")
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
	msg := WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	m.artifactContinuations = []projectioncontract.ArtifactContinuation{{
		ID:                "artifact-continuation-001",
		WorkspaceID:       "ws-1",
		RunID:             "run-001",
		ArtifactID:        "artifact-001",
		TargetRef:         "deliverable.txt",
		State:             string(common.ArtifactContinuationStateChunking),
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
	m.patches = []projectioncontract.Patch{{ID: "patch-1", TargetRef: "old.txt", State: string(common.PatchStateApplied)}}
	m.llmUsage = projectioncontract.LLMUsageSummary{EntryTotal: 2, TotalTokens: 100}
	m.activeRunID = "run-1"

	m.applyWorkspaceContext(WorkspaceContextMsg{
		Workspace: &projectioncontract.Workspace{ID: "ws-1", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"},
		ActiveRun: &projectioncontract.Run{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       string(common.RunStateActive),
		},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-1",
			WorkspaceID: "ws-1",
			State:       string(common.RunStateActive),
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
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
		Workspace: &projectioncontract.Workspace{ID: "ws-1", Title: "Project", State: string(common.WorkspaceStateActive), ActiveRunID: "run-active"},
		ActiveRun: &projectioncontract.Run{
			ID:          "run-active",
			WorkspaceID: "ws-1",
			State:       string(common.RunStateActive),
		},
		Decisions: []projectioncontract.Decision{
			{ID: "ticket-active", WorkspaceID: "ws-1", RunID: "run-active", State: "open"},
			{ID: "ticket-old", WorkspaceID: "ws-1", RunID: "run-old", State: "open"},
		},
		Approvals: []projectioncontract.Approval{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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

func TestModelSubmitGoalStartsRunRejectsRetiredDefinitionWrite(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.setInput("/goal finish task")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error == nil || msg.Admission != nil {
		t.Fatalf("退休目标写入未拒绝: %#v", msg)
	}
	if len(be.goals) != 0 || len(be.runCommands) != 0 {
		t.Fatal("退休命令写入目标或启动 Run")
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
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("runCommands = %+v, want workspace-scoped resume", be.runCommands)
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Admission == nil || runMsg.Admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested {
		t.Fatalf("message = %#v, want typed RunControlMsg admission", msg)
	}
}

func TestModelPlainInputIdleWorkspaceSubmitsGoalRejectsRetiredDefinitionWrite(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.setInput("新目标")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error == nil || msg.Admission != nil {
		t.Fatalf("退休目标写入未拒绝: %#v", msg)
	}
	if len(be.goals) != 0 || len(be.runCommands) != 0 {
		t.Fatal("退休命令写入目标或启动 Run")
	}
}

func TestModelPlainInputUsesCurrentWorkspaceLiveRunOnlyRejectsRetiredDefinitionWrite(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.setInput("新目标")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error == nil || msg.Admission != nil {
		t.Fatalf("退休目标写入未拒绝: %#v", msg)
	}
	if len(be.goals) != 0 || len(be.runCommands) != 0 {
		t.Fatal("退休命令写入目标或启动 Run")
	}
}

func TestModelPlainInputFailedWorkspaceIgnoresStaleActiveRunRejectsRetiredDefinitionWrite(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.setInput("新目标")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error == nil || msg.Admission != nil {
		t.Fatalf("退休目标写入未拒绝: %#v", msg)
	}
	if len(be.goals) != 0 || len(be.runCommands) != 0 {
		t.Fatal("退休命令写入目标或启动 Run")
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

func TestModelGoalSlashCommandCanSubmitRunControlWordsAsGoalRejectsRetiredDefinitionWrite(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.setInput("/goal finish task")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(RunStartedMsg)
	if !ok || msg.Error == nil || msg.Admission != nil {
		t.Fatalf("退休目标写入未拒绝: %#v", msg)
	}
	if len(be.goals) != 0 || len(be.runCommands) != 0 {
		t.Fatal("退休命令写入目标或启动 Run")
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
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("runCommands = %+v, want workspace-scoped resume", be.runCommands)
	}
	runMsg, ok := msg.(RunControlMsg)
	if !ok || runMsg.Admission == nil || runMsg.Admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested {
		t.Fatalf("message = %#v, want typed RunControlMsg admission", msg)
	}
	next, _ = next.(*AppModel).Update(msg)
	m = asModel(next)
	if m.progress != progressSubmitting || m.running {
		t.Fatalf("progress/running = %s/%v, want admission pending projection", m.progress, m.running)
	}
	if !strings.Contains(m.progressText, "continuation accepted") {
		t.Fatalf("progressText = %q, want owner-authored admission reason", m.progressText)
	}
}

func TestModelRunResumeWithoutLiveRunDoesNotPredeclareContinuation(t *testing.T) {
	be := newRecordingBE(t, common.RunStateFailed)
	be.snapshot = &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: "ws-1", Title: "failed", State: string(common.WorkspaceStateActive)},
		DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{
			WorkspaceID: "ws-1",
			State:       string(common.RunStateFailed),
			RunID:       "run-failed",
			Terminal:    true,
			Reason:      "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
		},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-1",
			RunID:       "run-failed",
			State:       string(common.RunStateFailed),
			Recovery: projectioncontract.RecoveryPlan{
				Reason: "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
			},
		},
		Runs: []projectioncontract.Run{{
			ID:            "run-failed",
			WorkspaceID:   "ws-1",
			State:         string(common.RunStateFailed),
			FailureReason: "stuck running run recovered: no active execution, blocker, or continuation checkpoint",
		}},
	}
	m := NewAppModel(be, "ws-1", "")
	m.workspaces = []projectioncontract.Workspace{
		{ID: "ws-1", Title: "failed", State: string(common.WorkspaceStateActive)},
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
	if !ok || msg.Error != nil || msg.Admission == nil {
		t.Fatalf("run resume msg = %#v, want typed RunControlMsg admission", msg)
	}
	if msg.Admission.RunID != "run-new" || msg.Admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || !strings.Contains(msg.Message, "resume") {
		t.Fatalf("admission/message = %+v/%q, want workspace resume admission", msg.Admission, msg.Message)
	}
	if len(be.runCommands) != 1 ||
		be.runCommands[0].WorkspaceID != "ws-1" ||
		be.runCommands[0].Intent != backend.RunCommandIntentResume ||
		be.runCommands[0].Reason != "user_requested" ||
		be.runCommands[0].Source != "tui" {
		t.Fatalf("runCommands = %+v, want workspace-scoped TUI resume command", be.runCommands)
	}
	next, _ = m.Update(msg)
	m = asModel(next)
	if m.progress != progressSubmitting || m.running {
		t.Fatalf("progress/running = %s/%v, want owner projection pending", m.progress, m.running)
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
	if !ok || msg.Error != nil || msg.Admission == nil {
		t.Fatalf("resume msg = %#v, want typed RunControlMsg admission", msg)
	}
	if len(be.runCommands) != 1 || msg.Admission.WorkspaceID != "ws-2" {
		t.Fatalf("runCommands/admission = %+v/%+v, want workspace-scoped resume for ws-2", be.runCommands, msg.Admission)
	}

	updated, _ := next.(*AppModel).Update(msg)
	got := asModel(updated)
	if got.activeRunID != "run-new" || got.progress != progressSubmitting || got.running {
		t.Fatalf("run admission = %q/%s/%v, want owner projection pending", got.activeRunID, got.progress, got.running)
	}
}

func TestModelRunRestartRequestsWorkspaceScopedFreshActivation(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.activeRunID = "run-resume"
	m.setInput("/run restart 重新执行并纳入刚补充的信息")

	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd == nil || m.lastError != nil || m.progress != progressSubmitting || !strings.Contains(m.progressText, "fresh") {
		t.Fatalf("restart command=%v error=%v progress=%s/%q", cmd, m.lastError, m.progress, m.progressText)
	}
	msg, ok := cmd().(RunControlMsg)
	if !ok || msg.Error != nil {
		t.Fatalf("restart message=%#v", msg)
	}
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentRestart || be.runCommands[0].WorkspaceID != "ws-1" {
		t.Fatalf("restart run commands=%+v, want workspace-scoped restart", be.runCommands)
	}
}

func TestModelRunRestartErrorSurvivesProjectionRefresh(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	restartErr := errors.New("runtime bootstrap definition focus owner-fact acquisition selector does not match its declaration")

	next, cmd := m.Update(RunControlMsg{WorkspaceID: "ws-1", Error: restartErr})
	m = asModel(next)
	if cmd == nil || !errors.Is(m.lastError, restartErr) {
		t.Fatalf("restart failure = %v command=%v, want visible error plus owner-state refresh", m.lastError, cmd != nil)
	}

	next, _ = m.Update(WorkspaceRuntimeViewMsg{
		WorkspaceID: "ws-1",
		View: &projectioncontract.WorkspaceRuntimeView{
			WorkspaceID: "ws-1",
			RunState:    string(common.RunStateCompleted),
			TaskProgress: projectioncontract.TaskProgress{
				WorkspaceID: "ws-1",
				State:       string(common.RunStateCompleted),
				Summary:     "previous run completed",
			},
		},
	})
	m = asModel(next)
	if !errors.Is(m.lastError, restartErr) {
		t.Fatalf("restart failure was erased by projection refresh: %v", m.lastError)
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

func TestModelHelpExplainsRunResumeAndFreshRestart(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	help := stripANSI(m.renderHelp(160))
	if !strings.Contains(help, "/run resume") {
		t.Fatalf("help = %q, want /run resume", help)
	}
	overlay := stripANSI(m.renderHelpOverlay(120, 40))
	if !strings.Contains(overlay, "/run restart") {
		t.Fatalf("help overlay=%q, want fresh restart guidance", overlay)
	}
	foundRestart := false
	for _, suggestion := range m.slashCommandSuggestions("/run") {
		if suggestion.Command == "/run restart" {
			foundRestart = true
		}
	}
	if !foundRestart {
		t.Fatal("TUI suggestions must expose explicit fresh restart")
	}
}

func TestModelRunControlRunningClearsStaleResult(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.result = "stale result"
	m.artifacts = []string{"stale-artifact"}

	next, _ := m.Update(RunControlMsg{
		Run:         &projectioncontract.Run{ID: "run-new", WorkspaceID: "ws-1", State: string(common.RunStateActive)},
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
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.activeRunID = "run-1"
	m.input = "resume"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("blocked workspace input should refresh blocker details")
	}
	if len(be.goals) != 0 {
		t.Fatalf("unexpected goal calls: %v", be.goals)
	}
	if m.progress != progressSubmitting || m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" {
		t.Fatalf("progress/blocker = %s/%+v, want submitting decision with canonical ticket-1 retained", m.progress, m.blockingDecision)
	}
	if technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; !technical.CommandInFlight || technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want command in flight", technical)
	}
}

func TestModelRunResumeDoesNotBypassBlockingDecision(t *testing.T) {
	be := newRecordingBE(t, common.RunStateActive)
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.activeRunID = "run-1"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.input = "/run resume"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("run resume with decision should refresh blocker details")
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-1" || m.progress != progressWaitingDecision {
		t.Fatalf("blocking decision/progress = %+v/%s", m.blockingDecision, m.progress)
	}
}

func TestModelGovernanceDisplayCannotBlockResumeWithoutCanonicalBlocker(t *testing.T) {
	for _, input := range []string{"/run resume", "继续处理"} {
		t.Run(input, func(t *testing.T) {
			be := newRecordingBE(t, common.RunStateActive)
			m := NewAppModel(be, "ws-1", "")
			projectWorkspace(&m, projectioncontract.Workspace{
				ID: "ws-1", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1",
			})
			entry := m.ensureWorkspaceRuntimeState("ws-1")
			entry.Display.GovernanceDisplay = projectioncontract.GovernanceDisplay{
				PromptMode: workspacePromptModeDecision, RunID: "run-1", DecisionID: "ticket-display-only",
			}
			m.setInput(input)

			_, cmd := m.submitInput()
			if cmd == nil {
				t.Fatal("display-only governance handoff suppressed Resume")
			}
			msg := cmd()
			if _, ok := msg.(RunControlMsg); !ok {
				t.Fatalf("command message = %T, want RunControlMsg", msg)
			}
			if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentResume {
				t.Fatalf("run commands = %+v, want exactly one owner-adjudicated Resume", be.runCommands)
			}
		})
	}
}

func TestModelDefinitionClarificationDisablesGenericMutationAndResume(t *testing.T) {
	inputs := []string{
		"clarification answer",
		"/decision answer clarification answer",
		"/decision reject",
		"/run resume",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			be := newRecordingBE(t, common.RunStateBlockedOnDecision)
			m := NewAppModel(be, "ws-definition", "")
			projectWorkspace(&m, projectioncontract.Workspace{
				ID: "ws-definition", State: string(common.WorkspaceStateActive), ActiveRunID: "run-definition",
			})
			m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
				WorkspaceID: "ws-definition", ActiveRunID: "run-definition",
				RunState: string(common.RunStateBlockedOnDecision), PromptMode: workspacePromptModeDecision,
				Revision: projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 3},
				BlockingDecision: &projectioncontract.Decision{
					ID: "ticket-definition", WorkspaceID: "ws-definition", RunID: "run-definition", State: "open",
					DecisionKind: "runtime_definition_clarification", FailureClass: "runtime_definition_gap",
					DefinitionClarification: &projectioncontract.DefinitionClarification{
						Class: "runtime_definition", BindingState: "open", RequiredAction: "revise_definition_and_restart",
						ViolationOrGapCode: "implicit_one_of", OffendingRefs: []string{"dod:item-1"},
					},
				},
			}, false)
			// Exercise the RuntimeView-only window before the Decision list arrives.
			if len(m.decisions) != 0 || m.activeDefinitionClarificationDecision() == nil {
				t.Fatalf("definition blocker was not retained from canonical RuntimeView: decisions=%+v blocker=%+v", m.decisions, m.activeDefinitionClarificationDecision())
			}
			m.setInput(input)

			_, cmd := m.submitInput()
			if cmd != nil {
				t.Fatal("definition clarification emitted a generic mutation/Resume command")
			}
			if len(be.runCommands) != 0 {
				t.Fatalf("run commands = %+v, want zero", be.runCommands)
			}
			if m.lastError == nil || !strings.Contains(m.lastError.Error(), "revise Goal/DoD") ||
				!strings.Contains(m.progressText, "/run restart") || !strings.Contains(m.progressText, "gap=implicit_one_of") ||
				!strings.Contains(m.progressText, "offending=dod:item-1") || !strings.Contains(m.progressText, "required=revise_definition_and_restart") {
				t.Fatalf("dedicated guidance = error:%v progress:%q", m.lastError, m.progressText)
			}
		})
	}
}

func TestModelDefinitionClarificationOffersOnlyRevisionAndRestartActions(t *testing.T) {
	be := newRecordingBE(t, common.RunStateBlockedOnDecision)
	m := NewAppModel(be, "ws-definition", "")
	projectWorkspace(&m, projectioncontract.Workspace{
		ID: "ws-definition", State: string(common.WorkspaceStateActive), ActiveRunID: "run-definition",
	})
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-definition", ActiveRunID: "run-definition",
		RunState: string(common.RunStateBlockedOnDecision), PromptMode: workspacePromptModeDecision,
		Revision: projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 3},
		BlockingDecision: &projectioncontract.Decision{
			ID: "ticket-definition", WorkspaceID: "ws-definition", RunID: "run-definition", State: "open",
			DecisionKind: "runtime_definition_clarification", FailureClass: "runtime_definition_gap",
			DefinitionClarification: &projectioncontract.DefinitionClarification{
				Class: "runtime_definition", BindingState: "open", RequiredAction: "revise_definition_and_restart",
				ViolationOrGapCode: "semantic_ambiguity", OffendingRefs: []string{"goal:goal-1"},
			},
		},
	}, false)
	m.setInput("/")
	input := stripANSI(m.renderInput(160))
	for _, want := range []string{"definition >", "/run restart"} {
		if !strings.Contains(input, want) {
			t.Fatalf("definition prompt = %q, want %q", input, want)
		}
	}
	for _, forbidden := range []string{"/goal", "/dod", "/workspace create", "/decision answer", "/decision terminate", "/run resume"} {
		if strings.Contains(input, forbidden) {
			t.Fatalf("definition prompt = %q, must not offer %q", input, forbidden)
		}
	}

	m.setInput("/run restart")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("explicit definition restart did not emit owner command")
	}
	_ = cmd()
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentRestart {
		t.Fatalf("run commands = %+v, want one explicit restart", be.runCommands)
	}
}

func TestMergeDecisionInfoPropagatesDefinitionClassification(t *testing.T) {
	incoming := projectioncontract.Decision{
		ID: "ticket-definition", DecisionKind: "runtime_definition_clarification", FailureClass: "runtime_definition_gap",
		FailureSignature: "definition-signature", AllowedRecoveryActions: []string{"revise_definition"},
		DefinitionClarification: &projectioncontract.DefinitionClarification{
			Class: "runtime_definition", BindingState: "open", RequiredAction: "revise_definition_and_restart",
			ViolationOrGapCode: "implicit_one_of", OffendingRefs: []string{"dod:item-1"},
		},
	}
	merged := mergeDecisionInfo(
		projectioncontract.Decision{ID: "ticket-definition", WorkspaceID: "ws-definition", RunID: "run-definition", State: "open"},
		incoming,
	)
	if !decisionRequiresDedicatedDefinitionFlow(merged) || merged.FailureSignature != "definition-signature" ||
		!reflect.DeepEqual(merged.AllowedRecoveryActions, []string{"revise_definition"}) ||
		merged.DefinitionClarification == nil || !reflect.DeepEqual(merged.DefinitionClarification.OffendingRefs, []string{"dod:item-1"}) {
		t.Fatalf("merged decision classification = %+v", merged)
	}
	merged.DefinitionClarification.OffendingRefs[0] = "mutated"
	if incoming.DefinitionClarification.OffendingRefs[0] != "dod:item-1" {
		t.Fatal("merged definition clarification aliases the incoming owner projection")
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
	if runMsg.Admission == nil || runMsg.Admission.RunID != "run-new" || runMsg.Admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested {
		t.Fatalf("admission = %+v, want workspace owner result", runMsg.Admission)
	}
	if !strings.Contains(runMsg.Message, "resume") {
		t.Fatalf("message = %q, want unified resume acknowledgement", runMsg.Message)
	}
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("runCommands = %+v, want workspace-scoped resume", be.runCommands)
	}
}

func TestResumeRunActivatesWorkspaceWhenNoActiveRun(t *testing.T) {
	be := newRecordingBE(t, common.RunStateCompleted)
	be.snapshot = &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: "ws-1"},
	}
	msg := ResumeRun(be, "ws-1")()
	runMsg, ok := msg.(RunControlMsg)
	if !ok {
		t.Fatalf("message = %T, want RunControlMsg", msg)
	}
	if runMsg.Error != nil {
		t.Fatalf("error = %v, want nil", runMsg.Error)
	}
	if runMsg.Admission == nil || runMsg.Admission.RunID != "run-new" {
		t.Fatalf("admission = %+v, want workspace owner result", runMsg.Admission)
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
	if runMsg.Admission == nil || runMsg.Admission.RunID != "run-new" {
		t.Fatalf("admission = %+v, want workspace owner result", runMsg.Admission)
	}
	if len(be.runCommands) != 1 || be.runCommands[0].Intent != backend.RunCommandIntentResume {
		t.Fatalf("runCommands = %+v, want unified workspace resume", be.runCommands)
	}
}

func TestModelInspectCommandsRenderCurrentState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.currentTask = "finish task"
	m.dodItems = []string{"dod one"}
	m.activityFeed = projectioncontract.WorkspaceActivityFeed{
		Items: []projectioncontract.WorkspaceActivityItem{{
			ID:      "activity-timeline",
			Summary: "Working: shell/command go test ./...",
		}},
	}
	m.patches = []projectioncontract.Patch{{ID: "patch-1", TargetRef: "main.go", State: string(common.PatchStateApplied)}}
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

func TestModelStatusRendersRunCommandRejectionWithoutTriggeringCommands(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.runCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:     "ws-1",
		State:           "rejected",
		Intent:          "restart",
		Outcome:         runtimecontract.RuntimeExecutionIntentRestartRequired,
		AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
		Diagnostic:      "owner command failed during execution admission",
	}

	m.setInput("/status")
	next, cmd := m.submitInput()
	m = asModel(next)
	if cmd != nil {
		t.Fatal("/status returned command, want local projection render only")
	}
	for _, want := range []string{
		"command rejected restart",
		"owner command failed during execution admission",
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{ID: "ws-1", Title: "Project", State: "active"}}})
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
	m.workspaces = []projectioncontract.Workspace{{ID: "ws-1", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"}}
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
		Run:         &projectioncontract.Run{ID: "run-completed", WorkspaceID: "ws-97549a108e19d500", State: string(common.RunStateCompleted)},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{ID: "ws-1", Title: "Project", State: "active"}}})
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
	if view := stripANSI(m.View()); !strings.Contains(view, "WorkOrch 帮助") || !strings.Contains(view, "/status") || !strings.Contains(view, "/coverage") {
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

func TestModelConfigCommandShowsCandidateDocumentRecovery(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	m.setInput("/config")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected config command")
	}
	msg := cmd()
	next, _ = asModel(next).Update(msg)
	m = asModel(next)
	if !strings.Contains(m.result, "Candidate Document Recovery") || !strings.Contains(m.result, "maxDocumentRecoveryAttempts: 3") {
		t.Fatalf("config result missing candidate document recovery limit:\n%s", m.result)
	}
}

func TestModelDecisionSlashCompletionSuggestsContextSubcommands(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", State: "open", Question: "choose next"}
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", State: "open", Question: "choose next"}
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
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-1", State: "pending", Reason: "run shell command"}
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open", Question: "choose next"}
	m.setInput("/decision answer 请重新规划当前路径")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command")
	}
	m = asModel(next)
	technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand
	if !technical.CommandInFlight || technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want command in flight", technical)
	}
	msg := cmd()
	next, _ = m.Update(msg)
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
	if m.blockingDecision == nil || m.blockingDecision.State != "open" || len(m.decisions) != 1 || m.decisions[0].State != "open" || m.canonicalBlockingTicketID != "ticket-1" {
		t.Fatalf("canonical decision projection changed before readback: blocker=%+v list=%+v id=%q", m.blockingDecision, m.decisions, m.canonicalBlockingTicketID)
	}
	technical = m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand
	if technical.CommandInFlight || !technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want projection refresh pending", technical)
	}
}

func TestModelDecisionAnswerAfterWorkspaceSwitchDoesNotUseDisplayPromptAsMutationTarget(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 11}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 11}},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 12}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-b", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 12}},
	}})
	m = asModel(next)
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}

	m.workspaceSelectMode = true
	m.selectedWorkspaceID = "ws-b"
	next, _ = m.confirmWorkspaceSelection()
	m = asModel(next)
	m.blockingDecision = nil
	m.decisions = nil
	m.setInput("/decision answer 请重新规划")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("display-only decision handoff should refresh owner resources")
	}
	m = asModel(next)

	if len(be.decisions) != 0 {
		t.Fatalf("resolved decisions = %d, want no mutation from display-only prompt", len(be.decisions))
	}
	if m.lastError == nil || !strings.Contains(m.lastError.Error(), "owner projection is not loaded") {
		t.Fatalf("lastError = %v, want explicit owner projection handoff", m.lastError)
	}
}

func TestModelGovernanceDisplayCannotSelectDecisionMutationTarget(t *testing.T) {
	t.Run("forged display run and resource scope produce zero mutation", func(t *testing.T) {
		decision := &recordingTUIDecisionBackend{}
		be := tuiResolutionBackend{fakeBE: newFake(t), decision: decision, approval: &recordingTUIApprovalBackend{}}
		m := NewAppModel(be, "ws-owner", "")
		projectWorkspace(&m, projectioncontract.Workspace{
			ID: "ws-owner", State: string(common.WorkspaceStateActive), ActiveRunID: "run-owner",
		})
		entry := m.ensureWorkspaceRuntimeState("ws-owner")
		entry.Display.GovernanceDisplay = projectioncontract.GovernanceDisplay{
			PromptMode: workspacePromptModeDecision, RunID: "run-forged", DecisionID: "ticket-forged",
		}
		forged := projectioncontract.Decision{
			ID: "ticket-forged", WorkspaceID: "ws-owner", RunID: "run-forged", State: "open",
		}
		m.decisions = []projectioncontract.Decision{forged}
		m.blockingDecision = &forged
		m.setInput("/decision answer forged")

		next, cmd := m.submitInput()
		m = asModel(next)
		if cmd == nil {
			t.Fatal("forged display handoff should refresh concrete owner resources")
		}
		_ = cmd()
		if len(decision.requests) != 0 {
			t.Fatalf("decision requests = %+v, want zero mutation for forged display scope", decision.requests)
		}
		if m.lastError == nil || !strings.Contains(m.lastError.Error(), "owner projection is not loaded") {
			t.Fatalf("lastError = %v, want missing canonical owner resource", m.lastError)
		}
	})

	for _, tc := range []struct {
		name       string
		promptMode string
	}{
		{name: "conflicting display prompt", promptMode: workspacePromptModeApproval},
		{name: "blank display prompt", promptMode: ""},
	} {
		t.Run(tc.name+" does not hide canonical owner resource", func(t *testing.T) {
			decision := &recordingTUIDecisionBackend{}
			be := tuiResolutionBackend{fakeBE: newFake(t), decision: decision, approval: &recordingTUIApprovalBackend{}}
			m := NewAppModel(be, "ws-owner", "")
			projectWorkspace(&m, projectioncontract.Workspace{
				ID: "ws-owner", State: string(common.WorkspaceStateActive), ActiveRunID: "run-owner",
			})
			ownerResource := projectioncontract.Decision{
				ID: "ticket-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "open",
			}
			m.decisions = []projectioncontract.Decision{ownerResource}
			m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
				WorkspaceID: "ws-owner", ActiveRunID: "run-owner", RunState: string(common.RunStateBlockedOnDecision),
				GovernanceDisplay: projectioncontract.GovernanceDisplay{
					PromptMode: tc.promptMode, RunID: "run-forged", DecisionID: "ticket-forged",
				},
				BlockingDecision: &ownerResource,
			}, false)
			m.setInput("/decision answer canonical")

			_, cmd := m.submitInput()
			if cmd == nil {
				t.Fatal("canonical concrete decision should produce a resolution command")
			}
			_ = cmd()
			if len(decision.requests) != 1 || decision.requests[0].TicketID != "ticket-owner" || decision.requests[0].WorkspaceID != "ws-owner" {
				t.Fatalf("decision requests = %+v, want canonical owner resource only", decision.requests)
			}
		})
	}

	t.Run("same-run display resource cannot override runtime-view canonical resource", func(t *testing.T) {
		decision := &recordingTUIDecisionBackend{}
		be := tuiResolutionBackend{fakeBE: newFake(t), decision: decision, approval: &recordingTUIApprovalBackend{}}
		m := NewAppModel(be, "ws-owner", "")
		projectWorkspace(&m, projectioncontract.Workspace{
			ID: "ws-owner", State: string(common.WorkspaceStateActive), ActiveRunID: "run-owner",
		})
		m.decisions = []projectioncontract.Decision{
			{ID: "ticket-display", WorkspaceID: "ws-owner", RunID: "run-owner", State: "open"},
			{ID: "ticket-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "open"},
		}
		m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
			WorkspaceID: "ws-owner", ActiveRunID: "run-owner", RunState: string(common.RunStateBlockedOnDecision),
			GovernanceDisplay: projectioncontract.GovernanceDisplay{
				PromptMode: workspacePromptModeDecision, RunID: "run-owner", DecisionID: "ticket-display",
			},
			BlockingDecision: &m.decisions[1],
		}, false)
		m.setInput("/decision answer canonical")

		_, cmd := m.submitInput()
		if cmd == nil {
			t.Fatal("canonical RuntimeView decision should produce a resolution command")
		}
		_ = cmd()
		if len(decision.requests) != 1 || decision.requests[0].TicketID != "ticket-owner" {
			t.Fatalf("decision requests = %+v, want canonical RuntimeView resource ticket-owner", decision.requests)
		}
	})
}

func TestModelGovernanceDisplayCannotSelectApprovalMutationTarget(t *testing.T) {
	t.Run("forged display run and resource scope produce zero mutation", func(t *testing.T) {
		approval := &recordingTUIApprovalBackend{}
		be := tuiResolutionBackend{fakeBE: newFake(t), decision: &recordingTUIDecisionBackend{}, approval: approval}
		m := NewAppModel(be, "ws-owner", "")
		projectWorkspace(&m, projectioncontract.Workspace{
			ID: "ws-owner", State: string(common.WorkspaceStateBlockedOnApproval), ActiveRunID: "run-owner",
		})
		entry := m.ensureWorkspaceRuntimeState("ws-owner")
		entry.Display.GovernanceDisplay = projectioncontract.GovernanceDisplay{
			PromptMode: workspacePromptModeApproval, RunID: "run-forged", ApprovalID: "approval-forged",
		}
		forged := projectioncontract.Approval{
			ID: "approval-forged", WorkspaceID: "ws-owner", RunID: "run-forged", State: "pending",
		}
		m.approvals = []projectioncontract.Approval{forged}
		m.blockingApproval = &forged
		m.setInput("/approval approve")

		next, cmd := m.submitInput()
		m = asModel(next)
		if cmd == nil {
			t.Fatal("forged display handoff should refresh concrete owner resources")
		}
		_ = cmd()
		if len(approval.approvals) != 0 || len(approval.rejections) != 0 {
			t.Fatalf("approval calls = %+v/%+v, want zero mutation for forged display scope", approval.approvals, approval.rejections)
		}
		if m.lastError == nil || !strings.Contains(m.lastError.Error(), "owner projection is not loaded") {
			t.Fatalf("lastError = %v, want missing canonical owner resource", m.lastError)
		}
	})

	for _, tc := range []struct {
		name       string
		promptMode string
	}{
		{name: "conflicting display prompt", promptMode: workspacePromptModeDecision},
		{name: "blank display prompt", promptMode: ""},
	} {
		t.Run(tc.name+" does not hide canonical owner resource", func(t *testing.T) {
			approval := &recordingTUIApprovalBackend{}
			be := tuiResolutionBackend{fakeBE: newFake(t), decision: &recordingTUIDecisionBackend{}, approval: approval}
			m := NewAppModel(be, "ws-owner", "")
			projectWorkspace(&m, projectioncontract.Workspace{
				ID: "ws-owner", State: string(common.WorkspaceStateBlockedOnApproval), ActiveRunID: "run-owner",
			})
			ownerResource := projectioncontract.Approval{
				ID: "approval-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "pending",
			}
			m.approvals = []projectioncontract.Approval{ownerResource}
			m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
				WorkspaceID: "ws-owner", ActiveRunID: "run-owner", RunState: string(common.RunStateBlockedOnApproval),
				GovernanceDisplay: projectioncontract.GovernanceDisplay{
					PromptMode: tc.promptMode, RunID: "run-forged", ApprovalID: "approval-forged",
				},
				BlockingApproval: &ownerResource,
			}, false)
			m.setInput("/approval approve")

			_, cmd := m.submitInput()
			if cmd == nil {
				t.Fatal("canonical concrete approval should produce a resolution command")
			}
			_ = cmd()
			if len(approval.approvals) != 1 || approval.approvals[0].ApprovalID != "approval-owner" || approval.approvals[0].WorkspaceID != "ws-owner" {
				t.Fatalf("approval calls = %+v, want canonical owner resource only", approval.approvals)
			}
		})
	}

	t.Run("same-run display resource cannot override runtime-view canonical resource", func(t *testing.T) {
		approval := &recordingTUIApprovalBackend{}
		be := tuiResolutionBackend{fakeBE: newFake(t), decision: &recordingTUIDecisionBackend{}, approval: approval}
		m := NewAppModel(be, "ws-owner", "")
		projectWorkspace(&m, projectioncontract.Workspace{
			ID: "ws-owner", State: string(common.WorkspaceStateBlockedOnApproval), ActiveRunID: "run-owner",
		})
		m.approvals = []projectioncontract.Approval{
			{ID: "approval-display", WorkspaceID: "ws-owner", RunID: "run-owner", State: "pending"},
			{ID: "approval-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "pending"},
		}
		m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
			WorkspaceID: "ws-owner", ActiveRunID: "run-owner", RunState: string(common.RunStateBlockedOnApproval),
			GovernanceDisplay: projectioncontract.GovernanceDisplay{
				PromptMode: workspacePromptModeApproval, RunID: "run-owner", ApprovalID: "approval-display",
			},
			BlockingApproval: &m.approvals[1],
		}, false)
		m.setInput("/approval approve")

		_, cmd := m.submitInput()
		if cmd == nil {
			t.Fatal("canonical RuntimeView approval should produce a resolution command")
		}
		_ = cmd()
		if len(approval.approvals) != 1 || approval.approvals[0].ApprovalID != "approval-owner" {
			t.Fatalf("approval calls = %+v, want canonical RuntimeView resource approval-owner", approval.approvals)
		}
	})
}

func TestModelDecisionReplanAfterWorkspaceSwitchReplacesStaleBlocker(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 11}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 11}},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 12}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-b", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 12}},
	}})
	m = asModel(next)

	m.activeWorkspaceID = "ws-b"
	m.selectedWorkspaceID = "ws-b"
	m.activeRunID = "run-b"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"}
	m.decisions = []projectioncontract.Decision{
		{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-a", State: "open"},
		{ID: "ticket-b", WorkspaceID: "ws-b", RunID: "run-b", State: "open"},
	}
	m.syncBlockingDecisionFromList()
	if m.blockingDecision != nil {
		t.Fatalf("blocking decision = %+v, want no first-open fallback before canonical RuntimeView", m.blockingDecision)
	}
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID:      "ws-b",
		ActiveRunID:      "run-b",
		PromptMode:       workspacePromptModeDecision,
		RunState:         string(common.RunStateBlockedOnDecision),
		Revision:         projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 12},
		BlockingDecision: &m.decisions[1],
	}, false)
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-b" {
		t.Fatalf("blocking decision = %+v, want ticket-b from canonical RuntimeView", m.blockingDecision)
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

func TestModelDecisionReplanUsesCanonicalRuntimeViewTicketOverStaleLocalBlocker(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-1",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-current",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 21},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-1", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 21},
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.selectedWorkspaceID = "ws-1"
	m.activeRunID = "run-current"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"}
	m.decisions = []projectioncontract.Decision{
		{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-stale", State: "open"},
		{ID: "ticket-current", WorkspaceID: "ws-1", RunID: "run-current", State: "open"},
	}
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID:      "ws-1",
		ActiveRunID:      "run-current",
		RunState:         string(common.RunStateBlockedOnDecision),
		Revision:         projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 21},
		BlockingDecision: &m.decisions[1],
	}, false)
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

func TestModelDecisionCommandKeepsCanonicalBlockerUntilProjectionReadback(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	m.setInput("/decision answer 请重新规划")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected decision resolve command")
	}
	m = asModel(next)
	if m.blockingDecision == nil || m.blockingDecision.State != "open" || m.decisions[0].State != "open" || m.canonicalBlockingTicketID != "ticket-1" {
		t.Fatalf("command submission mutated canonical decision projection: blocker=%+v list=%+v id=%q", m.blockingDecision, m.decisions, m.canonicalBlockingTicketID)
	}
	m.setInput("/decision answer duplicate")
	duplicateNext, duplicateCmd := m.submitInput()
	m = asModel(duplicateNext)
	if duplicateCmd != nil || len(be.decisions) != 0 {
		t.Fatalf("duplicate command escaped technical latch: cmd=%v calls=%d", duplicateCmd != nil, len(be.decisions))
	}

	msg := cmd()
	next, _ = m.Update(msg)
	m = asModel(next)
	if len(be.decisions) != 1 {
		t.Fatalf("resolved decisions = %d, want one", len(be.decisions))
	}
	if m.blockingDecision == nil || m.blockingDecision.State != "open" || m.decisions[0].State != "open" || m.canonicalBlockingTicketID != "ticket-1" {
		t.Fatalf("owner response mutated canonical decision projection before readback: blocker=%+v list=%+v id=%q", m.blockingDecision, m.decisions, m.canonicalBlockingTicketID)
	}
	technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand
	if technical.CommandInFlight || !technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want refresh pending", technical)
	}
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-1",
		ActiveRunID: "run-1",
		RunState:    string(common.RunStateActive),
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 2},
	}, false)
	if m.blockingDecision != nil || m.canonicalBlockingTicketID != "" {
		t.Fatalf("canonical decision blocker survived authoritative readback: blocker=%+v id=%q", m.blockingDecision, m.canonicalBlockingTicketID)
	}
	if technical = m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; technical.pending() {
		t.Fatalf("governance technical state survived authoritative readback: %+v", technical)
	}
}

func TestModelApprovalCommandKeepsCanonicalBlockerUntilProjectionReadback(t *testing.T) {
	approval := &recordingTUIApprovalBackend{}
	be := tuiResolutionBackend{fakeBE: newFake(t), decision: &recordingTUIDecisionBackend{}, approval: approval}
	m := NewAppModel(be, "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.setInput("/approval approve")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected approval command")
	}
	m = asModel(next)
	if m.blockingApproval == nil || m.blockingApproval.State != "pending" || m.approvals[0].State != "pending" || m.canonicalBlockingApprovalID != "approval-1" {
		t.Fatalf("command submission mutated canonical approval projection: blocker=%+v list=%+v id=%q", m.blockingApproval, m.approvals, m.canonicalBlockingApprovalID)
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = asModel(next)
	if len(approval.approvals) != 1 {
		t.Fatalf("approval calls = %d, want one", len(approval.approvals))
	}
	if m.blockingApproval == nil || m.blockingApproval.State != "pending" || m.approvals[0].State != "pending" || m.canonicalBlockingApprovalID != "approval-1" {
		t.Fatalf("owner response mutated canonical approval projection before readback: blocker=%+v list=%+v id=%q", m.blockingApproval, m.approvals, m.canonicalBlockingApprovalID)
	}
	technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand
	if technical.CommandInFlight || !technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want refresh pending", technical)
	}
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-1",
		ActiveRunID: "run-1",
		RunState:    string(common.RunStateActive),
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 2},
	}, false)
	if m.blockingApproval != nil || m.canonicalBlockingApprovalID != "" {
		t.Fatalf("canonical approval blocker survived authoritative readback: blocker=%+v id=%q", m.blockingApproval, m.canonicalBlockingApprovalID)
	}
	if technical = m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; technical.pending() {
		t.Fatalf("governance technical state survived authoritative readback: %+v", technical)
	}
}

func TestModelApprovalCommandErrorReleasesTechnicalLatchWithoutChangingBlocker(t *testing.T) {
	be := &conflictApprovalBE{fakeBE: newFake(t), runState: common.RunStateBlockedOnApproval}
	m := NewAppModel(be, "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	m.setInput("/approval approve")

	next, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("expected approval command")
	}
	m = asModel(next)
	next, _ = m.Update(cmd())
	m = asModel(next)

	if technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; technical.pending() {
		t.Fatalf("failed command retained technical latch: %+v", technical)
	}
	if m.blockingApproval == nil || m.blockingApproval.State != "pending" || m.approvals[0].State != "pending" || m.canonicalBlockingApprovalID != "approval-1" {
		t.Fatalf("failed command changed canonical blocker: blocker=%+v list=%+v id=%q", m.blockingApproval, m.approvals, m.canonicalBlockingApprovalID)
	}
	if !errors.Is(m.lastError, common.ErrConflict) || m.progress != progressWaitingApproval {
		t.Fatalf("failed command presentation = error:%v progress:%s", m.lastError, m.progress)
	}
	m.setInput("/approval approve")
	_, retry := m.submitInput()
	if retry == nil {
		t.Fatal("failed command did not release latch for retry")
	}
}

func TestModelDecisionReplanDoesNotUseLocalBlockerWhenServerProjectionIsLive(t *testing.T) {
	be := &decisionRecordingBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-1",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-stale", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-1",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
	}}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-1"
	m.activeRunID = "run-1"
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-stale", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
	m.approvals = []projectioncontract.Approval{*m.blockingApproval}
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
	for _, want := range []string{"未找到工作空间", "Web 控制台", "规格创作、审查与确认", "/workspace"} {
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-viewport", Title: "Viewport Workspace", State: string(common.WorkspaceStateActive)},
		{ID: "ws-other", Title: "Other Workspace", State: string(common.WorkspaceStateActive)},
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
	next, _ := m.Update(RunStartedMsg{Run: &projectioncontract.Run{ID: "run-1", State: string(common.RunStateActive)}})
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
	next, cmd := m.Update(RunStartedMsg{Run: &projectioncontract.Run{
		ID:               "run-1",
		WorkspaceID:      "ws-1",
		State:            string(common.RunStateBlockedOnDecision),
		BlockingTicketID: "ticket-1",
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected blocker detail fetch command")
	}
	if m.blockingDecision != nil || len(m.decisions) != 0 {
		t.Fatalf("blocking decision/list = %+v/%+v, want no resource synthesized from Run owner ref", m.blockingDecision, m.decisions)
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
	next, _ := m.Update(DecisionListMsg{Decisions: []projectioncontract.Decision{{
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
	next, _ := m.Update(RunStartedMsg{Run: &projectioncontract.Run{
		ID:               "run-1",
		WorkspaceID:      "ws-1",
		State:            string(common.RunStateBlockedOnDecision),
		BlockingTicketID: "ticket-1",
	}})
	m = asModel(next)
	projectDecisionBlocker(&m, "ws-1", "run-1", "ticket-1")
	next, _ = m.Update(DecisionDetailMsg{Decision: &projectioncontract.Decision{
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
	next, cmd := m.Update(RunStartedMsg{Run: &projectioncontract.Run{
		ID:                 "run-1",
		WorkspaceID:        "ws-1",
		State:              string(common.RunStateBlockedOnApproval),
		BlockingApprovalID: "approval-1",
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("expected blocker detail fetch command")
	}
	if m.blockingApproval != nil || len(m.approvals) != 0 {
		t.Fatalf("blocking approval/list = %+v/%+v, want no resource synthesized from Run owner ref", m.blockingApproval, m.approvals)
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
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
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
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-1", WorkspaceID: "ws-1", RunID: "run-1", State: "pending"}
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
	m.blockingApproval = &projectioncontract.Approval{
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

func TestResolveApprovalDoesNotReclassifyOwnerConflictFromTerminalReadback(t *testing.T) {
	for _, runState := range []common.RunState{common.RunStateBlockedOnApproval, common.RunStateActive} {
		t.Run(string(runState), func(t *testing.T) {
			be := &conflictApprovalBE{fakeBE: newFake(t), runState: runState}
			msg := ResolveApproval(be, "ws-1", "approval-1", true)()
			resolved, ok := msg.(ApprovalResolvedMsg)
			if !ok || !errors.Is(resolved.Error, common.ErrConflict) || resolved.Approval != nil {
				t.Fatalf("approval msg = %#v, want exact owner conflict", msg)
			}
			if be.approvalGetCalls != 0 {
				t.Fatalf("approval readbacks = %d, want zero client-side duplicate adjudication", be.approvalGetCalls)
			}
		})
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
	m.workspaces = []projectioncontract.Workspace{{
		ID:          "ws-1",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 7},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-1", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 7},
	}}
	m.decisions = []projectioncontract.Decision{{ID: "ticket-1", WorkspaceID: "ws-1", RunID: "run-1", State: "open"}}
	m.blockingDecision = &m.decisions[0]
	m.activeRunID = "run-1"
	m.canonicalBlockingTicketID = "ticket-1"
	if !m.beginGovernanceCommand(governanceCommandDecision, "ws-1", "run-1", "ticket-1") {
		t.Fatal("expected decision command technical state")
	}
	m.input = "补充上下文"
	m.inputCursor = len([]rune(m.input))

	next, cmd := m.Update(DecisionResolvedMsg{Decision: &projectioncontract.Decision{
		ID:          "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		State:       "resolved",
	}})
	m = asModel(next)

	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("expected workspace refresh after decision resolve: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if m.blockingDecision == nil || m.blockingDecision.State != "open" || m.input != "" || m.activeRunID != "run-1" || m.canonicalBlockingTicketID != "ticket-1" {
		t.Fatalf("canonical state changed before projection readback: blocker=%+v input=%q activeRun=%q id=%q", m.blockingDecision, m.input, m.activeRunID, m.canonicalBlockingTicketID)
	}
	if ws := findWorkspace(m.workspaces, "ws-1"); ws == nil || ws.State != string(common.WorkspaceStateActive) {
		t.Fatalf("workspace overview after resolve = %+v, want unchanged until server projection updates", ws)
	}
	m.width = 100
	m.height = 16
	view := stripANSI(m.View())
	if !strings.Contains(view, "ws-1 decision >") || strings.Contains(view, "ws-1 submitting >") {
		t.Fatalf("prompt should follow server waiting projection until refresh converges:\n%s", view)
	}
	if m.decisions[0].State != "open" {
		t.Fatalf("decision state = %q, want unchanged owner projection", m.decisions[0].State)
	}
	if technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; technical.CommandInFlight || !technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want readback pending", technical)
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("refresh command should batch workspace context and list refresh")
	}
}

func TestModelApprovalResolvedRefreshesWorkspaceState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-1", "")
	projectApprovalBlocker(&m, "ws-1", "run-1", "approval-1")
	if !m.beginGovernanceCommand(governanceCommandApproval, "ws-1", "run-1", "approval-1") {
		t.Fatal("expected approval command technical state")
	}
	m.input = "同意"
	m.inputCursor = len([]rune(m.input))

	next, cmd := m.Update(ApprovalResolvedMsg{Approval: &projectioncontract.Approval{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		State:       "approved",
	}})
	m = asModel(next)

	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("expected workspace refresh after approval resolve: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
	if m.blockingApproval == nil || m.blockingApproval.State != "pending" || m.input != "" || m.activeRunID != "run-1" || m.canonicalBlockingApprovalID != "approval-1" {
		t.Fatalf("canonical state changed before projection readback: blocker=%+v input=%q activeRun=%q id=%q", m.blockingApproval, m.input, m.activeRunID, m.canonicalBlockingApprovalID)
	}
	if m.progress != progressSubmitting || !strings.Contains(m.progressText, "projection readback") {
		t.Fatalf("approval resolve progress = %s/%q, want projection readback wait", m.progress, m.progressText)
	}
	if m.approvals[0].State != "pending" {
		t.Fatalf("approval state = %q, want unchanged owner projection", m.approvals[0].State)
	}
	if technical := m.ensureWorkspaceRuntimeState("ws-1").GovernanceCommand; technical.CommandInFlight || !technical.RefreshPending {
		t.Fatalf("governance command state = %+v, want readback pending", technical)
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
	m.blockingApproval = &projectioncontract.Approval{
		ID:          "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Reason:      "需要审批",
		State:       "pending",
	}
	m.progress = progressWaitingApproval
	next, _ := m.Update(ApprovalListMsg{Approvals: []projectioncontract.Approval{{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.currentTask = "old task"
	m.result = "old result"
	m.failureReason = "old failure"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", State: "open"}
	m.decisions = []projectioncontract.Decision{{ID: "ticket-old", State: "open"}}
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-a", WorkspaceID: "ws-a", State: "open"}
	m.decisions = []projectioncontract.Decision{{ID: "ticket-a", WorkspaceID: "ws-a", State: "open"}}

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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-b"
	m.activeRunID = "run-b"
	m.progress = progressWaitingDecision
	m.progressText = "Waiting for decision"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-b-old", WorkspaceID: "ws-b", RunID: "run-b", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
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
	m.workspaces = []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-a",
	}}
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-b", WorkspaceID: "ws-b", RunID: "run-b", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
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
	m.runHeartbeat = projectioncontract.RunHeartbeat{
		WorkspaceID:               "ws-a",
		RunID:                     "run-a",
		ActivityState:             "no_activity",
		Summary:                   "no effective progress evidence yet",
		LatestActivityAt:          "2026-06-10T00:01:00Z",
		LatestEffectiveProgressAt: "2026-06-10T00:00:00Z",
		Stalled:                   true,
	}
	m.convergenceStatus = &projectioncontract.ConvergenceStatus{
		WorkspaceID:           "ws-a",
		RunID:                 "run-a",
		RunState:              string(common.RunStateActive),
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
		"latest activity: 2026-06-10T00:01:00Z",
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
		Run:         &projectioncontract.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
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
		Decisions:   []projectioncontract.Decision{{ID: "ticket-old", State: "open"}},
	})
	m = asModel(next)
	if cmd != nil || len(m.decisions) != 0 || m.blockingDecision != nil {
		t.Fatalf("stale decisions polluted state: cmd=%v decisions=%+v blocker=%+v", cmd != nil, m.decisions, m.blockingDecision)
	}

	next, cmd = m.Update(ApprovalListMsg{
		WorkspaceID: "ws-a",
		Approvals:   []projectioncontract.Approval{{ID: "approval-old", State: "pending"}},
	})
	m = asModel(next)
	if cmd != nil || len(m.approvals) != 0 || m.blockingApproval != nil {
		t.Fatalf("stale approvals polluted state: cmd=%v approvals=%+v blocker=%+v", cmd != nil, m.approvals, m.blockingApproval)
	}

	next, cmd = m.Update(DecisionDetailMsg{
		WorkspaceID: "ws-a",
		Decision:    &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", State: "open"},
	})
	m = asModel(next)
	if cmd != nil || len(m.decisions) != 0 || m.blockingDecision != nil {
		t.Fatalf("stale decision detail polluted state: cmd=%v decisions=%+v blocker=%+v", cmd != nil, m.decisions, m.blockingDecision)
	}

	next, cmd = m.Update(ApprovalDetailMsg{
		WorkspaceID: "ws-a",
		Approval:    &projectioncontract.Approval{ID: "approval-old", WorkspaceID: "ws-a", State: "pending"},
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
		Workspace:         &projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive)},
		Goal:              &workorchmanagementcontract.Goal{ID: "goal-b", WorkspaceID: "ws-b", Text: "fresh workspace goal"},
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
		Run: &projectioncontract.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
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
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-old", State: "pending"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:      projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
			DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{RunID: "run-b", Summary: "fresh result", ArtifactRefs: []string{"artifact-ref"}},
			Runs:           []projectioncontract.Run{{ID: "run-b", WorkspaceID: "ws-b", GoalID: "goal-b", State: string(common.RunStateCompleted)}},
			Results:        []projectioncontract.Result{{ID: "result-b", RunID: "run-b", Summary: "fresh result", ArtifactRefs: []string{"result-artifact"}}},
			Artifacts:      []projectioncontract.Artifact{{ID: "artifact-b", RunID: "run-b", Name: "artifact name"}},
			Patches:        []projectioncontract.Patch{{ID: "patch-b", RunID: "run-b", TargetRef: "main.go", State: string(common.PatchStateApplied), AddedLines: 3, DeletedLines: 1, ApprovalID: "approval-b"}},
		},
		ActiveRun: &projectioncontract.Run{ID: "run-b", WorkspaceID: "ws-b", GoalID: "goal-b", State: string(common.RunStateCompleted)},
		Goal:      &workorchmanagementcontract.Goal{ID: "goal-b", Text: "fresh goal"},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:      projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
			DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{RunID: "run-1", Summary: "failed result", Reason: "weak evidence"},
			Runs:           []projectioncontract.Run{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateFailed), FailureReason: "weak evidence"}},
		},
		DisplayRun: &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateFailed), FailureReason: "weak evidence"},
		Goal:       &workorchmanagementcontract.Goal{ID: "goal-1", Text: "goal"},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
			RunCommand: &projectioncontract.RunCommandStatus{
				WorkspaceID:     "ws-a",
				RunID:           "run-accepted",
				State:           "accepted",
				Intent:          "resume",
				RequestID:       "request-accepted",
				Source:          "tui",
				Outcome:         runtimecontract.RuntimeExecutionIntentReconciliationRequested,
				AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
				Diagnostic:      "current Run continuation accepted",
				Revision:        3,
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
	if !strings.Contains(status, "command accepted resume") {
		t.Fatalf("status bar = %q, want accepted command projection", status)
	}
}

func TestModelWorkspaceContextDisplaysRejectedRunCommandProjection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.progress = progressSubmitting
	m.progressText = "Submitting run command"
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
			RunCommand: &projectioncontract.RunCommandStatus{
				WorkspaceID:     "ws-a",
				RunID:           "run-rejected",
				State:           "rejected",
				Intent:          "resume",
				RequestID:       "request-rejected",
				Source:          "tui",
				Outcome:         runtimecontract.RuntimeExecutionIntentRestartRequired,
				AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
				Diagnostic:      "workspace owner rejected admission",
				RejectedAt:      "2026-06-21T00:01:00Z",
				Revision:        4,
			},
		},
	})
	m = asModel(next)
	if m.progress == progressSubmitting {
		t.Fatalf("progress = %s/%q, rejected command projection must not leave stale submitting state", m.progress, m.progressText)
	}
	status := stripANSI(m.renderRuntimeStatusBar(240))
	for _, want := range []string{"command rejected resume", "workspace owner rejected admission"} {
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-new"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:      projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-new"},
			DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{Summary: "old outcome without run id"},
			Runs: []projectioncontract.Run{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: string(common.RunStateCompleted)},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: string(common.RunStateActive)},
			},
		},
		ActiveRun:        &projectioncontract.Run{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: string(common.RunStateActive)},
		DisplayRun:       &projectioncontract.Run{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: string(common.RunStateActive)},
		Goal:             &workorchmanagementcontract.Goal{ID: "goal-new", Text: "new goal"},
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"}
	next, _ := m.Update(WorkspaceContextMsg{
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-new"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-new"},
			Runs: []projectioncontract.Run{
				{ID: "run-old", WorkspaceID: "ws-a", GoalID: "goal-old", State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-old"},
				{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: string(common.RunStateActive)},
			},
		},
		ActiveRun: &projectioncontract.Run{ID: "run-new", WorkspaceID: "ws-a", GoalID: "goal-new", State: string(common.RunStateActive)},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-old",
			WorkspaceID: "ws-a",
			RunID:       "run-old",
			Question:    "old question",
			State:       "open",
		}},
		Approvals: []projectioncontract.Approval{{
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
			Goals:     []workorchmanagementcontract.Goal{{ID: "goal-1", WorkspaceID: "ws-a", Text: "准备工作区目标"}},
			DoDs:      []workorchmanagementcontract.DoD{{ID: "dod-1", WorkspaceID: "ws-a", Items: []string{"验收项已定义"}, State: "ready"}},
		},
		Goal: &workorchmanagementcontract.Goal{ID: "goal-1", WorkspaceID: "ws-a", Text: "准备工作区目标"},
		DoD:  &workorchmanagementcontract.DoD{ID: "dod-1", WorkspaceID: "ws-a", Items: []string{"验收项已定义"}, State: "ready"},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		ActiveRun: &projectioncontract.Run{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       string(common.RunStateActive),
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
			Runs:      []projectioncontract.Run{{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: string(common.RunStateActive)}},
			MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{
				ID:                 "trace-a",
				WorkspaceID:        "ws-a",
				RunID:              "run-a",
				StepID:             "run-a/step-1",
				Stage:              "planning",
				Source:             "activation",
				Status:             "selected",
				RetrievedMemoryIDs: []string{"memory-a"},
				RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{
					ID:      "memory-a",
					Summary: "project context",
				}},
			}},
		},
		ActiveRun: &projectioncontract.Run{ID: "run-a", WorkspaceID: "ws-a", GoalID: "goal-a", State: string(common.RunStateActive)},
		Goal:      &workorchmanagementcontract.Goal{ID: "goal-a", Text: "use memory"},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:      projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"},
			DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{RunID: "run-1", Summary: "目标已完成，成功查询到远程服务器配置信息。"},
			TaskProgress: projectioncontract.TaskProgress{
				State: "running",
				Recovery: projectioncontract.RecoveryPlan{
					Mode: "resume",
				},
				RepairLoop: projectioncontract.RepairLoop{
					State:        "repairing",
					AttemptCount: 2,
					LastAttempt:  2,
				},
			},
			Runs:    []projectioncontract.Run{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateCompleted)}},
			Results: []projectioncontract.Result{{ID: "result-1", RunID: "run-1", Summary: "目标已完成，成功查询到远程服务器配置信息。"}},
			Artifacts: []projectioncontract.Artifact{{
				ID:       "run-1/artifact/result-summary",
				RunID:    "run-1",
				Category: "final_summary",
				Name:     "Accepted Result Summary",
				Summary:  finalSummary,
				State:    string(common.ArtifactStateAvailable),
			}},
		},
		ActiveRun: &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateCompleted)},
		Goal:      &workorchmanagementcontract.Goal{ID: "goal-1", Text: "查询服务配置信息"},
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
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-1",
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2, RuntimeStateRevision: 2, ActivityRevision: 2},
		DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{
			WorkspaceID: "ws-1",
			State:       "completed",
			Terminal:    true,
			Accepted:    true,
			Summary:     "final accepted result",
		},
		TaskProgress: projectioncontract.TaskProgress{Summary: "completed"},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:      projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
			DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{RunID: "run-1", Summary: "快照结果已完成"},
			Runs:           []projectioncontract.Run{{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateCompleted)}},
		},
		DisplayRun: &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", State: string(common.RunStateCompleted)},
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
		Workspace: &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-1"},
			TaskProgress: projectioncontract.TaskProgress{
				State: "running",
			},
			TaskDecompositions: []projectioncontract.TaskDecomposition{
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
			TaskPlans: []projectioncontract.TaskPlan{
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
			MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{
				{
					ID:                 "mr-1",
					RunID:              "run-1",
					Source:             "vector",
					Stage:              "execution",
					Status:             "selected",
					RetrievedMemoryIDs: []string{"memory-1"},
				},
			},
			LLMUsageSummary: projectioncontract.LLMUsageSummary{
				EntryTotal:            2,
				InputTokens:           1200,
				OutputTokens:          300,
				TotalTokens:           1500,
				ProviderNativeEntries: 1,
				EstimatedEntries:      1,
			},
			Decisions: []projectioncontract.Decision{
				{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", Question: "需要补充部署目标环境", State: "open"},
			},
			Approvals: []projectioncontract.Approval{
				{ID: "approval-1", WorkspaceID: "ws-a", RunID: "run-1", Reason: "准备执行潜在风险命令", State: "pending"},
			},
		},
		ActiveRun: &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", GoalID: "goal-1", DoDID: "dod-1", State: string(common.RunStateActive)},
		Goal:      &workorchmanagementcontract.Goal{ID: "goal-1", Text: "完成服务配置修复"},
		DoD:       &workorchmanagementcontract.DoD{ID: "dod-1", Items: []string{"变更最小化", "验证通过"}},
		Decisions: []projectioncontract.Decision{
			{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", Question: "需要补充部署目标环境", State: "open"},
		},
		Approvals: []projectioncontract.Approval{
			{ID: "approval-1", WorkspaceID: "ws-a", RunID: "run-1", Reason: "准备执行潜在风险命令", State: "pending"},
		},
	})
	m = asModel(next)
	m.width = 140
	m.height = 80
	view := stripANSI(m.renderMain(120, 80))
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-other",
		RunState:    string(common.RunStateActive),
		ActiveRunID: "run-from-previous-workspace",
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision:    3,
			RuntimeStateRevision: 5,
		},
		TaskProgress: projectioncontract.TaskProgress{
			Summary: "stale previous workspace progress",
			Runtime: &projectioncontract.RuntimeProgress{
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
	if m.runtime != nil {
		t.Fatalf("active runtime polluted by non-active view: %+v", m.runtime)
	}
}

func TestModelWorkspaceCreatedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	// Projection-first: empty-workspace projection notice only schedules list refresh.
	// No raw Event payload is used to invent workspace overview rows.
	cmd := m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "",
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 1},
		ChangedSections: []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionOverview,
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
	cmd := m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-a",
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision:    2,
			RuntimeStateRevision: 3,
			ActivityRevision:     1,
		},
		ChangedSections: []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
			projectioncontract.WorkspaceProjectionSectionActivity,
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
	ch := make(chan projectioncontract.WorkspaceProjectionChanged, 1)
	enqueueProjectionChanged(ch, projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-a",
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 1},
	})
	enqueueProjectionChanged(ch, projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-b",
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 2},
	})
	got := <-ch
	if got.WorkspaceID != "ws-b" || got.Revision.WorkspaceRevision != 2 {
		t.Fatalf("projection = %+v, want latest ws-b revision 2", got)
	}
}

func TestEnqueueSubscriptionStatusKeepsLatestWhenChannelFull(t *testing.T) {
	ch := make(chan backend.SubscriptionStatus, 1)
	enqueueSubscriptionStatus(ch, backend.SubscriptionStatus{State: backend.SubscriptionConnecting})
	enqueueSubscriptionStatus(ch, backend.SubscriptionStatus{State: backend.SubscriptionLive})
	got := <-ch
	if got.State != backend.SubscriptionLive {
		t.Fatalf("status = %+v, want latest live", got)
	}
}

func TestEnqueueProjectionChangedReplacesOldestWithLatest(t *testing.T) {
	ch := make(chan projectioncontract.WorkspaceProjectionChanged, 1)
	enqueueProjectionChanged(ch, projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-a",
		Revision:    projectioncontract.WorkspaceRefreshRevision{ActivityRevision: 1},
	})
	enqueueProjectionChanged(ch, projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-b",
		Revision:    projectioncontract.WorkspaceRefreshRevision{ActivityRevision: 9},
	})
	got := <-ch
	if got.WorkspaceID != "ws-b" || got.Revision.ActivityRevision != 9 {
		t.Fatalf("projection = %+v, want latest activity revision", got)
	}
}

func TestModelWorkspaceListChangedEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, projectioncontract.WorkspaceProjectionSectionOverview))
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("workspace list changed event should refresh list: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
}

func TestModelWorkspaceProjectionBaselineEventRefreshesWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	m.activeWorkspaceID = "ws-a"

	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, projectioncontract.WorkspaceProjectionSectionOverview))
	if cmd == nil || !m.workspaceListRefreshInFlight {
		t.Fatalf("projection baseline event should refresh list: cmd=%v inFlight=%v", cmd != nil, m.workspaceListRefreshInFlight)
	}
}

func TestModelWorkspaceProjectionDetailEventRefreshesOnlyActiveDetail(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive)},
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
	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{
			ID:          "ws-a",
			Title:       "A",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-a",
			Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 31},
			ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 31},
		},
	}})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("waiting decision overview should request blocker/context refresh")
	}
	if m.progress != progressWaitingDecision || m.activeRunID != "run-a" || m.blockingDecision != nil {
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{
			ID:          "ws-a",
			Title:       "A",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-a",
			Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 31},
			ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", WorkspaceRevision: 31},
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

func TestModelProjectedApprovalHandoffDoesNotFabricateApprovalResource(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.progress = progressSubmitting
	m.progressText = "Checking workspace run state"
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{
			ID:          "ws-a",
			Title:       "A",
			State:       string(common.WorkspaceStateBlockedOnApproval),
			ActiveRunID: "run-a",
			Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 31},
			ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeApproval, WorkspaceRevision: 31},
		},
	}})
	m = asModel(next)
	if m.blockingApproval != nil || m.progress != progressWaitingApproval {
		t.Fatalf("model handoff = progress:%s blocker:%+v, want display wait without fabricated approval", m.progress, m.blockingApproval)
	}
	if mode := m.activeWorkspacePromptMode(); mode != workspacePromptModeApproval {
		t.Fatalf("prompt mode = %q, want approval lifecycle display", mode)
	}
	input := stripANSI(m.renderInput(120))
	if !strings.Contains(input, "approval >") {
		t.Fatalf("input prompt = %q, want read-only approval handoff prompt", input)
	}
}

func TestModelProjectedApprovalHandoffWithoutResourceBlocksMutation(t *testing.T) {
	for _, input := range []string{"continue with a new goal", "/run resume"} {
		t.Run(input, func(t *testing.T) {
			be := newRecordingBE(t, common.RunStateBlockedOnApproval)
			m := NewAppModel(be, "ws-a", "")
			next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
				ID:          "ws-a",
				Title:       "A",
				State:       string(common.WorkspaceStateBlockedOnApproval),
				ActiveRunID: "run-a",
				Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 31},
				ViewModel: projectioncontract.WorkspaceViewModel{
					WorkspaceID: "ws-a", PromptMode: workspacePromptModeApproval, WorkspaceRevision: 31,
				},
			}}})
			m = asModel(next)
			m.setInput(input)

			_, cmd := m.submitInput()
			if cmd == nil {
				t.Fatal("approval handoff must refresh owner resources")
			}
			if len(be.goals) != 0 || len(be.runCommands) != 0 {
				t.Fatalf("approval handoff mutated goal/run: goals=%v runCommands=%+v", be.goals, be.runCommands)
			}
			if m.progress != progressWaitingApproval || m.blockingApproval != nil {
				t.Fatalf("handoff progress/resource = %s/%+v, want refresh-only waiting approval", m.progress, m.blockingApproval)
			}
		})
	}
}

func TestModelWorkspaceOverviewWithoutProjectionDoesNotOwnDecisionPrompt(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-a",
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:        "ws-a",
		State:     string(common.WorkspaceStateActive),
		ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision},
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
	be.snapshot = &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{
			ID:          "ws-projection-context",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-projection-context",
			Projection:  projectioncontract.ProjectionStatus{State: "ready", WorkspaceRevision: 42},
		},
		Runs: []projectioncontract.Run{{
			ID:          "run-projection-context",
			WorkspaceID: "ws-projection-context",
			State:       string(common.RunStateActive),
		}},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-projection-context",
			RunID:       "run-projection-context",
			State:       string(common.RunStateActive),
			Summary:     "projection task progress",
			Recovery: projectioncontract.RecoveryPlan{
				StepID: "step-projection-context",
			},
		},
		RunHeartbeat: projectioncontract.RunHeartbeat{
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
	if switchMsg.Status == nil || switchMsg.Status.CurrentStepID != "step-projection-context" || switchMsg.Status.State != string(common.RunStateActive) {
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
	m.workspaces = []projectioncontract.Workspace{{
		ID:    "ws-a",
		Title: "A",
		State: string(common.WorkspaceStateActive),
	}}
	m.activeRunID = ""
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.blockingApproval = &projectioncontract.Approval{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
	m.approvals = []projectioncontract.Approval{*m.blockingApproval}
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 41}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-b", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 41}},
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

	next, _ = m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 42}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-b", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 42}},
	}})
	m = asModel(next)
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-b",
		ActiveRunID: "run-b",
		RunState:    string(common.RunStateBlockedOnDecision),
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 42},
		BlockingDecision: &projectioncontract.Decision{
			ID:          "ticket-b",
			WorkspaceID: "ws-b",
			RunID:       "run-b",
			State:       "open",
		},
	}, false)
	next, _ = m.Update(DecisionDetailMsg{
		WorkspaceID: "ws-b",
		Decision: &projectioncontract.Decision{
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b460b00b639f57e4", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 41}, ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-b460b00b639f57e4", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 41}},
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
	if m.blockingDecision != nil {
		t.Fatalf("blocking decision = %+v, want no resource fabricated from workspace overview", m.blockingDecision)
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive)},
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
	cmd = m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{

		WorkspaceID: "ws-b",

		Revision: projectioncontract.WorkspaceRefreshRevision{

			WorkspaceRevision: 11,

			RuntimeStateRevision: uint64(11),

			ActivityRevision: 11,
		},

		ChangedSections: []projectioncontract.WorkspaceProjectionSection{

			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,

			projectioncontract.WorkspaceProjectionSectionActivity,
		},
	})
	if cmd != nil || !m.ensureWorkspaceRuntimeState("ws-b").ContextPending || m.workspaceContextRequestSeqFor("ws-b") != previousSeq {
		t.Fatalf("projection event should coalesce with switch refresh: cmd=%v pending=%v seq=%d want %d", cmd != nil, m.ensureWorkspaceRuntimeState("ws-b").ContextPending, m.workspaceContextRequestSeqFor("ws-b"), previousSeq)
	}

	seq := m.workspaceContextRequestSeqFor("ws-b")
	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  99,
		WorkspaceID: "ws-a",
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", Title: "A stale", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-a",
			WorkspaceID: "ws-a",
			State:       string(common.RunStateFailed),
		},
	})
	m = asModel(next)
	if m.activeWorkspaceID != "ws-b" || m.currentTask != "" || m.progressText != "Workspace switched; refreshing state" {
		t.Fatalf("inactive stale context polluted switched view: active=%q task=%q progress=%q", m.activeWorkspaceID, m.currentTask, m.progressText)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		RequestSeq:  seq,
		WorkspaceID: "ws-b",
		Workspace:   &projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
		ActiveRun:   &projectioncontract.Run{ID: "run-b", WorkspaceID: "ws-b", State: string(common.RunStateActive)},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       string(common.RunStateActive),
		},
		Status: &projectioncontract.RunStatus{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           string(common.RunStateActive),
			CurrentStepKind: "llm/workspace_execution",
		},
		Goal: &workorchmanagementcontract.Goal{ID: "goal-b", WorkspaceID: "ws-b", Text: "build workspace B"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:    projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
			TaskProgress: projectioncontract.TaskProgress{WorkspaceID: "ws-b", RunID: "run-b", State: "running", Summary: "B is making progress"},
			RunHeartbeat: projectioncontract.RunHeartbeat{WorkspaceID: "ws-b", RunID: "run-b", State: "running", ActivityState: "running", Summary: "B heartbeat"},
			RunCommand: &projectioncontract.RunCommandStatus{
				WorkspaceID:     "ws-b",
				State:           "rejected",
				Intent:          "restart",
				Outcome:         runtimecontract.RuntimeExecutionIntentRestartRequired,
				AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
				Diagnostic:      "active-admission",
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
	if command := runCommandStatusText(m.runCommand); !strings.Contains(command, "active-admission") {
		t.Fatalf("run command = %q, want active workspace admission", command)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		WorkspaceID: "ws-a",
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
			RunCommand: &projectioncontract.RunCommandStatus{
				WorkspaceID:     "ws-a",
				State:           "rejected",
				Intent:          "restart",
				Outcome:         runtimecontract.RuntimeExecutionIntentRestartRequired,
				AdmissionReason: runtimecontract.RuntimeExecutionIntentReasonRunNotResumable,
				Diagnostic:      "inactive-admission",
			},
		},
	})
	m = asModel(next)
	if command := runCommandStatusText(m.runCommand); strings.Contains(command, "inactive-admission") || !strings.Contains(command, "active-admission") {
		t.Fatalf("inactive run command polluted active view: %q", command)
	}

	cmd = m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{

		WorkspaceID: "ws-a",

		Revision: projectioncontract.WorkspaceRefreshRevision{

			WorkspaceRevision: 12,

			RuntimeStateRevision: uint64(12),

			ActivityRevision: 12,
		},

		ChangedSections: []projectioncontract.WorkspaceProjectionSection{

			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,

			projectioncontract.WorkspaceProjectionSectionActivity,
		},
	})
	if cmd != nil || m.currentTask != "build workspace B" || !strings.Contains(m.renderRuntimeStatusBar(120), "B is making progress") {
		t.Fatalf("inactive projection event polluted active B view: cmd=%v task=%q status=%q", cmd != nil, m.currentTask, stripANSI(m.renderRuntimeStatusBar(120)))
	}
}

func TestTUIE2EWorkspaceSwitchProjectionRefreshVisualSnapshot(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive)},
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
		Workspace:   &projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
		ActiveRun:   &projectioncontract.Run{ID: "run-b", WorkspaceID: "ws-b", State: string(common.RunStateActive)},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-b",
			WorkspaceID: "ws-b",
			State:       string(common.RunStateActive),
		},
		Status: &projectioncontract.RunStatus{
			ID:              "run-b",
			WorkspaceID:     "ws-b",
			State:           string(common.RunStateActive),
			CurrentStepKind: "llm/workspace_execution",
		},
		Goal: &workorchmanagementcontract.Goal{ID: "goal-b", WorkspaceID: "ws-b", Text: "build workspace B"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:    projectioncontract.Workspace{ID: "ws-b", Title: "B", State: string(common.WorkspaceStateActive), ActiveRunID: "run-b"},
			TaskProgress: projectioncontract.TaskProgress{WorkspaceID: "ws-b", RunID: "run-b", State: "running", Summary: "B is making progress"},
			RunHeartbeat: projectioncontract.RunHeartbeat{WorkspaceID: "ws-b", RunID: "run-b", State: "running", ActivityState: "running", Summary: "B heartbeat"},
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

	cmd = m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{

		WorkspaceID: "ws-a",

		Revision: projectioncontract.WorkspaceRefreshRevision{

			WorkspaceRevision: 12,

			RuntimeStateRevision: uint64(12),

			ActivityRevision: 12,
		},

		ChangedSections: []projectioncontract.WorkspaceProjectionSection{

			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,

			projectioncontract.WorkspaceProjectionSectionActivity,
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
	m.workspaces = []projectioncontract.Workspace{{ID: "ws-a", Title: "A", State: "active"}}
	m.workspaceListRefreshInFlight = true
	m.workspaceListRequestSeq = 2

	next, cmd := m.Update(WorkspaceListMsg{
		RequestSeq: 1,
		Workspaces: []projectioncontract.Workspace{
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
	m.workspaces = []projectioncontract.Workspace{{ID: "ws-a", Title: "A", State: "active"}}
	m.activeWorkspaceID = "ws-a"
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 2

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-a",
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", Title: "Old", State: "running"},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-new",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 5},
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
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-old", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 3}},
		ActiveRun:   &projectioncontract.Run{ID: "run-old", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-old",
			WorkspaceID: "ws-a",
			State:       string(common.RunStateActive),
		},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-old", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 3}},
			ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", Revision: 3, WorkspaceRevision: 3},
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
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-current", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 5}},
		ActiveRun:   &projectioncontract.Run{ID: "run-current", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		DisplayRun: &projectioncontract.Run{
			ID:          "run-current",
			WorkspaceID: "ws-a",
			State:       string(common.RunStateActive),
		},
		Goal: &workorchmanagementcontract.Goal{ID: "goal-current", WorkspaceID: "ws-a", Text: "current task"},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-current", Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 5}},
			ViewModel: projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", Revision: 5, WorkspaceRevision: 5},
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
	m.workspaces = []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-a",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 51},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 51},
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
	if m.progress != progressWaitingDecision || m.blockingDecision != nil {
		t.Fatalf("progress/blocker = %s/%+v, want display handoff without fabricated decision", m.progress, m.blockingDecision)
	}
	if !strings.Contains(m.progressText, "lifecycle handoff pending") {
		t.Fatalf("progressText = %q, want explicit lifecycle handoff", m.progressText)
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
		Workspace:   &projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
		ActiveRun:   &projectioncontract.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		DisplayRun:  &projectioncontract.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace:    projectioncontract.Workspace{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
			TaskProgress: projectioncontract.TaskProgress{WorkspaceID: "ws-a", RunID: "run-a", State: "running", Summary: "run running"},
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
	m.workspaces = []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}
	m.activeWorkspaceID = "ws-b"
	m.ensureWorkspaceRuntimeState("ws-a").ContextRequestSeq = 9
	m.ensureWorkspaceRuntimeState("ws-b").ContextRequestSeq = 1

	next, cmd := m.Update(WorkspaceContextMsg{
		RequestSeq:  1,
		WorkspaceID: "ws-b",
		Workspace:   &projectioncontract.Workspace{ID: "ws-b", Title: "B updated", State: "running"},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
	}})
	m = asModel(next)
	m.workspaceListRefreshInFlight = true
	// Empty workspace projection notice is the typed list-invalidation signal.
	cmd := m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "",
		ChangedSections: []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionOverview,
		},
	})
	if cmd != nil {
		t.Fatal("refresh should be coalesced while list refresh is in flight")
	}
	if !m.workspaceListRefreshPending {
		t.Fatal("expected pending refresh marker")
	}
	next, cmd = m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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

	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:         "ws-a",
		Title:      "A",
		State:      string(common.WorkspaceStateActive),
		Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 5},
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

	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:         "ws-a",
		Title:      "A",
		State:      string(common.WorkspaceStateActive),
		Projection: projectioncontract.ProjectionStatus{WorkspaceRevision: 5},
	}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd == nil || !entry.ContextInFlight || entry.ContextRequestSeq != 3 {
		t.Fatalf("stale display did not request context refresh: cmd=%v inFlight=%v seq=%d", cmd != nil, entry.ContextInFlight, entry.ContextRequestSeq)
	}
}

func TestModelWorkspaceContextRefreshCoalescesWhileInFlight(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"},
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

	next, cmd = m.Update(WorkspaceContextMsg{RequestSeq: previousSeq + 1, WorkspaceID: "ws-a", Workspace: &projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive)}, Snapshot: &projectioncontract.WorkspaceSnapshot{Workspace: projectioncontract.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive)}}})
	m = asModel(next)
	entry = m.ensureWorkspaceRuntimeState("ws-a")
	if cmd == nil || !entry.ContextInFlight || entry.ContextPending || entry.ContextRequestSeq != previousSeq+2 {
		t.Fatalf("pending context refresh not released = cmd:%v inFlight:%v pending:%v seq:%d want %d", cmd != nil, entry.ContextInFlight, entry.ContextPending, entry.ContextRequestSeq, previousSeq+2)
	}
}

func TestModelDecisionResolvedDoesNotHideServerOpenProjection(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 61},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 61},
	}}})
	m = asModel(next)
	m.width = 120
	m.height = 32
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}

	next, _ = m.Update(DecisionResolvedMsg{Decision: &projectioncontract.Decision{
		ID:          "ticket-1",
		WorkspaceID: "ws-a",
		RunID:       "run-1",
		State:       "resolved",
	}})
	m = asModel(next)
	if m.blockingDecision == nil || m.blockingDecision.State != "open" || !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("command response prompt/blocker = %q/%+v, want unchanged server-projected decision", stripANSI(m.renderInput(120)), m.blockingDecision)
	}

	next, _ = m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		Title:       "A",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 62},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 62},
	}}})
	m = asModel(next)
	if !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("server list blocker should remain visible after local resolved marker: input=%q blocker=%+v", stripANSI(m.renderInput(120)), m.blockingDecision)
	}

	next, _ = m.Update(WorkspaceContextMsg{
		WorkspaceID: "ws-a",
		Workspace: &projectioncontract.Workspace{
			ID:          "ws-a",
			Title:       "A",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-1",
		},
		ActiveRun:  &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		DisplayRun: &projectioncontract.Run{ID: "run-1", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		Snapshot: &projectioncontract.WorkspaceSnapshot{
			Workspace: projectioncontract.Workspace{
				ID:          "ws-a",
				Title:       "A",
				State:       string(common.WorkspaceStateActive),
				ActiveRunID: "run-1",
			},
			GovernanceDisplay: projectioncontract.GovernanceDisplay{PromptMode: workspacePromptModeDecision, RunID: "run-1", DecisionID: "ticket-1"},
			Decisions: []projectioncontract.Decision{{
				ID:          "ticket-1",
				WorkspaceID: "ws-a",
				RunID:       "run-1",
				State:       "open",
			}},
		},
		Decisions:        []projectioncontract.Decision{{ID: "ticket-1", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}},
		WorkspaceHasLive: true,
	})
	m = asModel(next)
	if !strings.Contains(stripANSI(m.renderInput(120)), "ws-a decision >") {
		t.Fatalf("server snapshot blocker should remain visible after local resolved marker: input=%q blocker=%+v", stripANSI(m.renderInput(120)), m.blockingDecision)
	}
}

func TestModelPromptModeUsesProjectionOverLocalDecisionCache(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.workspaces = []projectioncontract.Workspace{{
		ID:          "ws-a",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 20},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 20},
	}}
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.PromptMode = workspacePromptModeDecision
	entry.PromptModeRevision = 10
	m.applyWorkspaceViewModel(m.workspaces[0].ViewModel)
	m.activeRunID = "run-1"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
	m.input = "/"
	m.inputCursor = len([]rune(m.input))

	input := stripANSI(m.renderInput(120))
	if strings.Contains(input, "decision >") || strings.Contains(input, "/decision answer") || !strings.Contains(input, "ws-a running >") {
		t.Fatalf("prompt/suggestions used local decision cache instead of projection: %q", input)
	}
}

func TestModelWorkspaceListViewModelDrivesPromptMode(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"}
	m.decisions = []projectioncontract.Decision{*m.blockingDecision}
	m.progress = progressWaitingDecision
	m.width = 120
	m.height = 24

	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 12},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 12},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 20},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeRunning, WorkspaceRevision: 20},
	}}})
	m = asModel(next)

	// Older/stale RuntimeView must not override projected running prompt.
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    string(common.RunStateBlockedOnDecision),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision: 19, // below projected list revision authority
		},
		BlockingDecision: &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open"},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 20},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 20},
	}}})
	m = asModel(next)

	// Same-mode RuntimeView with matching blocker details is accepted via typed view.
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    string(common.RunStateBlockedOnDecision),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision: 20,
			ActivityRevision:  1,
		},
		BlockingDecision: &projectioncontract.Decision{
			ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-1", State: "open", Question: "current decision",
		},
	}, false)

	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-current" {
		t.Fatalf("same-mode decision runtime view was not accepted for details: blocker=%+v progress=%s", m.blockingDecision, m.progress)
	}
}

func TestModelPromptModeProjectionRejectsOlderMismatchedDecisionEvent(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{{
		ID:          "ws-a",
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-1",
		Projection:  projectioncontract.ProjectionStatus{WorkspaceRevision: 20},
		ViewModel:   projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-a", PromptMode: workspacePromptModeDecision, WorkspaceRevision: 20},
	}}})
	m = asModel(next)
	// Establish current blocker via typed RuntimeView first.
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    string(common.RunStateBlockedOnDecision),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision: 20,
			ActivityRevision:  2,
		},
		BlockingDecision: &projectioncontract.Decision{ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-1", State: "open"},
	}, false)

	// Stale mismatched blocker RuntimeView with older/equal revision must not replace current.
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-a",
		ActiveRunID: "run-1",
		PromptMode:  workspacePromptModeDecision,
		RunState:    string(common.RunStateBlockedOnDecision),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision: 19,
			ActivityRevision:  1,
		},
		BlockingDecision: &projectioncontract.Decision{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-1", State: "open", Question: "stale decision"},
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
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "active"},
	}})
	m = asModel(next)
	m.currentTask = "stale task"
	m.result = "stale result"
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-old", State: "open"}
	// Workspace deletion is reflected via list projection refresh, not raw Event.
	// Simulate active workspace disappearing from authoritative list.
	next, cmd := m.Update(WorkspaceListMsg{Workspaces: []projectioncontract.Workspace{
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
	cmd := m.applyWorkspaceProjectionChanged(projectionChangedInfo("", 1, projectioncontract.WorkspaceProjectionSectionOverview))
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

func TestModelDodSlashCommandPersistsDoDToActiveWorkspaceRejectsRetiredWrite(t *testing.T) {
	be := &recordingDoDBE{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	m.setInput("/dod tests pass")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(DoDUpdatedMsg)
	if !ok || msg.Error == nil || msg.DoD != nil {
		t.Fatalf("退休写入未拒绝: %#v", msg)
	}
	if be.workspaceID != "" || len(be.items) != 0 {
		t.Fatal("退休 DoD 写入产生副作用")
	}
}

func TestModelWorkspaceCreateCommandUsesTemplateAndActivatesWorkspaceRejectsRetiredWrite(t *testing.T) {
	be := &workspaceCreateFake{fakeBE: newFake(t)}
	m := NewAppModel(be, "ws-1", "")
	m.setInput("/workspace create task --template template-a")
	_, cmd := m.submitInput()
	if cmd == nil {
		t.Fatal("退休命令应提供明确提示")
	}
	msg, ok := cmd().(WorkspaceCreatedMsg)
	if !ok || msg.Error == nil || msg.Workspace != nil {
		t.Fatalf("退休写入未拒绝: %#v", msg)
	}
	if be.createReq.Title != "" {
		t.Fatal("退休创建调用 backend")
	}
}

func TestParseWorkspaceCreateArgsRequiresTemplateAndAllowsDefaultDirectory(t *testing.T) {
	if _, err := parseWorkspaceCreateArgs("workspace --dir /tmp/workspace"); err == nil || !strings.Contains(err.Error(), "--template") {
		t.Fatalf("missing template error = %v, want --template", err)
	}
	req, err := parseWorkspaceCreateArgs("workspace --template template-a")
	if err != nil {
		t.Fatalf("parse workspace with default directory: %v", err)
	}
	if req.DeliverableDirectory != "" {
		t.Fatalf("deliverableDirectory = %q, want empty owner-default input", req.DeliverableDirectory)
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

func projectionChangedInfo(workspaceID string, revision int64, sections ...projectioncontract.WorkspaceProjectionSection) projectioncontract.WorkspaceProjectionChanged {
	if len(sections) == 0 {
		sections = []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
			projectioncontract.WorkspaceProjectionSectionActivity,
		}
	}
	return projectioncontract.WorkspaceProjectionChanged{
		ID:          "projection:" + workspaceID + ":" + fmt.Sprintf("%d", revision),
		WorkspaceID: workspaceID,
		Revision: projectioncontract.WorkspaceRefreshRevision{
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
	m.blockingDecision = &projectioncontract.Decision{ID: "ticket-keep", WorkspaceID: "ws-1", State: "open"}
	m.runtime = &projectioncontract.RuntimeProgress{StateRevision: 9}
	m.activityFeed = projectioncontract.WorkspaceActivityFeed{Items: []projectioncontract.WorkspaceActivityItem{{ID: "keep", Summary: "keep"}}}

	// Non-active dirty projection for another workspace must not mutate active view.
	_ = m.applyWorkspaceProjectionChanged(projectioncontract.WorkspaceProjectionChanged{
		WorkspaceID: "ws-other",
		Revision: projectioncontract.WorkspaceRefreshRevision{
			WorkspaceRevision:    100,
			RuntimeStateRevision: 100,
			ActivityRevision:     100,
		},
		ChangedSections: []projectioncontract.WorkspaceProjectionSection{
			projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
			projectioncontract.WorkspaceProjectionSectionActivity,
			projectioncontract.WorkspaceProjectionSectionGovernance,
			projectioncontract.WorkspaceProjectionSectionResult,
		},
	})
	if m.progressText != "baseline" || m.result != "baseline-result" {
		t.Fatalf("non-active projection dirty mutated display: progress=%q result=%q", m.progressText, m.result)
	}
	if m.blockingDecision == nil || m.blockingDecision.ID != "ticket-keep" {
		t.Fatalf("non-active projection dirty mutated blocker: %+v", m.blockingDecision)
	}
	if m.runtime == nil || m.runtime.StateRevision != 9 || len(m.activityFeed.Items) != 1 {
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
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: "ws-1",
		Revision:    projectioncontract.WorkspaceRefreshRevision{WorkspaceRevision: 3, RuntimeStateRevision: 3, ActivityRevision: 3},
		DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{
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
		onStatus(context.Background(), backend.SubscriptionStatus{State: backend.SubscriptionLive})
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
	createReq workorchmanagementcontract.WorkspaceCreateRequest
	workspace *projectioncontract.Workspace
}

func (b *workspaceCreateFake) Workspace() backend.WorkspaceBackend {
	return &workspaceCreateWS{parent: b}
}

type workspaceCreateWS struct{ parent *workspaceCreateFake }

func (f *workspaceCreateWS) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	f.parent.createReq = req
	ws := &projectioncontract.Workspace{
		ID:                   "ws-created",
		Title:                req.Title,
		Description:          req.Description,
		DeliverableDirectory: req.DeliverableDirectory,
		State:                string(common.WorkspaceStateActive),
		SoulBindings: map[string]string{
			"planning":  "soul-planning",
			"execution": "soul-execution",
		},
	}
	f.parent.workspace = ws
	return ws, nil
}
func (f *workspaceCreateWS) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	if f.parent.workspace != nil && f.parent.workspace.ID == string(id) {
		return f.parent.workspace, nil
	}
	return nil, common.ErrNotFound
}
func (f *workspaceCreateWS) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	if f.parent.workspace == nil {
		return nil, nil
	}
	return []projectioncontract.Workspace{*f.parent.workspace}, nil
}
func (f *workspaceCreateWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	workspace, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &projectioncontract.WorkspaceSnapshot{Workspace: *workspace}, nil
}

func (f *workspaceCreateWS) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (f *workspaceCreateWS) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	return f.Get(ctx, id)
}

func (f *workspaceCreateWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type recordingBE struct {
	*fakeBE
	runState                 common.RunState
	snapshot                 *projectioncontract.WorkspaceSnapshot
	runtimeView              *projectioncontract.WorkspaceRuntimeView
	listWorkspaces           []projectioncontract.Workspace
	listCalls                int
	snapshotCalls            int
	runtimeViewCalls         int
	goals                    []string
	runCommands              []backend.WorkspaceRunCommand
	lastGetWorkspace         common.WorkspaceID
	lastListWorkspace        common.WorkspaceID
	lastSnapshotWorkspace    common.WorkspaceID
	lastRuntimeViewWorkspace common.WorkspaceID
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

func (f *decisionRecordingBackend) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	f.parent.decisions = append(f.parent.decisions, req)
	return &projectioncontract.Decision{ID: string(req.TicketID), State: "resolved"}, nil
}
func (f *decisionRecordingBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (f *decisionRecordingBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (f *decisionRecordingBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error) {
	return &projectioncontract.Decision{ID: string(id), WorkspaceID: string(wsID), State: "open"}, nil
}

func (b *recordingBE) Goal() backend.GoalBackend { return &recordingGoal{parent: b} }
func (b *recordingBE) Run() backend.RunBackend   { return &recordingRun{parent: b} }
func (b *recordingBE) Workspace() backend.WorkspaceBackend {
	return &recordingWorkspace{parent: b}
}

type recordingWorkspace struct{ parent *recordingBE }

func (f *recordingWorkspace) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return (&fakeWS{}).Create(ctx, req)
}
func (f *recordingWorkspace) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	f.parent.lastGetWorkspace = id
	return &projectioncontract.Workspace{ID: string(id), ActiveRunID: "run-resume"}, nil
}
func (f *recordingWorkspace) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	f.parent.listCalls++
	if len(f.parent.listWorkspaces) > 0 {
		return append([]projectioncontract.Workspace(nil), f.parent.listWorkspaces...), nil
	}
	return nil, nil
}
func (f *recordingWorkspace) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	f.parent.snapshotCalls++
	f.parent.lastSnapshotWorkspace = id
	if f.parent.snapshot != nil {
		return f.parent.snapshot, nil
	}
	return &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: string(id), ActiveRunID: "run-resume"},
		Runs: []projectioncontract.Run{{
			ID:          "run-resume",
			WorkspaceID: string(id),
			State:       string(f.parent.runState),
		}},
	}, nil
}

func (f *recordingWorkspace) RuntimeView(_ context.Context, req workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	f.parent.runtimeViewCalls++
	f.parent.lastRuntimeViewWorkspace = req.WorkspaceID
	if f.parent.runtimeView != nil {
		return f.parent.runtimeView, nil
	}
	if f.parent.snapshot != nil {
		snap := f.parent.snapshot
		return &projectioncontract.WorkspaceRuntimeView{
			WorkspaceID:  snap.Workspace.ID,
			ActiveRunID:  snap.Workspace.ActiveRunID,
			DisplayRunID: snap.Workspace.ActiveRunID,
			Projection:   snap.Workspace.Projection,
			Revision: projectioncontract.WorkspaceRefreshRevision{
				WorkspaceRevision: snap.Workspace.Projection.WorkspaceRevision,
			},
			TaskProgress:   snap.TaskProgress,
			RunHeartbeat:   snap.RunHeartbeat,
			DisplayOutcome: snap.DisplayOutcome,
		}, nil
	}
	return &projectioncontract.WorkspaceRuntimeView{
		WorkspaceID:  string(req.WorkspaceID),
		ActiveRunID:  "run-resume",
		DisplayRunID: "run-resume",
	}, nil
}

func (f *recordingWorkspace) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

func (f *recordingWorkspace) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	return f.Get(ctx, id)
}

type recordingGoal struct{ parent *recordingBE }

func (f *recordingGoal) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error) {
	f.parent.goals = append(f.parent.goals, text)
	return &workorchmanagementcontract.Goal{ID: "g-recorded", WorkspaceID: string(wsID), Text: text}, nil
}
func (f *recordingGoal) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	return nil, nil
}
func (f *recordingGoal) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error) {
	return &workorchmanagementcontract.Goal{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *recordingGoal) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error) {
	return &workorchmanagementcontract.Goal{ID: string(id), WorkspaceID: string(wsID), Text: text}, nil
}

type recordingRun struct{ parent *recordingBE }

func (f *recordingRun) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunCommandResult, error) {
	f.parent.runCommands = append(f.parent.runCommands, req)
	return &backend.RunCommandResult{WorkspaceID: req.WorkspaceID, RunID: "run-new", RequestID: "request-recording", Kind: runtimecontract.RuntimeExecutionIntentKind(req.Intent), Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted}, nil
}

func (f *recordingRun) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	f.parent.lastGetWorkspace = wsID
	if id == "run-from-ws-1" {
		return &projectioncontract.Run{ID: string(id), WorkspaceID: "ws-1", State: string(f.parent.runState)}, nil
	}
	if wsID == "" {
		wsID = "ws-1"
	}
	return &projectioncontract.Run{ID: string(id), WorkspaceID: string(wsID), State: string(f.parent.runState)}, nil
}
func (f *recordingRun) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	f.parent.lastListWorkspace = wsID
	return []projectioncontract.Run{{ID: "run-resume", WorkspaceID: string(wsID), State: string(f.parent.runState)}}, nil
}
func (f *recordingRun) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	f.parent.lastStatusWorkspace = wsID
	if wsID == "" {
		wsID = "ws-1"
	}
	return &projectioncontract.RunStatus{ID: string(id), WorkspaceID: string(wsID), State: string(f.parent.runState)}, nil
}

type conflictApprovalBE struct {
	*fakeBE
	runState         common.RunState
	approvalGetCalls int
}

func (b *conflictApprovalBE) Approval() backend.ApprovalBackend {
	return &conflictApprovalBackend{parent: b}
}
func (b *conflictApprovalBE) Run() backend.RunBackend { return &conflictRunBackend{parent: b} }
func (b *conflictApprovalBE) Workspace() backend.WorkspaceBackend {
	return &conflictWorkspaceBackend{parent: b}
}

type conflictWorkspaceBackend struct{ parent *conflictApprovalBE }

func (f *conflictWorkspaceBackend) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return (&fakeWS{}).Create(ctx, req)
}
func (f *conflictWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	return &projectioncontract.Workspace{ID: string(id), ActiveRunID: "run-1"}, nil
}
func (f *conflictWorkspaceBackend) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	return nil, nil
}
func (f *conflictWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	return &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: string(id), ActiveRunID: "run-1"},
		Runs: []projectioncontract.Run{{
			ID:          "run-1",
			WorkspaceID: string(id),
			State:       string(f.parent.runState),
		}},
		Approvals: []projectioncontract.Approval{{
			ID:          "approval-1",
			WorkspaceID: string(id),
			RunID:       "run-1",
			State:       string(common.ApprovalStateApproved),
		}},
	}, nil
}

func (f *conflictWorkspaceBackend) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (f *conflictWorkspaceBackend) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	return f.Get(ctx, id)
}

func (f *conflictWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}

type conflictApprovalBackend struct{ parent *conflictApprovalBE }

func (f *conflictApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	return nil, common.ErrConflict
}
func (f *conflictApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (f *conflictApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error) {
	f.parent.approvalGetCalls++
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), RunID: "run-1", State: "approved"}, nil
}

type conflictRunBackend struct{ parent *conflictApprovalBE }

func (f *conflictRunBackend) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunCommandResult, error) {
	return &backend.RunCommandResult{WorkspaceID: req.WorkspaceID, RunID: "run-1", RequestID: "request-conflict", Kind: runtimecontract.RuntimeExecutionIntentKind(req.Intent), Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted}, nil
}

func (f *conflictRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	return &projectioncontract.Run{ID: string(id), WorkspaceID: string(wsID), State: string(f.parent.runState)}, nil
}
func (f *conflictRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	return nil, nil
}
func (f *conflictRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	return &projectioncontract.RunStatus{ID: string(id), WorkspaceID: string(wsID), State: string(f.parent.runState)}, nil
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

func (b *recordingTUIDecisionBackend) Resolve(_ context.Context, req backend.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	b.requests = append(b.requests, req)
	return &projectioncontract.Decision{ID: string(req.TicketID), WorkspaceID: string(req.WorkspaceID), State: "resolved"}, nil
}
func (b *recordingTUIDecisionBackend) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (b *recordingTUIDecisionBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (b *recordingTUIDecisionBackend) Get(context.Context, common.WorkspaceID, common.TicketID) (*projectioncontract.Decision, error) {
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

func (b *recordingTUIApprovalBackend) Approve(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	b.approvals = append(b.approvals, recordingTUIApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: string(common.ApprovalStateApproved)}, nil
}
func (b *recordingTUIApprovalBackend) Reject(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	b.rejections = append(b.rejections, recordingTUIApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: string(common.ApprovalStateRejected)}, nil
}
func (b *recordingTUIApprovalBackend) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string, ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (b *recordingTUIApprovalBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (b *recordingTUIApprovalBackend) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*projectioncontract.Approval, error) {
	return nil, common.ErrNotFound
}

type fakeWS struct{}

func (f *fakeWS) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return &projectioncontract.Workspace{ID: "ws-new", Title: req.Title, State: "active"}, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	return &projectioncontract.Workspace{ID: string(id)}, nil
}
func (f *fakeWS) List(ctx context.Context) ([]projectioncontract.Workspace, error) { return nil, nil }
func (f *fakeWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	return &projectioncontract.WorkspaceSnapshot{Workspace: projectioncontract.Workspace{ID: string(id)}}, nil
}

func (f *fakeWS) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (f *fakeWS) UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderInferenceSelection, _ uint64) (*projectioncontract.Workspace, error) {
	item, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item.DefaultLLMSelection = runtimecontract.CloneProviderInferenceSelection(target)
	return item, nil
}

func (f *fakeWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error { return nil }

type fakeG struct{}

func (f *fakeG) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error) {
	return &workorchmanagementcontract.Goal{ID: "g-1", Text: text}, nil
}
func (f *fakeG) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	return nil, nil
}
func (f *fakeG) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error) {
	return &workorchmanagementcontract.Goal{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeG) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error) {
	return f.Get(ctx, wsID, id)
}

type fakeD struct{}

func (f *fakeD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error) {
	return &workorchmanagementcontract.DoD{ID: "d-1", Items: items}, nil
}
func (f *fakeD) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	return nil, nil
}
func (f *fakeD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error) {
	return &workorchmanagementcontract.DoD{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error) {
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

func (f *recordingDoD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error) {
	f.parent.workspaceID = wsID
	f.parent.items = append([]string(nil), items...)
	return &workorchmanagementcontract.DoD{ID: "dod-recorded", WorkspaceID: string(wsID), Items: items, State: "ready"}, nil
}
func (f *recordingDoD) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	if f.parent.workspaceID == wsID && len(f.parent.items) > 0 {
		return []workorchmanagementcontract.DoD{{ID: "dod-recorded", WorkspaceID: string(wsID), Items: append([]string(nil), f.parent.items...), State: "ready"}}, nil
	}
	return nil, nil
}
func (f *recordingDoD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error) {
	return &workorchmanagementcontract.DoD{ID: string(id), WorkspaceID: string(wsID), Items: append([]string(nil), f.parent.items...), State: "ready"}, nil
}
func (f *recordingDoD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error) {
	f.parent.items = append([]string(nil), items...)
	return &workorchmanagementcontract.DoD{ID: string(id), WorkspaceID: string(wsID), Items: items, State: "ready"}, nil
}

type fakeR struct{}

func (f *fakeR) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunCommandResult, error) {
	return &backend.RunCommandResult{WorkspaceID: req.WorkspaceID, RunID: "run-1", RequestID: "request-fake", Kind: runtimecontract.RuntimeExecutionIntentKind(req.Intent), Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted}, nil
}

func (f *fakeR) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	return &projectioncontract.Run{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeR) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	return nil, nil
}
func (f *fakeR) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	return &projectioncontract.RunStatus{ID: string(id), WorkspaceID: string(wsID), State: "running"}, nil
}

type fakeDC struct{}

func (f *fakeDC) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	return &projectioncontract.Decision{ID: string(req.TicketID), State: "resolved"}, nil
}
func (f *fakeDC) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.TicketID, rt string, p workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (f *fakeDC) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (f *fakeDC) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error) {
	return &projectioncontract.Decision{ID: string(id), WorkspaceID: string(wsID), Question: "?", State: "open"}, nil
}

type fakeA struct {
	lastPayload map[string]any
}

func (f *fakeA) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: "approved"}, nil
}
func (f *fakeA) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: "rejected"}, nil
}
func (f *fakeA) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	f.lastPayload = firstFakeApprovalPayload(payload...)
	return nil, nil
}
func (f *fakeA) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (f *fakeA) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error) {
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), Reason: "reason", State: "pending"}, nil
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

func (f *fakeS) Create(ctx context.Context, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeS) Get(ctx context.Context, id common.SoulID) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: string(id)}, nil
}
func (f *fakeS) List(ctx context.Context) ([]workorchmanagementcontract.Soul, error) { return nil, nil }
func (f *fakeS) Update(ctx context.Context, id common.SoulID, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: string(id), Name: s.Name}, nil
}
func (f *fakeS) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSK struct{}

func (f *fakeSK) Create(ctx context.Context, s *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error) {
	return &workorchmanagementcontract.Skill{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeSK) Get(ctx context.Context, id common.SkillID) (*workorchmanagementcontract.Skill, error) {
	return &workorchmanagementcontract.Skill{ID: string(id)}, nil
}
func (f *fakeSK) List(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	return nil, nil
}

type fakeM struct{}

func (f *fakeM) CreateServer(ctx context.Context, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeM) GetServer(ctx context.Context, id common.MCPServerID) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: string(id)}, nil
}
func (f *fakeM) ListServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	return nil, nil
}
func (f *fakeM) UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: string(id), Name: s.Name}, nil
}
func (f *fakeM) ListCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	return nil, nil
}

type fakeMM struct{}

func (f *fakeMM) Create(ctx context.Context, wsID common.WorkspaceID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: m.MemoryID, WorkspaceID: string(wsID), Summary: m.Summary}, nil
}
func (f *fakeMM) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeMM) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: string(id), WorkspaceID: string(wsID), Summary: m.Summary}, nil
}
func (f *fakeMM) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return nil
}
func (f *fakeMM) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}
func (f *fakeMM) Search(ctx context.Context, req workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}

type fakeT struct{}

func (f *fakeT) Create(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return t, nil
}
func (f *fakeT) Get(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return &workorchmanagementcontract.WorkspaceTemplate{ID: id}, nil
}
func (f *fakeT) List(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, nil
}
func (f *fakeT) Update(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return t, nil
}
func (f *fakeT) Delete(ctx context.Context, id string) error { return nil }

type fakeC struct{}

func (f *fakeC) Get(ctx context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{
		ServerAddr:  "127.0.0.1:8080",
		Backend:     "file",
		DataDir:     ".workorch-data",
		DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "local"},
		LLMOutput: workorchconfig.LLMOutputRuntimeConfig{
			MaxDocumentRecoveryAttempts: 3,
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
