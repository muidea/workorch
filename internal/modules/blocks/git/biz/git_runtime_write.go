package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (a *RuntimeAdapter) executeWrite(ctx context.Context, request RuntimeOperationRequest, repo repoContext, operation string, timeout time.Duration) (RuntimeOperationResult, error) {
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

func (a *RuntimeAdapter) executeInit(ctx context.Context, request RuntimeOperationRequest, timeout time.Duration) (RuntimeOperationResult, error) {
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
		output.ZeroWork = true
		output.Initialized = false
		output.ExistingRepository = true
		if repo.HeadCommit == "" {
			output.UnbornHead = true
		}
		output.Summary = "git init skipped because workspace repository already exists"
		return finalizeGitResult(output, nil)
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
	output.Initialized = true
	output.ExistingRepository = false
	output.UnbornHead = true
	output.Summary = "git init completed"
	return finalizeGitResult(output, nil)
}

func (a *RuntimeAdapter) executeAdd(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	pathspecs, err := requiredPathspecs(request.Pathspecs)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationAdd, repo, gitcommon.ErrorPathRejected, err)
	}
	stateRequest := request
	stateRequest.Operation = gitcommon.OperationStatus
	stateRequest.Pathspecs = nil
	beforeStatus, err := a.executeStatus(ctx, stateRequest, repo, timeout)
	if err != nil || beforeStatus.Truncated {
		if err == nil {
			err = fmt.Errorf("%w: Git repository state exceeds the bounded owner observation", common.ErrConflict)
		}
		return a.failedResultWithRepo(request, gitcommon.OperationAdd, repo, gitcommon.ErrorGitCommandFailed, err)
	}
	beforeUnstaged, beforeStaged := gitStatusStateFlags(beforeStatus.ChangedFiles)
	beforeRevision := gitRepositoryStateRevision(beforeStatus, true, beforeUnstaged, beforeStaged)
	args := append([]string{"add"}, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationAdd, cmd, classifyGitError(cmd, err), err)
	}
	afterStatus, err := a.executeStatus(ctx, stateRequest, repo, timeout)
	if err != nil || afterStatus.Truncated {
		if err == nil {
			err = fmt.Errorf("%w: Git repository state exceeds the bounded owner observation", common.ErrConflict)
		}
		return a.failedResultWithRepo(request, gitcommon.OperationAdd, repo, gitcommon.ErrorGitCommandFailed, err)
	}
	afterUnstaged, afterStaged := gitStatusStateFlags(afterStatus.ChangedFiles)
	afterRevision := gitRepositoryStateRevision(afterStatus, true, afterUnstaged, afterStaged)
	staged, _ := a.stagedFiles(ctx, repo.RepoRoot, timeout)
	changed := gitAddIndexDelta(beforeStatus.ChangedFiles, afterStatus.ChangedFiles)
	output := a.baseOutput(request, repo, gitcommon.OperationAdd, cmd, "completed")
	output.Pathspecs = pathspecs
	output.StagedFiles = staged
	output.ChangedFiles = changed
	output.BeforeHash = beforeRevision
	output.AfterHash = afterRevision
	if beforeRevision == afterRevision {
		output.ZeroWork = true
		output.ChangedFiles = nil
	}
	output.Summary = fmt.Sprintf("git add completed with %d index change(s)", len(output.ChangedFiles))
	return finalizeGitResult(output, nil)
}

func gitAddIndexDelta(before, after []GitFileChange) []GitFileChange {
	byPath := func(values []GitFileChange) map[string]GitFileChange {
		ret := make(map[string]GitFileChange, len(values))
		for _, value := range values {
			ret[value.Path] = value
		}
		return ret
	}
	beforeByPath := byPath(before)
	afterByPath := byPath(after)
	ret := make([]GitFileChange, 0)
	for path, current := range afterByPath {
		previous, found := beforeByPath[path]
		if !found || previous.Staged != current.Staged || previous.Unstaged != current.Unstaged || previous.Untracked != current.Untracked || previous.Status != current.Status || previous.OldPath != current.OldPath {
			if current.Staged || (found && previous.Staged) {
				ret = append(ret, current)
			}
		}
	}
	for path, previous := range beforeByPath {
		if _, found := afterByPath[path]; !found && previous.Staged {
			ret = append(ret, previous)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	sort.Slice(ret, func(left, right int) bool { return ret[left].Path < ret[right].Path })
	return limitChangedFiles(ret)
}

func (a *RuntimeAdapter) executeCommit(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	if len(request.Pathspecs) > 0 {
		err := fmt.Errorf("%w: git commit does not accept pathspecs; stage files with git add first", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorPathRejected, err)
	}
	if repo.DetachedHead {
		err := fmt.Errorf("%w: git commit is rejected on detached HEAD", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorOperationDenied, err)
	}
	message := strings.TrimSpace(request.Message)
	if message == "" {
		err := fmt.Errorf("%w: git commit message is required", common.ErrInvalidArgument)
		return a.rejectedResultWithRepo(request, gitcommon.OperationCommit, repo, gitcommon.ErrorInvalidRequest, err)
	}
	expectedHead := strings.TrimSpace(request.ExpectedHead)
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
		output.ZeroWork = true
		output.ErrorCode = gitcommon.ErrorNoStagedChanges
		output.StagedFiles = []GitFileChange{}
		output.Summary = "git commit completed with no staged changes"
		return finalizeGitResult(output, nil)
	}
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, "-c", "core.hooksPath=/dev/null", "commit", "--no-verify", "-m", message)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationCommit, cmd, classifyGitError(cmd, err), err)
	}
	nextHead, err := a.gitOutput(ctx, repo.RepoRoot, timeout, "--no-replace-objects", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return RuntimeOperationResult{}, fmt.Errorf("Git commit effect completed but HEAD readback failed: %w", err)
	}
	if !gitDependencyObjectID(strings.TrimSuffix(nextHead, "\n")) || strings.TrimSuffix(nextHead, "\n") == repo.HeadCommit {
		return RuntimeOperationResult{}, fmt.Errorf("Git commit effect completed without a distinct exact HEAD readback")
	}
	output := a.baseOutput(request, repo, gitcommon.OperationCommit, cmd, "completed")
	output.HeadCommitBefore = repo.HeadCommit
	output.HeadCommitAfter = strings.TrimSpace(nextHead)
	output.BeforeHash, output.AfterHash = output.HeadCommitBefore, output.HeadCommitAfter
	output.Message = message
	output.StagedFiles = staged
	output.CommittedFiles = staged
	output.ChangedFiles = staged
	output.CommitHash = strings.TrimSpace(nextHead)
	output.CommitSubject = firstLine(message)
	output.Summary = "git commit completed"
	return finalizeGitResult(output, nil)
}
