package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterReadStatus(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")

	adapter := NewRuntimeAdapter(false)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationStatus},
	})
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["operation"] != gitcommon.OperationStatus {
		t.Fatalf("status output = %+v, want completed status", result.Output)
	}
	files, ok := result.Output["changedFiles"].([]map[string]any)
	if !ok || len(files) != 1 || files[0]["path"] != "README.md" {
		t.Fatalf("changedFiles = %#v, want README.md", result.Output["changedFiles"])
	}
}

func TestRuntimeAdapterReadStatusInEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")

	adapter := NewRuntimeAdapter(false)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationStatus},
	})
	if err != nil {
		t.Fatalf("execute status in empty repo: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["operation"] != gitcommon.OperationStatus {
		t.Fatalf("status output = %+v, want completed status", result.Output)
	}
	if result.Output["headCommit"] != "" {
		t.Fatalf("headCommit = %v, want empty value before first commit", result.Output["headCommit"])
	}
	if result.Output["unbornHead"] != true {
		t.Fatalf("unbornHead = %v, want true before first commit", result.Output["unbornHead"])
	}
}

func TestRuntimeAdapterReadLogAndShowInEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "-q")

	adapter := NewRuntimeAdapter(false)
	for _, operation := range []string{gitcommon.OperationLog, gitcommon.OperationShow} {
		result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
			Kind:                          gitcommon.StepKindRead,
			WorkspaceDeliverableDirectory: repo,
			Input:                         map[string]any{"operation": operation},
		})
		if err != nil {
			t.Fatalf("execute %s in empty repo: %v\noutput=%+v", operation, err, result.Output)
		}
		if result.Output["status"] != "completed" || result.Output["zeroWork"] != true || result.Output["unbornHead"] != true {
			t.Fatalf("%s output = %+v, want completed zero-work unborn HEAD baseline", operation, result.Output)
		}
		if operation == gitcommon.OperationLog {
			commits, ok := result.Output["commits"].([]map[string]any)
			if !ok || len(commits) != 0 {
				t.Fatalf("commits = %#v, want empty list", result.Output["commits"])
			}
		}
	}
}

func TestRuntimeAdapterReadPathspecDotMeansRepositoryRoot(t *testing.T) {
	repo := newTestRepo(t)

	adapter := NewRuntimeAdapter(false)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationStatus, "pathspecs": []any{"."}},
	})
	if err != nil {
		t.Fatalf("execute status with root pathspec: %v\noutput=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("status output = %+v, want completed", result.Output)
	}
	pathspecs, ok := result.Output["pathspecs"].([]string)
	if !ok || len(pathspecs) != 0 {
		t.Fatalf("pathspecs = %#v, want repository-root pathspec normalized away", result.Output["pathspecs"])
	}
}

func TestRuntimeAdapterWriteInitInitializesWorkspaceRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	repo := t.TempDir()

	adapter := NewRuntimeAdapter(true)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationInit},
	})
	if err != nil {
		t.Fatalf("execute init: %v\noutput=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" || result.Output["operation"] != gitcommon.OperationInit {
		t.Fatalf("init output = %+v, want completed init", result.Output)
	}
	if result.Output["initialized"] != true || result.Output["existingRepository"] != false {
		t.Fatalf("init flags = %+v, want initialized new repository", result.Output)
	}
	if result.Output["unbornHead"] != true || result.Output["headCommit"] != "" {
		t.Fatalf("head output = %+v, want unborn repository without head commit", result.Output)
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

	adapter := NewRuntimeAdapter(true)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationInit},
	})
	if err != nil {
		t.Fatalf("execute idempotent init: %v\noutput=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" || result.Output["operation"] != gitcommon.OperationInit {
		t.Fatalf("init output = %+v, want completed init", result.Output)
	}
	if result.Output["zeroWork"] != true || result.Output["existingRepository"] != true || result.Output["initialized"] != false {
		t.Fatalf("init flags = %+v, want idempotent existing repository", result.Output)
	}
	if result.Output["headCommit"] == "" {
		t.Fatalf("headCommit empty for existing committed repository: %+v", result.Output)
	}
}

func TestRuntimeAdapterReadInitRejected(t *testing.T) {
	dir := t.TempDir()
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: dir,
		Input:                         map[string]any{"operation": gitcommon.OperationInit},
	})
	if err == nil {
		t.Fatal("execute init with git/read: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want init rejected for git/read", result.Output)
	}
}

func TestRuntimeAdapterWriteInitRejectedWhenWriteDisabled(t *testing.T) {
	dir := t.TempDir()
	adapter := NewRuntimeAdapter(false)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: dir,
		Input:                         map[string]any{"operation": gitcommon.OperationInit},
	})
	if err == nil {
		t.Fatal("execute init with git/write disabled: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want init rejected when write disabled", result.Output)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git stat = %v, want no repository initialized", err)
	}
}

func TestRuntimeAdapterRejectsUnsafePathspec(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []any{"../outside"}},
	})
	if err == nil {
		t.Fatal("execute add with unsafe pathspec: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected path", result.Output)
	}
}

func TestRuntimeAdapterWritePathspecDotStillRejected(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []any{"."}},
	})
	if err == nil {
		t.Fatal("execute add with broad root pathspec: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected broad root pathspec", result.Output)
	}
}

func TestRuntimeAdapterWriteDisabledRejectsAdd(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	adapter := NewRuntimeAdapter(false)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []any{"README.md"}},
	})
	if err == nil {
		t.Fatal("execute add with write disabled: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want write rejected", result.Output)
	}
}

func TestRuntimeAdapterAddAndCommitDoesNotRunHooks(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	writeFile(t, filepath.Join(repo, ".git", "hooks", "post-commit"), "#!/bin/sh\ntouch hook-ran\n")
	if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "post-commit"), 0o755); err != nil {
		t.Fatalf("chmod hook: %v", err)
	}
	adapter := NewRuntimeAdapter(true)

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []any{"README.md"}},
	}); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "docs: add readme", "expectedHead": head},
	})
	if err != nil {
		t.Fatalf("execute commit: %v\noutput=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("commit output = %+v, want completed", result.Output)
	}
	commit, ok := result.Output["commit"].(map[string]any)
	if !ok || commit["hash"] == "" {
		t.Fatalf("commit = %#v, want hash", result.Output["commit"])
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
	adapter := NewRuntimeAdapter(true)

	status, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationStatus},
	})
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if status.Output["detachedHead"] == true || status.Output["unbornHead"] != true {
		t.Fatalf("status output = %+v, want unborn branch without detached HEAD", status.Output)
	}
	if _, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []any{"README.md"}},
	}); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "docs: initial readme"},
	})
	if err != nil {
		t.Fatalf("execute initial commit: %v\noutput=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" || result.Output["detachedHead"] == true {
		t.Fatalf("commit output = %+v, want completed non-detached commit", result.Output)
	}
	if result.Output["headCommitAfter"] == "" {
		t.Fatalf("commit output = %+v, want headCommitAfter", result.Output)
	}
}

func TestRuntimeAdapterCommitRejectsPathspec(t *testing.T) {
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "bad", "pathspecs": []any{"README.md"}},
	})
	if err == nil {
		t.Fatal("execute commit with pathspecs: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorPathRejected {
		t.Fatalf("output = %+v, want rejected pathspec", result.Output)
	}
}

func TestRuntimeAdapterFailsOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: dir,
		Input:                         map[string]any{"operation": gitcommon.OperationStatus},
	})
	if err == nil {
		t.Fatal("execute status outside git repo: expected error")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != gitcommon.ErrorNotGitRepository {
		t.Fatalf("output = %+v, want not_git_repository failure", result.Output)
	}
}

func TestRuntimeAdapterCommitRejectsDetachedHead(t *testing.T) {
	repo := newTestRepo(t)
	runGitForTest(t, repo, "checkout", "--detach", "HEAD")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitForTest(t, repo, "add", "README.md")
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "docs: add readme"},
	})
	if err == nil {
		t.Fatal("execute commit on detached HEAD: expected error")
	}
	if result.Output["status"] != "rejected" || result.Output["errorCode"] != gitcommon.ErrorOperationDenied {
		t.Fatalf("output = %+v, want detached HEAD rejection", result.Output)
	}
}

func TestRuntimeAdapterCommitFailsOnExpectedHeadMismatch(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitForTest(t, repo, "add", "README.md")
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "docs: add readme", "expectedHead": strings.Repeat("0", 40)},
	})
	if err == nil {
		t.Fatal("execute commit with mismatched expectedHead: expected error")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != gitcommon.ErrorHeadMismatch {
		t.Fatalf("output = %+v, want head mismatch failure", result.Output)
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
	adapter := NewRuntimeAdapter(true)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindWrite,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationCommit, "message": "docs: add readme"},
	})
	if err == nil {
		t.Fatal("execute commit without identity: expected error")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != gitcommon.ErrorIdentityMissing {
		t.Fatalf("output = %+v, want identity_missing failure", result.Output)
	}
}

func TestRuntimeAdapterDiffLargeOutputIsTruncated(t *testing.T) {
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "large.txt"), "base\n")
	runGitForTest(t, repo, "add", "large.txt")
	runGitForTest(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", "test: add large")
	writeFile(t, filepath.Join(repo, "large.txt"), strings.Repeat("changed line\n", 20000))
	adapter := NewRuntimeAdapter(false)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          gitcommon.StepKindRead,
		WorkspaceDeliverableDirectory: repo,
		Input:                         map[string]any{"operation": gitcommon.OperationDiff, "pathspecs": []any{"large.txt"}},
	})
	if err != nil {
		t.Fatalf("execute diff: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["truncated"] != true {
		t.Fatalf("output = %+v, want completed truncated diff", result.Output)
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
