package workspacecore

import (
	"os"
	"path/filepath"
	"strings"
)

func PurgeDirectories(repositoryBackend string, repositoryDataDir string, workspaceID string, deliverableDirectory string, internalDirectory string) error {
	paths := make([]string, 0, 2)
	if strings.TrimSpace(internalDirectory) != "" {
		paths = append(paths, internalDirectory)
	} else if strings.EqualFold(repositoryBackend, "file") && strings.TrimSpace(repositoryDataDir) != "" {
		paths = append(paths, filepath.Join(repositoryDataDir, "internal", "workspaces", workspaceID))
	}
	paths = append(paths, deliverableDirectory)

	seen := map[string]struct{}{}
	for _, item := range paths {
		path, err := SafeRemovalPath(repositoryDataDir, item)
		if err != nil {
			return err
		}
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
