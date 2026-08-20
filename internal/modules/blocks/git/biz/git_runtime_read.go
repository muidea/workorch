package git

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (a *RuntimeAdapter) executeRead(ctx context.Context, request RuntimeOperationRequest, repo repoContext, operation string, timeout time.Duration) (RuntimeOperationResult, error) {
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

func (a *RuntimeAdapter) executeStatus(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	pathspecs, err := optionalPathspecs(request.Pathspecs)
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
	output.Pathspecs = pathspecs
	output.ChangedFiles = limitChangedFiles(files)
	output.Truncated = output.Truncated || len(files) > maxChangedFiles
	output.AheadBehind = aheadBehind
	if branch != "" {
		output.Branch = branch
	}
	if head == "(initial)" {
		output.UnbornHead = true
	} else if head != "" {
		output.HeadCommit = head
	}
	output.Summary = fmt.Sprintf("git status completed with %d changed file(s)", len(files))
	return finalizeGitResult(output, nil)
}

func (a *RuntimeAdapter) executeDiff(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	pathspecs, err := optionalPathspecs(request.Pathspecs)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationDiff, repo, gitcommon.ErrorPathRejected, err)
	}
	ref, err := normalizeGitRevision(request.Ref)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationDiff, repo, gitcommon.ErrorInvalidRequest, err)
	}
	baseRef, err := normalizeGitRevision(request.BaseRef)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationDiff, repo, gitcommon.ErrorInvalidRequest, err)
	}
	args := []string{"diff"}
	if ref != "" {
		args = append(args, "--end-of-options")
		if baseRef != "" {
			args = append(args, baseRef+".."+ref)
		} else {
			args = append(args, ref)
		}
	} else if baseRef != "" {
		args = append(args, "--end-of-options", baseRef)
	}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxDiffBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationDiff, cmd, classifyGitError(cmd, err), err)
	}
	statusFiles, _ := a.diffChangedFiles(ctx, repo.RepoRoot, timeout, pathspecs)
	output := a.baseOutput(request, repo, gitcommon.OperationDiff, cmd, "completed")
	output.Pathspecs = pathspecs
	output.ChangedFiles = limitChangedFiles(statusFiles)
	output.Summary = fmt.Sprintf("git diff completed with %d changed file(s)", len(statusFiles))
	return finalizeGitResult(output, nil)
}

func (a *RuntimeAdapter) executeLog(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	pathspecs, err := optionalPathspecs(request.Pathspecs)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationLog, repo, gitcommon.ErrorPathRejected, err)
	}
	limit, clamped := logLimit(request.Limit)
	ref, err := normalizeGitRevision(request.Ref)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationLog, repo, gitcommon.ErrorInvalidRequest, err)
	}
	if ref == "" {
		ref = "HEAD"
	}
	if repo.UnbornHead && ref == "HEAD" {
		output := a.baseOutput(request, repo, gitcommon.OperationLog, gitCommandResult{}, "completed")
		output.Pathspecs = pathspecs
		output.Commits = []GitCommit{}
		output.Limit = int64(limit)
		output.LimitClamped = clamped
		output.ZeroWork = true
		output.Summary = "git log completed with no commits because repository HEAD is unborn"
		return finalizeGitResult(output, nil)
	}
	format := "%H%x1f%h%x1f%an <%ae>%x1f%aI%x1f%s"
	args := []string{"log", "--format=" + format, "-n", strconv.Itoa(limit), "--end-of-options", ref}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationLog, cmd, classifyGitError(cmd, err), err)
	}
	commits := parseLogCommits(cmd.Stdout)
	output := a.baseOutput(request, repo, gitcommon.OperationLog, cmd, "completed")
	output.Pathspecs = pathspecs
	output.Commits = commits
	output.Limit = int64(limit)
	output.LimitClamped = clamped
	output.Summary = fmt.Sprintf("git log completed with %d commit(s)", len(commits))
	if clamped {
		output.Summary = output.Summary + "; limit clamped to maximum"
	}
	return finalizeGitResult(output, nil)
}

func (a *RuntimeAdapter) executeShow(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	pathspecs, err := optionalPathspecs(request.Pathspecs)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationShow, repo, gitcommon.ErrorPathRejected, err)
	}
	ref, err := normalizeGitRevision(request.Ref)
	if err != nil {
		return a.rejectedResultWithRepo(request, gitcommon.OperationShow, repo, gitcommon.ErrorInvalidRequest, err)
	}
	if ref == "" {
		ref = "HEAD"
	}
	if repo.UnbornHead && ref == "HEAD" {
		output := a.baseOutput(request, repo, gitcommon.OperationShow, gitCommandResult{}, "completed")
		output.Pathspecs = pathspecs
		output.Ref = ref
		output.ZeroWork = true
		output.Summary = "git show completed with no commit because repository HEAD is unborn"
		return finalizeGitResult(output, nil)
	}
	args := []string{"show", "--stat", "--summary", "--format=%H%x1f%h%x1f%an <%ae>%x1f%aI%x1f%s", "--end-of-options", ref}
	args = append(args, pathspecArgs(pathspecs)...)
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxDiffBytes, args...)
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationShow, cmd, classifyGitError(cmd, err), err)
	}
	output := a.baseOutput(request, repo, gitcommon.OperationShow, cmd, "completed")
	output.Pathspecs = pathspecs
	output.Ref = ref
	output.Summary = "git show completed"
	return finalizeGitResult(output, nil)
}

func (a *RuntimeAdapter) executeBranch(ctx context.Context, request RuntimeOperationRequest, repo repoContext, timeout time.Duration) (RuntimeOperationResult, error) {
	cmd, err := a.runGit(ctx, repo.RepoRoot, timeout, maxOutputBytes, "branch", "--format=%(refname:short)%09%(objectname)%09%(HEAD)%09%(upstream:short)")
	if err != nil {
		return a.commandFailureResult(request, repo, gitcommon.OperationBranch, cmd, classifyGitError(cmd, err), err)
	}
	output := a.baseOutput(request, repo, gitcommon.OperationBranch, cmd, "completed")
	output.Branches = parseBranches(cmd.Stdout)
	output.DetachedHead = repo.DetachedHead
	output.Summary = "git branch completed"
	return finalizeGitResult(output, nil)
}
