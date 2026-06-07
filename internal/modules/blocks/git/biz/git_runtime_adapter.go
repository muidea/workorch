package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	DefaultCommandTimeout = 30 * time.Second
	MaxCommandTimeout     = 10 * time.Minute
	maxOutputBytes        = 64 * 1024
	maxDiffBytes          = 128 * 1024
	maxChangedFiles       = 500
	defaultLogLimit       = 20
	maxLogLimit           = 100
)

type RuntimeAdapter struct {
	timeout      time.Duration
	writeEnabled bool
	envAllowlist map[string]struct{}
}

type gitCommandResult struct {
	Stdout         string
	Stderr         string
	ExitCode       int
	DurationMillis int64
	Truncated      bool
}

type repoContext struct {
	DeliverableDirectory string
	RepoRoot             string
	Branch               string
	HeadCommit           string
	DetachedHead         bool
	UnbornHead           bool
}

func NewRuntimeAdapter(writeEnabled bool) *RuntimeAdapter {
	return &RuntimeAdapter{
		timeout:      DefaultCommandTimeout,
		writeEnabled: writeEnabled,
		envAllowlist: map[string]struct{}{
			"HOME":   {},
			"PATH":   {},
			"TMPDIR": {},
			"LANG":   {},
		},
	}
}

func NewRuntimeAdapterWithTimeout(writeEnabled bool, timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter(writeEnabled)
	adapter.timeout = timeout
	return adapter
}

func NewRuntimeAdapterFromTimeoutString(writeEnabled bool, timeoutValue string) (*RuntimeAdapter, error) {
	adapter := NewRuntimeAdapter(writeEnabled)
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue == "" {
		return adapter, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must be a valid duration", common.ErrInvalidArgument)
	}
	if timeout < time.Second {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must be at least 1s", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must not exceed %s", common.ErrInvalidArgument, MaxCommandTimeout)
	}
	adapter.timeout = timeout
	return adapter, nil
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	operation := strings.ToLower(strings.TrimSpace(stringFromMap(request.Input, "operation")))
	if operation == "" {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, common.ErrInvalidArgument)
	}
	timeout, err := gitCommandTimeout(request.Input, a.timeout)
	if err != nil {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, err)
	}
	if operation == gitcommon.OperationInit {
		if request.Kind != gitcommon.StepKindWrite {
			return a.rejectedResult(request, operation, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: git init requires git/write", common.ErrInvalidArgument))
		}
		if a == nil || !a.writeEnabled {
			return a.rejectedResult(request, operation, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: git write runtime is disabled", common.ErrInvalidArgument))
		}
		return a.executeInit(ctx, request, timeout)
	}
	repo, err := a.resolveRepo(ctx, request, timeout)
	if err != nil {
		return a.failedResult(request, operation, gitcommon.ErrorNotGitRepository, err)
	}
	switch request.Kind {
	case gitcommon.StepKindRead:
		return a.executeRead(ctx, request, repo, operation, timeout)
	case gitcommon.StepKindWrite:
		if a == nil || !a.writeEnabled {
			return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: git write runtime is disabled", common.ErrInvalidArgument))
		}
		return a.executeWrite(ctx, request, repo, operation, timeout)
	default:
		return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: unsupported git step kind %q", common.ErrInvalidArgument, request.Kind))
	}
}

func (a *RuntimeAdapter) executeRead(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, operation string, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	switch operation {
	case gitcommon.OperationStatus:
		return a.executeStatus(ctx, request, repo, timeout)
	case gitcommon.OperationDiff:
		return a.executeDiff(ctx, request, repo, timeout)
	case gitcommon.OperationLog:
		return a.executeLog(ctx, request, repo, timeout)
	case gitcommon.OperationShow:
		return a.executeShow(ctx, request, repo, timeout)
	case gitcommon.OperationBranch:
		return a.executeBranch(ctx, request, repo, timeout)
	default:
		err := fmt.Errorf("%w: unsupported git read operation %q", common.ErrInvalidArgument, operation)
		return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, err)
	}
}

func (a *RuntimeAdapter) executeWrite(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, operation string, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	switch operation {
	case gitcommon.OperationInit:
		return a.executeInit(ctx, request, timeout)
	case gitcommon.OperationAdd:
		return a.executeAdd(ctx, request, repo, timeout)
	case gitcommon.OperationCommit:
		return a.executeCommit(ctx, request, repo, timeout)
	default:
		err := fmt.Errorf("%w: unsupported git write operation %q", common.ErrInvalidArgument, operation)
		return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, err)
	}
}

func (a *RuntimeAdapter) executeInit(ctx context.Context, request runtimemodel.StepExecutionRequest, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	deliverable, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || deliverable == "" {
		return a.rejectedResult(request, gitcommon.OperationInit, gitcommon.ErrorInvalidRequest, common.ErrInvalidArgument)
	}
	info, err := os.Stat(deliverable)
	if err != nil {
		return a.failedResult(request, gitcommon.OperationInit, gitcommon.ErrorInvalidRequest, err)
	}
	if !info.IsDir() {
		err := fmt.Errorf("%w: workspace deliverable directory must be a directory", common.ErrInvalidArgument)
		return a.rejectedResult(request, gitcommon.OperationInit, gitcommon.ErrorInvalidRequest, err)
	}
	if repo, ok, existingErr := a.existingRepoContext(ctx, request, deliverable, timeout); existingErr != nil {
		return a.failedResult(request, gitcommon.OperationInit, gitcommon.ErrorNotGitRepository, existingErr)
	} else if ok {
		output := a.baseOutput(request, repo, gitcommon.OperationInit, gitCommandResult{}, "completed")
		output["zeroWork"] = true
		output["initialized"] = false
		output["existingRepository"] = true
		if repo.HeadCommit == "" {
			output["unbornHead"] = true
		}
		output["summary"] = "git init skipped because workspace repository already exists"
		return runtimemodel.StepExecutionResult{Output: output}, nil
	}
	cmd, err := a.runGit(ctx, deliverable, timeout, maxOutputBytes, "init", "-q")
	if err != nil {
		return a.commandFailureResult(request, repoContext{DeliverableDirectory: deliverable, RepoRoot: deliverable}, gitcommon.OperationInit, cmd, classifyGitError(cmd, err), err)
	}
	for _, args := range [][]string{
		{"config", "user.name", "WorkOrch"},
		{"config", "user.email", "workorch@example.local"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		if configCmd, configErr := a.runGit(ctx, deliverable, timeout, maxOutputBytes, args...); configErr != nil {
			return a.commandFailureResult(request, repoContext{DeliverableDirectory: deliverable, RepoRoot: deliverable}, gitcommon.OperationInit, configCmd, classifyGitError(configCmd, configErr), configErr)
		}
	}
	repo, err := a.resolveRepo(ctx, request, timeout)
	if err != nil {
		return a.failedResult(request, gitcommon.OperationInit, gitcommon.ErrorNotGitRepository, err)
	}
	output := a.baseOutput(request, repo, gitcommon.OperationInit, cmd, "completed")
	output["initialized"] = true
	output["existingRepository"] = false
	output["unbornHead"] = true
	output["summary"] = "git init completed"
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeStatus(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	pathspecs, err := optionalPathspecs(request.Input)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationStatus, repo, gitcommon.ErrorPathRejected, err)
	}
	args := append([]string{"status", "--porcelain=v2", "--branch", "--untracked-files=all"}, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationStatus, cmd, classifyGitError(cmd, err), err)
	}
	files, aheadBehind, branch, head := parsePorcelainStatus(cmd.Stdout)
	output := a.baseOutput(request, repo, gitcommon.OperationStatus, cmd, "completed")
	output["pathspecs"] = pathspecs
	output["changedFiles"] = limitChangedFiles(files)
	output["aheadBehind"] = aheadBehind
	if branch != "" {
		output["branch"] = branch
	}
	if head == "(initial)" {
		output["unbornHead"] = true
	} else if head != "" {
		output["headCommit"] = head
	}
	output["summary"] = fmt.Sprintf("git status completed with %d changed file(s)", len(files))
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeDiff(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	pathspecs, err := optionalPathspecs(request.Input)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationDiff, repo, gitcommon.ErrorPathRejected, err)
	}
	args := []string{"diff"}
	if ref := strings.TrimSpace(stringFromMap(request.Input, "ref")); ref != "" {
		if base := strings.TrimSpace(stringFromMap(request.Input, "baseRef")); base != "" {
			args = append(args, base+".."+ref)
		} else {
			args = append(args, ref)
		}
	} else if base := strings.TrimSpace(stringFromMap(request.Input, "baseRef")); base != "" {
		args = append(args, base)
	}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxDiffBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationDiff, cmd, classifyGitError(cmd, err), err)
	}
	statusFiles, _ := a.diffChangedFiles(ctx, repo.RepoRoot, timeout, pathspecs)
	output := a.baseOutput(request, repo, gitcommon.OperationDiff, cmd, "completed")
	output["pathspecs"] = pathspecs
	output["changedFiles"] = limitChangedFiles(statusFiles)
	output["summary"] = fmt.Sprintf("git diff completed with %d changed file(s)", len(statusFiles))
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeLog(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	pathspecs, err := optionalPathspecs(request.Input)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationLog, repo, gitcommon.ErrorPathRejected, err)
	}
	limit, clamped := logLimit(request.Input)
	ref := strings.TrimSpace(stringFromMap(request.Input, "ref"))
	if ref == "" {
		ref = "HEAD"
	}
	format := "%H%x1f%h%x1f%an <%ae>%x1f%aI%x1f%s"
	args := []string{"log", "--format=" + format, "-n", strconv.Itoa(limit), ref}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationLog, cmd, classifyGitError(cmd, err), err)
	}
	commits := parseLogCommits(cmd.Stdout)
	output := a.baseOutput(request, repo, gitcommon.OperationLog, cmd, "completed")
	output["pathspecs"] = pathspecs
	output["commits"] = commits
	output["limit"] = limit
	output["limitClamped"] = clamped
	output["summary"] = fmt.Sprintf("git log completed with %d commit(s)", len(commits))
	if clamped {
		output["summary"] = output["summary"].(string) + "; limit clamped to maximum"
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeShow(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	pathspecs, err := optionalPathspecs(request.Input)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationShow, repo, gitcommon.ErrorPathRejected, err)
	}
	ref := strings.TrimSpace(stringFromMap(request.Input, "ref"))
	if ref == "" {
		ref = "HEAD"
	}
	args := []string{"show", "--stat", "--summary", "--format=%H%x1f%h%x1f%an <%ae>%x1f%aI%x1f%s", ref}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxDiffBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationShow, cmd, classifyGitError(cmd, err), err)
	}
	output := a.baseOutput(request, repo, gitcommon.OperationShow, cmd, "completed")
	output["pathspecs"] = pathspecs
	output["ref"] = ref
	output["summary"] = "git show completed"
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeBranch(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, "branch", "--format=%(refname:short)%09%(objectname)%09%(HEAD)%09%(upstream:short)")
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationBranch, cmd, classifyGitError(cmd, err), err)
	}
	output := a.baseOutput(request, repo, gitcommon.OperationBranch, cmd, "completed")
	output["branches"] = parseBranches(cmd.Stdout)
	output["detachedHead"] = repo.DetachedHead
	output["summary"] = "git branch completed"
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeAdd(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	pathspecs, err := requiredPathspecs(request.Input)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationAdd, repo, gitcommon.ErrorPathRejected, err)
	}
	args := append([]string{"add"}, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationAdd, cmd, classifyGitError(cmd, err), err)
	}
	staged, _ := a.stagedFiles(ctx, repo.RepoRoot, timeout)
	output := a.baseOutput(request, repo, gitcommon.OperationAdd, cmd, "completed")
	output["pathspecs"] = pathspecs
	output["stagedFiles"] = staged
	output["changedFiles"] = staged
	if len(staged) == 0 {
		output["zeroWork"] = true
	}
	output["summary"] = fmt.Sprintf("git add completed with %d staged file(s)", len(staged))
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) executeCommit(ctx context.Context, request runtimemodel.StepExecutionRequest, repo repoContext, timeout time.Duration) (runtimemodel.StepExecutionResult, error) {
	if _, ok := request.Input["pathspecs"]; ok {
		err := fmt.Errorf("%w: git commit does not accept pathspecs; stage files with git add first", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorPathRejected, err)
	}
	if repo.DetachedHead {
		err := fmt.Errorf("%w: git commit is rejected on detached HEAD", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorOperationDenied, err)
	}
	message := strings.TrimSpace(stringFromMap(request.Input, "message"))
	if message == "" {
		err := fmt.Errorf("%w: git commit message is required", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorInvalidRequest, err)
	}
	expectedHead := strings.TrimSpace(stringFromMap(request.Input, "expectedHead"))
	if expectedHead != "" && repo.HeadCommit != "" && expectedHead != repo.HeadCommit {
		err := fmt.Errorf("%w: git HEAD mismatch", common.ErrInvalidArgument)
		return a.failedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorHeadMismatch, err)
	}
	staged, err := a.stagedFiles(ctx, repo.RepoRoot, timeout)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationCommit, gitCommandResult{}, gitcommon.ErrorGitCommandFailed, err)
	}
	if len(staged) == 0 {
		output := a.baseOutput(request, repo, gitcommon.OperationCommit, gitCommandResult{}, "completed")
		output["zeroWork"] = true
		output["errorCode"] = gitcommon.ErrorNoStagedChanges
		output["stagedFiles"] = []map[string]any{}
		output["summary"] = "git commit completed with no staged changes"
		return runtimemodel.StepExecutionResult{Output: output}, nil
	}
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", message)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationCommit, cmd, classifyGitError(cmd, err), err)
	}
	nextHead, _ := a.gitOutput(ctx, repo.RepoRoot, timeout, "rev-parse", "HEAD")
	output := a.baseOutput(request, repo, gitcommon.OperationCommit, cmd, "completed")
	output["headCommitBefore"] = repo.HeadCommit
	output["headCommitAfter"] = strings.TrimSpace(nextHead)
	output["message"] = message
	output["stagedFiles"] = staged
	output["committedFiles"] = staged
	output["changedFiles"] = staged
	output["commit"] = map[string]any{
		"hash":    strings.TrimSpace(nextHead),
		"subject": firstLine(message),
	}
	output["summary"] = "git commit completed"
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

func (a *RuntimeAdapter) resolveRepo(ctx context.Context, request runtimemodel.StepExecutionRequest, timeout time.Duration) (repoContext, error) {
	deliverable, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || deliverable == "" {
		return repoContext{}, common.ErrInvalidArgument
	}
	root, err := a.gitOutput(ctx, deliverable, timeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return repoContext{}, fmt.Errorf("%w: git repository not found", common.ErrInvalidArgument)
	}
	root = strings.TrimSpace(root)
	rootAbs, err := filepath.Abs(root)
	if err != nil || rootAbs == "" {
		return repoContext{}, common.ErrInvalidArgument
	}
	if !samePath(deliverable, rootAbs) {
		return repoContext{}, fmt.Errorf("%w: git repository root must match workspace deliverable directory", common.ErrInvalidArgument)
	}
	bare, err := a.gitOutput(ctx, rootAbs, timeout, "rev-parse", "--is-bare-repository")
	if err != nil {
		return repoContext{}, err
	}
	if strings.TrimSpace(bare) == "true" {
		return repoContext{}, fmt.Errorf("%w: bare git repository is not supported", common.ErrInvalidArgument)
	}
	branch, _ := a.gitOutput(ctx, rootAbs, timeout, "symbolic-ref", "--short", "HEAD")
	head, _ := a.gitOutput(ctx, rootAbs, timeout, "rev-parse", "--verify", "HEAD")
	branch = strings.TrimSpace(branch)
	head = strings.TrimSpace(head)
	if branch == "" {
		abbrev, _ := a.gitOutput(ctx, rootAbs, timeout, "rev-parse", "--abbrev-ref", "HEAD")
		abbrev = strings.TrimSpace(abbrev)
		if abbrev != "" && abbrev != "HEAD" {
			branch = abbrev
		}
	}
	detachedHead := branch == "" && head != ""
	if branch == "" {
		branch = "HEAD"
	}
	return repoContext{
		DeliverableDirectory: deliverable,
		RepoRoot:             rootAbs,
		Branch:               branch,
		HeadCommit:           head,
		DetachedHead:         detachedHead,
		UnbornHead:           head == "",
	}, nil
}

func (a *RuntimeAdapter) existingRepoContext(ctx context.Context, request runtimemodel.StepExecutionRequest, deliverable string, timeout time.Duration) (repoContext, bool, error) {
	if _, err := os.Stat(filepath.Join(deliverable, ".git")); err != nil {
		if os.IsNotExist(err) {
			return repoContext{}, false, nil
		}
		return repoContext{}, false, err
	}
	repo, err := a.resolveRepo(ctx, request, timeout)
	if err != nil {
		return repoContext{}, false, err
	}
	return repo, true, nil
}

func (a *RuntimeAdapter) runGit(ctx context.Context, repoRoot string, timeout time.Duration, limit int64, args ...string) (gitCommandResult, error) {
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = repoRoot
	cmd.Env = a.filteredEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	out, outTruncated := truncateString(stdout.String(), limit)
	errText, errTruncated := truncateString(stderr.String(), maxOutputBytes)
	result := gitCommandResult{
		Stdout:         out,
		Stderr:         errText,
		ExitCode:       exitCode,
		DurationMillis: duration.Milliseconds(),
		Truncated:      outTruncated || errTruncated,
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return result, context.DeadlineExceeded
	}
	return result, err
}

func (a *RuntimeAdapter) gitOutput(ctx context.Context, repoRoot string, timeout time.Duration, args ...string) (string, error) {
	result, err := a.runGit(ctx, repoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return result.Stdout, err
	}
	return result.Stdout, nil
}

func (a *RuntimeAdapter) filteredEnv() []string {
	if a == nil || len(a.envAllowlist) == 0 {
		return []string{"GIT_TERMINAL_PROMPT=0"}
	}
	out := make([]string, 0, len(a.envAllowlist)+1)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, allowed := a.envAllowlist[key]; allowed {
			out = append(out, item)
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	return out
}

func (a *RuntimeAdapter) stagedFiles(ctx context.Context, repoRoot string, timeout time.Duration) ([]map[string]any, error) {
	cmd, err := a.runGit(ctx, repoRoot, timeout, maxOutputBytes, "diff", "--cached", "--name-status")
	if err != nil {
		return nil, err
	}
	return parseNameStatus(cmd.Stdout, true), nil
}

func (a *RuntimeAdapter) diffChangedFiles(ctx context.Context, repoRoot string, timeout time.Duration, pathspecs []string) ([]map[string]any, error) {
	args := append([]string{"diff", "--name-status"}, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return nil, err
	}
	files := parseNameStatus(cmd.Stdout, false)
	if len(files) > 0 {
		return files, nil
	}
	args = append([]string{"diff", "--cached", "--name-status"}, pathspecArgs(pathspecs)...)
	cmd, err = a.runGit(ctx, repoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(cmd.Stdout, true), nil
}

func (a *RuntimeAdapter) baseOutput(_ runtimemodel.StepExecutionRequest, repo repoContext, operation string, cmd gitCommandResult, status string) map[string]any {
	output := map[string]any{
		"status":         status,
		"operation":      operation,
		"stage":          stageForOperation(operation),
		"repoRoot":       repo.RepoRoot,
		"workingDir":     repo.RepoRoot,
		"branch":         repo.Branch,
		"headCommit":     repo.HeadCommit,
		"detachedHead":   repo.DetachedHead,
		"stdout":         cmd.Stdout,
		"stderr":         cmd.Stderr,
		"exitCode":       cmd.ExitCode,
		"durationMillis": cmd.DurationMillis,
		"timeoutMillis":  a.timeout.Milliseconds(),
		"truncated":      cmd.Truncated,
	}
	if repo.UnbornHead {
		output["unbornHead"] = true
	}
	return output
}

func (a *RuntimeAdapter) rejectedResult(request runtimemodel.StepExecutionRequest, operation string, code string, err error) (runtimemodel.StepExecutionResult, error) {
	return a.rejectedResultWithRepo(request, operation, repoContext{}, code, err)
}

func (a *RuntimeAdapter) rejectedResultWithRepo(request runtimemodel.StepExecutionRequest, operation string, repo repoContext, code string, err error) (runtimemodel.StepExecutionResult, error) {
	output := a.baseOutput(request, repo, operation, gitCommandResult{}, "rejected")
	output["errorCode"] = code
	output["error"] = err.Error()
	output["summary"] = "git operation rejected"
	return runtimemodel.StepExecutionResult{Output: output}, err
}

func (a *RuntimeAdapter) failedResult(request runtimemodel.StepExecutionRequest, operation string, code string, err error) (runtimemodel.StepExecutionResult, error) {
	return a.failedResultWithRepo(request, operation, repoContext{}, code, err)
}

func (a *RuntimeAdapter) failedResultWithRepo(request runtimemodel.StepExecutionRequest, operation string, repo repoContext, code string, err error) (runtimemodel.StepExecutionResult, error) {
	output := a.baseOutput(request, repo, operation, gitCommandResult{}, "failed")
	output["errorCode"] = code
	output["error"] = err.Error()
	output["summary"] = "git operation failed"
	return runtimemodel.StepExecutionResult{Output: output}, err
}

func (a *RuntimeAdapter) commandFailureResult(request runtimemodel.StepExecutionRequest, repo repoContext, operation string, cmd gitCommandResult, code string, err error) (runtimemodel.StepExecutionResult, error) {
	status := "failed"
	if errors.Is(err, context.DeadlineExceeded) {
		status = "timeout"
		code = gitcommon.ErrorTimeout
	}
	output := a.baseOutput(request, repo, operation, cmd, status)
	output["errorCode"] = code
	output["error"] = err.Error()
	output["summary"] = "git command failed"
	return runtimemodel.StepExecutionResult{Output: output}, err
}

func stageForOperation(operation string) string {
	switch operation {
	case gitcommon.OperationInit, gitcommon.OperationAdd, gitcommon.OperationCommit:
		return "git_write"
	default:
		return "git_read"
	}
}

func gitCommandTimeout(input map[string]any, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultCommandTimeout
	}
	if input == nil || input["timeoutMillis"] == nil {
		return defaultTimeout, nil
	}
	value, ok := numericMillis(input["timeoutMillis"])
	if !ok || value <= 0 {
		return 0, common.ErrInvalidArgument
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout < time.Second {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
	}
	return timeout, nil
}

func numericMillis(input any) (int64, bool) {
	switch typed := input.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		if typed != float32(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func optionalPathspecs(input map[string]any) ([]string, error) {
	if input == nil || input["pathspecs"] == nil {
		return nil, nil
	}
	return parsePathspecs(input["pathspecs"], false)
}

func requiredPathspecs(input map[string]any) ([]string, error) {
	if input == nil {
		return nil, common.ErrInvalidArgument
	}
	return parsePathspecs(input["pathspecs"], true)
}

func parsePathspecs(input any, required bool) ([]string, error) {
	var raw []string
	switch typed := input.(type) {
	case nil:
	case []string:
		raw = append(raw, typed...)
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			raw = append(raw, value)
		}
	default:
		return nil, common.ErrInvalidArgument
	}
	if required && len(raw) == 0 {
		return nil, fmt.Errorf("%w: pathspecs are required", common.ErrInvalidArgument)
	}
	ret := make([]string, 0, len(raw))
	for _, item := range raw {
		pathspec, err := normalizePathspec(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, pathspec)
	}
	return ret, nil
}

func normalizePathspec(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: empty pathspec is not allowed", common.ErrInvalidArgument)
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: absolute pathspec is not allowed", common.ErrInvalidArgument)
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, ":") {
		return "", fmt.Errorf("%w: git option or magic pathspec is not allowed", common.ErrInvalidArgument)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == "*" || cleaned == ":/" {
		return "", fmt.Errorf("%w: broad pathspec is not allowed", common.ErrInvalidArgument)
	}
	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == ".." {
			return "", fmt.Errorf("%w: parent pathspec is not allowed", common.ErrInvalidArgument)
		}
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git"+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: .git pathspec is not allowed", common.ErrInvalidArgument)
	}
	return filepath.ToSlash(cleaned), nil
}

func pathspecArgs(pathspecs []string) []string {
	if len(pathspecs) == 0 {
		return nil
	}
	ret := []string{"--"}
	ret = append(ret, pathspecs...)
	return ret
}

func parsePorcelainStatus(stdout string) ([]map[string]any, map[string]any, string, string) {
	var files []map[string]any
	aheadBehind := map[string]any{}
	branch := ""
	head := ""
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# branch.head ") {
			branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			continue
		}
		if strings.HasPrefix(line, "# branch.oid ") {
			head = strings.TrimSpace(strings.TrimPrefix(line, "# branch.oid "))
			continue
		}
		if strings.HasPrefix(line, "# branch.ab ") {
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) >= 2 {
				aheadBehind["ahead"] = trimSignedInt(fields[0])
				aheadBehind["behind"] = trimSignedInt(fields[1])
			}
			continue
		}
		if strings.HasPrefix(line, "? ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "? "))
			files = append(files, map[string]any{"path": path, "status": "untracked", "untracked": true})
			continue
		}
		if strings.HasPrefix(line, "1 ") {
			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}
			xy := fields[1]
			path := strings.Join(fields[8:], " ")
			files = append(files, map[string]any{
				"path":     path,
				"status":   statusFromXY(xy),
				"staged":   len(xy) > 0 && xy[0] != '.',
				"unstaged": len(xy) > 1 && xy[1] != '.',
			})
			continue
		}
	}
	return files, aheadBehind, branch, head
}

func parseNameStatus(stdout string, staged bool) []map[string]any {
	var files []map[string]any
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := statusFromCode(fields[0])
		path := fields[len(fields)-1]
		item := map[string]any{"path": path, "status": status, "staged": staged}
		if strings.HasPrefix(fields[0], "R") && len(fields) >= 3 {
			item["oldPath"] = fields[1]
		}
		files = append(files, item)
	}
	return files
}

func parseLogCommits(stdout string) []map[string]any {
	var commits []map[string]any
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x1f", 5)
		if len(fields) != 5 {
			continue
		}
		commits = append(commits, map[string]any{
			"hash":       fields[0],
			"shortHash":  fields[1],
			"author":     fields[2],
			"authorTime": fields[3],
			"subject":    fields[4],
		})
	}
	return commits
}

func parseBranches(stdout string) []map[string]any {
	var branches []map[string]any
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		item := map[string]any{
			"name":    fields[0],
			"commit":  fields[1],
			"current": fields[2] == "*",
		}
		if len(fields) > 3 && fields[3] != "" {
			item["upstream"] = fields[3]
		}
		branches = append(branches, item)
	}
	return branches
}

func logLimit(input map[string]any) (int, bool) {
	if input == nil || input["limit"] == nil {
		return defaultLogLimit, false
	}
	value, ok := numericMillis(input["limit"])
	if !ok || value <= 0 {
		return defaultLogLimit, false
	}
	if value > maxLogLimit {
		return maxLogLimit, true
	}
	return int(value), false
}

func statusFromXY(xy string) string {
	if strings.Contains(xy, "U") {
		return "conflicted"
	}
	for _, ch := range xy {
		switch ch {
		case 'A':
			return "added"
		case 'M':
			return "modified"
		case 'D':
			return "deleted"
		case 'R':
			return "renamed"
		case 'C':
			return "copied"
		}
	}
	return "modified"
}

func statusFromCode(code string) string {
	switch {
	case strings.HasPrefix(code, "A"):
		return "added"
	case strings.HasPrefix(code, "M"):
		return "modified"
	case strings.HasPrefix(code, "D"):
		return "deleted"
	case strings.HasPrefix(code, "R"):
		return "renamed"
	case strings.HasPrefix(code, "C"):
		return "copied"
	case strings.HasPrefix(code, "U"):
		return "conflicted"
	default:
		return strings.ToLower(code)
	}
}

func classifyGitError(result gitCommandResult, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return gitcommon.ErrorTimeout
	}
	text := strings.ToLower(result.Stderr + "\n" + err.Error())
	switch {
	case strings.Contains(text, "not a git repository"):
		return gitcommon.ErrorNotGitRepository
	case strings.Contains(text, "author identity unknown") || strings.Contains(text, "unable to auto-detect email address"):
		return gitcommon.ErrorIdentityMissing
	default:
		return gitcommon.ErrorGitCommandFailed
	}
}

func truncateString(value string, limit int64) (string, bool) {
	if limit <= 0 || int64(len(value)) <= limit {
		return value, false
	}
	n := int(limit)
	if limit < 32 {
		return value[:n], true
	}
	return value[:n] + "\n[truncated]", true
}

func limitChangedFiles(files []map[string]any) []map[string]any {
	if len(files) <= maxChangedFiles {
		return files
	}
	return files[:maxChangedFiles]
}

func stringFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func trimSignedInt(value string) int {
	value = strings.TrimLeft(strings.TrimSpace(value), "+")
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func samePath(left string, right string) bool {
	leftEval, leftErr := filepath.EvalSymlinks(left)
	rightEval, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil {
		left = leftEval
	}
	if rightErr == nil {
		right = rightEval
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
