package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterReadStatus(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationStatus,
	})
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if result.Status != "completed" || result.Operation != gitcommon.OperationStatus {
		t.Fatalf("status output = %+v, want completed status", result)
	}
	files := result.ChangedFiles
	if len(files) != 1 || files[0].Path != "README.md" {
		t.Fatalf("changedFiles = %#v, want README.md", result.ChangedFiles)
	}
}

func TestRuntimeAdapterReadStatusInEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationStatus,
	})
	if err != nil {
		t.Fatalf("execute status in empty repo: %v", err)
	}
	if result.Status != "completed" || result.Operation != gitcommon.OperationStatus {
		t.Fatalf("status output = %+v, want completed status", result)
	}
	if result.HeadCommit != "" {
		t.Fatalf("headCommit = %v, want empty value before first commit", result.HeadCommit)
	}
	if result.UnbornHead != true {
		t.Fatalf("unbornHead = %v, want true before first commit", result.UnbornHead)
	}
}

func TestRuntimeAdapterReadLogAndShowInEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")

	adapter := NewRuntimeAdapter()
	for _, operation := range []string{gitcommon.OperationLog, gitcommon.OperationShow} {
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
			Kind:                          gitcommon.StepKindRead,
			WorkspaceDeliverableDirectory: repo,
			Operation:                     operation,
		})
		if err != nil {
			t.Fatalf("execute %s in empty repo: %v\noutput=%+v", operation, err, result)
		}
		if result.Status != "completed" || result.ZeroWork != true || result.UnbornHead != true {
			t.Fatalf("%s output = %+v, want completed zero-work unborn HEAD baseline", operation, result)
		}
		if operation == gitcommon.OperationLog {
			commits := result.Commits
			if len(commits) != 0 {
				t.Fatalf("commits = %#v, want empty list", result.Commits)
			}
		}
	}
}

func TestRuntimeAdapterReadBranchLogAndShowFromCommittedRepository(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")

	branch, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationBranch,
	})
	if err != nil {
		t.Fatalf("execute branch: %v\nresult=%+v", err, branch)
	}
	if branch.Status != "completed" || len(branch.Branches) != 1 || !branch.Branches[0].Current || branch.Branches[0].Commit != head || branch.Branches[0].Name == "" {
		t.Fatalf("branch result = %+v, want the current committed branch", branch)
	}

	logResult, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationLog, Limit: 1,
	})
	if err != nil {
		t.Fatalf("execute log: %v\nresult=%+v", err, logResult)
	}
	if logResult.Status != "completed" || len(logResult.Commits) != 1 || logResult.Commits[0].Hash != head || logResult.Commits[0].Subject != "chore: initial" || logResult.Commits[0].Author == "" {
		t.Fatalf("log result = %+v, want one decoded initial commit", logResult)
	}

	show, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationShow, Ref: "HEAD",
	})
	if err != nil {
		t.Fatalf("execute show: %v\nresult=%+v", err, show)
	}
	if show.Status != "completed" || show.Ref != "HEAD" || show.OutputBytes == 0 || !strings.Contains(show.Stdout, head) || !strings.Contains(show.Stdout, "chore: initial") {
		t.Fatalf("show result = %+v, want bounded non-empty commit output", show)
	}
}

func TestRuntimeAdapterReadPathspecDotMeansRepositoryRoot(t *testing.T) {
	repo := newTestRepo(t)

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationStatus,
		Pathspecs:                     []string{"."},
	})
	if err != nil {
		t.Fatalf("execute status with root pathspec: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" {
		t.Fatalf("status output = %+v, want completed", result)
	}
	pathspecs := result.Pathspecs
	if len(pathspecs) != 0 {
		t.Fatalf("pathspecs = %#v, want repository-root pathspec normalized away", result.Pathspecs)
	}
}

func TestRuntimeAdapterReadRejectsOptionShapedRevisionBeforeGit(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()
	for _, test := range []struct {
		name      string
		operation string
		ref       string
		baseRef   string
	}{
		{name: "diff ref option", operation: gitcommon.OperationDiff, ref: "--output={output}"},
		{name: "diff base option", operation: gitcommon.OperationDiff, baseRef: "--output={output}"},
		{name: "log ref option", operation: gitcommon.OperationLog, ref: "--output={output}"},
		{name: "show ref option", operation: gitcommon.OperationShow, ref: "--output={output}"},
		{name: "control character", operation: gitcommon.OperationDiff, ref: "HEAD\n--output={output}"},
		{name: "oversized", operation: gitcommon.OperationShow, ref: strings.Repeat("a", maxGitRevisionBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "injected-output")
			ref := strings.ReplaceAll(test.ref, "{output}", outputPath)
			baseRef := strings.ReplaceAll(test.baseRef, "{output}", outputPath)
			result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
				Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: repo,
				Operation: test.operation, Ref: ref, BaseRef: baseRef,
			})
			if err == nil || result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorInvalidRequest {
				t.Fatalf("option-shaped ref result=%+v err=%v, want invalid request", result, err)
			}
			if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
				t.Fatalf("option-shaped read created %q: %v", outputPath, statErr)
			}
		})
	}
}

func TestRuntimeAdapterRejectsUndeclaredRemoteAndHistoryRewriteOperations(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	for _, operation := range []string{"push", "pull", "fetch", "remote", "rebase", "reset", "checkout", "merge", "cherry-pick", "revert", "clean", "rm", "tag"} {
		for _, kind := range []string{runtimecontract.StepKindGitRead, runtimecontract.StepKindGitWrite} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
					Kind: kind, WorkspaceDeliverableDirectory: repo, Operation: operation,
				})
				if err == nil || result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorOperationDenied {
					t.Fatalf("operation=%q kind=%q result=%+v err=%v", operation, kind, result, err)
				}
				if current := gitOutputForTest(t, repo, "rev-parse", "HEAD"); current != head {
					t.Fatalf("undeclared operation changed HEAD: got=%q want=%q", current, head)
				}
			})
		}
	}
}

func TestRuntimeAdapterWriteInitInitializesWorkspaceRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationInit,
	})
	if err != nil {
		t.Fatalf("execute init: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" || result.Operation != gitcommon.OperationInit {
		t.Fatalf("init output = %+v, want completed init", result)
	}
	if result.Initialized != true || result.ExistingRepository != false {
		t.Fatalf("init flags = %+v, want initialized new repository", result)
	}
	if result.UnbornHead != true || result.HeadCommit != "" {
		t.Fatalf("head output = %+v, want unborn repository without head commit", result)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatalf(".git stat: %v, want initialized repository", err)
	}
	if got := strings.TrimSpace(gitOutputForTest(t, repo, "config", "core.hooksPath")); got != "/dev/null" {
		t.Fatalf("core.hooksPath = %q, want /dev/null", got)
	}
}

func TestRuntimeAdapterWriteInitIsIdempotentForExistingRepository(t *testing.T) {
	repo := newTestRepo(t)

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationInit,
	})
	if err != nil {
		t.Fatalf("execute idempotent init: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" || result.Operation != gitcommon.OperationInit {
		t.Fatalf("init output = %+v, want completed init", result)
	}
	if result.ZeroWork != true || result.ExistingRepository != true || result.Initialized != false {
		t.Fatalf("init flags = %+v, want idempotent existing repository", result)
	}
	if result.HeadCommit == "" {
		t.Fatalf("headCommit empty for existing committed repository: %+v", result)
	}
}

func TestRuntimeAdapterReadInitRejected(t *testing.T) {
	dir := t.TempDir()
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: dir,
		Operation:                     gitcommon.OperationInit,
	})
	if err == nil {
		t.Fatal("execute init with git/read: expected error")
	}
	if result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want init rejected for git/read", result)
	}
}

func TestRuntimeAdapterRejectsUnsafePathspec(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationAdd,
		Pathspecs:                     []string{"../outside"},
	})
	if err == nil {
		t.Fatal("execute add with unsafe pathspec: expected error")
	}
	if result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected path", result)
	}
}

func TestRuntimeAdapterWritePathspecDotStillRejected(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationAdd,
		Pathspecs:                     []string{"."},
	})
	if err == nil {
		t.Fatal("execute add with broad root pathspec: expected error")
	}
	if result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected broad root pathspec", result)
	}
}

func TestRuntimeAdapterAddStagesDeclaredPath(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationAdd,
		Pathspecs:                     []string{"README.md"},
	})
	if err != nil {
		t.Fatalf("execute add: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" || result.Operation != gitcommon.OperationAdd {
		t.Fatalf("output = %+v, want add completed", result)
	}
}

func TestRuntimeAdapterAddReportsOnlyCurrentIndexDelta(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "already-staged.txt"), "content\n")
	adapter := NewRuntimeAdapter()
	request := RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitWrite, WorkspaceDeliverableDirectory: repo,
		Operation: gitcommon.OperationAdd, Pathspecs: []string{"already-staged.txt"},
	}
	first, err := adapter.ExecuteOperation(context.Background(), request)
	if err != nil || first.ZeroWork || len(first.ChangedFiles) != 1 || first.BeforeHash == first.AfterHash {
		t.Fatalf("first add result=%+v err=%v", first, err)
	}
	second, err := adapter.ExecuteOperation(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if !second.ZeroWork || len(second.ChangedFiles) != 0 || second.BeforeHash == "" || second.BeforeHash != second.AfterHash || !second.Evidence.ZeroWork {
		t.Fatalf("duplicate add result=%+v, want zero-work with stable index revision", second)
	}
	if staged := gitOutputForTest(t, repo, "diff", "--cached", "--name-only"); staged != "already-staged.txt" {
		t.Fatalf("staged files=%q, want existing staged file preserved", staged)
	}
}

func TestRuntimeAdapterAddAndCommitDoesNotRunHooks(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	writeFile(t, filepath.Join(repo, ".git", "hooks", "post-commit"), "#!/bin/sh\ntouch hook-ran\n")
	if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "post-commit"), 0o755); err != nil {
		t.Fatalf("chmod hook: %v", err)
	}
	adapter := NewRuntimeAdapter()

	if _, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationAdd,
		Pathspecs:                     []string{"README.md"},
	}); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "docs: add readme",
		ExpectedHead:                  head,
	})
	if err != nil {
		t.Fatalf("execute commit: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" {
		t.Fatalf("commit output = %+v, want completed", result)
	}
	commit := result
	if commit.CommitHash == "" {
		t.Fatalf("commit = %#v, want hash", result)
	}
	if _, err := os.Stat(filepath.Join(repo, "hook-ran")); !os.IsNotExist(err) {
		t.Fatalf("post-commit hook ran or stat failed: %v", err)
	}
}

func TestRuntimeAdapterCommitAllowsUnbornBranchInitialCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")
	runGitForTest(t, repo, "config", "user.email", "workorch@example.test")
	runGitForTest(t, repo, "config", "user.name", "WorkOrch Test")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	adapter := NewRuntimeAdapter()

	status, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationStatus,
	})
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if status.DetachedHead == true || status.UnbornHead != true {
		t.Fatalf("status output = %+v, want unborn branch without detached HEAD", status)
	}
	if _, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationAdd,
		Pathspecs:                     []string{"README.md"},
	}); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "docs: initial readme",
	})
	if err != nil {
		t.Fatalf("execute initial commit: %v\noutput=%+v", err, result)
	}
	if result.Status != "completed" || result.DetachedHead == true {
		t.Fatalf("commit output = %+v, want completed non-detached commit", result)
	}
	if result.HeadCommitAfter == "" {
		t.Fatalf("commit output = %+v, want headCommitAfter", result)
	}
}

func TestRuntimeAdapterCommitRejectsPathspec(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "bad",
		Pathspecs:                     []string{"README.md"},
	})
	if err == nil {
		t.Fatal("execute commit with pathspecs: expected error")
	}
	if result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected pathspec", result)
	}
}

func TestCapabilityStateReacquiresSequenceAfterStageChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "changed\n")
	adapter := NewRuntimeAdapter()

	beforeStage, err := adapter.CapabilityState(context.Background(), "git-capability", "workspace_deliverable", repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := beforeStage.RequiredOperationSequence; len(got) != 2 || got[0].RequirementID != "stage-changes" || got[0].VariantID != gitcommon.OperationAdd ||
		got[1].RequirementID != "create-commit" || got[1].VariantID != gitcommon.OperationCommit || len(got[1].DependsOnRequirementIDs) != 1 || got[1].DependsOnRequirementIDs[0] != "stage-changes" {
		t.Fatalf("pre-stage sequence = %+v, want stage then commit", got)
	}

	runGitForTest(t, repo, "add", "README.md")
	afterStage, err := adapter.CapabilityState(context.Background(), "git-capability", "workspace_deliverable", repo)
	if err != nil {
		t.Fatal(err)
	}
	if afterStage.SnapshotID == beforeStage.SnapshotID || afterStage.ObservedRevision == beforeStage.ObservedRevision {
		t.Fatalf("post-stage snapshot was reused: before=%+v after=%+v", beforeStage, afterStage)
	}
	if got := afterStage.RequiredOperationSequence; len(got) != 1 || got[0].RequirementID != "create-commit" || got[0].VariantID != gitcommon.OperationCommit || len(got[0].DependsOnRequirementIDs) != 0 {
		t.Fatalf("post-stage sequence = %+v, want only fresh create-commit", got)
	}

	runGitForTest(t, repo, "commit", "-m", "docs: stage capability state")
	afterCommit, err := adapter.CapabilityState(context.Background(), "git-capability", "workspace_deliverable", repo)
	if err != nil {
		t.Fatal(err)
	}
	if afterCommit.SnapshotID == afterStage.SnapshotID || len(afterCommit.RequiredOperationSequence) != 0 {
		t.Fatalf("post-commit capability snapshot = %+v, want new ready snapshot", afterCommit)
	}
}

func TestRuntimeAdapterFailsOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: dir,
		Operation:                     gitcommon.OperationStatus,
	})
	if err == nil {
		t.Fatal("execute status outside git repo: expected error")
	}
	if result.Status != "failed" || result.ErrorCode != gitcommon.ErrorNotGitRepository {
		t.Fatalf("output = %+v, want not_git_repository failure", result)
	}
}

func TestRuntimeAdapterRejectsParentAndExternalGitControlRepositories(t *testing.T) {
	adapter := NewRuntimeAdapter()

	t.Run("parent repository", func(t *testing.T) {
		parent := newTestRepo(t)
		nested := filepath.Join(parent, "nested-workspace")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
			Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: nested, Operation: gitcommon.OperationStatus,
		})
		if err == nil || result.ErrorCode != gitcommon.ErrorNotGitRepository {
			t.Fatalf("parent repository result=%+v err=%v", result, err)
		}
	})

	for _, markerKind := range []string{"gitdir-file", "gitdir-symlink"} {
		t.Run(markerKind, func(t *testing.T) {
			external := newTestRepo(t)
			workspace := t.TempDir()
			if err := exec.Command("git", "-C", external, "config", "core.worktree", workspace).Run(); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(workspace, ".git")
			if markerKind == "gitdir-file" {
				if err := os.WriteFile(marker, []byte("gitdir: "+filepath.Join(external, ".git")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(external, ".git"), marker); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(workspace, "outside-control.txt"), "must not stage\n")
			result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
				Kind: runtimecontract.StepKindGitWrite, WorkspaceDeliverableDirectory: workspace,
				Operation: gitcommon.OperationAdd, Pathspecs: []string{"outside-control.txt"},
			})
			if err == nil || result.ErrorCode != gitcommon.ErrorNotGitRepository {
				t.Fatalf("external git control result=%+v err=%v", result, err)
			}
			cmd := exec.Command("git", "--git-dir", filepath.Join(external, ".git"), "diff", "--cached", "--name-only")
			output, commandErr := cmd.Output()
			if commandErr != nil || strings.TrimSpace(string(output)) != "" {
				t.Fatalf("external index changed: output=%q err=%v", output, commandErr)
			}
		})
	}
}

func TestRuntimeAdapterStagesWorkspaceSymlinkIdentityWithoutFollowingTarget(t *testing.T) {
	repo := newTestRepo(t)
	external := filepath.Join(t.TempDir(), "external.txt")
	writeFile(t, external, "outside\n")
	if err := os.Symlink(external, filepath.Join(repo, "external-link")); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitWrite, WorkspaceDeliverableDirectory: repo,
		Operation: gitcommon.OperationAdd, Pathspecs: []string{"external-link"},
	})
	if err != nil || result.Status != "completed" {
		t.Fatalf("stage symlink result=%+v err=%v", result, err)
	}
	if content, readErr := os.ReadFile(external); readErr != nil || string(content) != "outside\n" {
		t.Fatalf("outside target changed: content=%q err=%v", content, readErr)
	}
	if staged := gitOutputForTest(t, repo, "ls-files", "-s", "external-link"); !strings.HasPrefix(staged, "120000 ") {
		t.Fatalf("staged entry=%q, want symlink identity", staged)
	}
}

func TestRuntimeAdapterCommitRejectsDetachedHead(t *testing.T) {
	repo := newTestRepo(t)
	runGitForTest(t, repo, "checkout", "--detach", "HEAD")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitForTest(t, repo, "add", "README.md")
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "docs: add readme",
	})
	if err == nil {
		t.Fatal("execute commit on detached HEAD: expected error")
	}
	if result.Status != "rejected" || result.ErrorCode != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want detached HEAD rejection", result)
	}
}

func TestRuntimeAdapterCommitFailsOnExpectedHeadMismatch(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitForTest(t, repo, "add", "README.md")
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "docs: add readme",
		ExpectedHead:                  strings.Repeat("0", 40),
	})
	if err == nil {
		t.Fatal("execute commit with mismatched expectedHead: expected error")
	}
	if result.Status != "failed" || result.ErrorCode != gitcommon.ErrorHeadMismatch {
		t.Fatalf("output = %+v, want head mismatch failure", result)
	}
}

func TestRuntimeAdapterCommitFailsWhenIdentityMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, ".gitignore"), "\n")
	runGitForTest(t, repo, "add", ".gitignore")
	runGitForTest(t, repo, "-c", "user.email=initial@example.test", "-c", "user.name=Initial", "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", "chore: initial")
	runGitForTest(t, repo, "config", "user.useConfigOnly", "true")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitForTest(t, repo, "add", "README.md")
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationCommit,
		Message:                       "docs: add readme",
	})
	if err == nil {
		t.Fatal("execute commit without identity: expected error")
	}
	if result.Status != "failed" || result.ErrorCode != gitcommon.ErrorIdentityMissing {
		t.Fatalf("output = %+v, want identity_missing failure", result)
	}
}

func TestClassifyGitErrorRecognizesLocalizedIdentityMissing(t *testing.T) {
	result := gitCommandResult{
		Stderr: "作者身份未知\nfatal: 未提供邮件地址且自动探测被禁用\n",
	}
	if got := classifyGitError(result, errors.New("exit status 128")); got != gitcommon.ErrorIdentityMissing {
		t.Fatalf("classifyGitError = %q, want %q", got, gitcommon.ErrorIdentityMissing)
	}
}

func TestRuntimeAdapterDiffLargeOutputIsTruncated(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "large.txt"), "base\n")
	runGitForTest(t, repo, "add", "large.txt")
	runGitForTest(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", "test: add large")
	writeFile(t, filepath.Join(repo, "large.txt"), strings.Repeat("changed line\n", 20000))
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Operation:                     gitcommon.OperationDiff,
		Pathspecs:                     []string{"large.txt"},
	})
	if err != nil {
		t.Fatalf("execute diff: %v", err)
	}
	if result.Status != "completed" || result.Truncated != true {
		t.Fatalf("output = %+v, want completed truncated diff", result)
	}
	if len(result.Stdout) > maxDiffBytes+len(gitTruncatedOutputMarker) || result.OutputBytes <= maxDiffBytes || len(result.Evidence.EvidenceRefs) > maxGitEvidenceRefs {
		t.Fatalf("diff output is not bounded: retained=%d total=%d refs=%d", len(result.Stdout), result.OutputBytes, len(result.Evidence.EvidenceRefs))
	}
}

func TestRuntimeAdapterLargeDirtyRepositoryObservationAndEvidenceAreBounded(t *testing.T) {
	repo := newTestRepo(t)
	for idx := range 900 {
		name := filepath.Join(repo, "dirty", strings.Repeat("x", 64), fmt.Sprintf("file-%04d.txt", idx))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, name, "dirty\n")
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindGitRead, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationStatus,
	})
	if err != nil || result.Status != "completed" || !result.Truncated {
		t.Fatalf("large dirty status result=%+v err=%v", result, err)
	}
	if len(result.Stdout) > maxOutputBytes+len(gitTruncatedOutputMarker) || result.OutputBytes <= maxOutputBytes {
		t.Fatalf("status output is not bounded: retained=%d total=%d", len(result.Stdout), result.OutputBytes)
	}
	if len(result.ChangedFiles) > maxChangedFiles || len(result.Evidence.EvidenceRefs) > maxGitEvidenceRefs {
		t.Fatalf("bounded collections changed=%d refs=%d", len(result.ChangedFiles), len(result.Evidence.EvidenceRefs))
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	runGitForTest(t, dir, "init", "-q")
	runGitForTest(t, dir, "config", "user.email", "workorch@example.test")
	runGitForTest(t, dir, "config", "user.name", "WorkOrch Test")
	writeFile(t, filepath.Join(dir, ".gitignore"), "\n")
	runGitForTest(t, dir, "add", ".gitignore")
	runGitForTest(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", "chore: initial")
	return dir
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out[:len(out)-1])
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
