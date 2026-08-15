package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	args := append([]string{"add"}, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationAdd, cmd, classifyGitError(cmd, err), err)
	}
	staged, _ := a.stagedFiles(ctx, repo.RepoRoot, timeout)
	output := a.baseOutput(request, repo, gitcommon.OperationAdd, cmd, "completed")
	output.Pathspecs = pathspecs
	output.StagedFiles = staged
	output.ChangedFiles = staged
	if len(staged) == 0 {
		output.ZeroWork = true
	}
	output.Summary = fmt.Sprintf("git add completed with %d staged file(s)", len(staged))
	return finalizeGitResult(output, nil)
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
	nextHead, _ := a.gitOutput(ctx, repo.RepoRoot, timeout, "rev-parse", "HEAD")
	output := a.baseOutput(request, repo, gitcommon.OperationCommit, cmd, "completed")
	output.HeadCommitBefore = repo.HeadCommit
	output.HeadCommitAfter = strings.TrimSpace(nextHead)
	output.Message = message
	output.StagedFiles = staged
	output.CommittedFiles = staged
	output.ChangedFiles = staged
	output.CommitHash = strings.TrimSpace(nextHead)
	output.CommitSubject = firstLine(message)
	output.Summary = "git commit completed"
	return finalizeGitResult(output, nil)
}
