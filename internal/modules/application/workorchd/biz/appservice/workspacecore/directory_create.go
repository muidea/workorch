package workspacecore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func EnsureDirectoryLayout(layout DirectoryLayout) error {
	if strings.TrimSpace(layout.DeliverableDirectory) == "" || strings.TrimSpace(layout.InternalDirectory) == "" {
		return common.ErrInvalidArgument
	}
	if !layout.Managed {
		if err := os.MkdirAll(layout.DeliverableDirectory, 0o755); err != nil {
			return err
		}
		return os.MkdirAll(layout.InternalDirectory, 0o755)
	}
	for _, dir := range managedDirectoryLayoutPaths(layout) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func managedDirectoryLayoutPaths(layout DirectoryLayout) []string {
	return []string{
		layout.DeliverableDirectory,
		layout.InternalDirectory,
		filepath.Join(layout.InternalDirectory, "config"),
		filepath.Join(layout.InternalDirectory, "runs"),
		filepath.Join(layout.InternalDirectory, "artifacts"),
		filepath.Join(layout.InternalDirectory, "events"),
		filepath.Join(layout.InternalDirectory, "llm-calls"),
		filepath.Join(layout.InternalDirectory, "indexes"),
	}
}
