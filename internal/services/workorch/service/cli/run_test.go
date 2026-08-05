package cli

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPatchesForRunFiltersWorkspaceSnapshotPatches(t *testing.T) {
	runID := common.RunID("run-1")
	got := patchesForRun(&projectioncontract.WorkspaceSnapshot{
		Patches: []projectioncontract.Patch{
			{ID: "patch-1", RunID: string(runID), TargetRef: "main.go"},
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
	got := eventRecordsForRun(&projectioncontract.WorkspaceSnapshot{
		EventRecords: []projectioncontract.EventRecord{
			{ID: "event-1", RunID: string(runID), Topic: "topic-a"},
			{ID: "event-2", RunID: "run-2", Topic: "topic-b"},
		},
	}, runID)
	if len(got) != 1 || got[0].ID != "event-1" {
		t.Fatalf("events = %+v, want only run-1 event", got)
	}
}

func TestEventRecordCompactSummaryPrefersPayloadDetail(t *testing.T) {
	got := eventRecordCompactSummary(projectioncontract.EventRecord{
		Payload: workorcheventpayload.New(workorcheventpayload.StringField("state", string(common.RunStateActive))),
	})
	if got != "active" {
		t.Fatalf("summary = %q, want concrete payload state", got)
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
	})

	runBackend := &recordingRunResumeBackend{}
	cliBackend = cliRunResumeBackend{run: runBackend}
	if err := runResumeCmd.RunE(runResumeCmd, []string{"ws-cli-resume"}); err != nil {
		t.Fatalf("run resume: %v", err)
	}
	if runBackend.resumeCalled {
		t.Fatal("run resume used legacy Run.Resume path")
	}
	got := runBackend.command
	if got.WorkspaceID != "ws-cli-resume" ||
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
		resetRunActivateFlagsForTest(t)
	})
	runBackend := &recordingRunResumeBackend{}
	cliBackend = cliRunResumeBackend{run: runBackend}
	_ = runActivateCmd.Flags().Set("priority", "3")

	if err := runActivateCmd.RunE(runActivateCmd, []string{"ws-cli-activate"}); err != nil {
		t.Fatalf("run activate: %v", err)
	}
	if runBackend.activateCalled {
		t.Fatal("run activate used legacy Run.Activate path")
	}
	got := runBackend.command
	if got.WorkspaceID != "ws-cli-activate" ||
		got.Intent != backend.RunCommandIntentRestart ||
		got.Reason != "user_requested" ||
		got.Source != "cli" || got.Priority != 3 {
		t.Fatalf("command = %+v, want workspace-scoped restart from cli", got)
	}
}

func TestRunActivateRejectsInvalidSoulBindingFlag(t *testing.T) {
	t.Skip("activation patch flags were removed; Workspace owner compiles current configuration")
}

func TestRunResumeRequiresWorkspaceScope(t *testing.T) {
	t.Skip("resume now requires workspace-id as its positional argument")
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

func (b *recordingRunResumeBackend) Command(_ context.Context, req backend.WorkspaceRunCommand) (*backend.RunCommandResult, error) {
	b.command = req
	return &backend.RunCommandResult{WorkspaceID: req.WorkspaceID, RunID: "run-command", RequestID: "request-command", Kind: runtimecontract.RuntimeExecutionIntentKind(req.Intent), Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted}, nil
}

func (b *recordingRunResumeBackend) Activate(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*projectioncontract.Run, error) {
	b.activateCalled = true
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Restart(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*projectioncontract.Run, error) {
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Resume(context.Context, common.WorkspaceID, common.RunID, ...map[string]any) (*projectioncontract.Run, error) {
	b.resumeCalled = true
	return nil, common.ErrInvalidArgument
}

func (b *recordingRunResumeBackend) Get(context.Context, common.WorkspaceID, common.RunID) (*projectioncontract.Run, error) {
	return nil, common.ErrNotFound
}

func (b *recordingRunResumeBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Run, error) {
	return nil, nil
}

func (b *recordingRunResumeBackend) Status(context.Context, common.WorkspaceID, common.RunID) (*projectioncontract.RunStatus, error) {
	return nil, common.ErrNotFound
}

func resetRunActivateFlagsForTest(t *testing.T) {
	t.Helper()
	if err := runActivateCmd.Flags().Set("priority", "0"); err != nil {
		t.Fatalf("reset priority flag: %v", err)
	}
}
