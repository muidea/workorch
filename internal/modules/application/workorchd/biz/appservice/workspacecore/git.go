package workspacecore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func InitializeGitRepository(ctx context.Context, deliverableDirectory string) error {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	if deliverableDirectory == "" {
		return common.ErrInvalidArgument
	}
	if _, err := os.Stat(filepath.Join(deliverableDirectory, ".git")); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("%w: git binary is required to initialize workspace version control", common.ErrInvalidArgument)
	}
	gitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := runGitCommand(gitCtx, gitPath, deliverableDirectory, "init", "-q"); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"config", "user.name", "WorkOrch"},
		{"config", "user.email", "workorch@example.local"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		if err := runGitCommand(gitCtx, gitPath, deliverableDirectory, args...); err != nil {
			return err
		}
	}
	return nil
}

func runGitCommand(ctx context.Context, gitPath string, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, gitPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: git %s failed: %s", common.ErrInvalidArgument, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
