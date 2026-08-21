package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

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
	if marker, markerErr := os.Lstat(filepath.Join(deliverable, ".git")); markerErr == nil && marker.Mode()&os.ModeSymlink != 0 {
		return repoContext{}, fmt.Errorf("%w: workspace git control directory must not be a symlink", common.ErrInvalidArgument)
	}
	gitDir, err := a.gitOutput(ctx, rootAbs, timeout, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return repoContext{}, err
	}
	gitDir = strings.TrimSpace(gitDir)
	if gitDir == "" || !pathInsideRoot(rootAbs, gitDir) {
		return repoContext{}, fmt.Errorf("%w: git control directory must remain inside workspace deliverable directory", common.ErrInvalidArgument)
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

func pathInsideRoot(root, target string) bool {
	rootAbs, rootErr := filepath.Abs(root)
	targetAbs, targetErr := filepath.Abs(target)
	if rootErr != nil || targetErr != nil {
		return false
	}
	if value, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = value
	}
	if value, err := filepath.EvalSymlinks(targetAbs); err == nil {
		targetAbs = value
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
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
