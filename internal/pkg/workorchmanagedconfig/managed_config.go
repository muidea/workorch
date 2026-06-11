package workorchmanagedconfig

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func CleanPath(relPath string) (string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", common.ErrInvalidArgument
	}
	cleanRelPath := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRelPath) || cleanRelPath == "." || strings.HasPrefix(cleanRelPath, ".."+string(os.PathSeparator)) || cleanRelPath == ".." {
		return "", common.ErrInvalidArgument
	}
	return cleanRelPath, nil
}

func SystemDirectory(workspaceInternalDirectory string) string {
	workspaceInternalDirectory = strings.TrimSpace(workspaceInternalDirectory)
	if workspaceInternalDirectory == "" {
		return ""
	}
	internalRoot := filepath.Dir(filepath.Dir(workspaceInternalDirectory))
	if filepath.Base(internalRoot) == "internal" {
		return filepath.Join(filepath.Dir(internalRoot), "system", "managed-configs")
	}
	return filepath.Join(filepath.Dir(workspaceInternalDirectory), "system", "managed-configs")
}

func PathForWorkspace(workspaceInternalDirectory string, relPath string) string {
	cleanRelPath, err := CleanPath(relPath)
	if err != nil || strings.TrimSpace(workspaceInternalDirectory) == "" {
		return ""
	}
	workspacePath := filepath.Join(workspaceInternalDirectory, cleanRelPath)
	if _, err := os.Stat(workspacePath); err == nil {
		return workspacePath
	}
	return filepath.Join(SystemDirectory(workspaceInternalDirectory), cleanRelPath)
}

func ExpandPlaceholders(value string, workspaceInternalDirectory string) string {
	const prefix = "{workspaceManagedConfig:"
	for {
		start := strings.Index(value, prefix)
		if start < 0 {
			return value
		}
		relStart := start + len(prefix)
		end := strings.Index(value[relStart:], "}")
		if end < 0 {
			return value
		}
		relPath := value[relStart : relStart+end]
		resolved := PathForWorkspace(workspaceInternalDirectory, relPath)
		value = value[:start] + resolved + value[relStart+end+1:]
	}
}
