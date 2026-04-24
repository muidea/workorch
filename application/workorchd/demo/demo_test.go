package demo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunAllWithMemoryRuntimeReturnsExpectedSlices(t *testing.T) {
	slices := mustRunAllWithBackend(t, "memory", "")

	if len(slices) != 17 {
		t.Fatalf("expected 17 slices, got %d", len(slices))
	}

	wantTitles := []string{
		"workorch runnable slice",
		"workorch runtime-failed-terminal slice",
		"workorch multi-step-completed slice",
		"workorch conditional-branch-completed slice",
		"workorch conditional-branch-review-completed slice",
		"workorch runtime-multi-decision-gate-completed slice",
		"workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice",
		"workorch runtime-decision-blocked-resume slice",
		"workorch runtime-decision-rejected-terminal slice",
		"workorch runtime-approval-blocked-resume slice",
		"workorch runtime-approval-rejected-terminal slice",
		"workorch runtime-mixed-decision-approval-completed slice",
		"workorch runtime-mixed-decision-approval-rejected-terminal slice",
		"workorch decision-blocked-resume slice",
		"workorch decision-rejected-terminal slice",
		"workorch approval-blocked-resume slice",
		"workorch approval-rejected-terminal slice",
	}
	for idx, want := range wantTitles {
		if slices[idx].Title != want {
			t.Fatalf("slice %d title mismatch, want %q got %q", idx, want, slices[idx].Title)
		}
		if slices[idx].Snapshot == nil {
			t.Fatalf("slice %d snapshot is nil", idx)
		}
	}

	if got := slices[0].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runnable slice run state mismatch: %s", got)
	}
	if got := slices[1].Snapshot.Run.State; got != "failed" {
		t.Fatalf("runtime failed terminal run state mismatch: %s", got)
	}
	if got := slices[2].Snapshot.Run.State; got != "completed" {
		t.Fatalf("multi-step completed run state mismatch: %s", got)
	}
	if got := slices[3].Snapshot.Run.State; got != "completed" {
		t.Fatalf("conditional branch completed run state mismatch: %s", got)
	}
	if got := slices[4].Snapshot.Run.State; got != "completed" {
		t.Fatalf("conditional review branch completed run state mismatch: %s", got)
	}
	if got := slices[5].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runtime multi decision gate completed run state mismatch: %s", got)
	}
	if got := slices[6].Snapshot.Run.State; got != "failed" {
		t.Fatalf("runtime multi decision gate second gate rejected run state mismatch: %s", got)
	}
	if got := slices[7].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runtime decision blocked/resume run state mismatch: %s", got)
	}
	if got := slices[8].Snapshot.Run.State; got != "failed" {
		t.Fatalf("runtime decision rejected terminal run state mismatch: %s", got)
	}
	if got := slices[9].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runtime approval blocked/resume run state mismatch: %s", got)
	}
	if got := slices[10].Snapshot.Run.State; got != "failed" {
		t.Fatalf("runtime approval rejected terminal run state mismatch: %s", got)
	}
	if got := slices[11].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runtime mixed decision approval completed run state mismatch: %s", got)
	}
	if got := slices[12].Snapshot.Run.State; got != "failed" {
		t.Fatalf("runtime mixed decision approval rejected terminal run state mismatch: %s", got)
	}
	if got := slices[13].Snapshot.Run.State; got != "completed" {
		t.Fatalf("decision blocked/resume run state mismatch: %s", got)
	}
	if got := slices[14].Snapshot.Run.State; got != "failed" {
		t.Fatalf("decision rejected terminal run state mismatch: %s", got)
	}
	if got := slices[15].Snapshot.Run.State; got != "completed" {
		t.Fatalf("approval blocked/resume run state mismatch: %s", got)
	}
	if got := slices[16].Snapshot.Run.State; got != "failed" {
		t.Fatalf("approval rejected terminal run state mismatch: %s", got)
	}

	if slices[1].Snapshot.Step == nil || slices[1].Snapshot.Step.State != "failed" {
		t.Fatalf("runtime failed terminal slice should expose failed step, got %+v", slices[1].Snapshot.Step)
	}
	if slices[1].Snapshot.Step.Output["reason"] == nil {
		t.Fatalf("runtime failed terminal step output missing reason: %+v", slices[1].Snapshot.Step.Output)
	}
	if slices[2].Snapshot.Step == nil || slices[2].Snapshot.Step.ID != "run-workorch-multi-step/step-2" {
		t.Fatalf("multi-step completed slice should expose final step-2, got %+v", slices[2].Snapshot.Step)
	}
	if len(slices[2].Snapshot.Steps) != 2 {
		t.Fatalf("multi-step completed slice steps = %+v, want two steps", slices[2].Snapshot.Steps)
	}
	if slices[3].Snapshot.Blocking != nil {
		t.Fatal("conditional branch slice should not expose decision ticket")
	}
	if slices[3].Snapshot.Step == nil || slices[3].Snapshot.Step.ID != "run-workorch-conditional-branch/step-fast" {
		t.Fatalf("conditional branch slice should expose branch step-fast, got %+v", slices[3].Snapshot.Step)
	}
	if len(slices[3].Snapshot.Steps) != 2 || slices[3].Snapshot.Steps[0].Output["branch"] != "fast-track" {
		t.Fatalf("conditional branch slice steps = %+v, want branch metadata on step-1", slices[3].Snapshot.Steps)
	}
	if slices[4].Snapshot.Blocking != nil {
		t.Fatal("conditional review branch slice should not expose decision ticket")
	}
	if slices[4].Snapshot.Step == nil || slices[4].Snapshot.Step.ID != "run-workorch-conditional-branch-branch-slow/step-review" {
		t.Fatalf("conditional review branch slice should expose branch step-review, got %+v", slices[4].Snapshot.Step)
	}
	if len(slices[4].Snapshot.Steps) != 2 || slices[4].Snapshot.Steps[0].Output["branch"] != "review-path" {
		t.Fatalf("conditional review branch slice steps = %+v, want branch metadata review-path on step-1", slices[4].Snapshot.Steps)
	}
	if slices[5].Snapshot.Step == nil || slices[5].Snapshot.Step.ID != "run-workorch-multi-decision-gate/step-3" {
		t.Fatalf("runtime multi decision gate slice should expose final step-3, got %+v", slices[5].Snapshot.Step)
	}
	if len(slices[5].Snapshot.Steps) != 3 {
		t.Fatalf("runtime multi decision gate slice steps = %+v, want three steps", slices[5].Snapshot.Steps)
	}
	if slices[5].Snapshot.Blocking == nil || slices[5].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime multi decision gate slice missing decision ticket snapshot")
	}
	if slices[5].Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-gate-2" || slices[5].Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime multi decision gate decision ticket = %+v, want resolved gate-2 ticket", slices[5].Snapshot.Blocking.DecisionTicket)
	}
	if slices[6].Snapshot.Step != nil {
		t.Fatalf("runtime multi decision gate second gate rejected slice should not expose final step, got %+v", slices[6].Snapshot.Step)
	}
	if len(slices[6].Snapshot.Steps) != 2 || slices[6].Snapshot.Steps[0].ID != "run-workorch-multi-decision-gate-rejected/step-1" || slices[6].Snapshot.Steps[1].ID != "run-workorch-multi-decision-gate-rejected/step-2" {
		t.Fatalf("runtime multi decision gate second gate rejected slice steps = %+v, want completed step-1 and step-2", slices[6].Snapshot.Steps)
	}
	if slices[6].Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-1" {
		t.Fatalf("runtime multi decision gate second gate rejected step-1 output = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-1", slices[6].Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if slices[6].Snapshot.Steps[1].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" {
		t.Fatalf("runtime multi decision gate second gate rejected step-2 output = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-2", slices[6].Snapshot.Steps[1].Output["blockDecisionTicketId"])
	}
	if slices[6].Snapshot.Blocking == nil || slices[6].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime multi decision gate second gate rejected slice missing decision ticket snapshot")
	}
	if slices[6].Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" || slices[6].Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime multi decision gate second gate rejected decision ticket = %+v, want resolved gate-2 ticket", slices[6].Snapshot.Blocking.DecisionTicket)
	}
	if slices[7].Snapshot.Blocking == nil || slices[7].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime decision blocked/resume slice missing decision ticket snapshot")
	}
	if slices[8].Snapshot.Step != nil {
		t.Fatalf("runtime decision rejected terminal slice should not expose final step, got %+v", slices[8].Snapshot.Step)
	}
	if len(slices[8].Snapshot.Steps) != 1 || slices[8].Snapshot.Steps[0].ID != "run-workorch-runtime-decision-block-rejected/step-1" {
		t.Fatalf("runtime decision rejected terminal slice steps = %+v, want completed step-1 only", slices[8].Snapshot.Steps)
	}
	if slices[8].Snapshot.Blocking == nil || slices[8].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime decision rejected terminal slice missing decision ticket snapshot")
	}
	if slices[8].Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-runtime-decision-block-rejected-runtime-decision" || slices[8].Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime decision rejected terminal decision ticket = %+v, want resolved runtime decision ticket", slices[8].Snapshot.Blocking.DecisionTicket)
	}
	if slices[9].Snapshot.Step == nil || slices[9].Snapshot.Step.ID != "run-workorch-runtime-approval-block/step-2" {
		t.Fatalf("runtime approval blocked/resume slice should expose final step-2, got %+v", slices[9].Snapshot.Step)
	}
	if len(slices[9].Snapshot.Steps) != 2 {
		t.Fatalf("runtime approval blocked/resume slice steps = %+v, want two steps", slices[9].Snapshot.Steps)
	}
	if slices[9].Snapshot.Blocking == nil || slices[9].Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime approval blocked/resume slice missing approval snapshot")
	}
	if slices[9].Snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-runtime-approval" || slices[9].Snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("runtime approval blocked/resume approval = %+v, want approved runtime approval", slices[9].Snapshot.Blocking.Approval)
	}
	if slices[10].Snapshot.Step != nil {
		t.Fatalf("runtime approval rejected terminal slice should not expose final step, got %+v", slices[10].Snapshot.Step)
	}
	if len(slices[10].Snapshot.Steps) != 1 || slices[10].Snapshot.Steps[0].ID != "run-workorch-runtime-approval-block-rejected/step-1" {
		t.Fatalf("runtime approval rejected terminal slice steps = %+v, want completed step-1 only", slices[10].Snapshot.Steps)
	}
	if slices[10].Snapshot.Blocking == nil || slices[10].Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime approval rejected terminal slice missing approval snapshot")
	}
	if slices[10].Snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-rejected-runtime-approval" || slices[10].Snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("runtime approval rejected terminal approval = %+v, want rejected runtime approval", slices[10].Snapshot.Blocking.Approval)
	}
	if slices[11].Snapshot.Step == nil || slices[11].Snapshot.Step.ID != "run-workorch-mixed-decision-approval/step-3" {
		t.Fatalf("runtime mixed decision approval slice should expose final step-3, got %+v", slices[11].Snapshot.Step)
	}
	if len(slices[11].Snapshot.Steps) != 3 {
		t.Fatalf("runtime mixed decision approval slice steps = %+v, want three steps", slices[11].Snapshot.Steps)
	}
	if slices[11].Snapshot.Blocking == nil || slices[11].Snapshot.Blocking.DecisionTicket == nil || slices[11].Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime mixed decision approval slice missing mixed blocking snapshots")
	}
	if slices[11].Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-mixed-decision" || slices[11].Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime mixed decision approval decision ticket = %+v, want resolved mixed decision ticket", slices[11].Snapshot.Blocking.DecisionTicket)
	}
	if slices[11].Snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-mixed-approval" || slices[11].Snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("runtime mixed decision approval approval = %+v, want approved mixed approval", slices[11].Snapshot.Blocking.Approval)
	}
	if slices[12].Snapshot.Step != nil {
		t.Fatalf("runtime mixed decision approval rejected terminal slice should not expose final step, got %+v", slices[12].Snapshot.Step)
	}
	if len(slices[12].Snapshot.Steps) != 2 || slices[12].Snapshot.Steps[0].ID != "run-workorch-mixed-decision-approval-rejected/step-1" || slices[12].Snapshot.Steps[1].ID != "run-workorch-mixed-decision-approval-rejected/step-2" {
		t.Fatalf("runtime mixed decision approval rejected terminal slice steps = %+v, want completed step-1 and step-2", slices[12].Snapshot.Steps)
	}
	if slices[12].Snapshot.Blocking == nil || slices[12].Snapshot.Blocking.DecisionTicket == nil || slices[12].Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime mixed decision approval rejected terminal slice missing blocking snapshots")
	}
	if slices[12].Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision" || slices[12].Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime mixed decision approval rejected terminal decision ticket = %+v, want resolved mixed decision ticket", slices[12].Snapshot.Blocking.DecisionTicket)
	}
	if slices[12].Snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-rejected-mixed-approval" || slices[12].Snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("runtime mixed decision approval rejected terminal approval = %+v, want rejected mixed approval", slices[12].Snapshot.Blocking.Approval)
	}
	if slices[13].Snapshot.Blocking == nil || slices[13].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("decision blocked/resume slice missing decision ticket snapshot")
	}
	if slices[14].Snapshot.Blocking == nil || slices[14].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("decision rejected terminal slice missing decision ticket snapshot")
	}
	if slices[15].Snapshot.Blocking == nil || slices[15].Snapshot.Blocking.Approval == nil {
		t.Fatal("approval blocked/resume slice missing approval snapshot")
	}
	if slices[0].Snapshot.Step == nil || slices[0].Snapshot.Step.State != "completed" {
		t.Fatalf("runnable slice step mismatch: %+v", slices[0].Snapshot.Step)
	}
	if slices[14].Snapshot.Step != nil {
		t.Fatalf("decision rejected terminal slice should not dispatch runtime step, got %+v", slices[14].Snapshot.Step)
	}
	if slices[16].Snapshot.Step != nil {
		t.Fatalf("approval rejected terminal slice should not dispatch runtime step, got %+v", slices[16].Snapshot.Step)
	}
}

func TestRunAllWithFileRuntimePersistsExpectedFiles(t *testing.T) {
	dataDir := t.TempDir()
	slices := mustRunAllWithBackend(t, "file", dataDir)
	if len(slices) != 17 {
		t.Fatalf("expected 17 slices, got %d", len(slices))
	}

	files := []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected persisted file %s: %v", name, err)
		}
	}

	if got := slices[16].Snapshot.Run.State; got != "failed" {
		t.Fatalf("expected approval rejected terminal slice to stay failed, got %s", got)
	}
}

func TestRunAllFromEnvWithFileBackendUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataRoot := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", "")

	slices, err := RunAllFromEnv(context.Background())
	if err != nil {
		t.Fatalf("RunAllFromEnv: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("expected 17 slices, got %d", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(defaultDataRoot, name)); err != nil {
			t.Fatalf("expected persisted file %s in default data root: %v", name, err)
		}
	}
}

func TestRunAllWithBackendFileUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataRoot := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)

	slices, err := RunAllWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("RunAllWithBackend: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("expected 17 slices, got %d", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(defaultDataRoot, name)); err != nil {
			t.Fatalf("expected persisted file %s in default data root: %v", name, err)
		}
	}
}

func TestRunAllWithBackendFileRemainsStableAcrossReloadsWhenDefaultDataRootUnset(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	first, err := RunAllWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("first RunAllWithBackend: %v", err)
	}
	second, err := RunAllWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("second RunAllWithBackend: %v", err)
	}

	assertWorkorchSlicesRemainStableAcrossRuns(t, first, second)
}

func TestRunAllWithBackendReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	if _, err := RunAllWithBackend(context.Background(), "unsupported", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsupported backend err = %v, want invalid argument", err)
	}
}

func TestRunAllFromEnvReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "unsupported")
	t.Setenv("WORKORCH_DATA_DIR", t.TempDir())

	if _, err := RunAllFromEnv(context.Background()); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsupported env backend err = %v, want invalid argument", err)
	}
}
