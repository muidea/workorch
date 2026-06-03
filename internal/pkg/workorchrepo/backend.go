package workorchrepo

import (
	"os"
	"path/filepath"
)

func ResolveRepositoryDataRoot(backend string, dataRoot string) string {
	backend = NormalizeRepositoryBackend(backend)
	if backend != "file" {
		return ""
	}
	if dataRoot == "" {
		return filepath.Join(os.TempDir(), "workorch-data")
	}
	return dataRoot
}

func NormalizeRepositoryBackend(backend string) string {
	if backend == "" {
		return "file"
	}
	return backend
}
