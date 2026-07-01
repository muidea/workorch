package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestPatchesForRunFiltersWorkspaceSnapshotPatches(t *testing.T) {
	runID := common.RunID("run-1")
	got := patchesForRun(&backend.WorkspaceSnapshotInfo{
		Patches: []backend.PatchInfo{
			{ID: "patch-1", RunID: runID, TargetRef: "main.go"},
			{ID: "patch-2", RunID: "run-2", TargetRef: "other.go"},
		},
	}, runID)
	if len(got) != 1 || got[0].ID != "patch-1" {
		t.Fatalf("patches = %+v, want only run-1 patch", got)
	}
}

func TestNonEmptyApprovalIDPrefersApplyThenRevertApproval(t *testing.T) {
	if got := nonEmptyApprovalID("apply-approval", "revert-approval"); got != "apply-approval" {
		t.Fatalf("approval id = %q, want apply approval", got)
	}
	if got := nonEmptyApprovalID("", "revert-approval"); got != "revert-approval" {
		t.Fatalf("approval id = %q, want revert approval", got)
	}
}

func TestEventRecordsForRunFiltersWorkspaceSnapshotEvents(t *testing.T) {
	runID := common.RunID("run-1")
	got := eventRecordsForRun(&backend.WorkspaceSnapshotInfo{
		EventRecords: []backend.EventRecordInfo{
			{ID: "event-1", RunID: runID, Topic: "topic-a"},
			{ID: "event-2", RunID: "run-2", Topic: "topic-b"},
		},
	}, runID)
	if len(got) != 1 || got[0].ID != "event-1" {
		t.Fatalf("events = %+v, want only run-1 event", got)
	}
}

func TestEventRecordCompactSummaryPrefersPayloadDetail(t *testing.T) {
	got := eventRecordCompactSummary(backend.EventRecordInfo{
		Payload:        map[string]any{"state": "running"},
		PayloadSummary: map[string]any{"fieldCount": 1},
	})
	if got != "running" {
		t.Fatalf("summary = %q, want payload state", got)
	}
}

func TestMemoryCommandIsReadOnly(t *testing.T) {
	got := map[string]bool{}
	for _, cmd := range memoryCmd.Commands() {
		got[cmd.Name()] = true
	}
	for _, name := range []string{"list", "search", "get"} {
		if !got[name] {
			t.Fatalf("memory command missing read subcommand %q; got %v", name, got)
		}
	}
	for _, name := range []string{"create", "update", "delete"} {
		if got[name] {
			t.Fatalf("memory command exposes write subcommand %q; got %v", name, got)
		}
	}
}

func TestRunResumeUsesUnifiedWorkspaceRunCommand(t *testing.T) {
	previousBackend := cliBackend
	t.Cleanup(func() {
		cliBackend = previousBackend
		_ = runResumeCmd.Flags().Set("workspace", "")
	})

	runBackend := &recordingRunResumeBackend{}
	cliBackend = cliRunResumeBackend{run: runBackend}
	if err := runResumeCmd.Flags().Set("workspace", "ws-cli-resume"); err != nil {
		t.Fatalf("set workspace flag: %v", err)
	}

	if err := runResumeCmd.RunE(runResumeCmd, []string{"run-cli-resume"}); err != nil {
		t.Fatalf("run resume: %v", err)
	}
	if runBackend.resumeCalled {
		t.Fatal("run resume used legacy Run.Resume path")
	}
	got := runBackend.command
	if got.WorkspaceID != "ws-cli-resume" ||
		got.ExpectedRunID != "run-cli-resume" ||
		got.Intent != backend.RunCommandIntentResume ||
		got.Reason != "user_requested" ||
		got.Source != "cli" {
		t.Fatalf("command = %+v, want scoped resume command from cli", got)
	}
}

func TestRunActivateUsesUnifiedWorkspaceRunCommand(t *testing.T) {
	previousBackend := cliBackend
	t.Cleanup(func() {
		cliBackend = previousBackend
		_ = runActivateCmd.Flags().Set("goal", "")
		_ = runActivateCmd.Flags().Set("dod", "")
		_ = runActivateCmd.Flags().Set("soul", "")
		_ = runActivateCmd.Flags().Set("lang", "")
		_ = runActivateCmd.Flags().Set("priority", "0")
	})

	runBackend := &recordingRunResumeBackend{}
	cliBackend = cliRunResumeBackend{run: runBackend}
	_ = runActivateCmd.Flags().Set("goal", "goal-cli-activate")
	_ = runActivateCmd.Flags().Set("dod", "dod-cli-activate")
	_ = runActivateCmd.Flags().Set("soul", "soul-cli-activate")
	_ = runActivateCmd.Flags().Set("lang", "zh-CN")
	_ = runActivateCmd.Flags().Set("priority", "3")

	if err := runActivateCmd.RunE(runActivateCmd, []string{"ws-cli-activate"}); err != nil {
		t.Fatalf("run activate: %v", err)
	}
	if runBackend.activateCalled {
		t.Fatal("run activate used legacy Run.Activate path")
	}
	got := runBackend.command
	if got.WorkspaceID != "ws-cli-activate" ||
		got.ExpectedRunID != "" ||
		got.Intent != backend.RunCommandIntentResume ||
		got.Reason != "user_requested" ||
		got.Source != "cli" ||
		got.Activation.GoalID != "goal-cli-activate" ||
		got.Activation.DoDID != "dod-cli-activate" ||
		got.Activation.SoulID != "soul-cli-activate" ||
		got.Activation.DialogLanguage != "zh-CN" ||
		got.Activation.Priority != 3 {
		t.Fatalf("command = %+v, want scoped activation resume command from cli", got)
	}
}

func TestRunResumeRequiresWorkspaceScope(t *testing.T) {
	previousBackend := cliBackend
	t.Cleanup(func() {
		cliBackend = previousBackend
		_ = runResumeCmd.Flags().Set("workspace", "")
	})

	cliBackend = cliRunResumeBackend{run: &recordingRunResumeBackend{}}
	_ = runResumeCmd.Flags().Set("workspace", "")

	err := runResumeCmd.RunE(runResumeCmd, []string{"run-cli-resume"})
	if err == nil || !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("run resume error = %v, want workspace required", err)
	}
}

type cliRunResumeBackend struct {
	agentCommandFailureBackend
	run backend.RunBackend
}

func (b cliRunResumeBackend) Run() backend.RunBackend { return b.run }

type recordingRunResumeBackend struct {
	command        backend.WorkspaceRunCommand
	activateCalled bool
	resumeCalled   bool
}

func (b *recordingRunResumeBackend) Command(_ context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	b.command = req
	return &backend.RunInfo{ID: "run-command", WorkspaceID: req.WorkspaceID, State: common.RunStateRunning}, nil
}

func (b *recordingRunResumeBackend) Activate(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	b.activateCalled = true
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Restart(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Resume(context.Context, common.WorkspaceID, common.RunID, ...map[string]any) (*backend.RunInfo, error) {
	b.resumeCalled = true
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Get(context.Context, common.WorkspaceID, common.RunID) (*backend.RunInfo, error) {
	return nil, common.ErrNotFound
}

func (b *recordingRunResumeBackend) List(context.Context, common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}

func (b *recordingRunResumeBackend) Status(context.Context, common.WorkspaceID, common.RunID) (*backend.RunStatusInfo, error) {
	return nil, common.ErrNotFound
}
