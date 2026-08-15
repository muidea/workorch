package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (a *RuntimeAdapter) baseOutput(request RuntimeOperationRequest, repo repoContext, operation string, cmd gitCommandResult, status string) RuntimeOperationResult {
	timeoutMillis := request.TimeoutMillis
	if timeoutMillis <= 0 {
		timeoutMillis = a.timeout.Milliseconds()
	}
	return RuntimeOperationResult{
		Status:           status,
		Operation:        operation,
		Stage:            stageForOperation(operation),
		RepoRoot:         repo.RepoRoot,
		WorkingDirectory: repo.RepoRoot,
		Branch:           repo.Branch,
		HeadCommit:       repo.HeadCommit,
		DetachedHead:     repo.DetachedHead,
		UnbornHead:       repo.UnbornHead,
		Stdout:           cmd.Stdout,
		Stderr:           cmd.Stderr,
		ExitCode:         int64(cmd.ExitCode),
		DurationMillis:   cmd.DurationMillis,
		TimeoutMillis:    timeoutMillis,
		Truncated:        cmd.Truncated,
	}
}

func (a *RuntimeAdapter) rejectedResult(request RuntimeOperationRequest, operation string, code string, err error) (RuntimeOperationResult, error) {
	return a.rejectedResultWithRepo(request, operation, repoContext{}, code, err)
}

func (a *RuntimeAdapter) rejectedResultWithRepo(request RuntimeOperationRequest, operation string, repo repoContext, code string, err error) (RuntimeOperationResult, error) {
	output := a.baseOutput(request, repo, operation, gitCommandResult{}, "rejected")
	output.ErrorCode = code
	output.ErrorMessage = err.Error()
	output.Summary = "git operation rejected"
	return finalizeGitResult(output, err)
}

func (a *RuntimeAdapter) failedResult(request RuntimeOperationRequest, operation string, code string, err error) (RuntimeOperationResult, error) {
	return a.failedResultWithRepo(request, operation, repoContext{}, code, err)
}

func (a *RuntimeAdapter) failedResultWithRepo(request RuntimeOperationRequest, operation string, repo repoContext, code string, err error) (RuntimeOperationResult, error) {
	output := a.baseOutput(request, repo, operation, gitCommandResult{}, "failed")
	output.ErrorCode = code
	output.ErrorMessage = err.Error()
	output.Summary = "git operation failed"
	return finalizeGitResult(output, err)
}

func (a *RuntimeAdapter) commandFailureResult(request RuntimeOperationRequest, repo repoContext, operation string, cmd gitCommandResult, code string, err error) (RuntimeOperationResult, error) {
	status := "failed"
	if errors.Is(err, context.DeadlineExceeded) {
		status = "timeout"
		code = gitcommon.ErrorTimeout
	}
	output := a.baseOutput(request, repo, operation, cmd, status)
	output.ErrorCode = code
	output.ErrorMessage = err.Error()
	output.Summary = "git command failed"
	return finalizeGitResult(output, err)
}

func finalizeGitResult(output RuntimeOperationResult, executeErr error) (RuntimeOperationResult, error) {
	if executeErr != nil && strings.TrimSpace(output.ErrorMessage) == "" {
		output.ErrorMessage = executeErr.Error()
	}
	if executeErr != nil && strings.TrimSpace(output.ErrorCode) == "" {
		output.ErrorCode = "git_operation_failed"
	}
	if strings.TrimSpace(output.Summary) == "" {
		output.Summary = strings.TrimSpace(output.Status)
		if output.Summary == "" {
			output.Summary = fmt.Sprintf("git %s", output.Operation)
		}
	}
	exitCode := output.ExitCode
	output.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: output.Summary, ExitCode: &exitCode}
	return output, executeErr
}

func stageForOperation(operation string) string {
	switch operation {
	case gitcommon.OperationInit, gitcommon.OperationAdd, gitcommon.OperationCommit:
		return "git_write"
	default:
		return "git_read"
	}
}

func gitCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultCommandTimeout
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 1000 {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeoutMillis > int64(MaxCommandTimeout/time.Millisecond) {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
	}
	return time.Duration(timeoutMillis) * time.Millisecond, nil
}
