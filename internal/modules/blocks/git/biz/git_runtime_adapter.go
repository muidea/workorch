package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CapabilityState is a read-only owner observation used by Runtime V2 before a
// candidate is requested. It never initializes a repository or changes files.
func (a *RuntimeAdapter) CapabilityState(ctx context.Context, requirementID, subjectRef, deliverable string) (runtimecontract.RuntimeCapabilityStateSnapshot, error) {
	requirementID = strings.TrimSpace(requirementID)
	subjectRef = strings.TrimSpace(subjectRef)
	deliverable = strings.TrimSpace(deliverable)
	if a == nil || requirementID == "" || subjectRef == "" || deliverable == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	status, err := a.ExecuteOperation(ctx, RuntimeOperationRequest{Kind: gitcommon.StepKindRead, WorkspaceDeliverableDirectory: deliverable, Operation: gitcommon.OperationStatus})
	sequence := []runtimecontract.RuntimeRequiredOperationVariant{}
	disposition := runtimecontract.RuntimeCapabilityStateReady
	initialized := err == nil
	unstaged, staged := false, false
	if initialized {
		for _, change := range status.ChangedFiles {
			unstaged = unstaged || change.Unstaged || change.Untracked
			staged = staged || change.Staged
		}
		if unstaged {
			sequence = append(sequence, runtimecontract.RuntimeRequiredOperationVariant{RequirementID: "stage-changes", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd})
		}
		if unstaged || staged {
			dependsOn := []string(nil)
			if unstaged {
				dependsOn = []string{"stage-changes"}
			}
			sequence = append(sequence, runtimecontract.RuntimeRequiredOperationVariant{RequirementID: "create-commit", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit, DependsOnRequirementIDs: dependsOn})
		}
	} else if status.ErrorCode == gitcommon.ErrorNotGitRepository {
		entries, readErr := os.ReadDir(deliverable)
		if readErr != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, readErr
		}
		for _, entry := range entries {
			if entry.Name() != ".git" {
				unstaged = true
				break
			}
		}
		if unstaged {
			sequence = []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "repo-init", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationInit},
				{RequirementID: "stage-changes", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd, DependsOnRequirementIDs: []string{"repo-init"}},
				{RequirementID: "create-commit", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit, DependsOnRequirementIDs: []string{"stage-changes"}},
			}
		}
	} else {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if len(sequence) > 0 {
		disposition = runtimecontract.RuntimeCapabilityStateSequenceRequired
	}
	revision := strings.Join([]string{status.RepoRoot, status.HeadCommit, fmt.Sprintf("%t", initialized), fmt.Sprintf("%t", unstaged), fmt.Sprintf("%t", staged)}, "|")
	hash := sha256.Sum256([]byte(revision))
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: requirementID, AcquisitionKey: "capability-state/" + requirementID,
		OwnerModuleID: gitcommon.ModuleID, CapabilityStateKind: "repository_state", SubjectRef: subjectRef,
		ObservedRevision: revision, ContentHash: hex.EncodeToString(hash[:]), Disposition: disposition,
		RequiredOperationSequence: sequence, QueryReceiptRef: "capability-state-receipt/" + requirementID,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	return snapshot, nil
}

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
	envAllowlist map[string]struct{}
}

// SupportedRuntimeOperationVariants is the Git owner's executable operation
// surface. The descriptor remains the parameter/effect authority; this list is
// only an enumeration guard so an owner switch cannot silently drift from it.
func SupportedRuntimeOperationVariants() []runtimecontract.RuntimeOperationVariantRef {
	return []runtimecontract.RuntimeOperationVariantRef{
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationStatus},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationDiff},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationLog},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationShow},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationBranch},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationInit},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit},
	}
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
	_ = writeEnabled
	return &RuntimeAdapter{
		timeout: DefaultCommandTimeout,
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

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation == "" {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, common.ErrInvalidArgument)
	}
	timeout, err := gitCommandTimeout(request.TimeoutMillis, a.timeout)
	if err != nil {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, err)
	}
	if operation == gitcommon.OperationInit {
		if request.Kind != gitcommon.StepKindWrite {
			return a.rejectedResult(request, operation, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: git init requires git/write", common.ErrInvalidArgument))
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
		return a.executeWrite(ctx, request, repo, operation, timeout)
	default:
		return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: unsupported git step kind %q", common.ErrInvalidArgument, request.Kind))
	}
}

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
	args := []string{"diff"}
	if ref := strings.TrimSpace(request.Ref); ref != "" {
		if base := strings.TrimSpace(request.BaseRef); base != "" {
			args = append(args, base+".."+ref)
		} else {
			args = append(args, ref)
		}
	} else if base := strings.TrimSpace(request.BaseRef); base != "" {
		args = append(args, base)
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
	ref := strings.TrimSpace(request.Ref)
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
	args := []string{"log", "--format=" + format, "-n", strconv.Itoa(limit), ref}
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
	ref := strings.TrimSpace(request.Ref)
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
	args := []string{"show", "--stat", "--summary", "--format=%H%x1f%h%x1f%an <%ae>%x1f%aI%x1f%s", ref}
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

func (a *RuntimeAdapter) resolveRepo(ctx context.Context, request RuntimeOperationRequest, timeout time.Duration) (repoContext, error) {
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

func (a *RuntimeAdapter) existingRepoContext(ctx context.Context, request RuntimeOperationRequest, deliverable string, timeout time.Duration) (repoContext, bool, error) {
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

func (a *RuntimeAdapter) stagedFiles(ctx context.Context, repoRoot string, timeout time.Duration) ([]GitFileChange, error) {
	cmd, err := a.runGit(ctx, repoRoot, timeout, maxOutputBytes, "diff", "--cached", "--name-status")
	if err != nil {
		return nil, err
	}
	return parseNameStatus(cmd.Stdout, true), nil
}

func (a *RuntimeAdapter) diffChangedFiles(ctx context.Context, repoRoot string, timeout time.Duration, pathspecs []string) ([]GitFileChange, error) {
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

func optionalPathspecs(input []string) ([]string, error) {
	return parsePathspecs(input, false, true)
}

func requiredPathspecs(input []string) ([]string, error) {
	return parsePathspecs(input, true, false)
}

func parsePathspecs(input []string, required bool, allowRepositoryRoot bool) ([]string, error) {
	if required && len(input) == 0 {
		return nil, fmt.Errorf("%w: pathspecs are required", common.ErrInvalidArgument)
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		pathspec, err := normalizePathspec(item, allowRepositoryRoot)
		if err != nil {
			return nil, err
		}
		if pathspec != "" {
			ret = append(ret, pathspec)
		}
	}
	return ret, nil
}

func normalizePathspec(value string, allowRepositoryRoot bool) (string, error) {
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
	if cleaned == "." && allowRepositoryRoot {
		return "", nil
	}
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

func parsePorcelainStatus(stdout string) ([]GitFileChange, GitAheadBehind, string, string) {
	var files []GitFileChange
	aheadBehind := GitAheadBehind{}
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
				aheadBehind.Ahead = int64(trimSignedInt(fields[0]))
				aheadBehind.Behind = int64(trimSignedInt(fields[1]))
			}
			continue
		}
		if strings.HasPrefix(line, "? ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "? "))
			files = append(files, GitFileChange{Path: path, Status: "untracked", Untracked: true})
			continue
		}
		if strings.HasPrefix(line, "1 ") {
			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}
			xy := fields[1]
			path := strings.Join(fields[8:], " ")
			files = append(files, GitFileChange{Path: path, Status: statusFromXY(xy), Staged: len(xy) > 0 && xy[0] != '.', Unstaged: len(xy) > 1 && xy[1] != '.'})
			continue
		}
	}
	return files, aheadBehind, branch, head
}

func parseNameStatus(stdout string, staged bool) []GitFileChange {
	var files []GitFileChange
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
		item := GitFileChange{Path: path, Status: status, Staged: staged}
		if strings.HasPrefix(fields[0], "R") && len(fields) >= 3 {
			item.OldPath = fields[1]
		}
		files = append(files, item)
	}
	return files
}

func parseLogCommits(stdout string) []GitCommit {
	var commits []GitCommit
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x1f", 5)
		if len(fields) != 5 {
			continue
		}
		commits = append(commits, GitCommit{Hash: fields[0], ShortHash: fields[1], Author: fields[2], AuthorTime: fields[3], Subject: fields[4]})
	}
	return commits
}

func parseBranches(stdout string) []GitBranch {
	var branches []GitBranch
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		item := GitBranch{Name: fields[0], Commit: fields[1], Current: fields[2] == "*"}
		if len(fields) > 3 && fields[3] != "" {
			item.Upstream = fields[3]
		}
		branches = append(branches, item)
	}
	return branches
}

func logLimit(limit int64) (int, bool) {
	if limit <= 0 {
		return defaultLogLimit, false
	}
	if limit > maxLogLimit {
		return maxLogLimit, true
	}
	return int(limit), false
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
	case strings.Contains(text, "author identity unknown") ||
		strings.Contains(text, "unable to auto-detect email address") ||
		strings.Contains(text, "作者身份未知") ||
		strings.Contains(text, "未提供邮件地址") ||
		strings.Contains(text, "请告诉我您是谁"):
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

func limitChangedFiles(files []GitFileChange) []GitFileChange {
	if len(files) <= maxChangedFiles {
		return files
	}
	return files[:maxChangedFiles]
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
